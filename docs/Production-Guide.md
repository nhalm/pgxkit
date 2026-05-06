# Production Guide

**[← Back to Home](Home)**

What pgxkit handles for production: pool config, hooks, retries, `db.HealthCheck`, `db.Shutdown` with active-operation tracking. Container orchestration, deployment patterns, alerting, and circuit breakers belong in your infrastructure layer.

## Configuration

```go
db := pgxkit.NewDB()
err := db.Connect(ctx, pgxkit.GetDSN(),
    pgxkit.WithMaxConns(int32(runtime.NumCPU()*4)),
    pgxkit.WithMinConns(int32(runtime.NumCPU())),
    pgxkit.WithMaxConnLifetime(time.Hour),
    pgxkit.WithMaxConnIdleTime(15*time.Minute),
)
```

`pgxkit.GetDSN()` reads `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_SSLMODE`. For SSL, set `POSTGRES_SSLMODE=require` (or `verify-full` with `sslcert=`/`sslkey=`/`sslrootcert=` in the DSN).

## Read/write split

```go
_ = db.ConnectReadWrite(ctx,
    "postgres://user:pass@replica/db?sslmode=require&pool_max_conns=50",
    "postgres://user:pass@primary/db?sslmode=require&pool_max_conns=20",
)
```

`db.ReadStats()` reports the read pool separately from `db.Stats()`.

## Health and readiness

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

`db.HealthCheck` pings the write pool. For Kubernetes, point both the liveness and readiness probes at this — they're cheap.

## Graceful shutdown

`db.Shutdown(ctx)` waits for active operations (including in-flight transactions tracked by `BeginTx`), runs `OnShutdown` hooks, then closes the pools. Wire it after your HTTP server shuts down so new requests can't race in.

```go
sig := make(chan os.Signal, 1)
signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
<-sig

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

_ = httpServer.Shutdown(ctx)
_ = db.Shutdown(ctx)
```

## Logging and metrics

Hooks are the integration point for both. See [Examples → Hooks](Examples#hooks) for the full pattern. A minimal observability setup:

```go
db.Connect(ctx, "",
    pgxkit.WithAfterOperation(func(ctx context.Context, sql string, _ []interface{}, tag pgconn.CommandTag, err error) error {
        if err != nil {
            slog.ErrorContext(ctx, "query failed", "sql", sql, "err", err)
            metrics.QueryErrors.Inc()
            return nil
        }
        if tag.String() != "" {
            metrics.RowsAffected.Add(float64(tag.RowsAffected()))
        }
        return nil
    }),
    pgxkit.WithAfterTransaction(func(ctx context.Context, sql string, _ []interface{}, _ pgconn.CommandTag, err error) error {
        switch sql {
        case pgxkit.TxCommit:   metrics.TxCommits.Inc()
        case pgxkit.TxRollback: metrics.TxRollbacks.Inc()
        }
        return nil
    }),
)
```

For pool-level metrics, scrape `db.Stats()` periodically (acquired/idle/max counts) and emit gauges.

## Errors and retries

`pgxkit.RetryOperation` and `Retry[T]` retry transient PostgreSQL errors — connection drops, serialization failures (`40001`), deadlocks (`40P01`). Constraint violations and other deterministic errors pass through unchanged so the caller can react.

```go
err := pgxkit.RetryOperation(ctx, func(ctx context.Context) error {
    _, err := db.Exec(ctx, "UPDATE accounts SET balance = balance - $1 WHERE id = $2", amount, id)
    return err
}, pgxkit.WithMaxRetries(5))
```

For circuit-breaker semantics or richer recovery policies, wrap pgxkit calls in your service layer — pgxkit doesn't ship a circuit breaker.

## Security

- Always set `POSTGRES_SSLMODE=require` (or stronger) in production.
- Pass credentials via env vars or a secret manager; don't bake them into the DSN literal.
- Limit the database user to the minimum privileges the app needs.
- Use `pool_max_conn_lifetime` so connections rotate periodically.

## Production checklist

- `WithMaxConns` / `WithMinConns` set to your sized values.
- `db.HealthCheck` wired to liveness + readiness probes.
- `db.Shutdown` wired to `SIGTERM`, run after the HTTP server.
- One `WithAfterOperation` hook recording at least success/error counts and durations.
- One `WithAfterTransaction` hook recording commit vs rollback.
- `AssertPlan` baselines for the queries that matter most (CI gate).
- A retry policy for the writes you can safely retry.

---

**[← Back to Home](Home)**
