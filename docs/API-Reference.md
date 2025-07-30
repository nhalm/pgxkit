# API Reference

**[← Back to Home](Home)**

Complete API reference for pgxkit - the tool-agnostic PostgreSQL toolkit.

## Table of Contents

1. [Core Types](#core-types)
2. [Database Connection](#database-connection)
3. [Query Operations](#query-operations)
4. [Transaction Management](#transaction-management)
5. [Hook System](#hook-system)
6. [Retry Logic](#retry-logic)
7. [Type Helpers](#type-helpers)
8. [Health Checks](#health-checks)
9. [Testing Support](#testing-support)
10. [Utility Functions](#utility-functions)

## Core Types

### DB

```go
type DB struct {
    // Internal fields - access through methods
}
```

The main database abstraction that provides read/write pool management, hooks, and graceful shutdown capabilities.

**Key Features:**
- Safe-by-default design (all operations use write pool unless explicitly using Read* methods)
- Read/write pool abstraction
- Extensible hook system
- Graceful shutdown with operation tracking
- Built-in retry logic
- Connection pool statistics

### DBConfig

```go
type DBConfig struct {
    MaxConns        int32         // Maximum number of connections in the pool
    MinConns        int32         // Minimum number of connections in the pool
    MaxConnLifetime time.Duration // Maximum lifetime of a connection
    MaxConnIdleTime time.Duration // Maximum idle time for a connection
}
```

Configuration options for database connection pools.

## Database Connection

### NewDB

```go
func NewDB() *DB
```

Creates a new unconnected DB instance. Add hooks to this instance, then call `Connect()` to establish the database connection.

**Example:**
```go
db := pgxkit.NewDB()
db.AddHook(pgxkit.BeforeOperation, myLoggingHook)
err := db.Connect(ctx, "postgres://user:pass@localhost/db")
```

### Connect

```go
func (db *DB) Connect(ctx context.Context, dsn string) error
```

Establishes a database connection with a single pool (same pool for read/write). If dsn is empty, it uses environment variables to construct the connection string.

**Parameters:**
- `ctx` - Context for the connection operation
- `dsn` - Data source name (connection string). If empty, uses environment variables

**Environment Variables Used:**
- `POSTGRES_HOST` (default: "localhost")
- `POSTGRES_PORT` (default: 5432)
- `POSTGRES_USER` (default: "postgres")
- `POSTGRES_PASSWORD` (default: "")
- `POSTGRES_DB` (default: "postgres")
- `POSTGRES_SSLMODE` (default: "disable")

**Example:**
```go
// Using explicit DSN
err := db.Connect(ctx, "postgres://user:pass@localhost/db")

// Using environment variables
err := db.Connect(ctx, "")
```

### ConnectReadWrite

```go
func (db *DB) ConnectReadWrite(ctx context.Context, readDSN, writeDSN string) error
```

Establishes separate read and write database connections for optimal performance in read-heavy applications.

**Parameters:**
- `ctx` - Context for the connection operation
- `readDSN` - Data source name for read operations (replica)
- `writeDSN` - Data source name for write operations (primary)

**Example:**
```go
err := db.ConnectReadWrite(ctx,
    "postgres://user:pass@read-replica:5432/db",  // Read pool
    "postgres://user:pass@primary:5432/db")       // Write pool
```

### Shutdown

```go
func (db *DB) Shutdown(ctx context.Context) error
```

Gracefully shuts down the database connections, waiting for active operations to complete within the context timeout.

**Example:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
err := db.Shutdown(ctx)
```

### HealthCheck

```go
func (db *DB) HealthCheck(ctx context.Context) error
```

Performs a basic health check by executing a simple query to verify database connectivity.

**Example:**
```go
if err := db.HealthCheck(ctx); err != nil {
    log.Printf("Database health check failed: %v", err)
}
```

## Query Operations

### Query

```go
func (db *DB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
```

Executes a query using the write pool (safe by default). Returns rows for iteration.

**Example:**
```go
rows, err := db.Query(ctx, "SELECT id, name FROM users WHERE active = $1", true)
if err != nil {
    return err
}
defer rows.Close()

for rows.Next() {
    var id int
    var name string
    err := rows.Scan(&id, &name)
    // ...
}
```

### ReadQuery

```go
func (db *DB) ReadQuery(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
```

Executes a query using the read pool for optimization. Use for read-only operations when you have separate read/write pools.

**Example:**
```go
// Optimize reads with explicit read pool usage
rows, err := db.ReadQuery(ctx, "SELECT id, name FROM users")
```

### QueryRow

```go
func (db *DB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
```

Executes a query that returns a single row using the write pool.

**Example:**
```go
var count int
err := db.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
```

### ReadQueryRow

```go
func (db *DB) ReadQueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
```

Executes a query that returns a single row using the read pool for optimization.

**Example:**
```go
var user User
err := db.ReadQueryRow(ctx, "SELECT id, name FROM users WHERE id = $1", userID).
    Scan(&user.ID, &user.Name)
```

### Exec

```go
func (db *DB) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
```

Executes a command (INSERT, UPDATE, DELETE) using the write pool.

**Example:**
```go
tag, err := db.Exec(ctx, "INSERT INTO users (name, email) VALUES ($1, $2)", 
    "John Doe", "john@example.com")
if err != nil {
    return err
}
log.Printf("Inserted %d rows", tag.RowsAffected())
```

### ExecWithRetry

```go
func (db *DB) ExecWithRetry(ctx context.Context, config *RetryConfig, sql string, args ...interface{}) (pgconn.CommandTag, error)
```

Executes a command with automatic retry logic for transient failures.

**Example:**
```go
config := pgxkit.DefaultRetryConfig()
tag, err := db.ExecWithRetry(ctx, config, 
    "INSERT INTO users (name, email) VALUES ($1, $2)", 
    "John Doe", "john@example.com")
```

## Transaction Management

### BeginTx

```go
func (db *DB) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
```

Begins a new transaction using the write pool with the specified options.

**Example:**
```go
tx, err := db.BeginTx(ctx, pgx.TxOptions{
    IsoLevel: pgx.ReadCommitted,
})
if err != nil {
    return err
}
defer tx.Rollback(ctx)

// Use transaction...

err = tx.Commit(ctx)
```

### WithTransaction

```go
func (db *DB) WithTransaction(ctx context.Context, txOptions pgx.TxOptions, fn func(pgx.Tx) error) error
```

Executes a function within a transaction, automatically handling commit/rollback.

**Example:**
```go
err := db.WithTransaction(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
    // Insert user
    _, err := tx.Exec(ctx, "INSERT INTO users (name) VALUES ($1)", "John")
    if err != nil {
        return err
    }
    
    // Insert profile
    _, err = tx.Exec(ctx, "INSERT INTO profiles (user_id, bio) VALUES ($1, $2)", 
        userID, "Software Developer")
    return err
})
```

## Hook System

### HookType

```go
type HookType int

const (
    BeforeOperation   HookType = iota // Called before any query/exec operation
    AfterOperation                    // Called after any query/exec operation
    BeforeTransaction                 // Called before starting a transaction
    AfterTransaction                  // Called after a transaction completes
    OnShutdown                        // Called during graceful shutdown
)
```

### HookFunc

```go
type HookFunc func(ctx context.Context, sql string, args []interface{}, operationErr error) error
```

Universal hook function signature for operation-level hooks.

**Parameters:**
- `ctx` - The context for the operation
- `sql` - The SQL statement being executed (empty for shutdown hooks)
- `args` - The arguments for the SQL statement (nil for shutdown hooks)
- `operationErr` - The error from the operation (nil for before hooks)

### AddHook

```go
func (db *DB) AddHook(hookType HookType, hookFunc HookFunc)
```

Adds a hook function for the specified hook type.

**Example:**
```go
// Logging hook
db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    log.Printf("Executing: %s", sql)
    return nil
})

// Metrics hook
db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    duration := time.Since(start) // start from context
    queryCounter.WithLabelValues(operation, status).Inc()
    queryDuration.WithLabelValues(operation, status).Observe(duration.Seconds())
    return nil
})
```

## Retry Logic

### RetryConfig

```go
type RetryConfig struct {
    MaxRetries int           // Maximum number of retry attempts
    BaseDelay  time.Duration // Initial delay between retries
    MaxDelay   time.Duration // Maximum delay between retries
    Multiplier float64       // Multiplier for exponential backoff
}
```

Configuration for retry logic with exponential backoff.

### DefaultRetryConfig

```go
func DefaultRetryConfig() *RetryConfig
```

Returns a sensible default retry configuration (3 retries, 100ms base delay, 1s max delay, 2x multiplier).

**Example:**
```go
config := pgxkit.DefaultRetryConfig()
// Customize if needed:
config.MaxRetries = 5
config.MaxDelay = 5 * time.Second
```

### WithTimeout

```go
func WithTimeout[T any](ctx context.Context, timeout time.Duration, fn func(context.Context) (T, error)) (T, error)
```

Generic utility function that executes a function with a timeout.

**Example:**
```go
result, err := pgxkit.WithTimeout(ctx, 5*time.Second, func(ctx context.Context) (*User, error) {
    return getUserFromDatabase(ctx)
})
```

## Type Helpers

### String/Text Conversions

```go
func ToPgxText(s *string) pgtype.Text
func FromPgxText(t pgtype.Text) *string
func ToPgxTextFromString(s string) pgtype.Text
func FromPgxTextToString(t pgtype.Text) string
```

Seamless conversion between Go strings and pgx text types, handling null values appropriately.

**Example:**
```go
// Using with nullable strings
var name *string = nil
pgxName := pgxkit.ToPgxText(name) // Results in NULL

// Using with regular strings
pgxName := pgxkit.ToPgxTextFromString("John Doe")
```

### Integer Conversions

```go
func ToPgxInt8(i *int64) pgtype.Int8
func FromPgxInt8(i pgtype.Int8) *int64
func ToPgxInt4(i *int32) pgtype.Int4
func FromPgxInt4(i pgtype.Int4) *int32
func ToPgxInt2(i *int16) pgtype.Int2
func FromPgxInt2(i pgtype.Int2) *int16
```

Convert between Go integers and pgx integer types with null handling.

### Boolean Conversions

```go
func ToPgxBool(b *bool) pgtype.Bool
func FromPgxBool(b pgtype.Bool) *bool
```

Convert between Go booleans and pgx boolean types.

### Time Conversions

```go
func ToPgxTimestamp(t *time.Time) pgtype.Timestamp
func FromPgxTimestamp(t pgtype.Timestamp) *time.Time
func ToPgxDate(t *time.Time) pgtype.Date
func FromPgxDate(d pgtype.Date) *time.Time
```

Convert between Go time.Time and pgx timestamp/date types.

### UUID Conversions

```go
func ToPgxUUID(u *uuid.UUID) pgtype.UUID
func FromPgxUUID(u pgtype.UUID) *uuid.UUID
```

Convert between Google UUID and pgx UUID types.

### JSON Helpers

```go
func JSON[T any](data T) interface{}
func ScanJSON[T any](dest *T) interface{}
```

Generic helpers for JSON column handling.

**Example:**
```go
type UserSettings struct {
    Theme    string `json:"theme"`
    Language string `json:"language"`
}

settings := UserSettings{Theme: "dark", Language: "en"}

// Insert JSON
_, err := db.Exec(ctx, "UPDATE users SET settings = $1 WHERE id = $2", 
    pgxkit.JSON(settings), userID)

// Query JSON
var retrievedSettings UserSettings
err := db.QueryRow(ctx, "SELECT settings FROM users WHERE id = $1", userID).
    Scan(pgxkit.ScanJSON(&retrievedSettings))
```

## Health Checks

### Stats

```go
func (db *DB) Stats() *pgxpool.Stat
```

Returns connection pool statistics for the write pool.

**Example:**
```go
stats := db.Stats()
if stats != nil {
    utilization := float64(stats.AcquiredConns()) / float64(stats.MaxConns())
    log.Printf("Pool utilization: %.2f%%", utilization*100)
}
```

### ReadStats

```go
func (db *DB) ReadStats() *pgxpool.Stat
```

Returns connection pool statistics for the read pool (if using read/write split).

## Testing Support

### TestDB

```go
type TestDB struct {
    // Internal fields - access through methods
}
```

Testing utilities for database operations in tests.

### NewTestDB

```go
func NewTestDB() *TestDB
```

Creates a new test database instance with testing utilities.

### EnableGolden

```go
func (tdb *TestDB) EnableGolden(t *testing.T, testName string) *DB
```

Enables golden testing to capture and compare query execution plans for performance regression detection.

**Example:**
```go
func TestUserQueries(t *testing.T) {
    testDB := pgxkit.NewTestDB()
    db := testDB.EnableGolden(t, "TestUserQueries")
    
    // Queries will have their EXPLAIN plans captured
    rows, err := db.Query(ctx, "SELECT * FROM users WHERE active = true")
    // ...
}
```

## Utility Functions

### GetDSN

```go
func GetDSN() string
```

Returns a PostgreSQL connection string built from environment variables. Useful for scripts and tools that need a connection string.

**Example:**
```go
dsn := pgxkit.GetDSN()
// Use with other tools that need a connection string
```

## Error Handling

pgxkit provides PostgreSQL-aware error handling and categorization:

- **Connection errors** - Detected and handled by retry logic
- **Constraint violations** - Preserved and wrapped for application handling
- **Timeout errors** - Detected and can trigger circuit breaker logic
- **Serialization failures** - Automatically retried when using retry logic

## Thread Safety

All `DB` methods are thread-safe and can be called concurrently from multiple goroutines. The underlying connection pools handle concurrent access safely.

## Best Practices

1. **Use Read methods for optimization** - `ReadQuery()` and `ReadQueryRow()` when you have read/write splits
2. **Add hooks early** - Add hooks before calling `Connect()` for proper integration
3. **Handle context cancellation** - All methods respect context cancellation
4. **Use transactions for consistency** - Group related operations in transactions
5. **Monitor pool statistics** - Use `Stats()` and `ReadStats()` for monitoring
6. **Implement graceful shutdown** - Use `Shutdown()` with appropriate timeout

## See Also

- **[Getting Started](Getting-Started)** - Basic setup and configuration
- **[Examples](Examples)** - Practical code examples
- **[Performance Guide](Performance-Guide)** - Optimization strategies
- **[Production Guide](Production-Guide)** - Deployment best practices
- **[Testing Guide](Testing-Guide)** - Testing strategies

---

**[← Back to Home](Home)**

*This API reference covers all public types, functions, and methods in pgxkit. For practical usage examples, see the [Examples](Examples) page.*

*Last updated: December 2024* 