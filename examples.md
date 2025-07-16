# pgxkit Usage Examples

## Basic Usage with Connect() Pattern

```go
package main

import (
    "context"
    "log"

    "github.com/nhalm/pgxkit"
)

func main() {
    ctx := context.Background()
    
    // 1. Create a new DB instance (no connection yet)
    db := pgxkit.NewDB()
    
    // 2. Add hooks BEFORE connecting (hooks will be integrated during connection)
    err := db.AddHook("BeforeOperation", func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        log.Printf("Executing query: %s", sql)
        return nil
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Add connection-level hooks that will execute during pool lifecycle
    err = db.AddConnectionHook("OnConnect", func(conn *pgx.Conn) error {
        log.Printf("New connection established: PID %d", conn.PgConn().PID())
        return nil
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // 3. Connect to database with hooks already configured
    err = db.Connect(ctx, "postgres://user:password@localhost/dbname")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)
    
    // Now use the database - hooks will execute automatically
    rows, err := db.Query(ctx, "SELECT id, name FROM users WHERE active = $1", true)
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
    
    // Process results...
    for rows.Next() {
        var id int
        var name string
        if err := rows.Scan(&id, &name); err != nil {
            log.Fatal(err)
        }
        log.Printf("User: %d - %s", id, name)
    }
}
```

## Read/Write Split with Hooks

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
    
    // 1. Create a new DB instance (no connection yet)
    db := pgxkit.NewDB()
    
    // 2. Add hooks BEFORE connecting
    db.AddHook("BeforeOperation", func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start := time.Now()
        ctx = context.WithValue(ctx, "start_time", start)
        return nil
    })
    
    db.AddHook("AfterOperation", func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        if start, ok := ctx.Value("start_time").(time.Time); ok {
            duration := time.Since(start)
            log.Printf("Query took %v: %s", duration, sql)
        }
        return nil
    })
    
    // 3. Connect with separate read/write pools (hooks already configured)
    err := db.ConnectReadWrite(ctx, 
        "postgres://user:password@read-replica/dbname",
        "postgres://user:password@primary/dbname")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)
    
    // Use read pool for queries (hooks execute automatically)
    users, err := db.ReadQuery(ctx, "SELECT * FROM users")
    if err != nil {
        log.Fatal(err)
    }
    defer users.Close()
    
    // Use write pool for mutations (hooks execute automatically)
    _, err = db.Exec(ctx, "UPDATE users SET last_login = NOW() WHERE id = $1", 123)
    if err != nil {
        log.Fatal(err)
    }
}
```

## Comprehensive Hook Usage

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/nhalm/pgxkit"
)

func main() {
    ctx := context.Background()
    
    // Create DB instance
    db := pgxkit.NewDB()
    
    // Add operation-level hooks
    db.AddHook("BeforeOperation", func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        log.Printf("Starting query: %s", sql)
        return nil
    })
    
    db.AddHook("AfterOperation", func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        if operationErr != nil {
            log.Printf("Query failed: %v", operationErr)
        } else {
            log.Printf("Query completed successfully")
        }
        return nil
    })
    
    db.AddHook("BeforeTransaction", func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        log.Printf("Starting transaction")
        return nil
    })
    
    db.AddHook("AfterTransaction", func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        if operationErr != nil {
            log.Printf("Transaction failed: %v", operationErr)
        } else {
            log.Printf("Transaction completed successfully")
        }
        return nil
    })
    
    // Add connection-level hooks
    db.AddConnectionHook("OnConnect", func(conn *pgx.Conn) error {
        log.Printf("Connection established: PID %d", conn.PgConn().PID())
        // Set connection-specific settings
        _, err := conn.Exec(ctx, "SET application_name = 'pgxkit-example'")
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
    
    // Connect with hooks integrated
    err := db.Connect(ctx, "postgres://user:password@localhost/dbname")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)
    
    // Use database with transaction
    tx, err := db.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        log.Fatal(err)
    }
    
    _, err = tx.Exec(ctx, "INSERT INTO users (name, email) VALUES ($1, $2)", "John Doe", "john@example.com")
    if err != nil {
        tx.Rollback(ctx)
        log.Fatal(err)
    }
    
    err = tx.Commit(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Println("Transaction completed successfully")
}
```

## Key Benefits of This Approach

1. **Hooks configured before connection** - pgx connection-level hooks are properly integrated when pools are created
2. **Clean separation** - Create DB → Add hooks → Connect
3. **Proper abstraction** - pgxkit manages pool creation with hooks already configured
4. **Flexible configuration** - Add multiple hooks before connecting
5. **Type safety** - Hooks are validated when added, not when connecting
6. **Automatic execution** - Both operation-level and connection-level hooks execute automatically
7. **Read/write optimization** - Explicit read methods for performance with proper hook integration

## Available Hook Types

### Operation-Level Hooks
- `BeforeOperation` - Executed before any query/exec operation
- `AfterOperation` - Executed after any query/exec operation  
- `BeforeTransaction` - Executed before starting a transaction
- `AfterTransaction` - Executed after transaction commit/rollback
- `OnShutdown` - Executed during graceful shutdown

### Connection-Level Hooks (pgx lifecycle)
- `OnConnect` - Executed when a new connection is established
- `OnDisconnect` - Executed when a connection is closed
- `OnAcquire` - Executed when a connection is acquired from the pool
- `OnRelease` - Executed when a connection is released back to the pool
