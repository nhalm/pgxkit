# API Reference

**[<- Back to Home](Home)**

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

### Executor

```go
type Executor interface {
    Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
    Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
}
```

A unified interface for database operations that both `*DB` and `*Tx` implement. This allows writing functions that work with both regular database connections and transactions.

**Example:**
```go
// Function that works with both *DB and *Tx
func CreateUser(ctx context.Context, exec pgxkit.Executor, name string) (int, error) {
    var id int
    err := exec.QueryRow(ctx, "INSERT INTO users (name) VALUES ($1) RETURNING id", name).Scan(&id)
    return id, err
}

// Works with *DB
id, err := CreateUser(ctx, db, "Alice")

// Works with *Tx
tx, _ := db.BeginTx(ctx, pgx.TxOptions{})
id, err := CreateUser(ctx, tx, "Bob")
```

### DB

```go
type DB struct {
    // Internal fields - access through methods
}
```

The main database abstraction that provides read/write pool management, hooks, and graceful shutdown capabilities. Implements the `Executor` interface.

**Key Features:**
- Safe-by-default design (all operations use write pool unless explicitly using Read* methods)
- Read/write pool abstraction
- Extensible hook system
- Graceful shutdown with operation tracking
- Connection pool statistics

### Tx

```go
type Tx struct {
    // Internal fields - access through methods
}
```

Wraps a pgx.Tx to implement the `Executor` interface and provide transaction lifecycle management integrated with pgxkit's activeOps tracking. The `*Tx` type is NOT goroutine-safe.

**Methods:**

| Method | Description |
|--------|-------------|
| `Query(ctx, sql, args...)` | Execute a query within the transaction |
| `QueryRow(ctx, sql, args...)` | Execute a query returning a single row |
| `Exec(ctx, sql, args...)` | Execute a statement within the transaction |
| `Commit(ctx)` | Commit the transaction |
| `Rollback(ctx)` | Rollback the transaction |
| `Tx()` | Return the underlying pgx.Tx for advanced use cases |
| `IsFinalized()` | Returns true if transaction has been committed or rolled back |

**Commit and Rollback:**
- Fire the AfterTransaction hook
- Use atomic finalization to ensure activeOps.Done() is called exactly once
- Safe for the "defer Rollback() + explicit Commit()" pattern
- Propagate AfterTransaction hook errors when the underlying operation succeeds

**Example:**
```go
tx, err := db.BeginTx(ctx, pgx.TxOptions{})
if err != nil {
    return err
}
defer tx.Rollback(ctx) // Safe no-op if already committed

_, err = tx.Exec(ctx, "INSERT INTO users (name) VALUES ($1)", "Alice")
if err != nil {
    return err
}

return tx.Commit(ctx)
```

## Database Connection

### NewDB

```go
func NewDB() *DB
```

Creates a new unconnected DB instance. Call `Connect()` with options to establish the database connection.

**Example:**
```go
db := pgxkit.NewDB()
err := db.Connect(ctx, "postgres://user:pass@localhost/db",
    pgxkit.WithMaxConns(25),
    pgxkit.WithBeforeOperation(myLoggingHook),
)
```

### Connect

```go
func (db *DB) Connect(ctx context.Context, dsn string, opts ...ConnectOption) error
```

Establishes a database connection with a single pool (same pool for read/write). If dsn is empty, it uses environment variables to construct the connection string.

**Parameters:**
- `ctx` - Context for the connection operation
- `dsn` - Data source name (connection string). If empty, uses environment variables
- `opts` - Variadic connection options for pool configuration and hooks

**Environment Variables Used:**
- `POSTGRES_HOST` (default: "localhost")
- `POSTGRES_PORT` (default: 5432)
- `POSTGRES_USER` (default: "postgres")
- `POSTGRES_PASSWORD` (default: "")
- `POSTGRES_DB` (default: "postgres")
- `POSTGRES_SSLMODE` (default: "disable")

**Example:**
```go
// Using explicit DSN with options
err := db.Connect(ctx, "postgres://user:pass@localhost/db",
    pgxkit.WithMaxConns(25),
    pgxkit.WithMinConns(5),
    pgxkit.WithBeforeOperation(loggingHook),
)

// Using environment variables
err := db.Connect(ctx, "")
```

### ConnectReadWrite

```go
func (db *DB) ConnectReadWrite(ctx context.Context, readDSN, writeDSN string, opts ...ConnectOption) error
```

Establishes separate read and write database connections for optimal performance in read-heavy applications.

**Parameters:**
- `ctx` - Context for the connection operation
- `readDSN` - Data source name for read operations (replica)
- `writeDSN` - Data source name for write operations (primary)
- `opts` - Variadic connection options (applied to both pools)

**Example:**
```go
err := db.ConnectReadWrite(ctx,
    "postgres://user:pass@read-replica:5432/db",  // Read pool
    "postgres://user:pass@primary:5432/db",       // Write pool
    pgxkit.WithMaxConns(25),
    pgxkit.WithBeforeOperation(loggingHook),
)
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

## Transaction Management

### BeginTx

```go
func (db *DB) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (*Tx, error)
```

Begins a new transaction using the write pool with the specified options. Returns a `*Tx` that implements the `Executor` interface.

The transaction:
- Fires the BeforeTransaction hook on start
- Fires the AfterTransaction hook on Commit/Rollback
- Is tracked by activeOps for graceful shutdown

**Example:**
```go
tx, err := db.BeginTx(ctx, pgx.TxOptions{
    IsoLevel: pgx.ReadCommitted,
})
if err != nil {
    return err
}
defer tx.Rollback(ctx) // Safe no-op if already committed

_, err = tx.Exec(ctx, "INSERT INTO users (name) VALUES ($1)", name)
if err != nil {
    return err
}

return tx.Commit(ctx)
```

### Tx Methods

#### Query

```go
func (t *Tx) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
```

Executes a query within the transaction. Unlike DB.Query, this does not fire BeforeOperation/AfterOperation hooks.

#### QueryRow

```go
func (t *Tx) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
```

Executes a query that returns a single row within the transaction. Unlike DB.QueryRow, this does not fire BeforeOperation/AfterOperation hooks.

#### Exec

```go
func (t *Tx) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
```

Executes a statement within the transaction. Unlike DB.Exec, this does not fire BeforeOperation/AfterOperation hooks.

#### Commit

```go
func (t *Tx) Commit(ctx context.Context) error
```

Commits the transaction. Fires the AfterTransaction hook and propagates hook errors when the commit succeeds. Uses atomic finalization to ensure activeOps.Done() is called exactly once.

#### Rollback

```go
func (t *Tx) Rollback(ctx context.Context) error
```

Rolls back the transaction. Fires the AfterTransaction hook and propagates hook errors when the rollback succeeds. Uses atomic finalization to ensure activeOps.Done() is called exactly once. Safe to call after Commit (returns nil).

#### Tx

```go
func (t *Tx) Tx() pgx.Tx
```

Returns the underlying pgx.Tx for advanced use cases that require direct access to pgx transaction functionality.

## Hook System

Hooks are configured via `ConnectOption` functions passed to `Connect()` or `ConnectReadWrite()`.

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

### Operation Hook Options

```go
func WithBeforeOperation(fn HookFunc) ConnectOption
func WithAfterOperation(fn HookFunc) ConnectOption
func WithBeforeTransaction(fn HookFunc) ConnectOption
func WithAfterTransaction(fn HookFunc) ConnectOption
func WithOnShutdown(fn HookFunc) ConnectOption
```

### Connection Hook Options

```go
func WithOnConnect(fn func(*pgx.Conn) error) ConnectOption
func WithOnDisconnect(fn func(*pgx.Conn)) ConnectOption
func WithOnAcquire(fn func(context.Context, *pgx.Conn) error) ConnectOption
func WithOnRelease(fn func(*pgx.Conn)) ConnectOption
```

**Example:**
```go
db := pgxkit.NewDB()
err := db.Connect(ctx, dsn,
    // Logging hook
    pgxkit.WithBeforeOperation(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        log.Printf("Executing: %s", sql)
        return nil
    }),
    // Metrics hook
    pgxkit.WithAfterOperation(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        queryCounter.WithLabelValues(operation, status).Inc()
        return nil
    }),
    // Connection setup
    pgxkit.WithOnConnect(func(conn *pgx.Conn) error {
        _, err := conn.Exec(context.Background(), "SET application_name = 'myapp'")
        return err
    }),
)
```

### Pool Configuration Options

```go
func WithMaxConns(n int32) ConnectOption
func WithMinConns(n int32) ConnectOption
func WithMaxConnLifetime(d time.Duration) ConnectOption
func WithMaxConnIdleTime(d time.Duration) ConnectOption
```

**Example:**
```go
err := db.Connect(ctx, dsn,
    pgxkit.WithMaxConns(25),
    pgxkit.WithMinConns(5),
    pgxkit.WithMaxConnLifetime(time.Hour),
    pgxkit.WithMaxConnIdleTime(30*time.Minute),
)
```

## Retry Logic

### RetryOption

```go
type RetryOption func(*retryConfig)
```

Functional option for configuring retry behavior.

### Retry Option Functions

```go
func WithMaxRetries(n int) RetryOption        // Maximum retry attempts (default: 3)
func WithBaseDelay(d time.Duration) RetryOption   // Initial delay (default: 100ms)
func WithMaxDelay(d time.Duration) RetryOption    // Maximum delay (default: 1s)
func WithBackoffMultiplier(m float64) RetryOption // Backoff multiplier (default: 2.0)
```

### Timeout Behavior

The timeout (set via `context.WithTimeout`) applies to **all retry attempts combined**, not per-attempt. If your timeout is 5 seconds and the first attempt takes 3 seconds, subsequent retries share the remaining 2 seconds.

### Retryable Errors

The retry logic only retries specific transient errors that may succeed on subsequent attempts:

| Error Type | Retries? | Examples |
|------------|----------|----------|
| Network timeouts | Yes | context deadline exceeded during dial |
| Connection failures | Yes | connection refused, connection reset |
| PostgreSQL connection errors | Yes | 08000, 08003, 08006 |
| Server shutdown | Yes | 57P01, 57P02, 57P03 |
| Serialization/deadlock | Yes | 40001, 40P01 |
| Context cancellation | No | context canceled |
| No rows found | No | pgx.ErrNoRows |
| Constraint violations | No | unique_violation, foreign_key_violation |
| Syntax errors | No | syntax_error |

### IsRetryableError

```go
func IsRetryableError(err error) bool
```

Check if an error would be retried by the retry logic. Useful for custom retry handling or logging.

**Example:**
```go
if pgxkit.IsRetryableError(err) {
    log.Println("Transient error - would retry")
} else {
    log.Println("Permanent error - would not retry")
}
```

### RetryOperation

```go
func RetryOperation(ctx context.Context, operation func(ctx context.Context) error, opts ...RetryOption) error
```

Retries any operation that returns an error, using exponential backoff. Only retryable errors (connection issues, deadlocks, serialization failures) trigger retries.

**Example:**
```go
// Retry with default settings
err := pgxkit.RetryOperation(ctx, func(ctx context.Context) error {
    _, err := db.Exec(ctx, "INSERT INTO users (name, email) VALUES ($1, $2)",
        "John Doe", "john@example.com")
    return err
})

// Retry with custom configuration
err = pgxkit.RetryOperation(ctx, func(ctx context.Context) error {
    rows, err := db.Query(ctx, "SELECT * FROM users WHERE active = true")
    if err != nil {
        return err
    }
    defer rows.Close()
    // Process rows...
    return nil
}, pgxkit.WithMaxRetries(5), pgxkit.WithMaxDelay(5*time.Second))

// Retry a transaction
err = pgxkit.RetryOperation(ctx, func(ctx context.Context) error {
    tx, err := db.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)

    // Perform transaction operations...

    return tx.Commit(ctx)
}, pgxkit.WithMaxRetries(3))
```

### Retry

```go
func Retry[T any](ctx context.Context, fn func(context.Context) (T, error), opts ...RetryOption) (T, error)
```

Generic retry function that retries an operation returning a value and error, using exponential backoff. Only retryable errors (connection issues, deadlocks, serialization failures) trigger retries.

**Example:**
```go
// Basic retry returning a value
user, err := pgxkit.Retry(ctx, func(ctx context.Context) (*User, error) {
    return getUserFromDatabase(ctx)
})

// Retry with timeout using context.WithTimeout
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()
user, err := pgxkit.Retry(ctx, func(ctx context.Context) (*User, error) {
    return getUserFromDatabase(ctx)
}, pgxkit.WithMaxRetries(3))
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
func ToPgxUUID(id uuid.UUID) pgtype.UUID
func FromPgxUUID(pgxID pgtype.UUID) uuid.UUID
func ToPgxUUIDFromPtr(id *uuid.UUID) pgtype.UUID
func FromPgxUUIDToPtr(pgxID pgtype.UUID) *uuid.UUID
```

Convert between Google UUID and pgx UUID types. The base functions work with values directly, while the `Ptr` variants handle nullable UUIDs via pointers.

**Example:**
```go
// Using with non-nullable UUIDs (value types)
id := uuid.New()
pgxID := pgxkit.ToPgxUUID(id)
goID := pgxkit.FromPgxUUID(pgxID)

// Using with nullable UUIDs (pointer types)
var nullableID *uuid.UUID = nil
pgxID := pgxkit.ToPgxUUIDFromPtr(nullableID) // Results in NULL
goID := pgxkit.FromPgxUUIDToPtr(pgxID)       // Returns nil for NULL
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

Creates a new unconnected test database instance with testing utilities. Call `Connect()` to establish the database connection.

**Example:**
```go
func TestUserQueries(t *testing.T) {
    testDB := pgxkit.NewTestDB()
    ctx := context.Background()
    err := testDB.Connect(ctx, "") // Uses TEST_DATABASE_URL env var
    if err != nil {
        t.Skip("Test database not available:", err)
    }
    defer testDB.Shutdown(ctx)

    // Or connect with explicit DSN:
    // err := testDB.Connect(ctx, "postgres://user:pass@localhost/testdb")
}
```

### EnableGolden

```go
func (tdb *TestDB) EnableGolden(testName string) *DB
```

Enables golden testing to capture and compare query execution plans for performance regression detection. Supports SELECT, INSERT, UPDATE, and DELETE queries. DML operations are executed within a transaction that is rolled back to avoid side effects.

**Example:**
```go
func TestUserQueries(t *testing.T) {
    testDB := pgxkit.NewTestDB()
    ctx := context.Background()
    err := testDB.Connect(ctx, "")
    if err != nil {
        t.Skip("Test database not available:", err)
    }
    defer testDB.Shutdown(ctx)

    db := testDB.EnableGolden("TestUserQueries")

    // SELECT queries have their EXPLAIN plans captured
    rows, err := db.Query(ctx, "SELECT * FROM users WHERE active = true")
    // ...

    // DML queries are also captured (rolled back after EXPLAIN)
    db.Exec(ctx, "UPDATE users SET last_login = NOW() WHERE id = $1", userID)
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
2. **Configure via options** - Pass all configuration (pool settings, hooks) as options to `Connect()`
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

**[<- Back to Home](Home)**

*This API reference covers all public types, functions, and methods in pgxkit. For practical usage examples, see the [Examples](Examples) page.*
