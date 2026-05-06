# FAQ

**[← Back to Home](Home)**

## General

### Why pgxkit instead of pgx directly?

pgxkit is built on top of pgx and adds: read/write pool split, an extensible hook system, retry helpers, plan-regression testing, golden-transcript testing, graceful shutdown, and small type helpers. Use plain pgx for one-off scripts; reach for pgxkit when you want any of the above.

### Does pgxkit work with sqlc / skimatik / other code generators?

Yes — `*pgxkit.DB` satisfies the same `Query`/`QueryRow`/`Exec` shape, and you can also pass `db.WritePool()` / `db.ReadPool()` to anything expecting `*pgxpool.Pool`.

```go
queries := sqlc.New(db.WritePool())
```

## Setup

### How do I size the connection pool?

A reasonable starting point is `runtime.NumCPU() * 2` for CPU-bound, `* 4` for I/O-bound, and tune from `db.Stats()` under load. Set via DSN:

```go
dsn := fmt.Sprintf("%s?pool_max_conns=20&pool_min_conns=5", pgxkit.GetDSN())
```

### When should I use read/write splitting?

When you have read replicas and your workload is read-heavy. Use `ConnectReadWrite` and call `ReadQuery` / `ReadQueryRow` for reads that can tolerate replica lag. Skip it for write-heavy workloads or when you need read-your-own-writes.

### What environment variables does pgxkit read?

When `dsn == ""`, pgxkit builds a DSN from `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_SSLMODE`. `pgxkit.GetDSN()` returns the same string for tools that need it externally.

## Performance

### How do I monitor performance?

Two pgxkit features cover this:

- `db.Stats()` (and `db.ReadStats()`) returns a `*pgxpool.Stat` with pool counters — feed it to your metrics layer.
- `WithAfterOperation` fires for every `Query`/`Exec`. Track timing in the hook (start a timer in `WithBeforeOperation`, stop it here):

```go
pgxkit.WithAfterOperation(func(ctx context.Context, sql string, args []interface{}, tag pgconn.CommandTag, err error) error {
    metrics.RecordQuery(sql, time.Since(timeFromCtx(ctx)), err)
    return nil
})
```

### How do I do bulk inserts?

`pgx.Batch` for moderate sizes; `tx.CopyFrom` for thousands of rows. Both work through pgxkit's `Tx`:

```go
tx, _ := db.BeginTx(ctx, pgx.TxOptions{})
defer tx.Rollback(ctx)
_, err := tx.Tx().CopyFrom(ctx,
    pgx.Identifier{"users"},
    []string{"name", "email"},
    pgx.CopyFromSlice(len(users), func(i int) ([]any, error) {
        return []any{users[i].Name, users[i].Email}, nil
    }),
)
```

## Testing

### How do I set up a test database?

```go
func TestThing(t *testing.T) {
    testDB := pgxkit.RequireDB(t) // skips when TEST_DATABASE_URL is unset
    // ... use testDB.DB ...
}
```

`RequireDB` connects via `TEST_DATABASE_URL` and returns a `*TestDB` (which embeds `*DB`).

### When should I use Plan-Regression vs Golden testing?

They answer different questions:

- **`AssertPlan`** — asserts the structural query plan (`EXPLAIN (FORMAT JSON, COSTS OFF)`). Catches a seq-scan replacing an index-scan, a different join order, an extra sort, etc. Doesn't look at data.
- **`AssertGolden`** — asserts the ordered sequence of database events (`BEGIN`, every `Query`/`Exec` with SQL + normalized args, `rows_affected` for Exec, `COMMIT`/`ROLLBACK`). Catches an extra UPDATE, missing INSERT, different argument, or `COMMIT` that became `ROLLBACK`.

Pick one per scenario — `EnableAssertPlan` and `EnableGolden` each return a fresh `*DB` and don't compose.

```go
golden := testDB.EnableGolden("TestCreateOrder")
// ... run code under test ...
golden.AssertGolden(t, "TestCreateOrder")
```

Refresh either baseline with `go test -overwrite-plan` or `-overwrite-golden`.

### How do I test for `pgx.ErrNoRows` / constraint violations?

```go
_, err := repo.GetUser(ctx, 999)
if !errors.Is(err, pgx.ErrNoRows) { /* fail */ }
```

For constraint violations, use `errors.As` with `*pgconn.PgError` and check `.Code` (e.g. `"23505"` for unique).

## Production

### How do I implement graceful shutdown?

`db.Shutdown(ctx)` waits for active operations and runs `OnShutdown` hooks. Wire it to `SIGTERM`:

```go
sig := make(chan os.Signal, 1)
signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
<-sig
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
_ = db.Shutdown(ctx)
```

### How do I handle migrations?

pgxkit doesn't ship a migration tool. Use `golang-migrate`, `goose`, or whatever you already use. `pgxkit.GetDSN()` gives you the connection string.

### How do I expose a health endpoint?

`db.HealthCheck(ctx)` pings the write pool. Wrap it in your HTTP handler — return `503` on error, `200` otherwise.

## Troubleshooting

### Connection pool exhausted

Check `db.Stats().AcquiredConns()` vs `MaxConns()`. The two common causes:

- Forgotten `rows.Close()` — every `Query` must be closed even on error.
- Long-running queries — add `context.WithTimeout` to bound them.

If you really need more capacity and the database can serve it, raise `pool_max_conns` in the DSN.

### My queries are slow

Two complementary moves:

1. **Catch regressions in CI** with `EnableAssertPlan` / `AssertPlan`. Baselines live in `testdata/plans/<test>.json`; a plan flip (index-scan → seq-scan, hash-join → nested-loop) fails the test with a unified diff.
2. **Investigate ad-hoc** with `EXPLAIN (ANALYZE, BUFFERS) <your query>` against the database directly — pgxkit doesn't wrap this; `psql` is the right tool.

### SSL connection issues

Set `POSTGRES_SSLMODE` (`disable` / `prefer` / `require` / `verify-full`). For client certs, append `sslcert=`, `sslkey=`, `sslrootcert=` to the DSN.

### High memory usage

Almost always one of: large unbounded result sets (add `LIMIT` or stream with `for rows.Next()`), connection leaks (forgotten `rows.Close()`), or pool sized too high. `db.Stats()` and PostgreSQL's `pg_stat_activity` will tell you which.

## Migrating from `database/sql`

```go
// Before
db, _ := sql.Open("postgres", dsn)
rows, _ := db.Query("SELECT * FROM users")

// After — pgxkit methods take a context
db := pgxkit.NewDB()
_ = db.Connect(ctx, dsn)
rows, _ := db.Query(ctx, "SELECT * FROM users")
```

pgx has stricter type handling than `database/sql`; expect to scan into `int64` instead of `int` and use `pgtype` for nullable columns. The [pgx wiki](https://github.com/jackc/pgx/wiki) covers the differences.

---

**[← Back to Home](Home)**
