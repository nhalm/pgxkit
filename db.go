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
//	err := db.Connect(ctx, "", pgxkit.WithMaxConns(25))
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
//	db := pgxkit.NewDB()
//	err := db.Connect(ctx, "",
//	    pgxkit.WithBeforeOperation(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
//	        log.Printf("Executing: %s", sql)
//	        return nil
//	    }),
//	)
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

// Executor is a unified interface for database operations that both *DB and *Tx implement.
// This allows passing a single interface for database operations whether in a transaction or not.
type Executor interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
}

func getEnvWithDefault(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}

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

func getDSN() string {
	return getDSNWithSearchPath("")
}

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
	activeOps sync.WaitGroup
}

// ConnectOption configures a database connection.
type ConnectOption func(*connectConfig)

type connectConfig struct {
	maxConns        int32
	minConns        int32
	maxConnLifetime time.Duration
	maxConnIdleTime time.Duration
	readMaxConns    int32
	readMinConns    int32
	writeMaxConns   int32
	writeMinConns   int32
	hooks           *hooks
}

func newConnectConfig() *connectConfig {
	return &connectConfig{
		hooks: newHooks(),
	}
}

func WithMaxConns(n int32) ConnectOption {
	return func(c *connectConfig) {
		if n > 0 {
			c.maxConns = n
		}
	}
}

func WithMinConns(n int32) ConnectOption {
	return func(c *connectConfig) {
		if n >= 0 {
			c.minConns = n
		}
	}
}

func WithMaxConnLifetime(d time.Duration) ConnectOption {
	return func(c *connectConfig) {
		if d > 0 {
			c.maxConnLifetime = d
		}
	}
}

func WithMaxConnIdleTime(d time.Duration) ConnectOption {
	return func(c *connectConfig) {
		if d > 0 {
			c.maxConnIdleTime = d
		}
	}
}

func WithReadMaxConns(n int32) ConnectOption {
	return func(c *connectConfig) {
		if n > 0 {
			c.readMaxConns = n
		}
	}
}

func WithReadMinConns(n int32) ConnectOption {
	return func(c *connectConfig) {
		if n >= 0 {
			c.readMinConns = n
		}
	}
}

func WithWriteMaxConns(n int32) ConnectOption {
	return func(c *connectConfig) {
		if n > 0 {
			c.writeMaxConns = n
		}
	}
}

func WithWriteMinConns(n int32) ConnectOption {
	return func(c *connectConfig) {
		if n >= 0 {
			c.writeMinConns = n
		}
	}
}

func WithBeforeOperation(fn HookFunc) ConnectOption {
	return func(c *connectConfig) {
		c.hooks.addHook(BeforeOperation, fn)
	}
}

func WithAfterOperation(fn HookFunc) ConnectOption {
	return func(c *connectConfig) {
		c.hooks.addHook(AfterOperation, fn)
	}
}

func WithBeforeTransaction(fn HookFunc) ConnectOption {
	return func(c *connectConfig) {
		c.hooks.addHook(BeforeTransaction, fn)
	}
}

func WithAfterTransaction(fn HookFunc) ConnectOption {
	return func(c *connectConfig) {
		c.hooks.addHook(AfterTransaction, fn)
	}
}

func WithOnShutdown(fn HookFunc) ConnectOption {
	return func(c *connectConfig) {
		c.hooks.addHook(OnShutdown, fn)
	}
}

func WithOnConnect(fn func(*pgx.Conn) error) ConnectOption {
	return func(c *connectConfig) {
		c.hooks.connectionHooks.addOnConnect(fn)
	}
}

func WithOnDisconnect(fn func(*pgx.Conn)) ConnectOption {
	return func(c *connectConfig) {
		c.hooks.connectionHooks.addOnDisconnect(fn)
	}
}

func WithOnAcquire(fn func(context.Context, *pgx.Conn) error) ConnectOption {
	return func(c *connectConfig) {
		c.hooks.connectionHooks.addOnAcquire(fn)
	}
}

func WithOnRelease(fn func(*pgx.Conn)) ConnectOption {
	return func(c *connectConfig) {
		c.hooks.connectionHooks.addOnRelease(fn)
	}
}

// NewDB creates a new unconnected DB instance.
// Call Connect() with options to establish the database connection.
//
// Example:
//
//	db := pgxkit.NewDB()
//	err := db.Connect(ctx, "postgres://user:pass@localhost/db",
//	    pgxkit.WithMaxConns(25),
//	    pgxkit.WithBeforeOperation(myLoggingHook),
//	)
func NewDB() *DB {
	return &DB{
		hooks: newHooks(),
	}
}

// Connect establishes a database connection with a single pool (same pool for read/write).
// If dsn is empty, it uses environment variables to construct the connection string.
// Options are applied to configure pool settings and hooks.
//
// This is the recommended approach for most applications as it provides safety
// by default while still allowing read optimization through ReadQuery methods.
//
// Example:
//
//	db := pgxkit.NewDB()
//	err := db.Connect(ctx, "postgres://user:pass@localhost/db",
//	    pgxkit.WithMaxConns(25),
//	    pgxkit.WithOnConnect(func(conn *pgx.Conn) error {
//	        _, err := conn.Exec(context.Background(), "SET application_name = 'myapp'")
//	        return err
//	    }),
//	)
//	// Or use environment variables:
//	err := db.Connect(ctx, "")
func (db *DB) Connect(ctx context.Context, dsn string, opts ...ConnectOption) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.readPool != nil || db.writePool != nil {
		return fmt.Errorf("database is already connected")
	}

	if dsn == "" {
		dsn = getDSN()
	}

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("failed to parse DSN: %w", err)
	}

	cfg := newConnectConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.maxConns > 0 {
		config.MaxConns = cfg.maxConns
	}
	if cfg.minConns > 0 {
		config.MinConns = cfg.minConns
	}
	if cfg.maxConnLifetime > 0 {
		config.MaxConnLifetime = cfg.maxConnLifetime
	}
	if cfg.maxConnIdleTime > 0 {
		config.MaxConnIdleTime = cfg.maxConnIdleTime
	}

	db.hooks = cfg.hooks
	db.hooks.configurePool(config)

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
// Options are applied to both pools.
//
// This is useful for applications that want to optimize read performance by routing
// read queries to read replicas while ensuring writes go to the primary database.
//
// Example:
//
//	db := pgxkit.NewDB()
//	err := db.ConnectReadWrite(ctx, "postgres://user:pass@read-replica/db", "postgres://user:pass@primary/db",
//	    pgxkit.WithMaxConns(25),
//	)
//	// Now ReadQuery methods will use the read pool, while Query/Exec use the write pool
func (db *DB) ConnectReadWrite(ctx context.Context, readDSN, writeDSN string, opts ...ConnectOption) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.readPool != nil || db.writePool != nil {
		return fmt.Errorf("database is already connected")
	}

	if readDSN == "" {
		readDSN = getDSN()
	}
	if writeDSN == "" {
		writeDSN = getDSN()
	}

	readConfig, err := pgxpool.ParseConfig(readDSN)
	if err != nil {
		return fmt.Errorf("failed to parse read DSN: %w", err)
	}

	writeConfig, err := pgxpool.ParseConfig(writeDSN)
	if err != nil {
		return fmt.Errorf("failed to parse write DSN: %w", err)
	}

	cfg := newConnectConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	readMaxConns := cfg.maxConns
	if cfg.readMaxConns > 0 {
		readMaxConns = cfg.readMaxConns
	}
	if readMaxConns > 0 {
		readConfig.MaxConns = readMaxConns
	}

	readMinConns := cfg.minConns
	if cfg.readMinConns > 0 {
		readMinConns = cfg.readMinConns
	}
	if readMinConns > 0 {
		readConfig.MinConns = readMinConns
	}

	writeMaxConns := cfg.maxConns
	if cfg.writeMaxConns > 0 {
		writeMaxConns = cfg.writeMaxConns
	}
	if writeMaxConns > 0 {
		writeConfig.MaxConns = writeMaxConns
	}

	writeMinConns := cfg.minConns
	if cfg.writeMinConns > 0 {
		writeMinConns = cfg.writeMinConns
	}
	if writeMinConns > 0 {
		writeConfig.MinConns = writeMinConns
	}

	if cfg.maxConnLifetime > 0 {
		readConfig.MaxConnLifetime = cfg.maxConnLifetime
		writeConfig.MaxConnLifetime = cfg.maxConnLifetime
	}
	if cfg.maxConnIdleTime > 0 {
		readConfig.MaxConnIdleTime = cfg.maxConnIdleTime
		writeConfig.MaxConnIdleTime = cfg.maxConnIdleTime
	}

	db.hooks = cfg.hooks
	db.hooks.configurePool(readConfig)
	db.hooks.configurePool(writeConfig)

	readPool, err := pgxpool.NewWithConfig(ctx, readConfig)
	if err != nil {
		return fmt.Errorf("failed to create read pool: %w", err)
	}

	writePool, err := pgxpool.NewWithConfig(ctx, writeConfig)
	if err != nil {
		readPool.Close()
		return fmt.Errorf("failed to create write pool: %w", err)
	}

	db.readPool = readPool
	db.writePool = writePool

	return nil
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

	if err := db.hooks.executeBeforeTransaction(ctx, "", nil, nil); err != nil {
		return nil, fmt.Errorf("before transaction hook failed: %w", err)
	}

	tx, err := db.writePool.BeginTx(ctx, txOptions)

	hookErr := db.hooks.executeAfterTransaction(ctx, "", nil, err)
	if hookErr != nil && err == nil {
		if tx != nil {
			tx.Rollback(ctx)
		}
		return nil, fmt.Errorf("after transaction hook failed: %w", hookErr)
	}

	return tx, err
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

	done := make(chan struct{})
	go func() {
		db.activeOps.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}

	if err := db.hooks.executeOnShutdown(ctx, "", nil, nil); err != nil {
		return fmt.Errorf("shutdown hook failed: %w", err)
	}

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

// WritePool returns the underlying write connection pool.
// Useful for integrating with code generation tools like sqlc.
func (db *DB) WritePool() *pgxpool.Pool {
	return db.writePool
}

// ReadPool returns the underlying read connection pool.
// Returns nil if no separate read pool is configured.
func (db *DB) ReadPool() *pgxpool.Pool {
	return db.readPool
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

	db.activeOps.Add(1)
	defer db.activeOps.Done()

	if err := db.hooks.executeBeforeOperation(ctx, sql, args, nil); err != nil {
		return nil, fmt.Errorf("before operation hook failed: %w", err)
	}

	rows, err := pool.Query(ctx, sql, args...)

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

	db.activeOps.Add(1)
	defer db.activeOps.Done()

	if err := db.hooks.executeBeforeOperation(ctx, sql, args, nil); err != nil {
		return &shutdownRow{err: fmt.Errorf("before operation hook failed: %w", err)}
	}

	row := pool.QueryRow(ctx, sql, args...)

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

	db.activeOps.Add(1)
	defer db.activeOps.Done()

	if err := db.hooks.executeBeforeOperation(ctx, sql, args, nil); err != nil {
		return pgconn.CommandTag{}, fmt.Errorf("before operation hook failed: %w", err)
	}

	tag, err := pool.Exec(ctx, sql, args...)

	if hookErr := db.hooks.executeAfterOperation(ctx, sql, args, err); hookErr != nil {
		if err == nil {
			return tag, fmt.Errorf("after operation hook failed: %w", hookErr)
		}
	}

	return tag, err
}

type shutdownRow struct {
	err error
}

func (r *shutdownRow) Scan(dest ...interface{}) error {
	return r.err
}
