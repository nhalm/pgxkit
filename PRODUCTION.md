# Production Deployment Guide

This guide covers best practices for deploying pgxkit applications in production environments.

## Table of Contents

1. [Environment Configuration](#environment-configuration)
2. [Connection Pool Optimization](#connection-pool-optimization)
3. [Monitoring and Observability](#monitoring-and-observability)
4. [Security Best Practices](#security-best-practices)
5. [Scaling Strategies](#scaling-strategies)
6. [Health Checks and Readiness](#health-checks-and-readiness)
7. [Graceful Shutdown](#graceful-shutdown)
8. [Error Handling and Recovery](#error-handling-and-recovery)
9. [Performance Optimization](#performance-optimization)
10. [Deployment Patterns](#deployment-patterns)

## Environment Configuration

### Database Connection Settings

Configure these environment variables for production:

```bash
# Database Connection
POSTGRES_HOST=your-db-host.com
POSTGRES_PORT=5432
POSTGRES_USER=your-app-user
POSTGRES_PASSWORD=your-secure-password
POSTGRES_DB=your-production-db
POSTGRES_SSLMODE=require

# Connection Pool Settings
POSTGRES_MAX_CONNS=30
POSTGRES_MIN_CONNS=5
POSTGRES_MAX_CONN_LIFETIME=1h
POSTGRES_MAX_CONN_IDLE_TIME=30m
POSTGRES_HEALTH_CHECK_PERIOD=1m
```

### Application Configuration

```go
package main

import (
    "context"
    "log"
    "os"
    "strconv"
    "time"
    
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/nhalm/pgxkit"
)

func createProductionDB() *pgxkit.DB {
    config, err := pgxpool.ParseConfig(pgxkit.GetDSN())
    if err != nil {
        log.Fatal("Failed to parse database config:", err)
    }
    
    // Production pool settings
    config.MaxConns = getEnvInt("POSTGRES_MAX_CONNS", 30)
    config.MinConns = getEnvInt("POSTGRES_MIN_CONNS", 5)
    config.MaxConnLifetime = getEnvDuration("POSTGRES_MAX_CONN_LIFETIME", time.Hour)
    config.MaxConnIdleTime = getEnvDuration("POSTGRES_MAX_CONN_IDLE_TIME", 30*time.Minute)
    config.HealthCheckPeriod = getEnvDuration("POSTGRES_HEALTH_CHECK_PERIOD", time.Minute)
    
    pool, err := pgxpool.NewWithConfig(context.Background(), config)
    if err != nil {
        log.Fatal("Failed to create connection pool:", err)
    }
    
    db := pgxkit.NewDB(pool)
    
    // Add production hooks
    setupProductionHooks(db)
    
    return db
}

func getEnvInt(key string, defaultValue int) int32 {
    if val := os.Getenv(key); val != "" {
        if i, err := strconv.Atoi(val); err == nil {
            return int32(i)
        }
    }
    return int32(defaultValue)
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
    if val := os.Getenv(key); val != "" {
        if d, err := time.ParseDuration(val); err == nil {
            return d
        }
    }
    return defaultValue
}
```

## Connection Pool Optimization

### Pool Size Guidelines

```go
// Calculate optimal pool size based on your application
func calculatePoolSize() int32 {
    // Rule of thumb: 2-3 connections per CPU core
    cpuCores := runtime.NumCPU()
    baseConnections := cpuCores * 2
    
    // Adjust based on workload
    if isHighThroughput() {
        return int32(baseConnections * 2)
    }
    
    return int32(baseConnections)
}

// Production pool configuration
config.MaxConns = calculatePoolSize()
config.MinConns = config.MaxConns / 4  // 25% of max as minimum
config.MaxConnLifetime = time.Hour     // Rotate connections hourly
config.MaxConnIdleTime = 30 * time.Minute
```

### Read/Write Pool Splitting

```go
func createReadWriteDB() *pgxkit.DB {
    // Write pool (primary database)
    writeConfig, _ := pgxpool.ParseConfig(getWriteDSN())
    writeConfig.MaxConns = 20
    writePool, _ := pgxpool.NewWithConfig(context.Background(), writeConfig)
    
    // Read pool (read replicas)
    readConfig, _ := pgxpool.ParseConfig(getReadDSN())
    readConfig.MaxConns = 40  // More connections for read workload
    readPool, _ := pgxpool.NewWithConfig(context.Background(), readConfig)
    
    return pgxkit.NewReadWriteDB(readPool, writePool)
}
```

## Monitoring and Observability

### Structured Logging

```go
import (
    "log/slog"
    "os"
)

func setupProductionHooks(db *pgxkit.DB) {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
    
    // Query logging with performance metrics
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start := time.Now()
        ctx = context.WithValue(ctx, "query_start", start)
        
        logger.InfoContext(ctx, "query_start",
            slog.String("sql", sql),
            slog.Int("args_count", len(args)),
        )
        return nil
    })
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start, ok := ctx.Value("query_start").(time.Time)
        if !ok {
            return nil
        }
        
        duration := time.Since(start)
        
        if operationErr != nil {
            logger.ErrorContext(ctx, "query_error",
                slog.String("sql", sql),
                slog.Duration("duration", duration),
                slog.String("error", operationErr.Error()),
            )
        } else {
            logger.InfoContext(ctx, "query_success",
                slog.String("sql", sql),
                slog.Duration("duration", duration),
            )
        }
        return nil
    })
}
```

### Metrics Collection

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    queryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "pgxkit_query_duration_seconds",
            Help: "Database query duration in seconds",
            Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0},
        },
        []string{"operation", "status"},
    )
    
    connectionPoolStats = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "pgxkit_connection_pool",
            Help: "Connection pool statistics",
        },
        []string{"pool", "stat"},
    )
)

func setupMetricsHooks(db *pgxkit.DB) {
    // Query metrics
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start := time.Now()
        return context.WithValue(ctx, "metrics_start", start)
    })
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start, ok := ctx.Value("metrics_start").(time.Time)
        if !ok {
            return nil
        }
        
        duration := time.Since(start).Seconds()
        operation := getOperationType(sql)
        status := "success"
        if operationErr != nil {
            status = "error"
        }
        
        queryDuration.WithLabelValues(operation, status).Observe(duration)
        return nil
    })
    
    // Connection pool metrics
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        
        for range ticker.C {
            updatePoolMetrics(db)
        }
    }()
}

func updatePoolMetrics(db *pgxkit.DB) {
    if stats := db.Stats(); stats != nil {
        connectionPoolStats.WithLabelValues("write", "acquired").Set(float64(stats.AcquiredConns()))
        connectionPoolStats.WithLabelValues("write", "idle").Set(float64(stats.IdleConns()))
        connectionPoolStats.WithLabelValues("write", "total").Set(float64(stats.TotalConns()))
    }
    
    if stats := db.ReadStats(); stats != nil {
        connectionPoolStats.WithLabelValues("read", "acquired").Set(float64(stats.AcquiredConns()))
        connectionPoolStats.WithLabelValues("read", "idle").Set(float64(stats.IdleConns()))
        connectionPoolStats.WithLabelValues("read", "total").Set(float64(stats.TotalConns()))
    }
}
```

## Security Best Practices

### Connection Security

```go
// Use SSL/TLS in production
func getSecureConfig() *pgxpool.Config {
    config, _ := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
    
    // Enforce SSL
    config.ConnConfig.TLSConfig = &tls.Config{
        ServerName: config.ConnConfig.Host,
        MinVersion: tls.VersionTLS12,
    }
    
    return config
}
```

### Credential Management

```bash
# Use environment variables or secret management
export DATABASE_URL="postgres://user:$(cat /run/secrets/db_password)@host/db?sslmode=require"

# Or use a secrets manager
DATABASE_URL="postgres://user:password@host/db?sslmode=require"
```

### SQL Injection Prevention

```go
// Always use parameterized queries
func getUserByID(db *pgxkit.DB, userID int) (*User, error) {
    // GOOD: Parameterized query
    row := db.QueryRow(ctx, "SELECT id, name FROM users WHERE id = $1", userID)
    
    // BAD: String concatenation (vulnerable to SQL injection)
    // row := db.QueryRow(ctx, fmt.Sprintf("SELECT id, name FROM users WHERE id = %d", userID))
    
    var user User
    err := row.Scan(&user.ID, &user.Name)
    return &user, err
}
```

## Scaling Strategies

### Horizontal Scaling

```go
// Load balancer configuration for read replicas
func setupReadReplicas() *pgxkit.DB {
    readReplicas := []string{
        "postgres://user:pass@read-replica-1:5432/db",
        "postgres://user:pass@read-replica-2:5432/db",
        "postgres://user:pass@read-replica-3:5432/db",
    }
    
    // Create read pool with multiple replicas
    readConfig := createPoolConfig(readReplicas)
    readPool, _ := pgxpool.NewWithConfig(context.Background(), readConfig)
    
    // Single write pool
    writeConfig := createPoolConfig([]string{"postgres://user:pass@primary:5432/db"})
    writePool, _ := pgxpool.NewWithConfig(context.Background(), writeConfig)
    
    return pgxkit.NewReadWriteDB(readPool, writePool)
}
```

### Connection Pool Scaling

```go
// Auto-scaling based on load
func createAdaptivePool() *pgxkit.DB {
    config, _ := pgxpool.ParseConfig(pgxkit.GetDSN())
    
    // Dynamic pool sizing based on environment
    if isHighLoad() {
        config.MaxConns = 50
        config.MinConns = 10
    } else {
        config.MaxConns = 20
        config.MinConns = 5
    }
    
    pool, _ := pgxpool.NewWithConfig(context.Background(), config)
    return pgxkit.NewDB(pool)
}
```

## Health Checks and Readiness

### HTTP Health Check Endpoint

```go
import (
    "encoding/json"
    "net/http"
    "time"
)

func healthCheckHandler(db *pgxkit.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
        defer cancel()
        
        // Check database connectivity
        if err := db.HealthCheck(ctx); err != nil {
            w.WriteHeader(http.StatusServiceUnavailable)
            json.NewEncoder(w).Encode(map[string]interface{}{
                "status": "unhealthy",
                "error":  err.Error(),
                "timestamp": time.Now().UTC(),
            })
            return
        }
        
        // Get detailed statistics
        stats := db.Stats()
        readStats := db.ReadStats()
        
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "status": "healthy",
            "timestamp": time.Now().UTC(),
            "database": map[string]interface{}{
                "write_pool": poolStatsToMap(stats),
                "read_pool":  poolStatsToMap(readStats),
            },
        })
    }
}

func poolStatsToMap(stats *pgxpool.Stat) map[string]interface{} {
    if stats == nil {
        return nil
    }
    
    return map[string]interface{}{
        "acquired_conns":     stats.AcquiredConns(),
        "idle_conns":         stats.IdleConns(),
        "total_conns":        stats.TotalConns(),
        "max_conns":          stats.MaxConns(),
        "new_conns_count":    stats.NewConnsCount(),
        "max_lifetime_destroys": stats.MaxLifetimeDestroyCount(),
    }
}
```

### Kubernetes Probes

```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: app
    image: your-app:latest
    ports:
    - containerPort: 8080
    livenessProbe:
      httpGet:
        path: /health
        port: 8080
      initialDelaySeconds: 30
      periodSeconds: 10
      timeoutSeconds: 5
      failureThreshold: 3
    readinessProbe:
      httpGet:
        path: /ready
        port: 8080
      initialDelaySeconds: 5
      periodSeconds: 5
      timeoutSeconds: 3
      failureThreshold: 2
```

## Graceful Shutdown

### Signal Handling

```go
import (
    "os"
    "os/signal"
    "syscall"
)

func main() {
    db := createProductionDB()
    
    // Setup graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    
    // Start your application
    server := startHTTPServer(db)
    
    // Wait for shutdown signal
    <-sigChan
    log.Println("Shutting down gracefully...")
    
    // Shutdown with timeout
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // Shutdown HTTP server first
    if err := server.Shutdown(shutdownCtx); err != nil {
        log.Printf("HTTP server shutdown error: %v", err)
    }
    
    // Shutdown database connections
    if err := db.Shutdown(shutdownCtx); err != nil {
        log.Printf("Database shutdown error: %v", err)
    }
    
    log.Println("Shutdown complete")
}
```

## Error Handling and Recovery

### Circuit Breaker Pattern

```go
import "github.com/sony/gobreaker"

func setupCircuitBreaker(db *pgxkit.DB) {
    cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
        Name:        "database",
        MaxRequests: 3,
        Interval:    60 * time.Second,
        Timeout:     30 * time.Second,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures > 2
        },
    })
    
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        _, err := cb.Execute(func() (interface{}, error) {
            // The actual operation will be executed by pgxkit
            return nil, nil
        })
        return err
    })
}
```

### Retry with Backoff

```go
func performCriticalOperation(db *pgxkit.DB) error {
    config := &pgxkit.RetryConfig{
        MaxRetries: 5,
        BaseDelay:  100 * time.Millisecond,
        MaxDelay:   5 * time.Second,
        Multiplier: 2.0,
    }
    
    return pgxkit.RetryOperation(context.Background(), config, func(ctx context.Context) error {
        _, err := db.Exec(ctx, "INSERT INTO critical_data (value) VALUES ($1)", "important")
        return err
    })
}
```

## Performance Optimization

### Query Optimization

```go
// Use read pools for read-heavy operations
func getRecentUsers(db *pgxkit.DB) ([]User, error) {
    // Use read pool for better performance
    rows, err := db.ReadQuery(ctx, `
        SELECT id, name, email, created_at 
        FROM users 
        WHERE created_at > NOW() - INTERVAL '24 hours'
        ORDER BY created_at DESC
        LIMIT 100
    `)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var users []User
    for rows.Next() {
        var user User
        if err := rows.Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt); err != nil {
            return nil, err
        }
        users = append(users, user)
    }
    
    return users, nil
}
```

### Connection Pool Tuning

```go
// Monitor and adjust pool settings
func monitorPoolPerformance(db *pgxkit.DB) {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := db.Stats()
        if stats == nil {
            continue
        }
        
        // Alert if pool utilization is high
        utilization := float64(stats.AcquiredConns()) / float64(stats.MaxConns())
        if utilization > 0.8 {
            log.Printf("High pool utilization: %.2f%%", utilization*100)
            // Consider scaling up
        }
        
        // Alert if too many idle connections
        if stats.IdleConns() > stats.MaxConns()/2 {
            log.Printf("High idle connections: %d", stats.IdleConns())
            // Consider scaling down
        }
    }
}
```

## Deployment Patterns

### Blue-Green Deployment

```go
// Database migration strategy
func performMigration(db *pgxkit.DB) error {
    // Run migrations in a transaction
    tx, err := db.BeginTx(context.Background(), pgx.TxOptions{})
    if err != nil {
        return err
    }
    defer tx.Rollback(context.Background())
    
    // Apply schema changes
    if err := runMigrations(tx); err != nil {
        return err
    }
    
    // Validate changes
    if err := validateMigrations(tx); err != nil {
        return err
    }
    
    return tx.Commit(context.Background())
}
```

### Rolling Updates

```go
// Health check during rolling updates
func readinessCheck(db *pgxkit.DB) bool {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    
    // Check database connectivity
    if err := db.HealthCheck(ctx); err != nil {
        return false
    }
    
    // Check if migrations are complete
    if !areMigrationsComplete(db) {
        return false
    }
    
    return true
}
```

## Best Practices Summary

### DO's
- ✅ Use environment variables for configuration
- ✅ Implement comprehensive monitoring and logging
- ✅ Use connection pooling with appropriate sizing
- ✅ Implement graceful shutdown
- ✅ Use read/write splitting for better performance
- ✅ Add circuit breakers for resilience
- ✅ Use parameterized queries to prevent SQL injection
- ✅ Monitor connection pool statistics
- ✅ Implement proper health checks

### DON'Ts
- ❌ Don't use string concatenation for SQL queries
- ❌ Don't ignore connection pool limits
- ❌ Don't skip SSL/TLS in production
- ❌ Don't forget to implement timeouts
- ❌ Don't ignore database connection errors
- ❌ Don't use the same pool size for all environments
- ❌ Don't skip graceful shutdown handling
- ❌ Don't ignore monitoring and alerting

## Troubleshooting

### Common Issues

1. **Connection Pool Exhaustion**
   ```go
   // Increase pool size or reduce connection lifetime
   config.MaxConns = 50
   config.MaxConnLifetime = 30 * time.Minute
   ```

2. **Slow Queries**
   ```go
   // Add query timeout and monitoring
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   ```

3. **Memory Leaks**
   ```go
   // Always close rows and implement proper cleanup
   rows, err := db.Query(ctx, sql)
   if err != nil {
       return err
   }
   defer rows.Close() // Critical!
   ```

This production guide provides a comprehensive foundation for deploying pgxkit applications reliably and securely in production environments. 