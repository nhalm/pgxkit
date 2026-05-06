# pgxkit

A focused PostgreSQL toolkit for Go, built on `pgx`. pgxkit gives you a connection pool with read/write split, a hook system, retry helpers, plan-regression and golden-transcript testing, and graceful shutdown — and stays out of your way otherwise.

## Documentation

- [Getting Started](Getting-Started) — install, connect, first queries
- [Examples](Examples) — real patterns for hooks, retries, testing
- [API Reference](API-Reference) — every public type and function
- [Performance](Performance-Guide) — pool sizing, plan-regression, monitoring with `db.Stats()`
- [Production](Production-Guide) — graceful shutdown, health checks, observability hooks
- [Testing](Testing-Guide) — `RequireDB`, `EnableAssertPlan`, `EnableGolden`
- [FAQ](FAQ) — quick answers
- [Contributing](Contributing)

## Features

- Connection pool with optional read/write split (`Connect` / `ConnectReadWrite`).
- Extensible hook system: `BeforeOperation`, `AfterOperation`, `BeforeTransaction`, `AfterTransaction`, `OnShutdown`, plus pgx connection-lifecycle hooks. `AfterOperation` receives the `pgconn.CommandTag` for Exec.
- Retry helpers: `RetryOperation` and the typed `Retry[T]`, with PostgreSQL-aware error classification.
- Plan-regression testing (`EnableAssertPlan` / `AssertPlan`) and golden-transcript testing (`EnableGolden` / `AssertGolden`).
- Graceful shutdown with active-operation tracking.
- Type helpers for clean conversion between Go and pgx types.

## Quick start

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
    if err := db.Connect(ctx, "postgres://user:pass@localhost/db"); err != nil {
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)

    var name string
    if err := db.QueryRow(ctx, "SELECT name FROM users WHERE id = $1", 1).Scan(&name); err != nil {
        log.Fatal(err)
    }
    log.Println(name)
}
```

## Module

- Module path: `github.com/nhalm/pgxkit/v2`
- Go: 1.25+
- License: MIT
- Dependencies: `pgx`, `google/uuid`, `pmezard/go-difflib`

## Links

- [Repo](https://github.com/nhalm/pgxkit) · [Go package](https://pkg.go.dev/github.com/nhalm/pgxkit/v2) · [Issues](https://github.com/nhalm/pgxkit/issues)

---
