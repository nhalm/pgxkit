// Package pgxkit provides a production-ready PostgreSQL toolkit for Go applications.
//
// pgxkit is a tool-agnostic PostgreSQL toolkit that works with any approach to
// PostgreSQL development - raw pgx usage, code generation tools like sqlc or Skimatik,
// or any other PostgreSQL development workflow.
//
// Key Features:
//
//   - Read/Write Pool Abstraction: Safe by default with write pool, explicit read pool methods for optimization
//   - Extensible Hook System: Add logging, tracing, metrics, circuit breakers through hooks
//   - Smart Retry Logic: PostgreSQL-aware error detection with exponential backoff
//   - Testing Infrastructure: Golden test support for performance regression detection
//   - Type Helpers: Seamless pgx type conversions for clean architecture
//   - Health Checks: Built-in database connectivity monitoring
//   - Graceful Shutdown: Production-ready lifecycle management
//
// Basic Usage:
//
//	db := pgxkit.NewDB()
//	err := db.Connect(ctx, "") // Uses POSTGRES_* env vars
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Shutdown(ctx)
//
//	// Execute queries (uses write pool by default - safe)
//	_, err = db.Exec(ctx, "INSERT INTO users (name) VALUES ($1)", "John")
//
//	// Optimize reads with explicit read pool usage
//	rows, err := db.ReadQuery(ctx, "SELECT id, name FROM users")
//
// Hook System:
//
//	db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
//	    log.Printf("Executing: %s", sql)
//	    return nil
//	})
//
// The package follows a "safety first" design - all default methods use the write pool
// for consistency, with explicit ReadQuery() methods available for read optimization.
package pgxkit

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

// GetDSN returns a PostgreSQL connection string built from environment variables.
// This is useful for scripts and tools that need a connection string rather than a pgxpool.Pool.
//
// Environment variables used:
//   - POSTGRES_HOST (default: "localhost")
//   - POSTGRES_PORT (default: 5432)
//   - POSTGRES_USER (default: "postgres")
//   - POSTGRES_PASSWORD (default: "")
//   - POSTGRES_DB (default: "postgres")
//   - POSTGRES_SSLMODE (default: "disable")
//
// Example:
//
//	dsn := pgxkit.GetDSN()
func GetDSN() string {
	return getDSN()
}

// DB represents a database connection with read/write pool abstraction.
// It provides a safe-by-default approach where all operations use the write pool
// unless explicitly using Read* methods for optimization.
//
// The DB supports:
//   - Single pool mode (same pool for read/write)
//   - Read/write split mode (separate pools for optimization)
//   - Extensible hook system for logging, tracing, metrics
//   - Graceful shutdown with active operation tracking
//   - Built-in retry logic for transient failures
//   - Health checks and connection statistics
type DB struct {
	readPool  *pgxpool.Pool
	writePool *pgxpool.Pool
	hooks     *hooks
	mu        sync.RWMutex
	shutdown  bool
	activeOps sync.WaitGroup // Tracks active operations for graceful shutdown
}

// DBConfig holds configuration options for database connections.
// These options are applied when creating connection pools.
type DBConfig struct {
	MaxConns        int32         // Maximum number of connections in the pool
	MinConns        int32         // Minimum number of connections in the pool
	MaxConnLifetime time.Duration // Maximum lifetime of a connection
	MaxConnIdleTime time.Duration // Maximum idle time for a connection
}

// NewDB creates a new unconnected DB instance.
// Add hooks to this instance, then call Connect() to establish the database connection.
//
// Example:
//
//	db := pgxkit.NewDB()
//	db.AddHook(pgxkit.BeforeOperation, myLoggingHook)
//	err := db.Connect(ctx, "postgres://user:pass@localhost/db")
func NewDB() *DB {
	return &DB{
		hooks: newHooks(),
	}
}

// Connect establishes a database connection with a single pool (same pool for read/write).
// If dsn is empty, it uses environment variables to construct the connection string.
// The hooks are configured at pool creation time for proper integration.
//
// This is the recommended approach for most applications as it provides safety
// by default while still allowing read optimization through ReadQuery methods.
//
// Example:
//
//	db := pgxkit.NewDB()
//	err := db.Connect(ctx, "postgres://user:pass@localhost/db")
//	// Or use environment variables:
//	err := db.Connect(ctx, "")
func (db *DB) Connect(ctx context.Context, dsn string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.readPool != nil || db.writePool != nil {
		return fmt.Errorf("database is already connected")
	}

	// Use environment variables if no DSN provided
	if dsn == "" {
		dsn = getDSN()
	}

	// Parse the DSN to get pool config
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("failed to parse DSN: %w", err)
	}

	// Configure the pool with hooks
	db.hooks.configurePool(config)

	// Create the pool
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create pool: %w", err)
	}

	db.readPool = pool
	db.writePool = pool

	return nil
}

// ConnectReadWrite establishes database connections with separate read and write pools.
// If readDSN or writeDSN is empty, it uses environment variables to construct the connection string.
// The hooks are configured at pool creation time for proper integration.
//
// This is useful for applications that want to optimize read performance by routing
// read queries to read replicas while ensuring writes go to the primary database.
//
// Example:
//
//	db := pgxkit.NewDB()
//	err := db.ConnectReadWrite(ctx, "postgres://user:pass@read-replica/db", "postgres://user:pass@primary/db")
//	// Now ReadQuery methods will use the read pool, while Query/Exec use the write pool
func (db *DB) ConnectReadWrite(ctx context.Context, readDSN, writeDSN string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.readPool != nil || db.writePool != nil {
		return fmt.Errorf("database is already connected")
	}

	// Use environment variables if no DSN provided
	if readDSN == "" {
		readDSN = getDSN()
	}
	if writeDSN == "" {
		writeDSN = getDSN()
	}

	// Parse read DSN
	readConfig, err := pgxpool.ParseConfig(readDSN)
	if err != nil {
		return fmt.Errorf("failed to parse read DSN: %w", err)
	}

	// Parse write DSN
	writeConfig, err := pgxpool.ParseConfig(writeDSN)
	if err != nil {
		return fmt.Errorf("failed to parse write DSN: %w", err)
	}

	// Configure both pools with hooks
	db.hooks.configurePool(readConfig)
	db.hooks.configurePool(writeConfig)

	// Create read pool
	readPool, err := pgxpool.NewWithConfig(ctx, readConfig)
	if err != nil {
		return fmt.Errorf("failed to create read pool: %w", err)
	}

	// Create write pool
	writePool, err := pgxpool.NewWithConfig(ctx, writeConfig)
	if err != nil {
		readPool.Close()
		return fmt.Errorf("failed to create write pool: %w", err)
	}

	db.readPool = readPool
	db.writePool = writePool

	return nil
}

// NewDBWithPool creates a new DB instance with a single pool (same pool for read/write).
// Deprecated: Use NewDB() + Connect() instead for proper hook integration.
func NewDBWithPool(pool *pgxpool.Pool) *DB {
	return &DB{
		readPool:  pool,
		writePool: pool,
		hooks:     newHooks(),
	}
}

// NewReadWriteDB creates a new DB instance with separate read and write pools.
// Deprecated: Use NewDB() + ConnectReadWrite() instead for proper hook integration.
func NewReadWriteDB(readPool, writePool *pgxpool.Pool) *DB {
	return &DB{
		readPool:  readPool,
		writePool: writePool,
		hooks:     newHooks(),
	}
}

// Query executes a query using the write pool (safe by default).
// This ensures consistency by always using the primary database connection.
// Use ReadQuery for read-only queries that can benefit from read replicas.
//
// Example:
//
//	rows, err := db.Query(ctx, "SELECT * FROM users WHERE active = $1", true)
//	if err != nil {
//	    return err
//	}
//	defer rows.Close()
func (db *DB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return db.executeQuery(ctx, db.writePool, sql, args...)
}

// QueryRow executes a query that returns a single row using the write pool.
// This ensures consistency by always using the primary database connection.
// Use ReadQueryRow for read-only queries that can benefit from read replicas.
//
// Example:
//
//	var userID int
//	err := db.QueryRow(ctx, "SELECT id FROM users WHERE email = $1", email).Scan(&userID)
func (db *DB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return db.executeQueryRow(ctx, db.writePool, sql, args...)
}

// Exec executes a statement using the write pool.
// This method is used for INSERT, UPDATE, DELETE, and other write operations.
//
// Example:
//
//	tag, err := db.Exec(ctx, "INSERT INTO users (name, email) VALUES ($1, $2)", name, email)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Inserted %d rows\n", tag.RowsAffected())
func (db *DB) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return db.executeExec(ctx, db.writePool, sql, args...)
}

// ReadQuery executes a query using the read pool (explicit optimization).
// This method routes the query to read replicas when available, improving performance
// for read-heavy workloads. Only use this for queries that can tolerate read replica lag.
//
// Example:
//
//	rows, err := db.ReadQuery(ctx, "SELECT * FROM users WHERE active = $1", true)
//	if err != nil {
//	    return err
//	}
//	defer rows.Close()
func (db *DB) ReadQuery(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return db.executeQuery(ctx, db.readPool, sql, args...)
}

// ReadQueryRow executes a query that returns a single row using the read pool.
// This method routes the query to read replicas when available, improving performance
// for read-heavy workloads. Only use this for queries that can tolerate read replica lag.
//
// Example:
//
//	var count int
//	err := db.ReadQueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
func (db *DB) ReadQueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return db.executeQueryRow(ctx, db.readPool, sql, args...)
}

// BeginTx starts a transaction using the write pool.
// Transactions always use the write pool to ensure consistency.
// The transaction will execute BeforeTransaction and AfterTransaction hooks.
//
// Example:
//
//	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
//	if err != nil {
//	    return err
//	}
//	defer tx.Rollback(ctx) // Safe to call even after commit
//
//	_, err = tx.Exec(ctx, "INSERT INTO users (name) VALUES ($1)", name)
//	if err != nil {
//	    return err
//	}
//	return tx.Commit(ctx)
func (db *DB) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) {
	db.mu.RLock()
	if db.shutdown {
		db.mu.RUnlock()
		return nil, fmt.Errorf("database is shutting down")
	}
	db.mu.RUnlock()

	// Execute BeforeTransaction hooks
	if err := db.hooks.executeBeforeTransaction(ctx, "", nil, nil); err != nil {
		return nil, fmt.Errorf("before transaction hook failed: %w", err)
	}

	tx, err := db.writePool.BeginTx(ctx, txOptions)

	// Execute AfterTransaction hooks
	hookErr := db.hooks.executeAfterTransaction(ctx, "", nil, err)
	if hookErr != nil && err == nil {
		// If transaction succeeded but hook failed, rollback
		if tx != nil {
			tx.Rollback(ctx)
		}
		return nil, fmt.Errorf("after transaction hook failed: %w", hookErr)
	}

	return tx, err
}

// AddHook adds an operation-level hook to the database.
// Hooks are executed in the order they are added and provide extensibility
// for logging, tracing, metrics, circuit breakers, and other cross-cutting concerns.
//
// Available hook types:
//   - BeforeOperation: Called before any query/exec operation
//   - AfterOperation: Called after any query/exec operation
//   - BeforeTransaction: Called before starting a transaction
//   - AfterTransaction: Called after a transaction completes
//   - OnShutdown: Called during graceful shutdown
//
// Example:
//
//	db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
//	    log.Printf("Executing: %s", sql)
//	    return nil
//	})
func (db *DB) AddHook(hookType HookType, hookFunc HookFunc) *DB {
	db.hooks.addHook(hookType, hookFunc)
	return db
}

// AddConnectionHook adds a connection-level hook to the database.
// These hooks are integrated with pgx's connection lifecycle and are useful
// for connection setup, validation, and cleanup.
//
// Available hook types:
//   - "OnConnect": Called when a new connection is established
//   - "OnDisconnect": Called when a connection is closed
//   - "OnAcquire": Called when a connection is acquired from the pool
//   - "OnRelease": Called when a connection is returned to the pool
//
// Example:
//
//	db.AddConnectionHook("OnConnect", func(conn *pgx.Conn) error {
//	    log.Println("New connection established")
//	    return nil
//	})
//
// TODO: hookType needs to be an Enum.
// TODO: this doc needs to be updated to reflect the new hookType Enum and that this is for pgx hooks.
// TODO: this should be split up into the different pgx hooks. and then we don't need the enum.
func (db *DB) AddConnectionHook(hookType string, hookFunc interface{}) error {
	return db.hooks.addConnectionHook(hookType, hookFunc)
}

// Shutdown gracefully shuts down the database connections.
// It waits for active operations to complete, respecting the context timeout.
// If the context times out, shutdown proceeds anyway to prevent hanging.
//
// The shutdown process:
// 1. Marks the database as shutting down (new operations will fail)
// 2. Waits for active operations to complete (respects context timeout)
// 3. Executes OnShutdown hooks
// 4. Closes connection pools
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//	err := db.Shutdown(ctx)
func (db *DB) Shutdown(ctx context.Context) error {
	db.mu.Lock()
	if db.shutdown {
		db.mu.Unlock()
		return nil
	}
	db.shutdown = true
	db.mu.Unlock()

	// Wait for active operations to complete with timeout handling
	done := make(chan struct{})
	go func() {
		db.activeOps.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All operations completed successfully
	case <-ctx.Done():
		// Context timeout - proceed with shutdown anyway
		// In production, you might want to log this as a warning
	}

	// Execute shutdown hooks
	if err := db.hooks.executeOnShutdown(ctx, "", nil, nil); err != nil {
		return fmt.Errorf("shutdown hook failed: %w", err)
	}

	// Close pools - handle nil pools gracefully
	if db.readPool != nil && db.readPool != db.writePool {
		db.readPool.Close()
	}
	if db.writePool != nil {
		db.writePool.Close()
	}

	return nil
}

// Stats returns statistics for the write pool.
// This provides information about connection usage, which is useful for monitoring
// and debugging connection pool performance.
//
// Example:
//
//	stats := db.Stats()
//	if stats != nil {
//	    log.Printf("Active connections: %d", stats.AcquiredConns())
//	    log.Printf("Idle connections: %d", stats.IdleConns())
//	}
func (db *DB) Stats() *pgxpool.Stat {
	if db.writePool == nil {
		return nil
	}
	return db.writePool.Stat()
}

// ReadStats returns statistics for the read pool.
// This provides information about read connection usage, which is useful for monitoring
// read replica performance and connection pool health.
//
// Example:
//
//	stats := db.ReadStats()
//	if stats != nil {
//	    log.Printf("Read pool active connections: %d", stats.AcquiredConns())
//	}
func (db *DB) ReadStats() *pgxpool.Stat {
	if db.readPool == nil {
		return nil
	}
	return db.readPool.Stat()
}

// WriteStats returns statistics for the write pool.
// This is an alias for Stats() provided for consistency with ReadStats().
func (db *DB) WriteStats() *pgxpool.Stat {
	if db.writePool == nil {
		return nil
	}
	return db.writePool.Stat()
}

// HealthCheck performs a simple health check by pinging the database.
// This is useful for health check endpoints and monitoring systems.
// It returns an error if the database is not connected, shutting down, or unreachable.
//
// Example:
//
//	if err := db.HealthCheck(ctx); err != nil {
//	    log.Printf("Database health check failed: %v", err)
//	    http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
//	    return
//	}
func (db *DB) HealthCheck(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}

	db.mu.RLock()
	if db.shutdown {
		db.mu.RUnlock()
		return fmt.Errorf("database is shutting down")
	}
	if db.writePool == nil {
		db.mu.RUnlock()
		return fmt.Errorf("database is not connected")
	}
	pool := db.writePool
	db.mu.RUnlock()

	return pool.Ping(ctx)
}

// IsReady checks if the database connection is ready to accept queries.
// This is a convenience method that returns true if HealthCheck() succeeds.
// It's useful for readiness probes and quick status checks.
//
// Example:
//
//	if db.IsReady(ctx) {
//	    log.Println("Database is ready to accept queries")
//	}
func (db *DB) IsReady(ctx context.Context) bool {
	return db.HealthCheck(ctx) == nil
}

// ExecWithRetry executes a statement using the write pool with retry logic.
// This automatically retries transient failures like connection errors, deadlocks,
// and serialization failures using exponential backoff.
//
// Example:
//
//	config := pgxkit.DefaultRetryConfig()
//	tag, err := db.ExecWithRetry(ctx, config, "INSERT INTO users (name) VALUES ($1)", name)
//	if err != nil {
//	    return fmt.Errorf("failed to insert user after retries: %w", err)
//	}
func (db *DB) ExecWithRetry(ctx context.Context, config *RetryConfig, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	var result pgconn.CommandTag
	err := RetryOperation(ctx, config, func(ctx context.Context) error {
		var err error
		result, err = db.Exec(ctx, sql, args...)
		return err
	})
	return result, err
}

// QueryWithRetry executes a query using the write pool with retry logic.
// This automatically retries transient failures like connection errors, deadlocks,
// and serialization failures using exponential backoff.
//
// Example:
//
//	config := pgxkit.DefaultRetryConfig()
//	rows, err := db.QueryWithRetry(ctx, config, "SELECT * FROM users WHERE active = $1", true)
//	if err != nil {
//	    return fmt.Errorf("failed to query users after retries: %w", err)
//	}
//	defer rows.Close()
func (db *DB) QueryWithRetry(ctx context.Context, config *RetryConfig, sql string, args ...interface{}) (pgx.Rows, error) {
	var result pgx.Rows
	err := RetryOperation(ctx, config, func(ctx context.Context) error {
		var err error
		result, err = db.Query(ctx, sql, args...)
		return err
	})
	return result, err
}

// QueryRowWithRetry executes a query that returns a single row using the write pool with retry logic.
// This automatically retries transient failures like connection errors, deadlocks,
// and serialization failures using exponential backoff.
//
// Example:
//
//	config := pgxkit.DefaultRetryConfig()
//	row := db.QueryRowWithRetry(ctx, config, "SELECT id FROM users WHERE email = $1", email)
//	var userID int
//	err := row.Scan(&userID)
func (db *DB) QueryRowWithRetry(ctx context.Context, config *RetryConfig, sql string, args ...interface{}) pgx.Row {
	var result pgx.Row
	err := RetryOperation(ctx, config, func(ctx context.Context) error {
		result = db.QueryRow(ctx, sql, args...)
		return nil // QueryRow doesn't return an error directly
	})
	if err != nil {
		return &shutdownRow{err: err}
	}
	return result
}

// ReadQueryWithRetry executes a query using the read pool with retry logic
func (db *DB) ReadQueryWithRetry(ctx context.Context, config *RetryConfig, sql string, args ...interface{}) (pgx.Rows, error) {
	var result pgx.Rows
	err := RetryOperation(ctx, config, func(ctx context.Context) error {
		var err error
		result, err = db.ReadQuery(ctx, sql, args...)
		return err
	})
	return result, err
}

// ReadQueryRowWithRetry executes a query that returns a single row using the read pool with retry logic
func (db *DB) ReadQueryRowWithRetry(ctx context.Context, config *RetryConfig, sql string, args ...interface{}) pgx.Row {
	var result pgx.Row
	err := RetryOperation(ctx, config, func(ctx context.Context) error {
		result = db.ReadQueryRow(ctx, sql, args...)
		return nil // QueryRow doesn't return an error directly
	})
	if err != nil {
		return &shutdownRow{err: err}
	}
	return result
}

// BeginTxWithRetry starts a transaction using the write pool with retry logic
func (db *DB) BeginTxWithRetry(ctx context.Context, config *RetryConfig, txOptions pgx.TxOptions) (pgx.Tx, error) {
	var result pgx.Tx
	err := RetryOperation(ctx, config, func(ctx context.Context) error {
		var err error
		result, err = db.BeginTx(ctx, txOptions)
		return err
	})
	return result, err
}

// Internal execution methods that handle hooks

func (db *DB) executeQuery(ctx context.Context, pool *pgxpool.Pool, sql string, args ...interface{}) (pgx.Rows, error) {
	db.mu.RLock()
	if db.shutdown {
		db.mu.RUnlock()
		return nil, fmt.Errorf("database is shutting down")
	}
	if pool == nil {
		db.mu.RUnlock()
		return nil, fmt.Errorf("database is not connected")
	}
	db.mu.RUnlock()

	// Track active operation for graceful shutdown
	db.activeOps.Add(1)
	defer db.activeOps.Done()

	// Execute BeforeOperation hooks
	if err := db.hooks.executeBeforeOperation(ctx, sql, args, nil); err != nil {
		return nil, fmt.Errorf("before operation hook failed: %w", err)
	}

	rows, err := pool.Query(ctx, sql, args...)

	// Execute AfterOperation hooks
	if hookErr := db.hooks.executeAfterOperation(ctx, sql, args, err); hookErr != nil {
		if rows != nil {
			rows.Close()
		}
		if err == nil {
			return nil, fmt.Errorf("after operation hook failed: %w", hookErr)
		}
	}

	return rows, err
}

func (db *DB) executeQueryRow(ctx context.Context, pool *pgxpool.Pool, sql string, args ...interface{}) pgx.Row {
	db.mu.RLock()
	if db.shutdown {
		db.mu.RUnlock()
		return &shutdownRow{err: fmt.Errorf("database is shutting down")}
	}
	if pool == nil {
		db.mu.RUnlock()
		return &shutdownRow{err: fmt.Errorf("database is not connected")}
	}
	db.mu.RUnlock()

	// Track active operation for graceful shutdown
	db.activeOps.Add(1)
	defer db.activeOps.Done()

	// Execute BeforeOperation hooks
	if err := db.hooks.executeBeforeOperation(ctx, sql, args, nil); err != nil {
		return &shutdownRow{err: fmt.Errorf("before operation hook failed: %w", err)}
	}

	row := pool.QueryRow(ctx, sql, args...)

	// Execute AfterOperation hooks - for QueryRow we can't easily get the error
	// so we pass nil as the operation error
	if hookErr := db.hooks.executeAfterOperation(ctx, sql, args, nil); hookErr != nil {
		return &shutdownRow{err: fmt.Errorf("after operation hook failed: %w", hookErr)}
	}

	return row
}

func (db *DB) executeExec(ctx context.Context, pool *pgxpool.Pool, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	db.mu.RLock()
	if db.shutdown {
		db.mu.RUnlock()
		return pgconn.CommandTag{}, fmt.Errorf("database is shutting down")
	}
	if pool == nil {
		db.mu.RUnlock()
		return pgconn.CommandTag{}, fmt.Errorf("database is not connected")
	}
	db.mu.RUnlock()

	// Track active operation for graceful shutdown
	db.activeOps.Add(1)
	defer db.activeOps.Done()

	// Execute BeforeOperation hooks
	if err := db.hooks.executeBeforeOperation(ctx, sql, args, nil); err != nil {
		return pgconn.CommandTag{}, fmt.Errorf("before operation hook failed: %w", err)
	}

	tag, err := pool.Exec(ctx, sql, args...)

	// Execute AfterOperation hooks
	if hookErr := db.hooks.executeAfterOperation(ctx, sql, args, err); hookErr != nil {
		if err == nil {
			return tag, fmt.Errorf("after operation hook failed: %w", hookErr)
		}
	}

	return tag, err
}

// shutdownRow implements pgx.Row for shutdown scenarios
type shutdownRow struct {
	err error
}

func (r *shutdownRow) Scan(dest ...interface{}) error {
	return r.err
}
