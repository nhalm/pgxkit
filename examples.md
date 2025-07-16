# pgxkit Usage Examples

This document provides comprehensive examples of using pgxkit - the tool-agnostic PostgreSQL toolkit.

## Table of Contents

1. [Basic Usage](#basic-usage)
2. [Read/Write Split](#readwrite-split)
3. [Hook System](#hook-system)
4. [Retry Logic](#retry-logic)
5. [Health Checks](#health-checks)
6. [DSN Utilities](#dsn-utilities)
7. [Testing](#testing)
8. [Type Helpers](#type-helpers)
9. [Metrics and Observability](#metrics-and-observability)
10. [Production Patterns](#production-patterns)
11. [Integration with Code Generation Tools](#integration-with-code-generation-tools)

## Basic Usage

### Simple Connection and Queries

```go
package main

import (
    "context"
    "log"

    "github.com/nhalm/pgxkit"
)

func main() {
    ctx := context.Background()
    
    // Create and connect to database
    db := pgxkit.NewDB()
    
    // Connect using environment variables (POSTGRES_* env vars)
    err := db.Connect(ctx, "")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)
    
    // Execute queries (uses write pool by default - safe)
    _, err = db.Exec(ctx, "INSERT INTO users (name, email) VALUES ($1, $2)", "John Doe", "john@example.com")
    if err != nil {
        log.Fatal(err)
    }
    
    // Query data
    rows, err := db.Query(ctx, "SELECT id, name, email FROM users")
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
    
    for rows.Next() {
        var id int
        var name, email string
        if err := rows.Scan(&id, &name, &email); err != nil {
            log.Fatal(err)
        }
        log.Printf("User: %d - %s (%s)", id, name, email)
    }
}
```

### With Explicit DSN

```go
db := pgxkit.NewDB()
err := db.Connect(ctx, "postgres://user:password@localhost/dbname?sslmode=disable")
if err != nil {
    log.Fatal(err)
}
defer db.Shutdown(ctx)
```

## Read/Write Split

### Separate Read and Write Pools

```go
package main

import (
    "context"
    "log"

    "github.com/nhalm/pgxkit"
)

func main() {
    ctx := context.Background()
    
    db := pgxkit.NewDB()
    
    // Connect with separate read and write pools
    err := db.ConnectReadWrite(ctx, 
        "postgres://user:password@read-replica/dbname",
        "postgres://user:password@primary/dbname")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)
    
    // Writes go to write pool (safe by default)
    _, err = db.Exec(ctx, "INSERT INTO users (name) VALUES ($1)", "Alice")
    if err != nil {
        log.Fatal(err)
    }
    
    // Reads can use read pool for optimization
    rows, err := db.ReadQuery(ctx, "SELECT * FROM users ORDER BY created_at DESC LIMIT 10")
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
    
    // Process results...
}
```

## Hook System

### Basic Hook Usage

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/nhalm/pgxkit"
)

func main() {
    ctx := context.Background()
    
    // Create DB instance
    db := pgxkit.NewDB()
    
    // Add logging hooks
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        log.Printf("Executing query: %s", sql)
        return nil
    })
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        if operationErr != nil {
            log.Printf("Query failed: %v", operationErr)
        } else {
            log.Printf("Query completed successfully")
        }
        return nil
    })
    
    // Connect with hooks already configured
    err := db.Connect(ctx, "")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)
    
    // Use database - hooks execute automatically
    rows, err := db.Query(ctx, "SELECT id, name FROM users WHERE active = $1", true)
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
}
```

### Advanced Hook Patterns

```go
// Timing and metrics hook
db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    start := time.Now()
    return context.WithValue(ctx, "start_time", start)
})

db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    if start, ok := ctx.Value("start_time").(time.Time); ok {
        duration := time.Since(start)
        log.Printf("Query took %v: %s", duration, sql)
        
        // Record metrics
        if operationErr != nil {
            metrics.IncrementCounter("db.query.errors")
        } else {
            metrics.RecordHistogram("db.query.duration", duration)
        }
    }
    return nil
})

// Circuit breaker hook
db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    if circuitBreaker.IsOpen() {
        return errors.New("circuit breaker is open")
    }
    return nil
})

// Tracing hook
db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    span := trace.SpanFromContext(ctx)
    span.SetAttributes(attribute.String("db.statement", sql))
    return nil
})
```

### Connection-Level Hooks

```go
// Connection lifecycle hooks
db.AddConnectionHook("OnConnect", func(conn *pgx.Conn) error {
    log.Printf("New connection established: PID %d", conn.PgConn().PID())
    
    // Set connection-specific settings
    _, err := conn.Exec(context.Background(), "SET application_name = 'my-app'")
    return err
})

db.AddConnectionHook("OnDisconnect", func(conn *pgx.Conn) {
    log.Printf("Connection closed: PID %d", conn.PgConn().PID())
})

db.AddConnectionHook("OnAcquire", func(ctx context.Context, conn *pgx.Conn) error {
    log.Printf("Connection acquired from pool: PID %d", conn.PgConn().PID())
    return nil
})

db.AddConnectionHook("OnRelease", func(conn *pgx.Conn) {
    log.Printf("Connection released to pool: PID %d", conn.PgConn().PID())
})
```

### Transaction Hooks

```go
db.AddHook(pgxkit.BeforeTransaction, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    log.Println("Starting transaction")
    return nil
})

db.AddHook(pgxkit.AfterTransaction, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    if operationErr != nil {
        log.Printf("Transaction failed: %v", operationErr)
    } else {
        log.Println("Transaction completed successfully")
    }
    return nil
})

// Use transactions
tx, err := db.BeginTx(ctx, pgx.TxOptions{})
if err != nil {
    log.Fatal(err)
}

_, err = tx.Exec(ctx, "INSERT INTO users (name) VALUES ($1)", "Bob")
if err != nil {
    tx.Rollback(ctx)
    log.Fatal(err)
}

err = tx.Commit(ctx)
if err != nil {
    log.Fatal(err)
}
```

## Retry Logic

### Built-in Retry Methods

```go
package main

import (
    "context"
    "log"

    "github.com/jackc/pgx/v5"
    "github.com/nhalm/pgxkit"
)

func main() {
    ctx := context.Background()
    
    db := pgxkit.NewDB()
    err := db.Connect(ctx, "")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)
    
    // Configure retry behavior
    config := pgxkit.DefaultRetryConfig() // 3 retries, exponential backoff
    // Or customize:
    // config := &pgxkit.RetryConfig{
    //     MaxRetries: 5,
    //     BaseDelay:  200 * time.Millisecond,
    //     MaxDelay:   2 * time.Second,
    //     Multiplier: 1.5,
    // }
    
    // Retry database operations
    result, err := db.ExecWithRetry(ctx, config, "INSERT INTO users (name) VALUES ($1)", "Charlie")
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Inserted user, affected rows: %d", result.RowsAffected())
    
    // Retry queries
    rows, err := db.QueryWithRetry(ctx, config, "SELECT * FROM users")
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
    
    // Retry read queries (uses read pool)
    rows, err = db.ReadQueryWithRetry(ctx, config, "SELECT * FROM users ORDER BY created_at")
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
    
    // Retry transactions
    tx, err := db.BeginTxWithRetry(ctx, config, pgx.TxOptions{})
    if err != nil {
        log.Fatal(err)
    }
    defer tx.Rollback(ctx)
    
    _, err = tx.Exec(ctx, "UPDATE users SET last_login = NOW() WHERE id = $1", 1)
    if err != nil {
        log.Fatal(err)
    }
    
    err = tx.Commit(ctx)
    if err != nil {
        log.Fatal(err)
    }
}
```

### Generic Retry Functions

```go
// Retry any operation
config := pgxkit.DefaultRetryConfig()

err := pgxkit.RetryOperation(ctx, config, func(ctx context.Context) error {
    // Complex database operation
    tx, err := db.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    
    // Multiple operations in transaction
    _, err = tx.Exec(ctx, "INSERT INTO orders (user_id, total) VALUES ($1, $2)", userID, total)
    if err != nil {
        return err
    }
    
    _, err = tx.Exec(ctx, "UPDATE users SET order_count = order_count + 1 WHERE id = $1", userID)
    if err != nil {
        return err
    }
    
    return tx.Commit(ctx)
})

// Retry operations that return values
user, err := pgxkit.WithTimeoutAndRetry(ctx, 5*time.Second, config, func(ctx context.Context) (*User, error) {
    row := db.QueryRow(ctx, "SELECT id, name, email FROM users WHERE id = $1", userID)
    
    var user User
    err := row.Scan(&user.ID, &user.Name, &user.Email)
    if err != nil {
        return nil, err
    }
    
    return &user, nil
})
```

### Smart Error Detection

```go
// pgxkit automatically detects retryable errors
err := db.Exec(ctx, "INSERT INTO users ...")
if err != nil {
    if pgxkit.IsRetryableError(err) {
        // Connection errors, deadlocks, serialization failures, etc.
        log.Printf("Retryable error: %v", err)
    } else {
        // Application errors, constraint violations, etc.
        log.Printf("Non-retryable error: %v", err)
    }
}
```

## Health Checks

### Basic Health Checks

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/nhalm/pgxkit"
)

func main() {
    ctx := context.Background()
    
    db := pgxkit.NewDB()
    err := db.Connect(ctx, "")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)
    
    // Simple readiness check
    if db.IsReady(ctx) {
        log.Println("Database is ready")
    } else {
        log.Println("Database is not ready")
    }
    
    // Detailed health check
    if err := db.HealthCheck(ctx); err != nil {
        log.Printf("Database health check failed: %v", err)
    } else {
        log.Println("Database health check passed")
    }
}
```

### Health Check HTTP Handler

```go
func healthHandler(db *pgxkit.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
        defer cancel()
        
        if err := db.HealthCheck(ctx); err != nil {
            w.WriteHeader(http.StatusServiceUnavailable)
            json.NewEncoder(w).Encode(map[string]string{
                "status": "unhealthy",
                "error":  err.Error(),
            })
            return
        }
        
        // Get connection statistics
        stats := db.Stats()
        readStats := db.ReadStats()
        
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "status": "healthy",
            "stats": map[string]interface{}{
                "write_pool": map[string]interface{}{
                    "acquired_conns": stats.AcquiredConns(),
                    "idle_conns":     stats.IdleConns(),
                    "total_conns":    stats.TotalConns(),
                },
                "read_pool": map[string]interface{}{
                    "acquired_conns": readStats.AcquiredConns(),
                    "idle_conns":     readStats.IdleConns(),
                    "total_conns":    readStats.TotalConns(),
                },
            },
        })
    }
}
```

## DSN Utilities

### Environment Variable Configuration

```go
// Set environment variables
os.Setenv("POSTGRES_HOST", "localhost")
os.Setenv("POSTGRES_PORT", "5432")
os.Setenv("POSTGRES_USER", "myuser")
os.Setenv("POSTGRES_PASSWORD", "mypassword")
os.Setenv("POSTGRES_DB", "mydb")
os.Setenv("POSTGRES_SSLMODE", "require")

// Get DSN from environment variables
dsn := pgxkit.GetDSN()
log.Printf("DSN: %s", dsn)
// Output: postgres://myuser:mypassword@localhost:5432/mydb?sslmode=require

// Use with pgxkit
db := pgxkit.NewDB()
err := db.Connect(ctx, "") // Uses environment variables
```

### Integration with Migration Tools

```go
// Use with golang-migrate
import (
    "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/postgres"
    _ "github.com/golang-migrate/migrate/v4/source/file"
)

func runMigrations() error {
    m, err := migrate.New("file://migrations", pgxkit.GetDSN())
    if err != nil {
        return err
    }
    defer m.Close()
    
    return m.Up()
}
```

## Testing

### Basic Testing

```go
func TestUserOperations(t *testing.T) {
    // Set TEST_DATABASE_URL environment variable
    testDB := pgxkit.NewTestDB()
    
    err := testDB.Setup()
    if err != nil {
        t.Skip("Test database not available")
    }
    defer testDB.Clean()
    
    // Use testDB.DB for your tests
    _, err = testDB.Exec(context.Background(), "INSERT INTO users (name) VALUES ($1)", "Test User")
    if err != nil {
        t.Fatal(err)
    }
    
    rows, err := testDB.Query(context.Background(), "SELECT name FROM users WHERE name = $1", "Test User")
    if err != nil {
        t.Fatal(err)
    }
    defer rows.Close()
    
    if !rows.Next() {
        t.Fatal("Expected user not found")
    }
}
```

### Golden Testing for Performance Regression Detection

```go
func TestUserQueries(t *testing.T) {
    testDB := pgxkit.NewTestDB()
    err := testDB.Setup()
    if err != nil {
        t.Skip("Test database not available")
    }
    defer testDB.Clean()
    
    // Enable golden test hooks - captures EXPLAIN plans automatically
    db := testDB.EnableGolden(t, "TestUserQueries")
    
    // These queries will have their EXPLAIN plans captured
    // Plans saved to testdata/golden/TestUserQueries_query_1.json, etc.
    
    // Query 1: Basic select
    rows, err := db.Query(context.Background(), "SELECT * FROM users WHERE active = true")
    if err != nil {
        t.Fatal(err)
    }
    rows.Close()
    
    // Query 2: Complex join
    rows, err = db.Query(context.Background(), `
        SELECT u.id, u.name, COUNT(o.id) as order_count 
        FROM users u 
        LEFT JOIN orders o ON u.id = o.user_id 
        WHERE u.active = true 
        GROUP BY u.id, u.name 
        ORDER BY order_count DESC
    `)
    if err != nil {
        t.Fatal(err)
    }
    rows.Close()
    
    // Future test runs will compare EXPLAIN plans to detect performance regressions
}
```

## Type Helpers

### Basic Type Conversions

```go
package main

import (
    "log"
    "time"

    "github.com/google/uuid"
    "github.com/nhalm/pgxkit"
)

func main() {
    // String conversions
    myString := "hello"
    pgxText := pgxkit.ToPgxText(&myString)
    stringPtr := pgxkit.FromPgxText(pgxText)
    log.Printf("String: %s", *stringPtr)
    
    // Numeric conversions
    myInt := int64(42)
    pgxInt := pgxkit.ToPgxInt8(&myInt)
    intPtr := pgxkit.FromPgxInt8(pgxInt)
    log.Printf("Int: %d", *intPtr)
    
    // UUID conversions
    myUUID := uuid.New()
    pgxUUID := pgxkit.ToPgxUUID(myUUID)
    convertedUUID := pgxkit.FromPgxUUID(pgxUUID)
    log.Printf("UUID: %s", convertedUUID)
    
    // Time conversions
    now := time.Now()
    pgxTime := pgxkit.ToPgxTimestamptz(&now)
    timePtr := pgxkit.FromPgxTimestamptz(pgxTime)
    log.Printf("Time: %s", timePtr.Format(time.RFC3339))
    
    // Array conversions
    stringSlice := []string{"a", "b", "c"}
    pgxArray := pgxkit.ToPgxTextArray(stringSlice)
    convertedSlice := pgxkit.FromPgxTextArray(pgxArray)
    log.Printf("Array: %v", convertedSlice)
}
```

### Using Type Helpers in Database Operations

```go
func createUser(db *pgxkit.DB, user *User) error {
    ctx := context.Background()
    
    _, err := db.Exec(ctx, `
        INSERT INTO users (id, name, email, created_at, metadata) 
        VALUES ($1, $2, $3, $4, $5)
    `,
        pgxkit.ToPgxUUID(user.ID),
        pgxkit.ToPgxText(user.Name),
        pgxkit.ToPgxText(user.Email),
        pgxkit.ToPgxTimestamptz(user.CreatedAt),
        pgxkit.ToPgxTextArray(user.Tags),
    )
    
    return err
}

func getUser(db *pgxkit.DB, id uuid.UUID) (*User, error) {
    ctx := context.Background()
    
    row := db.QueryRow(ctx, `
        SELECT id, name, email, created_at, metadata 
        FROM users 
        WHERE id = $1
    `, pgxkit.ToPgxUUID(id))
    
    var user User
    var pgxID pgtype.UUID
    var pgxName pgtype.Text
    var pgxEmail pgtype.Text
    var pgxCreatedAt pgtype.Timestamptz
    var pgxTags pgtype.Array[pgtype.Text]
    
    err := row.Scan(&pgxID, &pgxName, &pgxEmail, &pgxCreatedAt, &pgxTags)
    if err != nil {
        return nil, err
    }
    
    user.ID = pgxkit.FromPgxUUID(pgxID)
    user.Name = pgxkit.FromPgxText(pgxName)
    user.Email = pgxkit.FromPgxText(pgxEmail)
    user.CreatedAt = pgxkit.FromPgxTimestamptz(pgxCreatedAt)
    user.Tags = pgxkit.FromPgxTextArray(pgxTags)
    
    return &user, nil
}
```

## Production Patterns

### Complete Production Setup

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/nhalm/pgxkit"
)

func main() {
    ctx := context.Background()
    
    // Create database with comprehensive hooks
    db := setupDatabase()
    
    // Connect to database
    err := db.Connect(ctx, "")
    if err != nil {
        log.Fatal(err)
    }
    
    // Setup graceful shutdown
    setupGracefulShutdown(db)
    
    // Start your application
    startApplication(db)
}

func setupDatabase() *pgxkit.DB {
    db := pgxkit.NewDB()
    
    // Logging hooks
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        log.Printf("Executing query: %s", sql)
        return nil
    })
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        if operationErr != nil {
            log.Printf("Query failed: %v", operationErr)
        }
        return nil
    })
    
    // Metrics hooks
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        // Record query start time
        return nil
    })
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        // Record query duration and success/failure
        return nil
    })
    
    // Connection hooks
    db.AddConnectionHook("OnConnect", func(conn *pgx.Conn) error {
        // Set connection-specific settings
        _, err := conn.Exec(context.Background(), "SET application_name = 'my-production-app'")
        return err
    })
    
    return db
}

func setupGracefulShutdown(db *pgxkit.DB) {
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    
    go func() {
        <-c
        log.Println("Shutting down gracefully...")
        
        // Create shutdown context with timeout
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        
        // Shutdown database
        if err := db.Shutdown(ctx); err != nil {
            log.Printf("Database shutdown error: %v", err)
        }
        
        os.Exit(0)
    }()
}

func startApplication(db *pgxkit.DB) {
    // Your application logic here
    log.Println("Application started")
    
    // Keep the application running
    select {}
}
```

### Error Handling Patterns

```go
func handleDatabaseOperation(db *pgxkit.DB) error {
    ctx := context.Background()
    
    // Use structured errors
    _, err := db.Exec(ctx, "INSERT INTO users (email) VALUES ($1)", "duplicate@example.com")
    if err != nil {
        // Check for specific error types
        var notFoundErr *pgxkit.NotFoundError
        var validationErr *pgxkit.ValidationError
        var dbErr *pgxkit.DatabaseError
        
        switch {
        case errors.As(err, &notFoundErr):
            return fmt.Errorf("resource not found: %w", err)
        case errors.As(err, &validationErr):
            return fmt.Errorf("validation failed: %w", err)
        case errors.As(err, &dbErr):
            return fmt.Errorf("database error: %w", err)
        default:
            return fmt.Errorf("unknown error: %w", err)
        }
    }
    
    return nil
}
```

## Metrics and Observability

pgxkit doesn't provide built-in metrics interfaces, allowing you to use your preferred metrics library directly in hooks. Here are examples with popular metrics libraries:

### Prometheus Metrics

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    dbConnections = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "db_connections_active",
            Help: "Number of active database connections",
        },
        []string{"pool"},
    )
    
    queryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "db_query_duration_seconds",
            Help: "Database query duration in seconds",
        },
        []string{"operation"},
    )
    
    queryTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "db_queries_total",
            Help: "Total number of database queries",
        },
        []string{"operation", "status"},
    )
)

func setupPrometheusMetrics(db *pgxkit.DB) {
    // Track query metrics
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        // Start timer (store in context)
        start := time.Now()
        ctx = context.WithValue(ctx, "query_start", start)
        
        // Count query attempts
        operation := getOperationType(sql)
        queryTotal.WithLabelValues(operation, "attempted").Inc()
        
        return nil
    })
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        // Record query duration
        if start, ok := ctx.Value("query_start").(time.Time); ok {
            duration := time.Since(start)
            operation := getOperationType(sql)
            queryDuration.WithLabelValues(operation).Observe(duration.Seconds())
            
            // Count success/failure
            status := "success"
            if operationErr != nil {
                status = "error"
            }
            queryTotal.WithLabelValues(operation, status).Inc()
        }
        
        return nil
    })
    
    // Track connection metrics
    db.AddHook(pgxkit.OnAcquire, func(ctx context.Context, conn *pgx.Conn) error {
        dbConnections.WithLabelValues("write").Inc()
        return nil
    })
    
    db.AddHook(pgxkit.OnRelease, func(conn *pgx.Conn) {
        dbConnections.WithLabelValues("write").Dec()
    })
}

func getOperationType(sql string) string {
    sql = strings.TrimSpace(strings.ToUpper(sql))
    if strings.HasPrefix(sql, "SELECT") {
        return "select"
    } else if strings.HasPrefix(sql, "INSERT") {
        return "insert"
    } else if strings.HasPrefix(sql, "UPDATE") {
        return "update"
    } else if strings.HasPrefix(sql, "DELETE") {
        return "delete"
    }
    return "other"
}
```

### OpenTelemetry Tracing

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/trace"
)

func setupOpenTelemetryTracing(db *pgxkit.DB) {
    tracer := otel.Tracer("pgxkit")
    
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        // Start span
        ctx, span := tracer.Start(ctx, "db.query")
        span.SetAttributes(
            attribute.String("db.statement", sql),
            attribute.Int("db.args.count", len(args)),
        )
        
        // Store span in context for AfterOperation hook
        ctx = context.WithValue(ctx, "otel_span", span)
        
        return nil
    })
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        // End span
        if span, ok := ctx.Value("otel_span").(trace.Span); ok {
            if operationErr != nil {
                span.RecordError(operationErr)
                span.SetAttributes(attribute.Bool("db.error", true))
            }
            span.End()
        }
        
        return nil
    })
}
```

### StatsD Metrics

```go
import "github.com/DataDog/datadog-go/statsd"

func setupStatsDMetrics(db *pgxkit.DB) {
    client, err := statsd.New("localhost:8125")
    if err != nil {
        log.Fatal(err)
    }
    
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        // Start timer
        start := time.Now()
        ctx = context.WithValue(ctx, "statsd_start", start)
        
        // Count query attempts
        operation := getOperationType(sql)
        client.Incr("db.query.attempts", []string{fmt.Sprintf("operation:%s", operation)}, 1)
        
        return nil
    })
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        // Record timing
        if start, ok := ctx.Value("statsd_start").(time.Time); ok {
            duration := time.Since(start)
            operation := getOperationType(sql)
            
            client.Timing("db.query.duration", duration, []string{fmt.Sprintf("operation:%s", operation)}, 1)
            
            // Count success/failure
            status := "success"
            if operationErr != nil {
                status = "error"
            }
            client.Incr("db.query.completed", []string{
                fmt.Sprintf("operation:%s", operation),
                fmt.Sprintf("status:%s", status),
            }, 1)
        }
        
        return nil
    })
}
```

### Custom Metrics with slog

```go
import "log/slog"

func setupStructuredLogging(db *pgxkit.DB) {
    logger := slog.Default()
    
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start := time.Now()
        ctx = context.WithValue(ctx, "log_start", start)
        
        logger.InfoContext(ctx, "database query started",
            slog.String("sql", sql),
            slog.Int("args_count", len(args)),
        )
        
        return nil
    })
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        if start, ok := ctx.Value("log_start").(time.Time); ok {
            duration := time.Since(start)
            
            if operationErr != nil {
                logger.ErrorContext(ctx, "database query failed",
                    slog.String("sql", sql),
                    slog.Duration("duration", duration),
                    slog.String("error", operationErr.Error()),
                )
            } else {
                logger.InfoContext(ctx, "database query completed",
                    slog.String("sql", sql),
                    slog.Duration("duration", duration),
                )
            }
        }
        
        return nil
    })
}
```

### Connection Pool Metrics

```go
// Monitor connection pool health
func monitorConnectionPool(db *pgxkit.DB) {
    ticker := time.NewTicker(30 * time.Second)
    go func() {
        for range ticker.C {
            stats := db.Stats()
            
            // Log pool statistics
            slog.Info("connection pool stats",
                slog.Int32("acquired_conns", stats.AcquiredConns()),
                slog.Int32("idle_conns", stats.IdleConns()),
                slog.Int32("max_conns", stats.MaxConns()),
                slog.Int32("total_conns", stats.TotalConns()),
            )
            
            // Send to your metrics system
            // prometheus.GaugeVec.WithLabelValues("acquired").Set(float64(stats.AcquiredConns()))
            // statsd.Gauge("db.pool.acquired", float64(stats.AcquiredConns()), nil, 1)
        }
    }()
}
```

## Integration with Code Generation Tools

### With sqlc

```go
// Generate your sqlc code as usual
//go:generate sqlc generate

func main() {
    // Setup pgxkit
    db := pgxkit.NewDB()
    err := db.Connect(ctx, "")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)
    
    // Use sqlc with pgxkit's connection
    queries := sqlc.New(db) // sqlc can use pgxkit.DB directly
    
    // Or get the underlying pool if needed
    queries := sqlc.New(db.WritePool()) // for write operations
    readQueries := sqlc.New(db.ReadPool()) // for read operations
    
    // Use your generated queries
    users, err := queries.GetAllUsers(ctx)
    if err != nil {
        log.Fatal(err)
    }
}
```

### With Skimatik or Other Tools

```go
// pgxkit works with any tool that accepts a pgxpool.Pool
func main() {
    db := pgxkit.NewDB()
    err := db.Connect(ctx, "")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)
    
    // Use with any code generation tool
    queries := skimatik.New(db.WritePool())
    readQueries := skimatik.New(db.ReadPool())
    
    // Or use pgxkit directly
    rows, err := db.Query(ctx, "SELECT * FROM users")
    // ...
}
```

This comprehensive examples document shows how to use all of pgxkit's features in real-world scenarios. The tool-agnostic design makes it easy to integrate with any PostgreSQL development approach while providing production-ready features out of the box.

```

```
