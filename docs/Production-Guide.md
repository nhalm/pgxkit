# Production Deployment Guide

**[← Back to Home](Home)**

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
    "time"

    "github.com/nhalm/pgxkit"
)

func createProductionDB() *pgxkit.DB {
    db := pgxkit.NewDB()

    err := db.Connect(context.Background(), pgxkit.GetDSN(),
        pgxkit.WithMaxConns(30),
        pgxkit.WithMinConns(5),
        pgxkit.WithMaxConnLifetime(time.Hour),
        pgxkit.WithMaxConnIdleTime(30*time.Minute),
    )
    if err != nil {
        log.Fatal("Failed to connect to database:", err)
    }

    return db
}
```

### Read/Write Split Configuration

```go
func createProductionReadWriteDB() *pgxkit.DB {
    // Read replicas (multiple for load balancing)
    readDSN := fmt.Sprintf(
        "postgres://%s:%s@%s:%s/%s?sslmode=require&pool_max_conns=50&pool_min_conns=10",
        os.Getenv("POSTGRES_READ_USER"),
        os.Getenv("POSTGRES_READ_PASSWORD"),
        os.Getenv("POSTGRES_READ_HOST"),
        os.Getenv("POSTGRES_PORT"),
        os.Getenv("POSTGRES_DB"),
    )
    
    // Primary database (writes)
    writeDSN := fmt.Sprintf(
        "postgres://%s:%s@%s:%s/%s?sslmode=require&pool_max_conns=20&pool_min_conns=5",
        os.Getenv("POSTGRES_WRITE_USER"),
        os.Getenv("POSTGRES_WRITE_PASSWORD"),
        os.Getenv("POSTGRES_WRITE_HOST"),
        os.Getenv("POSTGRES_PORT"),
        os.Getenv("POSTGRES_DB"),
    )
    
    db := pgxkit.NewDB()
    err := db.ConnectReadWrite(context.Background(), readDSN, writeDSN)
    if err != nil {
        log.Fatal("Failed to connect to database:", err)
    }
    
    return db
}
```

## Connection Pool Optimization

### Production Pool Settings

```go
func createOptimizedDB(ctx context.Context) *pgxkit.DB {
    cpuCores := int32(runtime.NumCPU())

    db := pgxkit.NewDB()
    err := db.Connect(ctx, pgxkit.GetDSN(),
        pgxkit.WithMaxConns(cpuCores*4),
        pgxkit.WithMinConns(cpuCores),
        pgxkit.WithMaxConnLifetime(2*time.Hour),
        pgxkit.WithMaxConnIdleTime(15*time.Minute),
    )
    if err != nil {
        log.Fatal("Failed to connect:", err)
    }

    return db
}
```

### Dynamic Pool Scaling

```go
type ProductionPoolManager struct {
    db               *pgxkit.DB
    metrics          *prometheus.Registry
    alertManager     AlertManager
    scaleUpThreshold float64
    scaleDownThreshold float64
}

func (ppm *ProductionPoolManager) monitorAndScale() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        writeStats := ppm.db.Stats()
        readStats := ppm.db.ReadStats()
        
        ppm.evaluateScaling(writeStats, "write")
        if readStats != nil {
            ppm.evaluateScaling(readStats, "read")
        }
    }
}

func (ppm *ProductionPoolManager) evaluateScaling(stats *pgxpool.Stat, poolType string) {
    utilization := float64(stats.AcquiredConns()) / float64(stats.MaxConns())
    
    // Alert on high utilization
    if utilization > 0.9 {
        ppm.alertManager.SendAlert(fmt.Sprintf(
            "High %s pool utilization: %.2f%% (%d/%d connections)",
            poolType, utilization*100, stats.AcquiredConns(), stats.MaxConns()))
    }
    
    // Log pool statistics
    log.Printf("%s pool: utilization=%.2f%%, acquired=%d, idle=%d, total=%d, max=%d",
        poolType, utilization*100, stats.AcquiredConns(), stats.IdleConns(), 
        stats.TotalConns(), stats.MaxConns())
}
```

## Monitoring and Observability

### Production Logging

```go
import (
    "log/slog"
    "github.com/prometheus/client_golang/prometheus"
)

func setupProductionHooks(db *pgxkit.DB) {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
    
    // Metrics collection
    setupMetrics(db)
    
    // Structured logging
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start := time.Now()
        return context.WithValue(ctx, "start_time", start)
    })
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start, ok := ctx.Value("start_time").(time.Time)
        if !ok {
            return nil
        }
        
        duration := time.Since(start)
        operation := extractOperation(sql)
        
        // Log slow queries
        if duration > 100*time.Millisecond {
            logger.WarnContext(ctx, "slow query detected",
                slog.String("operation", operation),
                slog.Duration("duration", duration),
                slog.String("sql", sql),
            )
        }
        
        // Log errors
        if operationErr != nil {
            logger.ErrorContext(ctx, "database operation failed",
                slog.String("operation", operation),
                slog.Duration("duration", duration),
                slog.String("error", operationErr.Error()),
                slog.String("sql", sql),
            )
        }
        
        return nil
    })
}
```

### Prometheus Metrics

```go
var (
    dbConnections = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "pgxkit_connections",
            Help: "Number of database connections",
        },
        []string{"pool", "state"},
    )
    
    queryDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "pgxkit_query_duration_seconds",
            Help:    "Database query duration",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 20),
        },
        []string{"operation", "status"},
    )
    
    queryTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "pgxkit_queries_total",
            Help: "Total number of database queries",
        },
        []string{"operation", "status"},
    )
)

func setupMetrics(db *pgxkit.DB) {
    prometheus.MustRegister(dbConnections, queryDuration, queryTotal)
    
    // Pool metrics collection
    go func() {
        ticker := time.NewTicker(10 * time.Second)
        defer ticker.Stop()
        
        for range ticker.C {
            collectPoolMetrics(db)
        }
    }()
    
    // Query metrics hooks
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start := time.Now()
        return context.WithValue(ctx, "metrics_start", start)
    })
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start, ok := ctx.Value("metrics_start").(time.Time)
        if !ok {
            return nil
        }
        
        duration := time.Since(start)
        operation := extractOperation(sql)
        status := "success"
        if operationErr != nil {
            status = "error"
        }
        
        queryDuration.WithLabelValues(operation, status).Observe(duration.Seconds())
        queryTotal.WithLabelValues(operation, status).Inc()
        
        return nil
    })
}

func collectPoolMetrics(db *pgxkit.DB) {
    if stats := db.Stats(); stats != nil {
        dbConnections.WithLabelValues("write", "acquired").Set(float64(stats.AcquiredConns()))
        dbConnections.WithLabelValues("write", "idle").Set(float64(stats.IdleConns()))
        dbConnections.WithLabelValues("write", "total").Set(float64(stats.TotalConns()))
    }
    
    if readStats := db.ReadStats(); readStats != nil {
        dbConnections.WithLabelValues("read", "acquired").Set(float64(readStats.AcquiredConns()))
        dbConnections.WithLabelValues("read", "idle").Set(float64(readStats.IdleConns()))
        dbConnections.WithLabelValues("read", "total").Set(float64(readStats.TotalConns()))
    }
}
```

## Security Best Practices

### SSL/TLS Configuration

```go
func setupSecureConnection() *pgxkit.DB {
    // Always use SSL in production
    dsn := fmt.Sprintf(
        "postgres://%s:%s@%s:%s/%s?sslmode=require&sslcert=%s&sslkey=%s&sslrootcert=%s",
        os.Getenv("POSTGRES_USER"),
        os.Getenv("POSTGRES_PASSWORD"),
        os.Getenv("POSTGRES_HOST"),
        os.Getenv("POSTGRES_PORT"),
        os.Getenv("POSTGRES_DB"),
        os.Getenv("POSTGRES_SSL_CERT"),
        os.Getenv("POSTGRES_SSL_KEY"),
        os.Getenv("POSTGRES_SSL_ROOT_CERT"),
    )
    
    db := pgxkit.NewDB()
    err := db.Connect(context.Background(), dsn)
    if err != nil {
        log.Fatal("Failed to connect securely:", err)
    }
    
    return db
}
```

### Connection Security

```go
func validateConnection(db *pgxkit.DB) error {
    ctx := context.Background()
    
    // Verify SSL connection
    var sslInUse bool
    err := db.QueryRow(ctx, "SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid()").Scan(&sslInUse)
    if err != nil {
        return fmt.Errorf("failed to check SSL status: %w", err)
    }
    
    if !sslInUse {
        return fmt.Errorf("SSL connection required but not active")
    }
    
    // Verify user permissions
    var currentUser string
    err = db.QueryRow(ctx, "SELECT current_user").Scan(&currentUser)
    if err != nil {
        return fmt.Errorf("failed to get current user: %w", err)
    }
    
    log.Printf("Connected as user: %s with SSL enabled", currentUser)
    return nil
}
```

## Scaling Strategies

### Horizontal Scaling

```go
type DatabaseCluster struct {
    primary   *pgxkit.DB
    replicas  []*pgxkit.DB
    loadBalancer *ReadLoadBalancer
}

func NewDatabaseCluster(primaryDSN string, replicaDSNs []string) *DatabaseCluster {
    // Connect to primary
    primary := pgxkit.NewDB()
    err := primary.Connect(context.Background(), primaryDSN)
    if err != nil {
        log.Fatal("Failed to connect to primary:", err)
    }
    
    // Connect to replicas
    var replicas []*pgxkit.DB
    for _, dsn := range replicaDSNs {
        replica := pgxkit.NewDB()
        err := replica.Connect(context.Background(), dsn)
        if err != nil {
            log.Printf("Failed to connect to replica %s: %v", dsn, err)
            continue
        }
        replicas = append(replicas, replica)
    }
    
    return &DatabaseCluster{
        primary:      primary,
        replicas:     replicas,
        loadBalancer: NewReadLoadBalancer(replicas),
    }
}

func (dc *DatabaseCluster) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
    if isReadQuery(sql) {
        return dc.loadBalancer.Query(ctx, sql, args...)
    }
    return dc.primary.Query(ctx, sql, args...)
}
```

### Connection Pooling at Scale

```go
func setupEnterpiseConnectionPool() *pgxkit.DB {
    // Large-scale production settings
    dsn := fmt.Sprintf("%s?"+
        "pool_max_conns=100&"+
        "pool_min_conns=20&"+
        "pool_max_conn_lifetime=4h&"+
        "pool_max_conn_idle_time=30m&"+
        "pool_health_check_period=30s&"+
        "pool_max_conn_lifetime_jitter=10m",
        pgxkit.GetDSN())
    
    db := pgxkit.NewDB()
    err := db.Connect(context.Background(), dsn)
    if err != nil {
        log.Fatal("Failed to create enterprise pool:", err)
    }
    
    return db
}
```

## Health Checks and Readiness

### Kubernetes Health Check

```go
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
        
        // Check pool statistics
        stats := db.Stats()
        utilization := float64(stats.AcquiredConns()) / float64(stats.MaxConns())
        
        health := map[string]interface{}{
            "status": "healthy",
            "timestamp": time.Now().UTC(),
            "database": map[string]interface{}{
                "connected": true,
                "pool_utilization": fmt.Sprintf("%.2f%%", utilization*100),
                "active_connections": stats.AcquiredConns(),
                "max_connections": stats.MaxConns(),
            },
        }
        
        // Warning if utilization is high
        if utilization > 0.8 {
            health["warnings"] = []string{
                fmt.Sprintf("High connection pool utilization: %.2f%%", utilization*100),
            }
        }
        
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(health)
    }
}

func readinessHandler(db *pgxkit.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
        defer cancel()
        
        // Simple connectivity test
        var result int
        err := db.QueryRow(ctx, "SELECT 1").Scan(&result)
        if err != nil {
            w.WriteHeader(http.StatusServiceUnavailable)
            return
        }
        
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("ready"))
    }
}
```

## Graceful Shutdown

### Production Shutdown Sequence

```go
func gracefulShutdown(db *pgxkit.DB, server *http.Server) {
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    
    <-c
    log.Println("Received shutdown signal")
    
    // Create shutdown context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // Shutdown HTTP server first
    log.Println("Shutting down HTTP server...")
    if err := server.Shutdown(ctx); err != nil {
        log.Printf("HTTP server shutdown error: %v", err)
    }
    
    // Allow ongoing database operations to complete
    log.Println("Waiting for database operations to complete...")
    time.Sleep(5 * time.Second)
    
    // Close database connections
    log.Println("Closing database connections...")
    if err := db.Shutdown(ctx); err != nil {
        log.Printf("Database shutdown error: %v", err)
    }
    
    log.Println("Shutdown complete")
}
```

### Circuit Breaker Pattern

```go
type CircuitBreaker struct {
    db              *pgxkit.DB
    failureCount    int64
    lastFailureTime time.Time
    state           CircuitState
    mutex           sync.RWMutex
}

type CircuitState int

const (
    StateClosed CircuitState = iota
    StateOpen
    StateHalfOpen
)

func (cb *CircuitBreaker) Execute(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
    cb.mutex.RLock()
    state := cb.state
    cb.mutex.RUnlock()
    
    switch state {
    case StateOpen:
        if time.Since(cb.lastFailureTime) > 60*time.Second {
            cb.mutex.Lock()
            cb.state = StateHalfOpen
            cb.mutex.Unlock()
        } else {
            return nil, fmt.Errorf("circuit breaker is open")
        }
    case StateHalfOpen:
        // Allow limited requests through
    case StateClosed:
        // Normal operation
    }
    
    rows, err := cb.db.Query(ctx, query, args...)
    if err != nil {
        cb.recordFailure()
        return nil, err
    }
    
    cb.recordSuccess()
    return rows, nil
}

func (cb *CircuitBreaker) recordFailure() {
    cb.mutex.Lock()
    defer cb.mutex.Unlock()
    
    cb.failureCount++
    cb.lastFailureTime = time.Now()
    
    if cb.failureCount >= 5 {
        cb.state = StateOpen
        log.Println("Circuit breaker opened due to failures")
    }
}

func (cb *CircuitBreaker) recordSuccess() {
    cb.mutex.Lock()
    defer cb.mutex.Unlock()
    
    cb.failureCount = 0
    cb.state = StateClosed
}
```

## Error Handling and Recovery

### Production Error Handling

```go
func setupProductionErrorHandling(db *pgxkit.DB) {
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        if operationErr == nil {
            return nil
        }
        
        // Categorize errors
        switch {
        case errors.Is(operationErr, pgx.ErrNoRows):
            // Not found - don't log as error
            return nil
            
        case errors.Is(operationErr, context.DeadlineExceeded):
            // Timeout - critical for monitoring
            log.Printf("TIMEOUT: Query exceeded deadline: %s", sql)
            alertManager.SendAlert("Database query timeout", map[string]string{
                "query": sql,
                "error": operationErr.Error(),
            })
            
        case isPgConnectionError(operationErr):
            // Connection issues - requires immediate attention
            log.Printf("CONNECTION ERROR: %v", operationErr)
            alertManager.SendAlert("Database connection error", map[string]string{
                "error": operationErr.Error(),
            })
            
        default:
            // Other database errors
            log.Printf("DATABASE ERROR: %s - %v", sql, operationErr)
        }
        
        return nil
    })
}

func isPgConnectionError(err error) bool {
    if err == nil {
        return false
    }
    
    errStr := err.Error()
    return strings.Contains(errStr, "connection refused") ||
           strings.Contains(errStr, "connection reset") ||
           strings.Contains(errStr, "connection timed out")
}
```

## Performance Optimization

### Production Query Optimization

```go
func setupQueryOptimization(db *pgxkit.DB) {
    slowQueryThreshold := 500 * time.Millisecond
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start, ok := ctx.Value("start_time").(time.Time)
        if !ok {
            return nil
        }
        
        duration := time.Since(start)
        
        // Log slow queries for optimization
        if duration > slowQueryThreshold {
            log.Printf("SLOW QUERY [%v]: %s", duration, sql)
            
            // Capture query plan for analysis
            go captureQueryPlan(db, sql, args...)
        }
        
        return nil
    })
}

func captureQueryPlan(db *pgxkit.DB, sql string, args ...interface{}) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    explainSQL := "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) " + sql
    row := db.QueryRow(ctx, explainSQL, args...)
    
    var planJSON string
    if err := row.Scan(&planJSON); err != nil {
        log.Printf("Failed to capture query plan: %v", err)
        return
    }
    
    // Store or send plan for analysis
    log.Printf("Query plan captured for: %s", sql)
    // Could send to monitoring system, store in database, etc.
}
```

## Deployment Patterns

### Blue-Green Deployment

```go
func validateDeployment(db *pgxkit.DB) error {
    ctx := context.Background()
    
    // Run deployment validation queries
    validationQueries := []string{
        "SELECT 1", // Basic connectivity
        "SELECT COUNT(*) FROM pg_tables WHERE schemaname = 'public'", // Schema check
        "SELECT version()", // PostgreSQL version
    }
    
    for _, query := range validationQueries {
        var result interface{}
        if err := db.QueryRow(ctx, query).Scan(&result); err != nil {
            return fmt.Errorf("validation query failed: %s - %w", query, err)
        }
        log.Printf("Validation passed: %s -> %v", query, result)
    }
    
    // Test connection pool
    stats := db.Stats()
    if stats.TotalConns() == 0 {
        return fmt.Errorf("no active connections in pool")
    }
    
    log.Printf("Deployment validation successful - %d connections active", stats.TotalConns())
    return nil
}
```

### Rolling Updates

```go
func rollingUpdateHealthCheck(db *pgxkit.DB) error {
    // Extended health check for rolling updates
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    // Check all critical tables are accessible
    criticalTables := []string{"users", "orders", "products"}
    
    for _, table := range criticalTables {
        var count int64
        query := fmt.Sprintf("SELECT COUNT(*) FROM %s LIMIT 1", table)
        if err := db.QueryRow(ctx, query).Scan(&count); err != nil {
            return fmt.Errorf("critical table %s not accessible: %w", table, err)
        }
    }
    
    // Verify read/write capabilities
    testID := fmt.Sprintf("health_check_%d", time.Now().Unix())
    
    // Test write
    _, err := db.Exec(ctx, "INSERT INTO health_checks (id, timestamp) VALUES ($1, $2) ON CONFLICT (id) DO UPDATE SET timestamp = $2", testID, time.Now())
    if err != nil {
        return fmt.Errorf("write test failed: %w", err)
    }
    
    // Test read
    var timestamp time.Time
    err = db.QueryRow(ctx, "SELECT timestamp FROM health_checks WHERE id = $1", testID).Scan(&timestamp)
    if err != nil {
        return fmt.Errorf("read test failed: %w", err)
    }
    
    // Cleanup
    _, _ = db.Exec(ctx, "DELETE FROM health_checks WHERE id = $1", testID)
    
    return nil
}
```

## Best Practices Summary

### Configuration Checklist

- [ ] SSL/TLS enabled and properly configured
- [ ] Connection pool sized appropriately for load
- [ ] Environment variables secured and validated
- [ ] Monitoring and alerting configured
- [ ] Health checks implemented
- [ ] Graceful shutdown handling
- [ ] Error handling and logging
- [ ] Performance monitoring enabled
- [ ] Security best practices followed
- [ ] Backup and recovery procedures tested

### Production Monitoring

- [ ] Connection pool utilization
- [ ] Query performance metrics
- [ ] Error rates and patterns
- [ ] SSL connection status
- [ ] Database replication lag (if applicable)
- [ ] Resource usage (CPU, memory, I/O)
- [ ] Alert thresholds configured
- [ ] Log aggregation and analysis
- [ ] Performance baseline established
- [ ] Capacity planning metrics

## See Also

- **[Getting Started](Getting-Started)** - Basic setup and configuration
- **[Performance Guide](Performance-Guide)** - Detailed performance optimization
- **[Examples](Examples)** - Practical code examples
- **[Testing Guide](Testing-Guide)** - Testing strategies
- **[API Reference](API-Reference)** - Complete API documentation

---

**[← Back to Home](Home)**

*Following these production deployment practices will help ensure your pgxkit applications run reliably and efficiently in production environments.*

*Last updated: December 2024* 