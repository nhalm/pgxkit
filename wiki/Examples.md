# pgxkit Usage Examples

**[← Back to Home](Home)**

This document provides comprehensive examples of using pgxkit - the tool-agnostic PostgreSQL toolkit.

## Table of Contents

1. [Basic Usage](#basic-usage)
2. [Read/Write Split](#readwrite-split) 
3. [Hook System](#hook-system)
4. [Retry Logic](#retry-logic)
5. [Health Checks](#health-checks)
6. [Testing](#testing)
7. [Type Helpers](#type-helpers)
8. [Metrics and Observability](#metrics-and-observability)
9. [Production Patterns](#production-patterns)
10. [Integration with Code Generation](#integration-with-code-generation)

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
    _, err = db.Exec(ctx, "INSERT INTO users (name, email) VALUES ($1, $2)", 
        "John Doe", "john@example.com")
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
func main() {
    ctx := context.Background()
    db := pgxkit.NewDB()
    
    // Connect with separate read and write pools
    err := db.ConnectReadWrite(ctx,
        "postgres://user:pass@read-replica:5432/db",  // Read pool
        "postgres://user:pass@primary:5432/db")       // Write pool
    if err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)
    
    // Use read pool for queries (better performance)
    rows, err := db.ReadQuery(ctx, "SELECT * FROM users WHERE active = true")
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
    
    // Use write pool for modifications
    _, err = db.Exec(ctx, "UPDATE users SET last_login = NOW() WHERE id = $1", userID)
    if err != nil {
        log.Fatal(err)
    }
}
```

### Smart Query Routing

```go
// Automatically route queries based on operation type
func smartQuery(db *pgxkit.DB, sql string, args ...interface{}) (pgx.Rows, error) {
    if isReadQuery(sql) {
        return db.ReadQuery(ctx, sql, args...)
    }
    return db.Query(ctx, sql, args...)
}

func isReadQuery(sql string) bool {
    sql = strings.ToUpper(strings.TrimSpace(sql))
    return strings.HasPrefix(sql, "SELECT") || 
           strings.HasPrefix(sql, "WITH") ||
           strings.HasPrefix(sql, "EXPLAIN")
}
```

## Hook System

### Logging Hook

```go
func setupLogging(db *pgxkit.DB) {
    // Log all queries
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start := time.Now()
        ctx = context.WithValue(ctx, "query_start", start)
        log.Printf("🔍 Executing: %s", sql)
        return nil
    })
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        if start, ok := ctx.Value("query_start").(time.Time); ok {
            duration := time.Since(start)
            if operationErr != nil {
                log.Printf("❌ Query failed in %v: %v", duration, operationErr)
            } else {
                log.Printf("✅ Query completed in %v", duration)
            }
        }
        return nil
    })
}
```

### Metrics Hook

```go
func setupMetrics(db *pgxkit.DB) {
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        operation := extractOperation(sql) // Parse SELECT, INSERT, etc.
        
        // Record query count
        if operationErr != nil {
            metrics.QueryErrors.WithLabelValues(operation).Inc()
        } else {
            metrics.QuerySuccess.WithLabelValues(operation).Inc()
        }
        
        return nil
    })
}
```

## Retry Logic

### Basic Retry Configuration

```go
func executeWithRetry(db *pgxkit.DB) {
    config := pgxkit.DefaultRetryConfig()
    config.MaxRetries = 3
    config.BaseDelay = 100 * time.Millisecond
    
    // Retry failed operations
    result, err := db.ExecWithRetry(ctx, config,
        "INSERT INTO users (name, email) VALUES ($1, $2)",
        "Jane Doe", "jane@example.com")
    if err != nil {
        log.Fatal("Failed after retries:", err)
    }
    
    log.Printf("Inserted user, affected rows: %d", result.RowsAffected())
}
```

### Custom Retry Logic

```go
func customRetryConfig() *pgxkit.RetryConfig {
    return &pgxkit.RetryConfig{
        MaxRetries:    5,
        BaseDelay:     50 * time.Millisecond,
        MaxDelay:      2 * time.Second,
        BackoffFactor: 2.0,
        
        // Custom retry condition
        ShouldRetry: func(err error) bool {
            // Only retry on connection errors
            return errors.Is(err, pgx.ErrNoRows) == false &&
                   strings.Contains(err.Error(), "connection")
        },
    }
}
```

## Health Checks

### Basic Health Check

```go
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
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
    
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "healthy",
    })
}
```

### Advanced Health Check

```go
func advancedHealthCheck(db *pgxkit.DB) error {
    ctx := context.Background()
    
    // Check basic connectivity
    if err := db.HealthCheck(ctx); err != nil {
        return fmt.Errorf("basic health check failed: %w", err)
    }
    
    // Check pool statistics
    stats := db.Stats()
    if stats.AcquiredConns() >= stats.MaxConns() {
        return fmt.Errorf("connection pool exhausted: %d/%d", 
            stats.AcquiredConns(), stats.MaxConns())
    }
    
    // Test actual query
    var result int
    err := db.QueryRow(ctx, "SELECT 1").Scan(&result)
    if err != nil {
        return fmt.Errorf("test query failed: %w", err)
    }
    
    return nil
}
```

## Testing

### Golden Tests

```go
func TestUserQueries(t *testing.T) {
    // Setup test database with golden test support
    testDB := pgxkit.NewTestDB(t)
    db := testDB.EnableGolden(t, "TestUserQueries")
    
    // Create test data
    testDB.LoadFixtures(t, "users.sql")
    
    // Execute query - EXPLAIN plan will be captured
    rows, err := db.Query(context.Background(), `
        SELECT u.id, u.name, COUNT(o.id) as order_count
        FROM users u
        LEFT JOIN orders o ON u.id = o.user_id
        GROUP BY u.id, u.name
        ORDER BY order_count DESC
    `)
    require.NoError(t, err)
    defer rows.Close()
    
    var users []User
    for rows.Next() {
        var user User
        var orderCount int
        err := rows.Scan(&user.ID, &user.Name, &orderCount)
        require.NoError(t, err)
        user.OrderCount = orderCount
        users = append(users, user)
    }
    
    // Golden test will compare query plan and results
    require.Len(t, users, 3)
}
```

### Test Database Helper

```go
func setupTestDB(t *testing.T) *pgxkit.DB {
    // Use test-specific database
    dsn := os.Getenv("TEST_DATABASE_URL")
    if dsn == "" {
        t.Skip("TEST_DATABASE_URL not set")
    }
    
    db := pgxkit.NewDB()
    err := db.Connect(context.Background(), dsn)
    require.NoError(t, err)
    
    t.Cleanup(func() {
        db.Shutdown(context.Background())
    })
    
    return db
}
```

## Type Helpers

### Clean Architecture Types

```go
// Domain types
type User struct {
    ID    UserID `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

type UserID int64

// Repository implementation
func (r *UserRepository) GetUser(ctx context.Context, id UserID) (*User, error) {
    var user User
    err := r.db.ReadQueryRow(ctx,
        "SELECT id, name, email FROM users WHERE id = $1",
        pgxkit.Int64(id)).Scan(
            pgxkit.ScanInt64(&user.ID),
            &user.Name,
            &user.Email)
    
    if err != nil {
        return nil, fmt.Errorf("failed to get user: %w", err)
    }
    
    return &user, nil
}
```

### JSON and Array Helpers

```go
// Working with JSON columns
type UserPreferences struct {
    Theme    string `json:"theme"`
    Language string `json:"language"`
}

func saveUserPreferences(db *pgxkit.DB, userID int, prefs UserPreferences) error {
    _, err := db.Exec(ctx,
        "UPDATE users SET preferences = $1 WHERE id = $2",
        pgxkit.JSON(prefs), userID)
    return err
}

func getUserPreferences(db *pgxkit.DB, userID int) (*UserPreferences, error) {
    var prefs UserPreferences
    err := db.QueryRow(ctx,
        "SELECT preferences FROM users WHERE id = $1",
        userID).Scan(pgxkit.ScanJSON(&prefs))
    return &prefs, err
}
```

## Metrics and Observability

### Prometheus Integration

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    queryDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "pgxkit_query_duration_seconds",
            Help: "Query execution duration",
        },
        []string{"operation", "status"},
    )
)

func setupPrometheusMetrics(db *pgxkit.DB) {
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start := time.Now()
        return context.WithValue(ctx, "metrics_start", start)
    })
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        if start, ok := ctx.Value("metrics_start").(time.Time); ok {
            duration := time.Since(start)
            operation := extractOperation(sql)
            status := "success"
            if operationErr != nil {
                status = "error"
            }
            
            queryDuration.WithLabelValues(operation, status).Observe(duration.Seconds())
        }
        return nil
    })
}
```

### Structured Logging with slog

```go
import "log/slog"

func setupStructuredLogging(db *pgxkit.DB) {
    logger := slog.Default()
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        if operationErr != nil {
            logger.ErrorContext(ctx, "database query failed",
                slog.String("sql", sql),
                slog.String("error", operationErr.Error()),
            )
        } else {
            logger.InfoContext(ctx, "database query completed",
                slog.String("sql", sql),
                slog.Int("args_count", len(args)),
            )
        }
        return nil
    })
}
```

## Production Patterns

### Graceful Shutdown

```go
func main() {
    db := pgxkit.NewDB()
    err := db.Connect(context.Background(), "")
    if err != nil {
        log.Fatal(err)
    }
    
    // Setup graceful shutdown
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    
    go func() {
        <-c
        log.Println("Shutting down gracefully...")
        
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        
        if err := db.Shutdown(ctx); err != nil {
            log.Printf("Error during shutdown: %v", err)
        }
        
        os.Exit(0)
    }()
    
    // Your application logic here
    select {}
}
```

### Connection Pool Monitoring

```go
func monitorConnectionPool(db *pgxkit.DB) {
    ticker := time.NewTicker(30 * time.Second)
    go func() {
        for range ticker.C {
            stats := db.Stats()
            
            utilization := float64(stats.AcquiredConns()) / float64(stats.MaxConns())
            
            slog.Info("connection pool stats",
                slog.Int32("acquired_conns", stats.AcquiredConns()),
                slog.Int32("idle_conns", stats.IdleConns()),
                slog.Int32("max_conns", stats.MaxConns()),
                slog.Float64("utilization", utilization),
            )
            
            // Alert if utilization is high
            if utilization > 0.8 {
                slog.Warn("high connection pool utilization",
                    slog.Float64("utilization", utilization))
            }
        }
    }()
}
```

## Integration with Code Generation

### With sqlc

```go
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
    
    // Or get specific pools
    writeQueries := sqlc.New(db.WritePool()) // for write operations
    readQueries := sqlc.New(db.ReadPool())   // for read operations
    
    // Use your generated queries
    users, err := queries.GetAllUsers(ctx)
    if err != nil {
        log.Fatal(err)
    }
}
```

### With Other Code Generation Tools

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

## See Also

- **[Getting Started](Getting-Started)** - Basic setup and configuration
- **[Performance Guide](Performance-Guide)** - Optimization strategies
- **[Production Guide](Production-Guide)** - Deployment best practices
- **[Testing Guide](Testing-Guide)** - Testing strategies and golden tests
- **[API Reference](API-Reference)** - Complete API documentation

---

**[← Back to Home](Home)**

*This comprehensive examples document shows how to use all of pgxkit's features in real-world scenarios. The tool-agnostic design makes it easy to integrate with any PostgreSQL development approach while providing production-ready features out of the box.*

*Last updated: December 2024* 