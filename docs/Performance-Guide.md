# Performance Guide

**[← Back to Home](Home)**

What pgxkit gives you for performance: pool sizing knobs, `db.Stats()` for monitoring, hooks for timing, and `AssertPlan` to catch query-plan regressions in CI. Caching, indexing strategy, and SQL tuning are application concerns — pgxkit doesn't ship those.

## Pool sizing

Start with `runtime.NumCPU() * 2` for CPU-bound, `* 4` for I/O-bound, then tune from observed `db.Stats()` under load.

```go
dsn := fmt.Sprintf(
    "%s?pool_max_conns=%d&pool_min_conns=%d&pool_max_conn_lifetime=1h&pool_max_conn_idle_time=30m",
    pgxkit.GetDSN(), 25, 5,
)
db := pgxkit.NewDB()
_ = db.Connect(ctx, dsn)
```

Note: pgxpool sizes are fixed at connect time. There's no in-place "scale up" — to change pool size, reconnect.

## Monitoring

`db.Stats()` (and `db.ReadStats()` for the read pool) returns a `*pgxpool.Stat`:

```go
stats := db.Stats()
slog.Info("pool",
    "acquired", stats.AcquiredConns(),
    "idle",     stats.IdleConns(),
    "max",      stats.MaxConns(),
)
```

Key signals:

- `AcquiredConns / MaxConns > 0.8` for sustained periods — increase pool size or fix connection leaks (forgotten `rows.Close()`).
- `IdleConns / TotalConns > 0.7` consistently — pool is oversized.
- Rising `NewConnsCount` — connections are being recycled too aggressively (raise `pool_max_conn_lifetime`).

## Per-query timing via hooks

Stash the start time on the context in `BeforeOperation`, read it back in `AfterOperation`:

```go
type ctxKey struct{}
db.Connect(ctx, "",
    pgxkit.WithBeforeOperation(func(ctx context.Context, _ string, _ []interface{}, _ pgconn.CommandTag, _ error) error {
        // store in ctx via your own mechanism, or use a request-scoped sync.Map keyed by goroutine
        return nil
    }),
    pgxkit.WithAfterOperation(func(ctx context.Context, sql string, _ []interface{}, _ pgconn.CommandTag, err error) error {
        metrics.QueryDuration.Observe(time.Since(startFromCtx(ctx)).Seconds())
        if err != nil {
            metrics.QueryErrors.Inc()
        }
        return nil
    }),
)
```

The hook signature is in [API Reference](API-Reference#hookfunc).

## Read/write split

Routes reads to a replica, writes to the primary. Use it when reads dominate and you can tolerate replica lag.

```go
_ = db.ConnectReadWrite(ctx, readDSN, writeDSN)
rows, _ := db.ReadQuery(ctx, "SELECT ...")  // replica
_, _   = db.Exec(ctx, "INSERT ...")         // primary
```

`db.ReadStats()` reports the read pool separately.

## Catching plan regressions in CI

`AssertPlan` captures `EXPLAIN (FORMAT JSON, COSTS OFF)` per query and compares to a checked-in baseline. A plan flip — index-scan → seq-scan, hash-join → nested-loop, an extra sort, a different join order — fails the test with a unified diff.

```go
func TestUserSearch_Plan(t *testing.T) {
    testDB := pgxkit.RequireDB(t)
    db := testDB.EnableAssertPlan("TestUserSearch")

    rows, err := db.Query(ctx, `
        SELECT u.id, u.name, COUNT(o.id)
        FROM users u LEFT JOIN orders o ON u.id = o.user_id
        WHERE u.name ILIKE $1
        GROUP BY u.id, u.name
        ORDER BY 3 DESC
        LIMIT 50
    `, "%john%")
    require.NoError(t, err)
    rows.Close()

    db.AssertPlan(t, "TestUserSearch")
}
```

Baselines live at `testdata/plans/<name>.json`. Refresh after intentional schema or query changes: `go test -overwrite-plan`.

## Bulk operations

For thousands of rows, `tx.Tx().CopyFrom` beats batched INSERTs by an order of magnitude. For moderate sizes, `pgx.Batch` halves the round-trip cost.

```go
tx, _ := db.BeginTx(ctx, pgx.TxOptions{})
defer tx.Rollback(ctx)
_, err := tx.Tx().CopyFrom(ctx,
    pgx.Identifier{"users"}, []string{"name", "email"},
    pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
        return []any{rows[i].Name, rows[i].Email}, nil
    }),
)
```

## Streaming large result sets

Don't accumulate large queries into a slice. Iterate and process:

```go
rows, err := db.ReadQuery(ctx, "SELECT id, payload FROM events WHERE day = $1", day)
if err != nil { return err }
defer rows.Close()
for rows.Next() {
    var e Event
    if err := rows.Scan(&e.ID, &e.Payload); err != nil { return err }
    if err := process(&e); err != nil { return err }
}
return rows.Err()
```

## Performance checklist

- Always `defer rows.Close()`.
- Bound queries with `context.WithTimeout`.
- Project specific columns; avoid `SELECT *`.
- Add indexes for predicates and join keys; check with `EXPLAIN ANALYZE`.
- Add `AssertPlan` baselines for the queries that matter.

---

**[← Back to Home](Home)**
