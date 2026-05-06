# Examples

**[← Back to Home](Home)**

Practical patterns. For installation and a first query, see [Getting Started](Getting-Started).

## Contents

1. [Read/write split](#readwrite-split)
2. [Executor interface](#executor-interface)
3. [Hooks](#hooks)
4. [Retry](#retry)
5. [Health checks](#health-checks)
6. [Testing](#testing)
7. [Type helpers](#type-helpers)
8. [Code generation](#code-generation)

## Read/write split

```go
db := pgxkit.NewDB()
_ = db.ConnectReadWrite(ctx,
    "postgres://user:pass@replica/db",
    "postgres://user:pass@primary/db",
)
defer db.Shutdown(ctx)

// Reads on the replica:
rows, _ := db.ReadQuery(ctx, "SELECT * FROM users WHERE active = true")
// Writes on the primary:
_, _ = db.Exec(ctx, "UPDATE users SET last_login = NOW() WHERE id = $1", id)
```

## Executor interface

`*DB` and `*Tx` both implement `Executor`. Write repository functions against the interface and they work in or out of a transaction.

```go
func CreateUser(ctx context.Context, exec pgxkit.Executor, name, email string) (int64, error) {
    var id int64
    err := exec.QueryRow(ctx,
        "INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id",
        name, email).Scan(&id)
    return id, err
}

// Outside a transaction:
id, _ := CreateUser(ctx, db, "Alice", "alice@example.com")

// Inside a transaction:
tx, _ := db.BeginTx(ctx, pgx.TxOptions{})
defer tx.Rollback(ctx)
id, _ = CreateUser(ctx, tx, "Bob", "bob@example.com")
_ = tx.Commit(ctx)
```

## Hooks

`HookFunc` is `func(ctx, sql, args, tag pgconn.CommandTag, err error) error`. `tag` is meaningful for `AfterOperation` on Exec; zero everywhere else.

```go
db.Connect(ctx, "",
    // Log every query.
    pgxkit.WithBeforeOperation(func(ctx context.Context, sql string, args []interface{}, tag pgconn.CommandTag, err error) error {
        slog.InfoContext(ctx, "query", "sql", sql)
        return nil
    }),
    // Count outcomes; tag.RowsAffected() is real for Exec.
    pgxkit.WithAfterOperation(func(ctx context.Context, sql string, args []interface{}, tag pgconn.CommandTag, err error) error {
        if err != nil {
            metrics.QueryErrors.Inc()
        } else if tag.String() != "" {
            metrics.RowsAffected.Add(float64(tag.RowsAffected()))
        }
        return nil
    }),
    // Distinguish commit vs rollback via the `sql` parameter.
    pgxkit.WithAfterTransaction(func(ctx context.Context, sql string, args []interface{}, tag pgconn.CommandTag, err error) error {
        switch sql {
        case pgxkit.TxCommit:   metrics.TxCommits.Inc()
        case pgxkit.TxRollback: metrics.TxRollbacks.Inc()
        }
        return nil
    }),
)
```

A before-hook returning a non-nil error aborts the operation. After-hook errors don't change the operation's result but are reported.

## Retry

`RetryOperation` retries on transient PostgreSQL errors (serialization failures, deadlocks, connection drops). For a typed return, use `Retry[T]`.

```go
err := pgxkit.RetryOperation(ctx, func(ctx context.Context) error {
    tx, err := db.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    if _, err := tx.Exec(ctx, "UPDATE accounts SET balance = balance - $1 WHERE id = $2", amount, fromID); err != nil {
        return err
    }
    if _, err := tx.Exec(ctx, "UPDATE accounts SET balance = balance + $1 WHERE id = $2", amount, toID); err != nil {
        return err
    }
    return tx.Commit(ctx)
}, pgxkit.WithMaxRetries(5))
```

```go
user, err := pgxkit.Retry(ctx, func(ctx context.Context) (*User, error) {
    var u User
    err := db.QueryRow(ctx, "SELECT id, name FROM users WHERE id = $1", id).Scan(&u.ID, &u.Name)
    return &u, err
})
```

## Health checks

```go
http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
    defer cancel()
    if err := db.HealthCheck(ctx); err != nil {
        http.Error(w, err.Error(), http.StatusServiceUnavailable)
        return
    }
    w.WriteHeader(http.StatusOK)
})
```

## Testing

`RequireDB` connects via `TEST_DATABASE_URL` and skips when it isn't set.

### Plan-regression

`AssertPlan` captures `EXPLAIN (FORMAT JSON, COSTS OFF)` per query and compares to `testdata/plans/<name>.json`. Catches plan-shape changes; doesn't look at data.

```go
func TestUserSummary_Plan(t *testing.T) {
    testDB := pgxkit.RequireDB(t)
    db := testDB.EnableAssertPlan("TestUserSummary")

    rows, err := db.Query(ctx, `
        SELECT u.id, u.name, COUNT(o.id)
        FROM users u LEFT JOIN orders o ON u.id = o.user_id
        GROUP BY u.id, u.name
    `)
    require.NoError(t, err)
    rows.Close()

    db.AssertPlan(t, "TestUserSummary")
}
```

Refresh after intentional changes: `go test -overwrite-plan`.

### Golden transcript

`AssertGolden` records the ordered event stream — `BEGIN`, every `Query`/`Exec` (with SQL + normalized args, plus `rows_affected` for Exec), `COMMIT`/`ROLLBACK` — and compares to `testdata/golden/<name>.json`.

```go
func TestCreateOrder(t *testing.T) {
    testDB := pgxkit.RequireDB(t)
    golden := testDB.EnableGolden("TestCreateOrder")

    tx, err := golden.BeginTx(ctx, pgx.TxOptions{})
    require.NoError(t, err)
    var orderID int
    require.NoError(t, tx.QueryRow(ctx,
        "INSERT INTO orders (total) VALUES ($1) RETURNING id", 100).Scan(&orderID))
    _, err = tx.Exec(ctx,
        "INSERT INTO order_items (order_id, sku, qty) VALUES ($1, $2, $3)",
        orderID, "SKU-1", 2)
    require.NoError(t, err)
    require.NoError(t, tx.Commit(ctx))

    golden.AssertGolden(t, "TestCreateOrder")
}
```

Refresh: `go test -overwrite-golden`. Custom normalizers via `pgxkit.WithGoldenNormalizer(...)`.

## Type helpers

`pgxkit/types.go` exposes thin converters between Go types and pgx's `pgtype.*` for clean repository code:

```go
func (r *Repo) GetUser(ctx context.Context, id int64) (*User, error) {
    var u User
    err := r.db.ReadQueryRow(ctx,
        "SELECT id, name, email FROM users WHERE id = $1", id,
    ).Scan(&u.ID, &u.Name, &u.Email)
    return &u, err
}
```

See [API Reference → Type Helpers](API-Reference#type-helpers) for the full list.

## Code generation

`*pgxkit.DB` implements the same `Query`/`QueryRow`/`Exec` shape generators expect, and `db.WritePool()` / `db.ReadPool()` return `*pgxpool.Pool` for tools that want the pool directly.

```go
// sqlc, skimatik, etc.
queries := sqlc.New(db.WritePool())
readQueries := sqlc.New(db.ReadPool())
```

---

**[← Back to Home](Home)**
