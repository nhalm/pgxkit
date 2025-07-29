# Getting Started with pgxkit

**[← Back to Home](Home)**

Get up and running with pgxkit in under 5 minutes.

## Table of Contents
- [Prerequisites](#prerequisites)
- [Step 1: Install pgxkit](#step-1-install-pgxkit)
- [Step 2: Set Environment Variables](#step-2-set-environment-variables)
- [Step 3: Create Your First App](#step-3-create-your-first-app)
- [Step 4: Run Your App](#step-4-run-your-app)
- [Step 5: Add Production Features](#step-5-add-production-features)
- [What's Next?](#whats-next)
- [Troubleshooting](#troubleshooting)

## Prerequisites

- Go 1.21 or later
- PostgreSQL 12 or later
- Basic familiarity with Go and PostgreSQL

## Step 1: Install pgxkit

```bash
go mod init your-app
go get github.com/nhalm/pgxkit
```

## Step 2: Set Environment Variables

Set these environment variables (or use a `.env` file):

```bash
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_USER=postgres
export POSTGRES_PASSWORD=yourpassword
export POSTGRES_DB=yourdb
export POSTGRES_SSLMODE=disable
```

## Step 3: Create Your First App

Create `main.go`:

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
    err := db.Connect(ctx, "") // Uses environment variables
    if err != nil {
        log.Fatal("Failed to connect:", err)
    }
    defer db.Shutdown(ctx)
    
    // Test the connection
    if err := db.HealthCheck(ctx); err != nil {
        log.Fatal("Database health check failed:", err)
    }
    
    log.Println("✅ Connected to PostgreSQL!")
    
    // Create a simple table
    _, err = db.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS users (
            id SERIAL PRIMARY KEY,
            name TEXT NOT NULL,
            email TEXT UNIQUE NOT NULL,
            created_at TIMESTAMP DEFAULT NOW()
        )
    `)
    if err != nil {
        log.Fatal("Failed to create table:", err)
    }
    
    // Insert a user
    _, err = db.Exec(ctx, 
        "INSERT INTO users (name, email) VALUES ($1, $2)",
        "John Doe", "john@example.com")
    if err != nil {
        log.Fatal("Failed to insert user:", err)
    }
    
    // Query users
    rows, err := db.Query(ctx, "SELECT id, name, email FROM users")
    if err != nil {
        log.Fatal("Failed to query users:", err)
    }
    defer rows.Close()
    
    log.Println("👥 Users:")
    for rows.Next() {
        var id int
        var name, email string
        if err := rows.Scan(&id, &name, &email); err != nil {
            log.Fatal("Failed to scan row:", err)
        }
        log.Printf("  %d: %s (%s)", id, name, email)
    }
    
    log.Println("🎉 Success! pgxkit is working.")
}
```

## Step 4: Run Your App

```bash
go run main.go
```

You should see:
```
✅ Connected to PostgreSQL!
👥 Users:
  1: John Doe (john@example.com)
🎉 Success! pgxkit is working.
```

## Step 5: Add Production Features

### Add Logging Hook

```go
// Add after creating db
db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    log.Printf("🔍 Executing: %s", sql)
    return nil
})
```

### Add Read/Write Splitting

```go
// Instead of db.Connect(), use:
err := db.ConnectReadWrite(ctx, 
    "postgres://user:pass@read-replica/db",   // Read pool
    "postgres://user:pass@primary/db")       // Write pool

// Use read pool for queries
rows, err := db.ReadQuery(ctx, "SELECT * FROM users")
```

### Add Retry Logic

```go
// Retry database operations
config := pgxkit.DefaultRetryConfig()
result, err := db.ExecWithRetry(ctx, config, 
    "INSERT INTO users (name, email) VALUES ($1, $2)",
    "Jane Doe", "jane@example.com")
```

## What's Next?

🎯 **Production Ready**: Your app now has:
- ✅ Connection pooling
- ✅ Health checks  
- ✅ Graceful shutdown
- ✅ Hook system for observability

📚 **Learn More**:
- **[Examples](Examples)** - Comprehensive usage examples
- **[Performance Guide](Performance-Guide)** - Optimization strategies
- **[Production Guide](Production-Guide)** - Deployment best practices
- **[Testing Guide](Testing-Guide)** - Golden test support

🔧 **Common Patterns**:
- **[API Reference](API-Reference)** - Complete API documentation
- **[FAQ](FAQ)** - Common questions and solutions

## Troubleshooting

### Connection Issues
```bash
# Test your connection string
psql "postgres://user:pass@host:port/db"
```

### Environment Variables
```bash
# Check your environment
env | grep POSTGRES_
```

### Health Check
```go
if err := db.HealthCheck(ctx); err != nil {
    log.Printf("Health check failed: %v", err)
}
```

### Common Solutions

| Issue | Solution |
|-------|----------|
| Connection timeout | Check firewall and network connectivity |
| Authentication failed | Verify username/password and database permissions |
| Database not found | Ensure database exists and name is correct |
| SSL errors | Check `POSTGRES_SSLMODE` setting |

## See Also

- **[Examples](Examples)** - More comprehensive examples
- **[Performance Guide](Performance-Guide)** - Optimize your application
- **[Production Guide](Production-Guide)** - Deploy to production
- **[Contributing](Contributing)** - Help improve pgxkit

---

**[← Back to Home](Home)**

*Last updated: December 2024* 