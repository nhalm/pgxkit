# Getting Started

**[← Back to Home](Home)**

## Install

```bash
go get github.com/nhalm/pgxkit/v2
```

Requires Go 1.25+ and PostgreSQL 12+.

## Connect

`Connect` accepts a DSN. If empty, it builds one from `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_SSLMODE`.

```go
package main

import (
    "context"
    "log"

    "github.com/nhalm/pgxkit/v2"
)

func main() {
    ctx := context.Background()
    db := pgxkit.NewDB()
    if err := db.Connect(ctx, ""); err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)

    if err := db.HealthCheck(ctx); err != nil {
        log.Fatal(err)
    }

    var n int
    if err := db.QueryRow(ctx, "SELECT 1").Scan(&n); err != nil {
        log.Fatal(err)
    }
    log.Println("ok:", n)
}
```

`db.Query`, `db.QueryRow`, `db.Exec` go through the write pool. `db.ReadQuery` and `db.ReadQueryRow` go through the read pool when you've configured one via `ConnectReadWrite`; otherwise they share the single pool.

## Read/write split

```go
err := db.ConnectReadWrite(ctx,
    "postgres://user:pass@replica/db",
    "postgres://user:pass@primary/db")
```

## Hooks

Every operation passes through configurable hooks. `AfterOperation` receives the `pgconn.CommandTag` from Exec.

```go
db.Connect(ctx, "",
    pgxkit.WithBeforeOperation(func(ctx context.Context, sql string, args []interface{}, tag pgconn.CommandTag, err error) error {
        log.Printf("query: %s", sql)
        return nil
    }),
)
```

See [Examples](Examples#hook-system) for metrics, slog, and transaction-outcome patterns.

## Retry

```go
err := pgxkit.RetryOperation(ctx, func(ctx context.Context) error {
    _, err := db.Exec(ctx, "INSERT INTO users (name) VALUES ($1)", "Jane")
    return err
})
```

`RetryOperation` retries on transient PostgreSQL errors (serialization failures, deadlocks, connection issues). Tune via `WithMaxRetries`, `WithBaseDelay`, etc.

## Next

- [Examples](Examples) — hooks, retry, testing, type helpers
- [Testing Guide](Testing-Guide) — `EnableAssertPlan`, `EnableGolden`, `RequireDB`
- [API Reference](API-Reference)
- [FAQ](FAQ)
