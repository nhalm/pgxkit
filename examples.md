# Usage Examples

## Basic Usage

Here's how to use the database package with your own sqlc-generated queries:

```go
package main

import (
    "context"
    "log"
    
    "github.com/nhalm/dbutil"
    "your-project/internal/repository/sqlc" // Your sqlc-generated package
)

func main() {
    ctx := context.Background()
    
    // Create a connection with your sqlc queries
    conn, err := dbutil.NewConnection(ctx, "", sqlc.New)
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
    
    "github.com/nhalm/dbutil"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    
    // Configure connection settings
    config := &dbutil.Config{
        MaxConns:        20,
        MinConns:        5,
        MaxConnLifetime: 1 * time.Hour,
        SearchPath:      "myschema",
    }
    
    // Create connection with custom config
    conn, err := dbutil.NewConnectionWithConfig(ctx, "", sqlc.New, config)
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
    
    "github.com/nhalm/dbutil"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    
    conn, err := dbutil.NewConnection(ctx, "", sqlc.New)
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
    
    "github.com/nhalm/dbutil"
    "your-project/internal/repository/sqlc"
)

func TestUserOperations(t *testing.T) {
    // Get shared test connection
    conn := dbutil.RequireTestDB(t, sqlc.New)
    
    // Clean up test data
    dbutil.CleanupTestData(conn,
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
    "github.com/nhalm/dbutil"
    "your-project/internal/repository/sqlc"
)

func getUserByID(ctx context.Context, conn *dbutil.Connection[*sqlc.Queries], id int64) (*sqlc.User, error) {
    user, err := conn.Queries().GetUserByID(ctx, id)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, dbutil.NewNotFoundError("User", id)
        }
        return nil, dbutil.NewDatabaseError("User", "query", err)
    }
    return &user, nil
}

func main() {
    ctx := context.Background()
    conn, err := dbutil.NewConnection(ctx, "", sqlc.New)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    user, err := getUserByID(ctx, conn, 123)
    if err != nil {
        var notFoundErr *dbutil.NotFoundError
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
    
    "github.com/nhalm/dbutil"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    
    // Connection for 'users' schema
    usersConn, err := dbutil.NewConnectionWithConfig(ctx, "", sqlc.New, &dbutil.Config{
        SearchPath: "users",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer usersConn.Close()
    
    // Connection for 'orders' schema  
    ordersConn, err := dbutil.NewConnectionWithConfig(ctx, "", sqlc.New, &dbutil.Config{
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
    
    "github.com/nhalm/dbutil"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    conn, err := dbutil.NewConnection(ctx, "", sqlc.New)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    // Using pgx type helpers
    var name *string = nil
    var age int = 25
    var score *float64 = nil
    
    // Convert Go types to pgx types
    pgxName := dbutil.ToPgxText(name)           // nil becomes Valid: false
    pgxAge := dbutil.ToPgxInt4FromInt(&age)     // 25 becomes Valid: true
    pgxScore := dbutil.ToPgxNumericFromFloat64Ptr(score) // nil becomes Valid: false
    
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
    userName := dbutil.FromPgxText(user.Name)        // Returns *string
    userAge := dbutil.FromPgxInt4(user.Age)          // Returns *int  
    userScore := dbutil.FromPgxNumericPtr(user.Score) // Returns *float64
    
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
    "github.com/nhalm/dbutil"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    
    // Method 1: Create connection with pre-built hooks
    logger := dbutil.NewDefaultLogger(dbutil.LogLevelInfo)
    conn, err := dbutil.NewConnectionWithLoggingHooks(ctx, "", sqlc.New, logger)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    // Method 2: Create custom hooks and add them
    hooks := dbutil.NewConnectionHooks()
    
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
    config := &dbutil.Config{
        MaxConns: 10,
        Hooks:    hooks,
    }
    
    conn2, err := dbutil.NewConnectionWithConfig(ctx, "", sqlc.New, config)
    if err != nil {
        log.Fatal(err)
    }
    defer conn2.Close()
    
    // Use pre-built hooks
    validationHooks := dbutil.ValidationHook()
    setupHooks := dbutil.SetupHook("SET timezone = 'UTC'")
    
    // Combine multiple hooks
    combinedHooks := dbutil.CombineHooks(
        dbutil.LoggingHook(logger),
        validationHooks,
        setupHooks,
    )
    
    // Create connection with combined hooks
    conn3, err := dbutil.NewConnectionWithHooks(ctx, "", sqlc.New, combinedHooks)
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
    
    "github.com/nhalm/dbutil"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    conn, err := dbutil.NewConnection(ctx, "", sqlc.New)
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
    
    "github.com/nhalm/dbutil"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    conn, err := dbutil.NewConnection(ctx, "", sqlc.New)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    // Custom retry configuration
    retryConfig := &dbutil.RetryConfig{
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
    result, err := dbutil.WithTimeoutAndRetry(ctx, 5*time.Second, retryConfig, func(ctx context.Context) (*sqlc.User, error) {
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
    
    "github.com/nhalm/dbutil"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    
    // Separate DSNs for read and write
    readDSN := "postgres://readonly:password@read-replica:5432/mydb"
    writeDSN := "postgres://user:password@primary:5432/mydb"
    
    // Create read/write connection
    rwConn, err := dbutil.NewReadWriteConnection(ctx, readDSN, writeDSN, sqlc.New)
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
    
    "github.com/nhalm/dbutil"
    "your-project/internal/repository/sqlc"
)

func main() {
    ctx := context.Background()
    
    // Create logging configuration
    loggingConfig := &dbutil.LoggingConfig{
        Logger:              dbutil.NewDefaultLogger(dbutil.LogLevelDebug),
        LogLevel:            dbutil.LogLevelDebug,
        LogSlowQueries:      true,
        SlowQueryThreshold:  500 * time.Millisecond,
        LogConnections:      true,
        LogTransactions:     true,
    }
    
    // Create connection with logging
    conn, err := dbutil.NewConnectionWithLogging(ctx, "", sqlc.New, loggingConfig)
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
    queryLogger := dbutil.NewQueryLogger(queries, loggingConfig.Logger)
    err = queryLogger.LogQuery(ctx, "GetUserByEmail", func() error {
        _, err := queries.GetUserByEmail(ctx, "logged@example.com")
        return err
    })
    
    if err != nil {
        log.Fatal(err)
    }
    
    // Slow query logging
    slowLogger := dbutil.NewSlowQueryLogger(loggingConfig.Logger, 100*time.Millisecond)
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
    
    "github.com/nhalm/dbutil"
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
    config := &dbutil.Config{
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
    conn, err := dbutil.NewConnectionWithConfig(ctx, "", sqlc.New, config)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    // Add metrics
    metrics := &MyMetrics{}
    conn = conn.WithMetrics(metrics)
    
    // Add logging
    logger := dbutil.NewDefaultLogger(dbutil.LogLevelInfo)
    loggingConn := conn.WithLogging(logger)
    
    // Add retry logic
    retryConfig := &dbutil.RetryConfig{
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
go get github.com/nhalm/dbutil
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
    "github.com/nhalm/dbutil"
    "your-project/internal/repository/sqlc"
)

// In your application
conn, err := dbutil.NewConnection(ctx, "", sqlc.New)
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
