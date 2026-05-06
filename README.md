# pgxkit

[![CI](https://github.com/nhalm/pgxkit/actions/workflows/ci.yml/badge.svg)](https://github.com/nhalm/pgxkit/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/nhalm/pgxkit/v2.svg)](https://pkg.go.dev/github.com/nhalm/pgxkit/v2)
[![Go Report Card](https://goreportcard.com/badge/github.com/nhalm/pgxkit/v2)](https://goreportcard.com/report/github.com/nhalm/pgxkit/v2)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A focused PostgreSQL toolkit for Go, built on `pgx`. Connection pool with read/write split, an extensible hook system, retry helpers, plan-regression and golden-transcript testing, graceful shutdown, and small type helpers — and nothing else.

## Install

```bash
go get github.com/nhalm/pgxkit/v2
```

Go 1.25+ · PostgreSQL 12+.

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
    if err := db.Connect(ctx, ""); err != nil { // "" → POSTGRES_* env vars
        log.Fatal(err)
    }
    defer db.Shutdown(ctx)

    if _, err := db.Exec(ctx, "INSERT INTO users (name) VALUES ($1)", "Alice"); err != nil {
        log.Fatal(err)
    }
    rows, err := db.ReadQuery(ctx, "SELECT id, name FROM users")
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
}
```

`Query` / `QueryRow` / `Exec` go through the write pool. `ReadQuery` / `ReadQueryRow` go through the read pool when one is configured via `ConnectReadWrite`; otherwise they share the single pool.

## Configuration

If `dsn == ""`, pgxkit builds one from `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_SSLMODE`. `pgxkit.GetDSN()` returns the same string for tools that need it externally.

```go
db.Connect(ctx, "",
    pgxkit.WithMaxConns(25),
    pgxkit.WithMinConns(5),
    pgxkit.WithMaxConnLifetime(time.Hour),
)

// Or with read/write split:
db.ConnectReadWrite(ctx, replicaDSN, primaryDSN)
```

## Hooks

`HookFunc` is `func(ctx, sql, args, tag pgconn.CommandTag, err error) error`. `tag` carries the real command tag for `AfterOperation` on Exec; it's the zero value everywhere else (including `AfterOperation` on Query — pgx doesn't fill the tag until rows are closed).

```go
db.Connect(ctx, "",
    pgxkit.WithBeforeOperation(func(ctx context.Context, sql string, _ []interface{}, _ pgconn.CommandTag, _ error) error {
        slog.InfoContext(ctx, "query", "sql", sql)
        return nil
    }),
    pgxkit.WithAfterOperation(func(ctx context.Context, _ string, _ []interface{}, tag pgconn.CommandTag, err error) error {
        if err != nil {
            metrics.QueryErrors.Inc()
        } else if tag.String() != "" { // Exec
            metrics.RowsAffected.Add(float64(tag.RowsAffected()))
        }
        return nil
    }),
)
```

Lifecycle hooks: `WithBeforeOperation`, `WithAfterOperation`, `WithBeforeTransaction`, `WithAfterTransaction`, `WithOnShutdown`. Connection-level hooks: `WithOnConnect`, `WithOnDisconnect`, `WithOnAcquire`, `WithOnRelease`.

## Retry

`RetryOperation` retries transient PostgreSQL errors (serialization failures, deadlocks, connection drops). Constraint violations and other deterministic errors pass through.

```go
err := pgxkit.RetryOperation(ctx, func(ctx context.Context) error {
    _, err := db.Exec(ctx, "UPDATE accounts SET balance = balance - $1 WHERE id = $2", amt, id)
    return err
}, pgxkit.WithMaxRetries(5))
```

The typed form `pgxkit.Retry[T]` returns a value. The retry budget is the context deadline — all attempts share it.

## Testing

```bash
make test-db-up    # containerized Postgres on a free port
make test
make test-db-down
```

`make` is the canonical entry point — see the project Makefile for `test-coverage`, `bench`, `lint`, etc.

```go
func TestThing(t *testing.T) {
    testDB := pgxkit.RequireDB(t) // skips when TEST_DATABASE_URL is unset
    // ... use testDB.DB ...
}
```

### Plan-regression

`AssertPlan` captures `EXPLAIN (FORMAT JSON, COSTS OFF)` per query and compares to `testdata/plans/<name>.json`. A plan-shape change (index-scan → seq-scan, hash-join → nested-loop, etc.) fails with a unified diff.

```go
db := testDB.EnableAssertPlan("TestUserSummary")
_, _ = db.Query(ctx, "SELECT ...")
db.AssertPlan(t, "TestUserSummary")
```

Refresh: `go test -overwrite-plan`.

### Golden transcript

`AssertGolden` records the ordered event stream (`BEGIN`, every `Query`/`Exec` with SQL + normalized args, `rows_affected` for Exec, `COMMIT`/`ROLLBACK`) and compares to `testdata/golden/<name>.json`. Catches an extra UPDATE, missing INSERT, different argument, `COMMIT` becoming `ROLLBACK`.

```go
golden := testDB.EnableGolden("TestCreateOrder")
// ... run code under test ...
golden.AssertGolden(t, "TestCreateOrder")
```

Refresh: `go test -overwrite-golden`.

## Type helpers

Thin converters between Go types and pgx's `pgtype.*`. See the [API reference](../../wiki/API-Reference#type-helpers) for the full list.

```go
pgxText := pgxkit.ToPgxText(&myString)
str     := pgxkit.FromPgxText(pgxText)
pgxUUID := pgxkit.ToPgxUUID(myUUID)
```

## Graceful shutdown

`db.Shutdown(ctx)` waits for active operations (including in-flight transactions started by `BeginTx`), runs `OnShutdown` hooks, then closes the pools.

```go
sig := make(chan os.Signal, 1)
signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
<-sig
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
_ = db.Shutdown(ctx)
```

## Code generation

`*pgxkit.DB` implements the same `Query`/`QueryRow`/`Exec` shape generators expect, and `db.WritePool()` / `db.ReadPool()` return `*pgxpool.Pool` for tools that want it directly.

```go
queries := sqlc.New(db.WritePool())
```

## Documentation

Full docs are in the [wiki](../../wiki):

- [Getting Started](../../wiki/Getting-Started) · [Examples](../../wiki/Examples)
- [API Reference](../../wiki/API-Reference)
- [Performance](../../wiki/Performance-Guide) · [Production](../../wiki/Production-Guide) · [Testing](../../wiki/Testing-Guide)
- [FAQ](../../wiki/FAQ) · [Contributing](../../wiki/Contributing)

## License

MIT — see [LICENSE](LICENSE).
