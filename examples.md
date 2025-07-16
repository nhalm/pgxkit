# Usage Examples

## Basic Usage

Here's how to use the database package with your own sqlc-generated queries:

```go
package main

import (
    "context"
    "log"
    
    "github.com/nhalm/pgxkit"
    "your-project/internal/repository/sqlc" // Your sqlc-generated package
)

func main() {
    ctx := context.Background()
    
    // Create a connection with your sqlc queries
    conn, err := pgxkit.NewConnection(ctx, "", sqlc.New)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    // Use your queries
    queries := conn.Queries()
    users, err := queries.GetAllUsers(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Found %d users", len(users))
}
```

## With Custom Configuration

```go
package main

import (
    "context"
    "log"
    "time"
    
    "github.com/nhalm/pgxkit"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    
    // Configure connection settings
    config := &pgxkit.Config{
        MaxConns:        20,
        MinConns:        5,
        MaxConnLifetime: 1 * time.Hour,
        SearchPath:      "myschema",
    }
    
    // Create connection with custom config
    conn, err := pgxkit.NewConnectionWithConfig(ctx, "", sqlc.New, config)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    // Use your queries
    queries := conn.Queries()
    // ... rest of your code
}
```

## Using Transactions

```go
package main

import (
    "context"
    "log"
    
    "github.com/nhalm/pgxkit"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    
    conn, err := pgxkit.NewConnection(ctx, "", sqlc.New)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    // High-level transaction usage
    err = conn.WithTransaction(ctx, func(ctx context.Context, tx *sqlc.Queries) error {
        // All operations within this function run in a transaction
        user, err := tx.CreateUser(ctx, sqlc.CreateUserParams{
            Name:  "John Doe",
            Email: "john@example.com",
        })
        if err != nil {
            return err
        }
        
        // Create related records
        return tx.CreateUserProfile(ctx, sqlc.CreateUserProfileParams{
            UserID: user.ID,
            Bio:    "Software developer",
        })
    })
    
    if err != nil {
        log.Fatal(err)
    }
}
```

## Integration Testing

```go
package main

import (
    "testing"
    
    "github.com/nhalm/pgxkit"
    "your-project/internal/repository/sqlc"
)

func TestUserOperations(t *testing.T) {
    // Get shared test connection
    conn := pgxkit.RequireTestDB(t, sqlc.New)
    
    // Clean up test data
    pgxkit.CleanupTestData(conn,
        "DELETE FROM users WHERE email LIKE 'test_%'",
        "DELETE FROM user_profiles WHERE user_id IS NULL",
    )
    
    // Run your test
    queries := conn.Queries()
    user, err := queries.CreateUser(ctx, sqlc.CreateUserParams{
        Name:  "Test User",
        Email: "test_user@example.com",
    })
    
    if err != nil {
        t.Fatal(err)
    }
    
    // Verify user was created
    if user.Name != "Test User" {
        t.Errorf("Expected 'Test User', got %s", user.Name)
    }
}
```

## Error Handling

```go
package main

import (
    "context"
    "errors"
    "log"
    
    "github.com/jackc/pgx/v5"
    "github.com/nhalm/pgxkit"
    "your-project/internal/repository/sqlc"
)

func getUserByID(ctx context.Context, conn *pgxkit.Connection[*sqlc.Queries], id int64) (*sqlc.User, error) {
    user, err := conn.Queries().GetUserByID(ctx, id)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, pgxkit.NewNotFoundError("User", id)
        }
        return nil, pgxkit.NewDatabaseError("User", "query", err)
    }
    return &user, nil
}

func main() {
    ctx := context.Background()
    conn, err := pgxkit.NewConnection(ctx, "", sqlc.New)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    user, err := getUserByID(ctx, conn, 123)
    if err != nil {
        var notFoundErr *pgxkit.NotFoundError
        if errors.As(err, &notFoundErr) {
            log.Printf("User not found: %v", notFoundErr)
            return
        }
        log.Fatal(err)
    }
    
    log.Printf("Found user: %s", user.Name)
}
```

## Multiple Database Schemas

```go
package main

import (
    "context"
    "log"
    
    "github.com/nhalm/pgxkit"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    
    // Connection for 'users' schema
    usersConn, err := pgxkit.NewConnectionWithConfig(ctx, "", sqlc.New, &pgxkit.Config{
        SearchPath: "users",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer usersConn.Close()
    
    // Connection for 'orders' schema  
    ordersConn, err := pgxkit.NewConnectionWithConfig(ctx, "", sqlc.New, &pgxkit.Config{
        SearchPath: "orders",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer ordersConn.Close()
    
    // Use each connection for its specific schema
    users := usersConn.Queries()
    orders := ordersConn.Queries()
    
    // ... use users and orders queries
}
```

## Type Conversion Helpers

```go
package main

import (
    "context"
    "log"
    
    "github.com/nhalm/pgxkit"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    conn, err := pgxkit.NewConnection(ctx, "", sqlc.New)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    // Using pgx type helpers
    var name *string = nil
    var age int = 25
    var score *float64 = nil
    
    // Convert Go types to pgx types
    pgxName := pgxkit.ToPgxText(name)           // nil becomes Valid: false
    pgxAge := pgxkit.ToPgxInt4FromInt(&age)     // 25 becomes Valid: true
    pgxScore := pgxkit.ToPgxNumericFromFloat64Ptr(score) // nil becomes Valid: false
    
    // Create user with converted types
    user, err := conn.Queries().CreateUserWithOptionalFields(ctx, sqlc.CreateUserWithOptionalFieldsParams{
        Name:  pgxName,
        Age:   pgxAge,
        Score: pgxScore,
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Convert back to Go types
    userName := pgxkit.FromPgxText(user.Name)        // Returns *string
    userAge := pgxkit.FromPgxInt4(user.Age)          // Returns *int  
    userScore := pgxkit.FromPgxNumericPtr(user.Score) // Returns *float64
    
    log.Printf("Created user: name=%v, age=%v, score=%v", userName, userAge, userScore)
}
```

## Connection Hooks and Events

```go
package main

import (
    "context"
    "log"
    
    "github.com/jackc/pgx/v5"
    "github.com/nhalm/pgxkit"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    
    // Method 1: Create connection with pre-built hooks
    logger := pgxkit.NewDefaultLogger(pgxkit.LogLevelInfo)
    conn, err := pgxkit.NewConnectionWithLoggingHooks(ctx, "", sqlc.New, logger)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    // Method 2: Create custom hooks and add them
    hooks := pgxkit.NewConnectionHooks()
    
    // Add connection lifecycle hooks
    hooks.AddOnConnect(func(conn *pgx.Conn) error {
        log.Printf("New connection established: PID %d", conn.PgConn().PID())
        // Set connection-specific settings
        _, err := conn.Exec(context.Background(), "SET application_name = 'myapp'")
        return err
    })
    
    hooks.AddOnDisconnect(func(conn *pgx.Conn) {
        log.Printf("Connection closed: PID %d", conn.PgConn().PID())
    })
    
    // Method 3: Add hooks to existing connection
    conn = conn.WithHooks(hooks)
    
    // Method 4: Create connection with hooks in config
    config := &pgxkit.Config{
        MaxConns: 10,
        Hooks:    hooks,
    }
    
    conn2, err := pgxkit.NewConnectionWithConfig(ctx, "", sqlc.New, config)
    if err != nil {
        log.Fatal(err)
    }
    defer conn2.Close()
    
    // Use pre-built hooks
    validationHooks := pgxkit.ValidationHook()
    setupHooks := pgxkit.SetupHook("SET timezone = 'UTC'")
    
    // Combine multiple hooks
    combinedHooks := pgxkit.CombineHooks(
        pgxkit.LoggingHook(logger),
        validationHooks,
        setupHooks,
    )
    
    // Create connection with combined hooks
    conn3, err := pgxkit.NewConnectionWithHooks(ctx, "", sqlc.New, combinedHooks)
    if err != nil {
        log.Fatal(err)
    }
    defer conn3.Close()
    
    // All operations will trigger the hooks
    queries := conn3.Queries()
    users, err := queries.GetAllUsers(ctx)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Found %d users", len(users))
}
```

## Health Checks and Monitoring

```go
package main

import (
    "context"
    "log"
    "time"
    
    "github.com/nhalm/pgxkit"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    conn, err := pgxkit.NewConnection(ctx, "", sqlc.New)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    // Health check
    if err := conn.HealthCheck(ctx); err != nil {
        log.Printf("Database health check failed: %v", err)
        return
    }
    
    // Quick ready check
    if conn.IsReady(ctx) {
        log.Println("Database is ready to accept queries")
    }
    
    // Connection pool statistics
    stats := conn.Stats()
    log.Printf("Pool stats - Total: %d, Idle: %d, Used: %d", 
        stats.TotalConns(), stats.IdleConns(), stats.AcquiredConns())
    
    // Periodic health monitoring
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        if !conn.IsReady(ctx) {
            log.Println("Database connection is not ready!")
            // Handle reconnection logic
        }
    }
}
```

## Retry Logic

```go
package main

import (
    "context"
    "log"
    "time"
    
    "github.com/nhalm/pgxkit"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    conn, err := pgxkit.NewConnection(ctx, "", sqlc.New)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    // Custom retry configuration
    retryConfig := &pgxkit.RetryConfig{
        MaxRetries: 5,
        BaseDelay:  200 * time.Millisecond,
        MaxDelay:   2 * time.Second,
        Multiplier: 2.0,
    }
    
    // Create retryable connection
    retryableConn := conn.WithRetry(retryConfig)
    
    // Transaction with automatic retry
    err = retryableConn.WithRetryableTransaction(ctx, func(ctx context.Context, tx *sqlc.Queries) error {
        user, err := tx.CreateUser(ctx, sqlc.CreateUserParams{
            Name:  "John Doe",
            Email: "john@example.com",
        })
        if err != nil {
            return err
        }
        
        // This will be retried if it fails due to transient errors
        return tx.CreateUserProfile(ctx, sqlc.CreateUserProfileParams{
            UserID: user.ID,
            Bio:    "Software developer",
        })
    })
    
    if err != nil {
        log.Printf("Transaction failed after retries: %v", err)
    }
    
    // Timeout with retry
    result, err := pgxkit.WithTimeoutAndRetry(ctx, 5*time.Second, retryConfig, func(ctx context.Context) (*sqlc.User, error) {
        return conn.Queries().GetUserByEmail(ctx, "john@example.com")
    })
    
    if err != nil {
        log.Printf("Query failed: %v", err)
    } else {
        log.Printf("User found: %s", result.Name)
    }
}
```

## Read/Write Connection Splitting

```go
package main

import (
    "context"
    "log"
    "time"
    
    "github.com/nhalm/pgxkit"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    
    // Separate DSNs for read and write
    readDSN := "postgres://readonly:password@read-replica:5432/mydb"
    writeDSN := "postgres://user:password@primary:5432/mydb"
    
    // Create read/write connection
    rwConn, err := pgxkit.NewReadWriteConnection(ctx, readDSN, writeDSN, sqlc.New)
    if err != nil {
        log.Fatal(err)
    }
    defer rwConn.Close()
    
    // Use read connection for queries
    readQueries := rwConn.ReadQueries()
    users, err := readQueries.GetAllUsers(ctx)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Found %d users", len(users))
    
    // Use write connection for modifications
    writeQueries := rwConn.WriteQueries()
    newUser, err := writeQueries.CreateUser(ctx, sqlc.CreateUserParams{
        Name:  "Jane Doe",
        Email: "jane@example.com",
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Created user: %s", newUser.Name)
    
    // Transactions always use the write connection
    err = rwConn.WithTransaction(ctx, func(ctx context.Context, tx *sqlc.Queries) error {
        // All operations within transaction use write connection
        return tx.UpdateUserEmail(ctx, sqlc.UpdateUserEmailParams{
            ID:    newUser.ID,
            Email: "jane.doe@example.com",
        })
    })
    
    if err != nil {
        log.Fatal(err)
    }
    
    // Health checks for both connections
    if err := rwConn.HealthCheck(ctx); err != nil {
        log.Printf("Read/write connection health check failed: %v", err)
    }
    
    // Separate stats for read and write pools
    readStats := rwConn.ReadStats()
    writeStats := rwConn.WriteStats()
    log.Printf("Read pool: %d connections, Write pool: %d connections", 
        readStats.TotalConns(), writeStats.TotalConns())
}
```

## Query Logging and Tracing

```go
package main

import (
    "context"
    "log"
    "time"
    
    "github.com/nhalm/pgxkit"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    
    // Create logging configuration
    loggingConfig := &pgxkit.LoggingConfig{
        Logger:              pgxkit.NewDefaultLogger(pgxkit.LogLevelDebug),
        LogLevel:            pgxkit.LogLevelDebug,
        LogSlowQueries:      true,
        SlowQueryThreshold:  500 * time.Millisecond,
        LogConnections:      true,
        LogTransactions:     true,
    }
    
    // Create connection with logging
    conn, err := pgxkit.NewConnectionWithLogging(ctx, "", sqlc.New, loggingConfig)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    // All database operations will be logged
    queries := conn.Queries()
    users, err := queries.GetAllUsers(ctx)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Found %d users", len(users))
    
    // Transactions are logged with timing
    err = conn.WithTransaction(ctx, func(ctx context.Context, tx *sqlc.Queries) error {
        return tx.CreateUser(ctx, sqlc.CreateUserParams{
            Name:  "Logged User",
            Email: "logged@example.com",
        })
    })
    
    if err != nil {
        log.Fatal(err)
    }
    
    // Manual query logging
    queryLogger := pgxkit.NewQueryLogger(queries, loggingConfig.Logger)
    err = queryLogger.LogQuery(ctx, "GetUserByEmail", func() error {
        _, err := queries.GetUserByEmail(ctx, "logged@example.com")
        return err
    })
    
    if err != nil {
        log.Fatal(err)
    }
    
    // Slow query logging
    slowLogger := pgxkit.NewSlowQueryLogger(loggingConfig.Logger, 100*time.Millisecond)
    start := time.Now()
    _, err = queries.GetAllUsers(ctx)
    duration := time.Since(start)
    slowLogger.LogIfSlow(ctx, "GetAllUsers", duration, err)
}
```

## Advanced Production Setup

```go
package main

import (
    "context"
    "embed"
    "log"
    "time"
    
    "github.com/nhalm/pgxkit"
    "your-project/internal/repository/sqlc"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type MyMetrics struct{}

func (m *MyMetrics) RecordConnectionAcquired(duration time.Duration) {
    log.Printf("Connection acquired in %v", duration)
}

func (m *MyMetrics) RecordConnectionReleased(duration time.Duration) {
    log.Printf("Connection released in %v", duration)
}

func (m *MyMetrics) RecordQueryExecuted(queryName string, duration time.Duration, err error) {
    if err != nil {
        log.Printf("Query %s failed in %v: %v", queryName, duration, err)
    } else {
        log.Printf("Query %s executed in %v", queryName, duration)
    }
}

func (m *MyMetrics) RecordTransactionStarted() {
    log.Println("Transaction started")
}

func (m *MyMetrics) RecordTransactionCommitted(duration time.Duration) {
    log.Printf("Transaction committed in %v", duration)
}

func (m *MyMetrics) RecordTransactionRolledBack(duration time.Duration) {
    log.Printf("Transaction rolled back in %v", duration)
}

func main() {
    ctx := context.Background()
    
    // Production configuration
    config := &pgxkit.Config{
        MaxConns:        20,
        MinConns:        5,
        MaxConnLifetime: 1 * time.Hour,
        SearchPath:      "production",
        OnConnect: func(conn *pgx.Conn) error {
            // Set production-specific connection settings
            _, err := conn.Exec(context.Background(), 
                "SET application_name = 'myapp-prod'; SET timezone = 'UTC'")
            return err
        },
    }
    
    // Create connection with all features
    conn, err := pgxkit.NewConnectionWithConfig(ctx, "", sqlc.New, config)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    // Add metrics
    metrics := &MyMetrics{}
    conn = conn.WithMetrics(metrics)
    
    // Add logging
    logger := pgxkit.NewDefaultLogger(pgxkit.LogLevelInfo)
    loggingConn := conn.WithLogging(logger)
    
    // Add retry logic
    retryConfig := &pgxkit.RetryConfig{
        MaxRetries: 3,
        BaseDelay:  100 * time.Millisecond,
        MaxDelay:   1 * time.Second,
        Multiplier: 2.0,
    }
    retryableConn := loggingConn.WithRetry(retryConfig)
    
    // Health check loop
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        
        for range ticker.C {
            if err := conn.HealthCheck(ctx); err != nil {
                log.Printf("Health check failed: %v", err)
            }
            
            stats := conn.Stats()
            log.Printf("Pool stats - Active: %d, Idle: %d, Total: %d", 
                stats.AcquiredConns(), stats.IdleConns(), stats.TotalConns())
        }
    }()
    
    // Use the fully configured connection
    err = retryableConn.WithRetryableTransaction(ctx, func(ctx context.Context, tx *sqlc.Queries) error {
        user, err := tx.CreateUser(ctx, sqlc.CreateUserParams{
            Name:  "Production User",
            Email: "prod@example.com",
        })
        if err != nil {
            return err
        }
        
        return tx.CreateUserProfile(ctx, sqlc.CreateUserProfileParams{
            UserID: user.ID,
            Bio:    "Created in production",
        })
    })
    
    if err != nil {
        log.Fatal(err)
    }
    
    log.Println("Production setup complete")
}
```

## Setting Up for Development

### 1. Install the package

```bash
go get github.com/nhalm/pgxkit
```

### 2. Generate your sqlc queries

Create your `sqlc.yaml`:

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "./queries"
    schema: "./schema"
    gen:
      go:
        package: "sqlc"
        out: "./internal/repository/sqlc"
```

### 3. Use with your queries

```go
import (
    "github.com/nhalm/pgxkit"
    "your-project/internal/repository/sqlc"
)

// In your application
conn, err := pgxkit.NewConnection(ctx, "", sqlc.New)
```

## Environment Variables

The package uses these environment variables with sensible defaults:

- `POSTGRES_HOST` (default: "localhost")
- `POSTGRES_PORT` (default: 5432)
- `POSTGRES_USER` (default: "postgres")
- `POSTGRES_PASSWORD` (default: "")
- `POSTGRES_DB` (default: "postgres")
- `POSTGRES_SSLMODE` (default: "disable")
- `TEST_DATABASE_URL` (for integration tests)

## Key Features

✅ **sqlc-focused**: Designed specifically for sqlc-generated queries
✅ **Generic**: Works with any sqlc-generated package
✅ **Configurable**: Flexible connection settings and schema paths
✅ **Transaction support**: Both high-level and low-level transaction APIs
✅ **Testing utilities**: Optimized shared connection for integration tests
✅ **Type helpers**: Comprehensive pgx type conversion utilities
✅ **Error handling**: Structured error types for consistent error handling
✅ **Connection hooks**: Event-driven connection lifecycle management
✅ **Health checks**: Built-in health monitoring for production
✅ **Metrics**: Connection pool statistics and custom metrics collection
✅ **Retry logic**: Automatic retry for transient database failures
✅ **Read/write splitting**: Separate connections for read and write operations
✅ **Query logging**: Comprehensive logging and tracing capabilities

## Hook System Examples

### Basic Hook Usage with DB Type

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/nhalm/pgxkit"
)

func main() {
    ctx := context.Background()
    
    // Create a pool configuration
    config, err := pgxpool.ParseConfig("postgres://user:password@localhost/dbname")
    if err != nil {
        log.Fatal(err)
    }
    
    // Create DB instance
    db := pgxkit.NewDB(nil) // We'll configure the pool with hooks
    
    // Add operation-level hooks
    err = db.AddHook("BeforeOperation", func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        log.Printf("Executing query: %s", sql)
        return nil
    })
    if err != nil {
        log.Fatal(err)
    }
    
    err = db.AddHook("AfterOperation", func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        if operationErr != nil {
            log.Printf("Query failed: %s, error: %v", sql, operationErr)
        } else {
            log.Printf("Query succeeded: %s", sql)
        }
        return nil
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Add connection-level hooks
    err = db.AddConnectionHook("OnConnect", func(conn *pgx.Conn) error {
        log.Printf("New connection established: PID %d", conn.PgConn().PID())
        // Set connection-specific settings
        _, err := conn.Exec(context.Background(), "SET application_name = 'myapp'")
        return err
    })
    if err != nil {
        log.Fatal(err)
    }
    
    err = db.AddConnectionHook("OnDisconnect", func(conn *pgx.Conn) {
        log.Printf("Connection closed: PID %d", conn.PgConn().PID())
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Configure the pool with hooks - this is the key integration step!
    db.Hooks().ConfigurePool(config)
    
    // Now create the pool with the configured hooks
    pool, err := pgxpool.NewWithConfig(ctx, config)
    if err != nil {
        log.Fatal(err)
    }
    defer pool.Close()
    
    // Create a new DB instance with the configured pool
    db = pgxkit.NewDB(pool)
    
    // Copy the hooks to the new DB instance
    db.hooks = hooks // You would need to expose this or create a method
    
    // Now all operations will trigger both operation and connection hooks
    rows, err := db.Query(ctx, "SELECT 1")
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
}
```

### Advanced Hook Integration Pattern

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/nhalm/pgxkit"
)

// CreateDBWithHooks creates a DB instance with hooks properly integrated
func CreateDBWithHooks(ctx context.Context, connString string) (*pgxkit.DB, error) {
    // Parse the connection string
    config, err := pgxpool.ParseConfig(connString)
    if err != nil {
        return nil, err
    }
    
    // Create hooks
    hooks := pgxkit.NewHooks()
    
    // Add operation-level hooks
    hooks.AddHook("BeforeOperation", func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        log.Printf("[%s] Executing: %s", time.Now().Format("15:04:05"), sql)
        return nil
    })
    
    hooks.AddHook("AfterOperation", func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        if operationErr != nil {
            log.Printf("[%s] Query failed: %v", time.Now().Format("15:04:05"), operationErr)
        }
        return nil
    })
    
    // Add connection-level hooks
    hooks.AddConnectionHook("OnConnect", func(conn *pgx.Conn) error {
        log.Printf("[%s] Connection established: PID %d", time.Now().Format("15:04:05"), conn.PgConn().PID())
        return nil
    })
    
    hooks.AddConnectionHook("OnDisconnect", func(conn *pgx.Conn) {
        log.Printf("[%s] Connection closed: PID %d", time.Now().Format("15:04:05"), conn.PgConn().PID())
    })
    
    // Configure pool with hooks
    hooks.ConfigurePool(config)
    
    // Create pool
    pool, err := pgxpool.NewWithConfig(ctx, config)
    if err != nil {
        return nil, err
    }
    
    // Create DB with the configured pool
    db := pgxkit.NewDB(pool)
    
    // The hooks are already integrated with the pool, so connection-level hooks will work
    // Operation-level hooks are handled by the DB methods
    
    return db, nil
}

func main() {
    ctx := context.Background()
    
    db, err := CreateDBWithHooks(ctx, "postgres://user:password@localhost/dbname")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)
    
    // All operations will trigger hooks
    rows, err := db.Query(ctx, "SELECT * FROM users LIMIT 10")
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
    
    // Read operations also trigger hooks
    rows, err = db.ReadQuery(ctx, "SELECT COUNT(*) FROM users")
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
}
```

### Hook Composition and Reuse

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/nhalm/pgxkit"
)

// CreateLoggingHooks creates hooks for logging
func CreateLoggingHooks() *pgxkit.ConnectionHooks {
    hooks := pgxkit.NewConnectionHooks()
    
    hooks.AddOnConnect(func(conn *pgx.Conn) error {
        log.Printf("📝 Connection established: PID %d", conn.PgConn().PID())
        return nil
    })
    
    hooks.AddOnDisconnect(func(conn *pgx.Conn) {
        log.Printf("📝 Connection closed: PID %d", conn.PgConn().PID())
    })
    
    return hooks
}

// CreateMetricsHooks creates hooks for metrics collection
func CreateMetricsHooks() *pgxkit.ConnectionHooks {
    hooks := pgxkit.NewConnectionHooks()
    
    hooks.AddOnAcquire(func(ctx context.Context, conn *pgx.Conn) error {
        log.Printf("📊 Connection acquired: PID %d", conn.PgConn().PID())
        return nil
    })
    
    hooks.AddOnRelease(func(conn *pgx.Conn) {
        log.Printf("📊 Connection released: PID %d", conn.PgConn().PID())
    })
    
    return hooks
}

// CreateValidationHooks creates hooks for connection validation
func CreateValidationHooks() *pgxkit.ConnectionHooks {
    hooks := pgxkit.NewConnectionHooks()
    
    hooks.AddOnConnect(func(conn *pgx.Conn) error {
        // Validate connection
        _, err := conn.Exec(context.Background(), "SELECT 1")
        if err != nil {
            log.Printf("❌ Connection validation failed: %v", err)
            return err
        }
        log.Printf("✅ Connection validated: PID %d", conn.PgConn().PID())
        return nil
    })
    
    return hooks
}

func main() {
    ctx := context.Background()
    
    // Create and combine multiple hook sets
    loggingHooks := CreateLoggingHooks()
    metricsHooks := CreateMetricsHooks()
    validationHooks := CreateValidationHooks()
    
    // Combine all hooks
    combinedHooks := pgxkit.CombineHooks(loggingHooks, metricsHooks, validationHooks)
    
    // Configure pool with combined hooks
    config, err := pgxpool.ParseConfig("postgres://user:password@localhost/dbname")
    if err != nil {
        log.Fatal(err)
    }
    
    combinedHooks.ConfigurePool(config)
    
    // Create pool and DB
    pool, err := pgxpool.NewWithConfig(ctx, config)
    if err != nil {
        log.Fatal(err)
    }
    defer pool.Close()
    
    db := pgxkit.NewDB(pool)
    
    // Add operation-level hooks
    db.AddHook("BeforeOperation", func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        log.Printf("🔍 Executing: %s", sql)
        return nil
    })
    
    // Now all operations will trigger all hooks
    rows, err := db.Query(ctx, "SELECT 1")
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
}
```
