// Package dbutil provides database connection utilities and testing infrastructure
// for applications using PostgreSQL with pgx and sqlc.
//
// This package is designed specifically for sqlc users who want:
//   - Type-safe database operations with any sqlc-generated queries
//   - Optimized testing infrastructure with shared connections
//   - Comprehensive pgx type helpers for seamless type conversions
//   - Structured error handling with consistent error types
//   - Connection lifecycle management with hooks
//   - Production-ready features like health checks, metrics, and retry logic
//   - Read/write connection splitting for scaled deployments
//
// The package uses Go generics to work with any sqlc-generated queries without
// requiring code generation or importing specific sqlc packages.
//
// Example usage:
//
//	conn, err := database.NewConnection(ctx, "", sqlc.New)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer conn.Close()
//
//	queries := conn.Queries()
//	users, err := queries.GetAllUsers(ctx)
package dbutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier represents the interface that sqlc-generated queries implement
type Querier interface {
	WithTx(tx pgx.Tx) Querier
}

// Connection represents a database connection with sqlc queries
type Connection[T Querier] struct {
	pool    *pgxpool.Pool
	queries T
	metrics MetricsCollector
	hooks   *ConnectionHooks
}

// Config holds configuration options for database connections
type Config struct {
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	SearchPath      string
	OnConnect       func(*pgx.Conn) error
	OnDisconnect    func(*pgx.Conn)
	Hooks           *ConnectionHooks
}

// TransactionFunc is a function that executes within a transaction
type TransactionFunc[T Querier] func(ctx context.Context, tx T) error

// MetricsCollector interface for collecting database metrics
type MetricsCollector interface {
	RecordConnectionAcquired(duration time.Duration)
	RecordConnectionReleased(duration time.Duration)
	RecordQueryExecuted(queryName string, duration time.Duration, err error)
	RecordTransactionStarted()
	RecordTransactionCommitted(duration time.Duration)
	RecordTransactionRolledBack(duration time.Duration)
}

// getEnvWithDefault returns the value of the environment variable or a default value
func getEnvWithDefault(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}

// getEnvIntWithDefault returns the value of the environment variable as an int or a default value
func getEnvIntWithDefault(key string, def int) int {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return n
}

// getDSN returns the database connection string using environment variables
func getDSN() string {
	return getDSNWithSearchPath("")
}

// getDSNWithSearchPath returns the database connection string with a custom search path
func getDSNWithSearchPath(searchPath string) string {
	host := getEnvWithDefault("POSTGRES_HOST", "localhost")
	port := getEnvIntWithDefault("POSTGRES_PORT", 5432)
	user := getEnvWithDefault("POSTGRES_USER", "postgres")
	password := getEnvWithDefault("POSTGRES_PASSWORD", "")
	dbname := getEnvWithDefault("POSTGRES_DB", "postgres")
	sslmode := getEnvWithDefault("POSTGRES_SSLMODE", "disable")

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s", user, password, host, port, dbname, sslmode)
	if searchPath != "" {
		dsn += "&search_path=" + searchPath
	}
	return dsn
}

// GetDSN returns the database connection string using the same POSTGRES_* environment
// variables that NewConnection uses. This is useful for tools like golang-migrate
// that need a connection string rather than a pgxpool.Pool.
func GetDSN() string {
	return getDSN()
}

// NewConnection initializes a new pgxpool.Pool connection with user-provided sqlc queries
func NewConnection[T Querier](ctx context.Context, dsn string, newQueriesFunc func(*pgxpool.Pool) T) (*Connection[T], error) {
	return NewConnectionWithConfig(ctx, dsn, newQueriesFunc, nil)
}

// NewConnectionWithHooks creates a connection with hooks enabled
func NewConnectionWithHooks[T Querier](ctx context.Context, dsn string, newQueriesFunc func(*pgxpool.Pool) T, hooks *ConnectionHooks) (*Connection[T], error) {
	config := &Config{
		Hooks: hooks,
	}
	return NewConnectionWithConfig(ctx, dsn, newQueriesFunc, config)
}

// NewConnectionWithLoggingHooks creates a connection with logging hooks enabled
func NewConnectionWithLoggingHooks[T Querier](ctx context.Context, dsn string, newQueriesFunc func(*pgxpool.Pool) T, logger Logger) (*Connection[T], error) {
	hooks := LoggingHook(logger)
	return NewConnectionWithHooks(ctx, dsn, newQueriesFunc, hooks)
}

// NewConnectionWithValidationHooks creates a connection with validation hooks enabled
func NewConnectionWithValidationHooks[T Querier](ctx context.Context, dsn string, newQueriesFunc func(*pgxpool.Pool) T) (*Connection[T], error) {
	hooks := ValidationHook()
	return NewConnectionWithHooks(ctx, dsn, newQueriesFunc, hooks)
}

// createPoolWithConfig creates a pgxpool.Pool with the given configuration.
// If dsn is empty, it uses environment variables to construct the connection string.
// The function applies default values and then overrides them with user-provided config.
func createPoolWithConfig(ctx context.Context, dsn string, cfg *Config) (*pgxpool.Pool, error) {
	if dsn == "" {
		searchPath := ""
		if cfg != nil && cfg.SearchPath != "" {
			searchPath = cfg.SearchPath
		}
		dsn = getDSNWithSearchPath(searchPath)
	}

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	// Set default values
	config.MaxConns = 10
	config.MinConns = 1
	config.MaxConnLifetime = 30 * time.Minute

	// Override with user config if provided
	if cfg != nil {
		if cfg.MaxConns > 0 {
			config.MaxConns = cfg.MaxConns
		}
		if cfg.MinConns > 0 {
			config.MinConns = cfg.MinConns
		}
		if cfg.MaxConnLifetime > 0 {
			config.MaxConnLifetime = cfg.MaxConnLifetime
		}
		if cfg.OnConnect != nil || cfg.Hooks != nil {
			config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
				// Execute individual OnConnect callback
				if cfg.OnConnect != nil {
					if err := cfg.OnConnect(conn); err != nil {
						return err
					}
				}
				// Execute hooks OnConnect callbacks
				if cfg.Hooks != nil {
					if err := cfg.Hooks.ExecuteOnConnect(conn); err != nil {
						return err
					}
				}
				return nil
			}
		}
		if cfg.OnDisconnect != nil || cfg.Hooks != nil {
			config.BeforeClose = func(conn *pgx.Conn) {
				// Execute individual OnDisconnect callback
				if cfg.OnDisconnect != nil {
					cfg.OnDisconnect(conn)
				}
				// Execute hooks OnDisconnect callbacks
				if cfg.Hooks != nil {
					cfg.Hooks.ExecuteOnDisconnect(conn)
				}
			}
		}
	}

	return pgxpool.NewWithConfig(ctx, config)
}

// NewConnectionWithConfig initializes a new pgxpool.Pool connection with configuration options
func NewConnectionWithConfig[T Querier](ctx context.Context, dsn string, newQueriesFunc func(*pgxpool.Pool) T, cfg *Config) (*Connection[T], error) {
	pool, err := createPoolWithConfig(ctx, dsn, cfg)
	if err != nil {
		return nil, err
	}

	hooks := (*ConnectionHooks)(nil)
	if cfg != nil && cfg.Hooks != nil {
		hooks = cfg.Hooks
	}

	return &Connection[T]{
		pool:    pool,
		queries: newQueriesFunc(pool),
		metrics: nil,
		hooks:   hooks,
	}, nil
}

// GetDB returns the underlying *pgxpool.Pool
func (c *Connection[T]) GetDB() *pgxpool.Pool {
	return c.pool
}

// Queries returns the cached sqlc queries instance for this connection
func (c *Connection[T]) Queries() T {
	return c.queries
}

// WithTransaction executes the given function within a database transaction.
// If the function returns an error, the transaction is rolled back.
// If the function completes successfully, the transaction is committed.
func (c *Connection[T]) WithTransaction(ctx context.Context, fn TransactionFunc[T]) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}
	if fn == nil {
		return fmt.Errorf("transaction function cannot be nil")
	}

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			// Rollback errors are expected after successful commit, so we only log unexpected ones
			if !errors.Is(rollbackErr, pgx.ErrTxClosed) {
				// Log the error but don't fail the transaction
				_ = rollbackErr // Explicitly ignore for linter
			}
		}
	}()

	// Use sqlc's built-in WithTx method on our cached queries
	txQuerier := c.queries.WithTx(tx)

	// Execute the function with the transaction querier
	if err := fn(ctx, txQuerier.(T)); err != nil {
		return err // Transaction will be rolled back by defer
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// BeginTransaction starts a new transaction and returns both the transaction and a querier.
// The caller is responsible for committing or rolling back the transaction.
// This is useful for more complex transaction management scenarios.
func (c *Connection[T]) BeginTransaction(ctx context.Context) (pgx.Tx, T, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return nil, *new(T), fmt.Errorf("failed to begin transaction: %w", err)
	}

	txQueries := c.queries.WithTx(tx)
	return tx, txQueries.(T), nil
}

// HealthCheck performs a simple health check by pinging the database
func (c *Connection[T]) HealthCheck(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}
	return c.pool.Ping(ctx)
}

// IsReady checks if the database connection is ready to accept queries
func (c *Connection[T]) IsReady(ctx context.Context) bool {
	return c.HealthCheck(ctx) == nil
}

// Stats returns the current connection pool statistics
func (c *Connection[T]) Stats() *pgxpool.Stat {
	return c.pool.Stat()
}

// WithMetrics returns a new connection with metrics collection enabled
func (c *Connection[T]) WithMetrics(metrics MetricsCollector) *Connection[T] {
	return &Connection[T]{
		pool:    c.pool,
		queries: c.queries,
		metrics: metrics,
		hooks:   c.hooks,
	}
}

// WithHooks returns a new connection with hooks enabled
func (c *Connection[T]) WithHooks(hooks *ConnectionHooks) *Connection[T] {
	return &Connection[T]{
		pool:    c.pool,
		queries: c.queries,
		metrics: c.metrics,
		hooks:   hooks,
	}
}

// AddHook adds a hook to the existing connection's hooks (creates hooks if none exist)
func (c *Connection[T]) AddHook(hook *ConnectionHooks) *Connection[T] {
	var combinedHooks *ConnectionHooks
	if c.hooks == nil {
		combinedHooks = hook
	} else {
		combinedHooks = CombineHooks(c.hooks, hook)
	}

	return &Connection[T]{
		pool:    c.pool,
		queries: c.queries,
		metrics: c.metrics,
		hooks:   combinedHooks,
	}
}

// GetHooks returns the current hooks (may be nil)
func (c *Connection[T]) GetHooks() *ConnectionHooks {
	return c.hooks
}

// Close closes the pool
func (c *Connection[T]) Close() {
	c.pool.Close()
}
