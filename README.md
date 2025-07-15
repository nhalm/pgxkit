# Database Utilities

[![Go Version](https://img.shields.io/github/go-mod/go-version/nhalm/dbutil)](https://golang.org/doc/devel/release.html)
[![CI Status](https://github.com/nhalm/dbutil/actions/workflows/ci.yml/badge.svg)](https://github.com/nhalm/dbutil/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/nhalm/dbutil)](https://goreportcard.com/report/github.com/nhalm/dbutil)
[![Release](https://img.shields.io/github/v/release/nhalm/dbutil)](https://github.com/nhalm/dbutil/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A reusable Go package that provides database connection utilities and testing infrastructure for applications using PostgreSQL with pgx and sqlc.

## Overview

This package is designed specifically for **sqlc users** who want:
- **Reusable database utilities** that work with any sqlc-generated queries
- **Optimized testing infrastructure** with shared connections for faster tests
- **Type-safe PostgreSQL operations** with comprehensive pgx type helpers
- **Structured error handling** with consistent error types
- **Production-ready features** like health checks, metrics, retry logic, and connection hooks

## Installation

```bash
go get github.com/nhalm/dbutil
```

## Quick Start

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
    
    // Create connection with your sqlc queries
    conn, err := dbutil.NewConnection(ctx, "", sqlc.New)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    // Use your queries directly
    queries := conn.Queries()
    users, err := queries.GetAllUsers(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Found %d users", len(users))
}
```

## Configuration

### Environment Variables
The package uses these environment variables with sensible defaults:
- `POSTGRES_HOST` (default: "localhost")
- `POSTGRES_PORT` (default: 5432)
- `POSTGRES_USER` (default: "postgres")
- `POSTGRES_PASSWORD` (default: "")
- `POSTGRES_DB` (default: "postgres")
- `POSTGRES_SSLMODE` (default: "disable")
- `TEST_DATABASE_URL` (for integration tests)

### Custom Configuration
```go
config := &dbutil.Config{
    MaxConns:        20,
    MinConns:        5,
    MaxConnLifetime: 1 * time.Hour,
    SearchPath:      "myschema",
}
conn, err := dbutil.NewConnectionWithConfig(ctx, "", sqlc.New, config)
```

## Key Features

### **Generic Design**
Works with any sqlc-generated queries without coupling to specific packages:
```go
conn, err := dbutil.NewConnection(ctx, "", myapp.New)
conn, err := dbutil.NewConnection(ctx, "", yourapp.New)
```

### **Transaction Support**
```go
err = conn.WithTransaction(ctx, func(ctx context.Context, tx *sqlc.Queries) error {
    // All operations run in transaction, automatically rolled back on error
    user, err := tx.CreateUser(ctx, params)
    if err != nil {
        return err
    }
    return tx.CreateUserProfile(ctx, profileParams)
})
```

### **Health Checks & Monitoring**
```go
if conn.IsReady(ctx) {
    log.Println("Database is ready")
}

stats := conn.Stats()
log.Printf("Active connections: %d", stats.TotalConns())

// With metrics and hooks
conn = conn.WithMetrics(myMetricsCollector)
conn = conn.WithHooks(myHooks)
```

### **Read/Write Splitting**
```go
rwConn, err := dbutil.NewReadWriteConnection(ctx, readDSN, writeDSN, sqlc.New)
readQueries := rwConn.ReadQueries()   // Use for SELECT queries
writeQueries := rwConn.WriteQueries() // Use for INSERT/UPDATE/DELETE
```

### **Retry Logic**
```go
retryableConn := conn.WithRetry(nil) // Uses defaults
err = retryableConn.WithRetryableTransaction(ctx, func(ctx context.Context, tx *sqlc.Queries) error {
    return tx.CreateUser(ctx, params)
})
```



## Testing

This package provides optimized testing utilities with shared connections for faster integration tests:

```go
func TestUserOperations(t *testing.T) {
    conn := dbutil.RequireTestDB(t, sqlc.New)     // Shared connection
    dbutil.CleanupTestData(conn,                  // Clean data between tests
        "DELETE FROM users WHERE email LIKE 'test_%'",
    )
    
    // Run your test logic
    queries := conn.Queries()
    user, err := queries.CreateUser(ctx, params)
    // ... test assertions
}
```

### Test Database Setup
```bash
# Start test database
docker run --name test-postgres -e POSTGRES_PASSWORD=testpass -p 5433:5432 -d postgres:15

# Set environment variable
export TEST_DATABASE_URL="postgres://postgres:testpass@localhost:5433/postgres?sslmode=disable"

# Run integration tests
go test ./...
```

### Test Utilities
- **`RequireTestDB(t, sqlc.New)`** - Returns shared test connection, skips if no database
- **`CleanupTestData(conn, "DELETE ...")`** - Cleans test data between tests
- **`GetTestConnection(sqlc.New)`** - Returns connection or nil if unavailable

## Type Helpers

Comprehensive pgx type conversion utilities:

```go
// String conversions
pgxText := dbutil.ToPgxText(&myString)
stringPtr := dbutil.FromPgxText(pgxText)

// Numeric conversions
pgxNum := dbutil.ToPgxNumericFromFloat64Ptr(&myFloat)
floatPtr := dbutil.FromPgxNumericPtr(pgxNum)

// Time conversions
pgxTime := dbutil.ToPgxTimestamptz(&myTime)
timePtr := dbutil.FromPgxTimestamptzPtr(pgxTime)

// UUID conversions
pgxUUID := dbutil.ToPgxUUID(myUUID)
myUUID := dbutil.FromPgxUUID(pgxUUID)
```

## Error Handling

Structured error types for consistent error handling:

```go
// Create structured errors
err := dbutil.NewNotFoundError("User", userID)
err := dbutil.NewValidationError("Email", "create", "address", "invalid format", nil)
err := dbutil.NewDatabaseError("Order", "query", originalErr)

// Use with errors.As for type checking
var notFoundErr *dbutil.NotFoundError
if errors.As(err, &notFoundErr) {
    // Handle not found case
}
```

## Examples

See [examples.md](examples.md) for comprehensive usage examples including:
- Custom configuration
- Transaction handling
- Error handling patterns
- Read/write splitting
- Retry logic
- Connection hooks
- Integration testing
- Type conversion helpers

## Integration with golang-migrate

Use `GetDSN()` with golang-migrate for database migrations:

```go
import (
    "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/postgres"
    _ "github.com/golang-migrate/migrate/v4/source/file"
)

m, err := migrate.New("file://migrations", dbutil.GetDSN())
if err != nil {
    log.Fatal(err)
}
defer m.Close()

if err := m.Up(); err != nil {
    log.Fatal(err)
}
```