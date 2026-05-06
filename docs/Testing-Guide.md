# Testing Guide

**[← Back to Home](Home)**

pgxkit ships three things that matter for tests: `RequireDB` (test-database setup with skip), `EnableAssertPlan` / `AssertPlan` (catch query-plan regressions), and `EnableGolden` / `AssertGolden` (catch behavioral changes — extra/missing/reordered statements, different args, commit vs rollback). Everything else — fixture loading, factory patterns, table-driven structure, mocking — is plain Go testing and isn't pgxkit's concern.

## Setup

```bash
export TEST_DATABASE_URL=postgres://user:pass@localhost:5432/test?sslmode=disable
```

`RequireDB` skips the test if `TEST_DATABASE_URL` is unset, otherwise returns a connected `*pgxkit.TestDB`:

```go
func TestThing(t *testing.T) {
    testDB := pgxkit.RequireDB(t)
    // ... use testDB.DB ...
}
```

Don't build your own `TestSuite` wrapper or `setupTestDB` helper — `RequireDB` plus `t.Cleanup` covers it. If you need shared setup across tests, do it in `TestMain`.

## Plan-regression testing

`EnableAssertPlan` returns a fresh `*DB` that captures `EXPLAIN (FORMAT JSON, COSTS OFF)` for every SELECT/INSERT/UPDATE/DELETE/WITH it sees. `AssertPlan` writes the baseline on first run, diffs against it on later runs.

```go
func TestUserSummary_Plan(t *testing.T) {
    testDB := pgxkit.RequireDB(t)
    db := testDB.EnableAssertPlan("TestUserSummary")

    rows, err := db.Query(ctx, `
        SELECT u.id, u.name, COUNT(o.id), COALESCE(SUM(o.total), 0)
        FROM users u LEFT JOIN orders o ON u.id = o.user_id
        WHERE u.active = true
        GROUP BY u.id, u.name
        ORDER BY 4 DESC
        LIMIT 10
    `)
    require.NoError(t, err)
    rows.Close()

    db.AssertPlan(t, "TestUserSummary")
}
```

Baselines live at `testdata/plans/<name>.json`. A plan-shape change — index-scan replaced by seq-scan, hash-join replaced by nested-loop, an extra sort node, a different join order — fails the test with a unified diff. Plan capture uses `EXPLAIN` without `ANALYZE`, so the underlying statement isn't executed and there are no side effects to clean up.

Refresh after intentional changes: `go test -overwrite-plan`.

`AssertPlan` does not compare result rows or measure execution time. Assert those in the test body if you need to.

## Golden transcript testing

`EnableGolden` returns a `*DB` that records every database event for the scenario. `AssertGolden` writes the baseline on first run, diffs on later runs.

```go
func TestCreateOrder(t *testing.T) {
    testDB := pgxkit.RequireDB(t)
    golden := testDB.EnableGolden("TestCreateOrder")

    tx, err := golden.BeginTx(ctx, pgx.TxOptions{})
    require.NoError(t, err)
    var orderID int
    require.NoError(t, tx.QueryRow(ctx,
        "INSERT INTO orders (total) VALUES ($1) RETURNING id", 100,
    ).Scan(&orderID))
    _, err = tx.Exec(ctx,
        "INSERT INTO order_items (order_id, sku, qty) VALUES ($1, $2, $3)",
        orderID, "SKU-1", 2)
    require.NoError(t, err)
    require.NoError(t, tx.Commit(ctx))

    golden.AssertGolden(t, "TestCreateOrder")
}
```

### What the transcript captures

In order:

- `BEGIN`, `COMMIT`, `ROLLBACK` for transactions started via `BeginTx`.
- `QUERY` events for every `Query`, `QueryRow`, and `Exec` — including DDL and `SET`. No SQL-prefix filter.
- The SQL string and normalized args.
- `rows_affected` for `Exec` (taken from the `pgconn.CommandTag`). Not present for `Query` / `QueryRow`.

The transcript catches: an extra UPDATE, a missing INSERT, a different argument, a `COMMIT` that became a `ROLLBACK`, a reordering of statements. It does **not** capture result rows — for "did this row change" assertions, scan and assert in the test body or use `AssertPlan` for plan-shape stability.

### Normalization

To keep transcripts stable across runs:

- `time.Time` (and `*time.Time`) → `"<TIMESTAMP>"`.
- UUIDs (`uuid.UUID`, `[16]byte`, canonical UUID strings) → `"<UUID:1>"`, `"<UUID:2>"`, ... assigned in first-seen order so the same UUID gets the same placeholder wherever it appears.

Args have no column hint and pass through other types unchanged.

### Custom normalization

```go
golden := testDB.EnableGolden("TestCreateOrder",
    pgxkit.WithGoldenNormalizer(func(v any) (any, bool) {
        if s, ok := v.(string); ok && strings.HasPrefix(s, "ord_") {
            return "<ORDER>", true
        }
        return nil, false
    }),
)
```

Custom normalizers run before the defaults. Return `(replacement, true)` to take over normalization; `(nil, false)` to fall through.

### Refreshing the baseline

`go test -overwrite-golden` regenerates baselines for any test it runs. Use it after intentional behavior changes (new column in a `RETURNING` clause, deliberate statement reorder, etc.).

## Plan vs golden — which to use

They answer different questions and don't compose on a single `*DB`.

- **`AssertPlan`** — query-plan shape. Catches missing indexes, accidental seq-scans, join-order changes from migrations.
- **`AssertGolden`** — behavioral sequence. Catches an extra UPDATE the refactor introduced, an INSERT that's now missing, a `COMMIT` that became a `ROLLBACK`.

Pick per scenario.

## Transactions in tests

`db.BeginTx` returns a `*pgxkit.Tx` that fires the same hooks as the parent DB. The "defer Rollback() + explicit Commit()" pattern is safe — atomic finalization makes the rollback a no-op once Commit succeeds.

```go
tx, err := db.BeginTx(ctx, pgx.TxOptions{})
require.NoError(t, err)
defer tx.Rollback(ctx)
// ... operations ...
require.NoError(t, tx.Commit(ctx))
```

## Errors

Use `errors.Is` for `pgx.ErrNoRows`. For PostgreSQL constraint codes, use `errors.As` with `*pgconn.PgError`:

```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" {
    // unique violation
}
```

---

**[← Back to Home](Home)**
