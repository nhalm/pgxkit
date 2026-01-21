# pgxkit Usage Examples

**[<- Back to Home](Home)**

This document provides comprehensive examples of using pgxkit - the tool-agnostic PostgreSQL toolkit.

## Table of Contents

1. [Basic Usage](#basic-usage)
2. [Read/Write Split](#readwrite-split)
3. [Executor Interface](#executor-interface)
4. [Hook System](#hook-system)
5. [Retry Logic](#retry-logic)
6. [Health Checks](#health-checks)
7. [Testing](#testing)
8. [Type Helpers](#type-helpers)
9. [Metrics and Observability](#metrics-and-observability)
10. [Production Patterns](#production-patterns)
11. [Integration with Code Generation](#integration-with-code-generation)

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

## Executor Interface

### Writing Reusable Repository Functions

The `Executor` interface allows you to write functions that work with both `*DB` and `*Tx`, making your code reusable across transactional and non-transactional contexts.

```go
// Repository function that accepts Executor interface
func CreateUser(ctx context.Context, exec pgxkit.Executor, name, email string) (int64, error) {
    var id int64
    err := exec.QueryRow(ctx,
        "INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id",
        name, email).Scan(&id)
    return id, err
}

func UpdateUserEmail(ctx context.Context, exec pgxkit.Executor, id int64, email string) error {
    _, err := exec.Exec(ctx,
        "UPDATE users SET email = $1 WHERE id = $2",
        email, id)
    return err
}

func GetUser(ctx context.Context, exec pgxkit.Executor, id int64) (*User, error) {
    var user User
    err := exec.QueryRow(ctx,
        "SELECT id, name, email FROM users WHERE id = $1",
        id).Scan(&user.ID, &user.Name, &user.Email)
    if err != nil {
        return nil, err
    }
    return &user, nil
}
```

### Using Repository Functions Without Transactions

```go
func main() {
    ctx := context.Background()
    db := pgxkit.NewDB()
    err := db.Connect(ctx, "")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)

    // Use repository functions directly with *DB
    id, err := CreateUser(ctx, db, "Alice", "alice@example.com")
    if err != nil {
        log.Fatal(err)
    }

    user, err := GetUser(ctx, db, id)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Created user: %+v", user)
}
```

### Using Repository Functions Within Transactions

```go
func TransferUserData(ctx context.Context, db *pgxkit.DB, fromID, toID int64) error {
    tx, err := db.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)

    // Get source user using the same repository function
    sourceUser, err := GetUser(ctx, tx, fromID)
    if err != nil {
        return fmt.Errorf("failed to get source user: %w", err)
    }

    // Update destination user using the same repository function
    err = UpdateUserEmail(ctx, tx, toID, sourceUser.Email)
    if err != nil {
        return fmt.Errorf("failed to update destination user: %w", err)
    }

    return tx.Commit(ctx)
}
```

### Service Layer Pattern

```go
type UserService struct {
    db *pgxkit.DB
}

func (s *UserService) CreateUserWithProfile(ctx context.Context, name, email, bio string) (*User, error) {
    tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return nil, err
    }
    defer tx.Rollback(ctx)

    // Create user
    userID, err := CreateUser(ctx, tx, name, email)
    if err != nil {
        return nil, fmt.Errorf("failed to create user: %w", err)
    }

    // Create profile in same transaction
    _, err = tx.Exec(ctx,
        "INSERT INTO profiles (user_id, bio) VALUES ($1, $2)",
        userID, bio)
    if err != nil {
        return nil, fmt.Errorf("failed to create profile: %w", err)
    }

    if err := tx.Commit(ctx); err != nil {
        return nil, err
    }

    // Fetch and return the complete user
    return GetUser(ctx, s.db, userID)
}
```

## Hook System

### Logging Hook

```go
func setupLogging() *pgxkit.DB {
    ctx := context.Background()
    db := pgxkit.NewDB()

    err := db.Connect(ctx, "",
        pgxkit.WithBeforeOperation(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
            log.Printf("Executing: %s", sql)
            return nil
        }),
        pgxkit.WithAfterOperation(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
            if operationErr != nil {
                log.Printf("Query failed: %v", operationErr)
            } else {
                log.Printf("Query completed successfully")
            }
            return nil
        }),
    )
    if err != nil {
        log.Fatal(err)
    }

    return db
}
```

### Metrics Hook

```go
func setupMetrics() *pgxkit.DB {
    ctx := context.Background()
    db := pgxkit.NewDB()

    err := db.Connect(ctx, "",
        pgxkit.WithAfterOperation(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
            operation := extractOperation(sql) // Parse SELECT, INSERT, etc.

            // Record query count
            if operationErr != nil {
                metrics.QueryErrors.WithLabelValues(operation).Inc()
            } else {
                metrics.QuerySuccess.WithLabelValues(operation).Inc()
            }

            return nil
        }),
    )
    if err != nil {
        log.Fatal(err)
    }

    return db
}
```

### Transaction Outcome Hook

```go
func setupTransactionMetrics() *pgxkit.DB {
    ctx := context.Background()
    db := pgxkit.NewDB()

    err := db.Connect(ctx, "",
        pgxkit.WithAfterTransaction(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
            // The sql parameter indicates transaction outcome
            switch sql {
            case pgxkit.TxCommit:
                if operationErr != nil {
                    metrics.TransactionCommitErrors.Inc()
                    log.Printf("Transaction commit failed: %v", operationErr)
                } else {
                    metrics.TransactionCommits.Inc()
                }
            case pgxkit.TxRollback:
                if operationErr != nil {
                    metrics.TransactionRollbackErrors.Inc()
                    log.Printf("Transaction rollback failed: %v", operationErr)
                } else {
                    metrics.TransactionRollbacks.Inc()
                }
            }
            return nil
        }),
    )
    if err != nil {
        log.Fatal(err)
    }

    return db
}
```

## Retry Logic

### Basic Retry

```go
func executeWithRetry(db *pgxkit.DB) {
    // Retry with default settings:
    // - 3 retry attempts
    // - 100ms initial delay
    // - 1s maximum delay
    // - 2x exponential backoff
    err := pgxkit.RetryOperation(ctx, func(ctx context.Context) error {
        _, err := db.Exec(ctx,
            "INSERT INTO users (name, email) VALUES ($1, $2)",
            "Jane Doe", "jane@example.com")
        return err
    })
    if err != nil {
        log.Fatal("Failed after retries:", err)
    }

    log.Println("User inserted successfully")
}
```

### Retry with Custom Configuration

```go
func executeWithCustomRetry(db *pgxkit.DB) {
    // Retry with custom settings using functional options
    err := pgxkit.RetryOperation(ctx, func(ctx context.Context) error {
        _, err := db.Exec(ctx,
            "INSERT INTO users (name, email) VALUES ($1, $2)",
            "Jane Doe", "jane@example.com")
        return err
    },
        pgxkit.WithMaxRetries(5),
        pgxkit.WithBaseDelay(50*time.Millisecond),
        pgxkit.WithMaxDelay(2*time.Second),
    )
    if err != nil {
        log.Fatal("Failed after retries:", err)
    }
}
```

### Retry Queries

```go
func queryWithRetry(db *pgxkit.DB) ([]User, error) {
    var users []User
    err := pgxkit.RetryOperation(ctx, func(ctx context.Context) error {
        rows, err := db.Query(ctx, "SELECT id, name, email FROM users WHERE active = true")
        if err != nil {
            return err
        }
        defer rows.Close()

        users = nil // Reset on retry
        for rows.Next() {
            var user User
            if err := rows.Scan(&user.ID, &user.Name, &user.Email); err != nil {
                return err
            }
            users = append(users, user)
        }
        return rows.Err()
    }, pgxkit.WithMaxRetries(3))

    return users, err
}
```

### Retry Transactions

```go
func executeTransactionWithRetry(db *pgxkit.DB) error {
    return pgxkit.RetryOperation(ctx, func(ctx context.Context) error {
        tx, err := db.BeginTx(ctx, pgx.TxOptions{})
        if err != nil {
            return err
        }
        defer tx.Rollback(ctx)

        // Perform transaction operations
        _, err = tx.Exec(ctx, "UPDATE accounts SET balance = balance - $1 WHERE id = $2", amount, fromID)
        if err != nil {
            return err
        }

        _, err = tx.Exec(ctx, "UPDATE accounts SET balance = balance + $1 WHERE id = $2", amount, toID)
        if err != nil {
            return err
        }

        return tx.Commit(ctx)
    }, pgxkit.WithMaxRetries(5), pgxkit.WithBackoffMultiplier(2.0))
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

Golden tests capture EXPLAIN plans for SELECT, INSERT, UPDATE, and DELETE queries. DML operations are safely executed in rolled-back transactions.

```go
func TestUserQueries(t *testing.T) {
    ctx := context.Background()

    // Setup test database
    testDB := pgxkit.NewTestDB()
    err := testDB.Connect(ctx, "")  // Uses TEST_DATABASE_URL env var
    if err != nil {
        t.Skip("Test database not available")
    }
    defer testDB.Shutdown(ctx)

    // Enable golden test support
    db := testDB.EnableGolden("TestUserQueries")

    // Load test data using your own fixture loading implementation
    // pgxkit does not provide a built-in fixture loader - use manual SQL or your preferred tool
    _, err = testDB.Exec(ctx, `
        INSERT INTO users (id, name, email) VALUES
        (1, 'John Doe', 'john@example.com'),
        (2, 'Jane Smith', 'jane@example.com')
        ON CONFLICT (id) DO NOTHING
    `)
    if err != nil {
        t.Fatal(err)
    }

    // Execute query - EXPLAIN plan will be captured
    rows, err := db.Query(ctx, `
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
        int64(id)).Scan(&user.ID, &user.Name, &user.Email)

    if err != nil {
        return nil, fmt.Errorf("failed to get user: %w", err)
    }

    return &user, nil
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

func setupPrometheusMetrics() *pgxkit.DB {
    ctx := context.Background()
    db := pgxkit.NewDB()

    err := db.Connect(ctx, "",
        pgxkit.WithAfterOperation(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
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
        }),
    )
    if err != nil {
        log.Fatal(err)
    }

    return db
}
```

### Structured Logging with slog

```go
import "log/slog"

func setupStructuredLogging() *pgxkit.DB {
    ctx := context.Background()
    logger := slog.Default()
    db := pgxkit.NewDB()

    err := db.Connect(ctx, "",
        pgxkit.WithAfterOperation(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
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
        }),
    )
    if err != nil {
        log.Fatal(err)
    }

    return db
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

**[<- Back to Home](Home)**

*This comprehensive examples document shows how to use all of pgxkit's features in real-world scenarios. The tool-agnostic design makes it easy to integrate with any PostgreSQL development approach while providing production-ready features out of the box.*
