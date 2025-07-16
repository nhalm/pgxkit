# pgxkit

[![Go Version](https://img.shields.io/github/go-mod/go-version/nhalm/pgxkit)](https://golang.org/doc/devel/release.html)
[![CI Status](https://github.com/nhalm/pgxkit/actions/workflows/ci.yml/badge.svg)](https://github.com/nhalm/pgxkit/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/nhalm/pgxkit)](https://goreportcard.com/report/github.com/nhalm/pgxkit)
[![Release](https://img.shields.io/github/v/release/nhalm/pgxkit)](https://github.com/nhalm/pgxkit/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A production-ready PostgreSQL toolkit for Go applications — tool-agnostic utilities for connection pooling, observability, testing, and type helpers.

## Overview

pgxkit is a **tool-agnostic** PostgreSQL toolkit that works with any approach to PostgreSQL development:

- **Raw pgx usage** - Use pgxkit directly with pgx for maximum control
- **Any code generation tool** - Works with sqlc, Skimatik, or any other tool
- **Clean architecture** - Separate your database layer from your business logic
- **Production-ready** - Built-in observability, retry logic, and graceful shutdown

## Key Features

- 🔄 **Read/Write Pool Abstraction** - Safe by default, optimized when needed
- 🎣 **Extensible Hook System** - Add logging, tracing, metrics, circuit breakers
- 🔁 **Smart Retry Logic** - PostgreSQL-aware error detection and exponential backoff
- 🧪 **Testing Infrastructure** - Golden test support for performance regression detection
- 🔧 **Type Helpers** - Seamless pgx type conversions
- 📊 **Health Checks** - Built-in database connectivity monitoring
- 🛡️ **Graceful Shutdown** - Production-ready lifecycle management

## Installation

```bash
go get github.com/nhalm/pgxkit
```

## Quick Start

### Basic Usage

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
    
    // Connect using environment variables or explicit DSN
    err := db.Connect(ctx, "") // Uses POSTGRES_* env vars
    if err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)
    
    // Execute queries (uses write pool by default - safe)
    _, err = db.Exec(ctx, "INSERT INTO users (name) VALUES ($1)", "John")
    if err != nil {
        log.Fatal(err)
    }
    
    // Optimize reads with explicit read pool usage
    rows, err := db.ReadQuery(ctx, "SELECT id, name FROM users")
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
    
    // Process results...
}
```

### With Read/Write Splitting

```go
db := pgxkit.NewDB()

// Connect with separate read and write pools
err := db.ConnectReadWrite(ctx, readDSN, writeDSN)
if err != nil {
    log.Fatal(err)
}

// Writes go to write pool (safe by default)
db.Exec(ctx, "INSERT INTO users ...")

// Reads can use read pool for optimization
db.ReadQuery(ctx, "SELECT * FROM users")
```

## Configuration

### Environment Variables

pgxkit uses these environment variables when no DSN is provided:

- `POSTGRES_HOST` (default: "localhost")
- `POSTGRES_PORT` (default: 5432)
- `POSTGRES_USER` (default: "postgres")
- `POSTGRES_PASSWORD` (default: "")
- `POSTGRES_DB` (default: "postgres")
- `POSTGRES_SSLMODE` (default: "disable")

### DSN Utilities

```go
// Get DSN from environment variables
dsn := pgxkit.GetDSN()

// Use with external tools like golang-migrate
migrate -database "$(pgxkit.GetDSN())" -path ./migrations up
```

## Hooks System

Add observability and custom functionality through hooks:

```go
db := pgxkit.NewDB()

// Add logging hook
db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    log.Printf("Executing: %s", sql)
    return nil
})

// Add metrics hook
db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    if operationErr != nil {
        metrics.IncrementCounter("db.errors")
    }
    return nil
})

// Add connection-level hooks
db.AddConnectionHook("OnConnect", func(conn *pgx.Conn) error {
    log.Println("New connection established")
    return nil
})
```

### Available Hook Types

**Operation-level hooks:**
- `BeforeOperation` - Before any query/exec
- `AfterOperation` - After any query/exec
- `BeforeTransaction` - Before transaction starts
- `AfterTransaction` - After transaction completes
- `OnShutdown` - During graceful shutdown

**Connection-level hooks:**
- `OnConnect` - When new connection is established
- `OnDisconnect` - When connection is closed
- `OnAcquire` - When connection is acquired from pool
- `OnRelease` - When connection is returned to pool

## Retry Logic

### Built-in Retry Methods

```go
config := pgxkit.DefaultRetryConfig() // 3 retries, exponential backoff

// Retry database operations
result, err := db.ExecWithRetry(ctx, config, "INSERT INTO users ...")
rows, err := db.QueryWithRetry(ctx, config, "SELECT * FROM users")
rows, err := db.ReadQueryWithRetry(ctx, config, "SELECT * FROM users") // Uses read pool
tx, err := db.BeginTxWithRetry(ctx, config, pgx.TxOptions{})
```

### Generic Retry Functions

```go
// Retry any operation
err := pgxkit.RetryOperation(ctx, config, func(ctx context.Context) error {
    return someComplexDatabaseOperation(ctx)
})

// Retry with timeout
result, err := pgxkit.WithTimeoutAndRetry(ctx, 5*time.Second, config, func(ctx context.Context) (*User, error) {
    return getUserFromDatabase(ctx)
})
```

### Smart Error Detection

pgxkit automatically detects which PostgreSQL errors are worth retrying:

```go
if pgxkit.IsRetryableError(err) {
    // Connection errors, deadlocks, serialization failures, etc.
}
```

## Health Checks

```go
// Check database connectivity
if db.IsReady(ctx) {
    log.Println("Database is ready")
}

// Detailed health check
if err := db.HealthCheck(ctx); err != nil {
    log.Printf("Database health check failed: %v", err)
}
```

## Testing

### Basic Testing

```go
func TestUserOperations(t *testing.T) {
    testDB := pgxkit.NewTestDB()
    err := testDB.Setup()
    if err != nil {
        t.Skip("Test database not available")
    }
    defer testDB.Clean()
    
    // Use testDB.DB for your tests
    _, err = testDB.Exec(ctx, "INSERT INTO users ...")
    // ... test assertions
}
```

### Golden Testing (Performance Regression Detection)

```go
func TestUserQueries(t *testing.T) {
    testDB := pgxkit.NewTestDB()
    testDB.Setup()
    defer testDB.Clean()
    
    // Enable golden test hooks - captures EXPLAIN plans automatically
    db := testDB.EnableGolden(t, "TestUserQueries")
    
    // These queries will have their EXPLAIN plans captured
    rows, err := db.Query(ctx, "SELECT * FROM users WHERE active = true")
    // ... more queries
    
    // Plans are saved to testdata/golden/TestUserQueries_query_1.json, etc.
    // Future runs compare plans to detect performance regressions
}
```

## Type Helpers

Seamless conversions between Go types and pgx types:

```go
// String conversions
pgxText := pgxkit.ToPgxText(&myString)
stringPtr := pgxkit.FromPgxText(pgxText)

// Numeric conversions
pgxInt := pgxkit.ToPgxInt8(&myInt64)
intPtr := pgxkit.FromPgxInt8(pgxInt)

// UUID conversions
pgxUUID := pgxkit.ToPgxUUID(myUUID)
uuid := pgxkit.FromPgxUUID(pgxUUID)

// Time conversions
pgxTime := pgxkit.ToPgxTimestamptz(&myTime)
timePtr := pgxkit.FromPgxTimestamptz(pgxTime)

// Array conversions
pgxArray := pgxkit.ToPgxTextArray(myStringSlice)
stringSlice := pgxkit.FromPgxTextArray(pgxArray)
```

## Production Features

### Graceful Shutdown

```go
// Graceful shutdown with timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

err := db.Shutdown(ctx)
if err != nil {
    log.Printf("Shutdown completed with warnings: %v", err)
}
```

### Connection Statistics

```go
// Get pool statistics
stats := db.Stats()          // Write pool stats
readStats := db.ReadStats()  // Read pool stats (if using read/write split)

log.Printf("Active connections: %d", stats.AcquiredConns())
log.Printf("Idle connections: %d", stats.IdleConns())
```

### Error Handling

```go
// Structured error types
err := pgxkit.NewNotFoundError("User", userID)
err := pgxkit.NewValidationError("Email", "create", "address", "invalid format", nil)
err := pgxkit.NewDatabaseError("Order", "query", originalErr)

// Type checking
var notFoundErr *pgxkit.NotFoundError
if errors.As(err, &notFoundErr) {
    // Handle not found error
}
```

## Architecture

pgxkit follows these design principles:

1. **Safety First** - All default methods use write pool for consistency
2. **Explicit Optimization** - Use `ReadQuery()` methods for read optimization
3. **Tool Agnostic** - Works with any PostgreSQL development approach
4. **Extensible** - Hook system for custom functionality
5. **Production Ready** - Built-in observability and lifecycle management

## Examples

### With Raw pgx

```go
db := pgxkit.NewDB()
db.Connect(ctx, "postgres://...")

// Use pgx directly with pgxkit utilities
rows, err := db.Query(ctx, "SELECT * FROM users")
for rows.Next() {
    var user User
    err := rows.Scan(&user.ID, &user.Name)
    // ...
}
```

### With Any Code Generation Tool

```go
// Works with sqlc, Skimatik, or any other tool
db := pgxkit.NewDB()
db.Connect(ctx, "postgres://...")

// Use your generated code with pgxkit's connection
queries := sqlc.New(db.GetPool()) // or your tool's constructor
users, err := queries.GetAllUsers(ctx)
```

### With Hooks for Observability

```go
db := pgxkit.NewDB()

// Add tracing
db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    span := trace.SpanFromContext(ctx)
    span.SetAttributes(attribute.String("db.statement", sql))
    return nil
})

// Add circuit breaker
db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    return circuitBreaker.Execute(func() error {
        // Operation will be executed by pgxkit
        return nil
    })
})
```

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

## License

MIT License - see [LICENSE](LICENSE) file for details.

