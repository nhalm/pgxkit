# Performance Optimization Guide

**[← Back to Home](Home)**

This guide covers strategies and techniques for optimizing database performance when using pgxkit in your applications.

## Table of Contents

1. [Performance Fundamentals](#performance-fundamentals)
2. [Connection Pool Optimization](#connection-pool-optimization)
3. [Query Optimization](#query-optimization)
4. [Read/Write Splitting](#readwrite-splitting)
5. [Caching Strategies](#caching-strategies)
6. [Monitoring and Profiling](#monitoring-and-profiling)
7. [Golden Testing for Performance](#golden-testing-for-performance)
8. [Database Schema Optimization](#database-schema-optimization)
9. [Application-Level Optimizations](#application-level-optimizations)
10. [Troubleshooting Performance Issues](#troubleshooting-performance-issues)

## Performance Fundamentals

### Understanding Database Performance

Key metrics to monitor:
- **Query execution time** - How long individual queries take
- **Connection pool utilization** - How efficiently connections are used
- **Throughput** - Queries per second your application can handle
- **Latency** - Time from request to response
- **Resource usage** - CPU, memory, and I/O consumption

### Performance Principles

1. **Measure first** - Always profile before optimizing
2. **Optimize the bottleneck** - Focus on the slowest component
3. **Use appropriate data structures** - Choose the right tool for the job
4. **Minimize database round trips** - Batch operations when possible
5. **Cache frequently accessed data** - Reduce database load
6. **Use indexes effectively** - Speed up query execution
7. **Monitor continuously** - Performance can degrade over time

## Connection Pool Optimization

### Pool Size Configuration

```go
func optimizeConnectionPool() *pgxkit.DB {
    // Calculate optimal pool size based on workload
    cpuCores := runtime.NumCPU()
    
    // For CPU-bound workloads: 1-2 connections per core
    // For I/O-bound workloads: 2-4 connections per core
    maxConns := cpuCores * 3
    
    // Configure DSN with connection pool parameters
    dsn := fmt.Sprintf("%s?pool_max_conns=%d&pool_min_conns=%d&pool_max_conn_lifetime=1h&pool_max_conn_idle_time=30m&pool_health_check_period=1m",
        pgxkit.GetDSN(),
        maxConns,
        maxConns/4, // Keep 25% as minimum
    )
    
    db := pgxkit.NewDB()
    err := db.Connect(context.Background(), dsn)
    if err != nil {
        log.Fatal(err)
    }
    
    return db
}
```

### Pool Monitoring

```go
func monitorConnectionPool(db *pgxkit.DB) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := db.Stats()
        if stats == nil {
            continue
        }
        
        // Calculate metrics
        utilization := float64(stats.AcquiredConns()) / float64(stats.MaxConns())
        idleRatio := float64(stats.IdleConns()) / float64(stats.TotalConns())
        
        log.Printf("Pool Stats: Utilization=%.2f%%, Idle=%.2f%%, Total=%d, Max=%d",
            utilization*100, idleRatio*100, stats.TotalConns(), stats.MaxConns())
        
        // Alerts for performance issues
        if utilization > 0.8 {
            log.Printf("WARNING: High pool utilization (%.2f%%) - consider scaling", utilization*100)
        }
        
        if idleRatio > 0.7 {
            log.Printf("INFO: High idle connections (%.2f%%) - consider reducing pool size", idleRatio*100)
        }
        
        // Track connection creation rate
        if stats.NewConnsCount() > 0 {
            log.Printf("New connections created: %d", stats.NewConnsCount())
        }
    }
}
```

### Dynamic Pool Sizing

```go
type AdaptivePoolManager struct {
    db               *pgxkit.DB
    targetUtilization float64
    scaleUpThreshold  float64
    scaleDownThreshold float64
    minConns         int32
    maxConns         int32
}

func NewAdaptivePoolManager(db *pgxkit.DB) *AdaptivePoolManager {
    return &AdaptivePoolManager{
        db:               db,
        targetUtilization: 0.7,
        scaleUpThreshold:  0.8,
        scaleDownThreshold: 0.3,
        minConns:         5,
        maxConns:         100,
    }
}

func (apm *AdaptivePoolManager) adjustPoolSize() {
    stats := apm.db.Stats()
    if stats == nil {
        return
    }
    
    utilization := float64(stats.AcquiredConns()) / float64(stats.MaxConns())
    
    if utilization > apm.scaleUpThreshold && stats.MaxConns() < apm.maxConns {
        // Scale up
        newSize := int32(float64(stats.MaxConns()) * 1.2)
        if newSize > apm.maxConns {
            newSize = apm.maxConns
        }
        log.Printf("Scaling up pool size to %d (utilization: %.2f%%)", newSize, utilization*100)
        // Note: Actual implementation would require reconnecting with new DSN parameters
    } else if utilization < apm.scaleDownThreshold && stats.MaxConns() > apm.minConns {
        // Scale down
        newSize := int32(float64(stats.MaxConns()) * 0.8)
        if newSize < apm.minConns {
            newSize = apm.minConns
        }
        log.Printf("Scaling down pool size to %d (utilization: %.2f%%)", newSize, utilization*100)
        // Note: Actual implementation would require reconnecting with new DSN parameters
    }
}
```

## Query Optimization

### Efficient Query Patterns

```go
// Good: Use specific columns instead of SELECT *
func getActiveUsers(db *pgxkit.DB) ([]User, error) {
    rows, err := db.ReadQuery(context.Background(), `
        SELECT id, name, email, created_at
        FROM users 
        WHERE active = true 
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

// Good: Use EXISTS instead of COUNT for existence checks
func userExists(db *pgxkit.DB, email string) (bool, error) {
    var exists bool
    err := db.ReadQueryRow(context.Background(), `
        SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)
    `, email).Scan(&exists)
    
    return exists, err
}
```

### Batch Operations

```go
// Efficient bulk insert
func createUsersBatch(db *pgxkit.DB, users []User) error {
    ctx := context.Background()
    
    tx, err := db.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    
    // Use COPY for large batches (1000+ records)
    if len(users) > 1000 {
        return createUsersWithCopy(tx, users)
    }
    
    // Use batch insert for smaller sets
    batch := &pgx.Batch{}
    for _, user := range users {
        batch.Queue("INSERT INTO users (name, email) VALUES ($1, $2)", user.Name, user.Email)
    }
    
    results := tx.SendBatch(ctx, batch)
    defer results.Close()
    
    // Process results
    for i := 0; i < len(users); i++ {
        _, err := results.Exec()
        if err != nil {
            return fmt.Errorf("failed to insert user %d: %w", i, err)
        }
    }
    
    return tx.Commit(ctx)
}

// COPY for very large datasets
func createUsersWithCopy(tx pgx.Tx, users []User) error {
    ctx := context.Background()
    
    _, err := tx.CopyFrom(ctx,
        pgx.Identifier{"users"},
        []string{"name", "email"},
        pgx.CopyFromSlice(len(users), func(i int) ([]interface{}, error) {
            return []interface{}{users[i].Name, users[i].Email}, nil
        }),
    )
    
    return err
}
```

### Query Caching

```go
type QueryCache struct {
    cache map[string]CacheEntry
    mutex sync.RWMutex
    ttl   time.Duration
}

type CacheEntry struct {
    Data      interface{}
    ExpiresAt time.Time
}

func NewQueryCache(ttl time.Duration) *QueryCache {
    return &QueryCache{
        cache: make(map[string]CacheEntry),
        ttl:   ttl,
    }
}

func (qc *QueryCache) Get(key string) (interface{}, bool) {
    qc.mutex.RLock()
    defer qc.mutex.RUnlock()
    
    entry, exists := qc.cache[key]
    if !exists || time.Now().After(entry.ExpiresAt) {
        return nil, false
    }
    
    return entry.Data, true
}

// Cached query example
func getCachedUser(db *pgxkit.DB, cache *QueryCache, userID int) (*User, error) {
    cacheKey := fmt.Sprintf("user:%d", userID)
    
    // Check cache first
    if cached, found := cache.Get(cacheKey); found {
        return cached.(*User), nil
    }
    
    // Query database
    var user User
    err := db.ReadQueryRow(context.Background(),
        "SELECT id, name, email FROM users WHERE id = $1",
        userID).Scan(&user.ID, &user.Name, &user.Email)
    if err != nil {
        return nil, err
    }
    
    // Cache result
    cache.Set(cacheKey, &user)
    
    return &user, nil
}
```

## Read/Write Splitting

### Optimal Read/Write Configuration

```go
func createOptimizedReadWriteDB() *pgxkit.DB {
    // Write pool configuration (smaller, optimized for consistency)
    writeDSN := fmt.Sprintf("%s?pool_max_conns=20&pool_min_conns=5&pool_max_conn_lifetime=2h",
        getWriteDSN())
    
    // Read pool configuration (larger, optimized for throughput)
    readDSN := fmt.Sprintf("%s?pool_max_conns=50&pool_min_conns=10&pool_max_conn_lifetime=1h&pool_max_conn_idle_time=15m",
        getReadDSN())
    
    db := pgxkit.NewDB()
    err := db.ConnectReadWrite(context.Background(), readDSN, writeDSN)
    if err != nil {
        log.Fatal(err)
    }
    
    return db
}
```

### Smart Query Routing

```go
type QueryRouter struct {
    db *pgxkit.DB
}

func NewQueryRouter(db *pgxkit.DB) *QueryRouter {
    return &QueryRouter{db: db}
}

func (qr *QueryRouter) ExecuteQuery(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
    // Route based on query type
    if isReadQuery(sql) {
        return qr.db.ReadQuery(ctx, sql, args...)
    }
    return qr.db.Query(ctx, sql, args...)
}

func isReadQuery(sql string) bool {
    sql = strings.TrimSpace(strings.ToUpper(sql))
    return strings.HasPrefix(sql, "SELECT") ||
           strings.HasPrefix(sql, "WITH") ||
           strings.HasPrefix(sql, "SHOW") ||
           strings.HasPrefix(sql, "EXPLAIN")
}
```

## Caching Strategies

### Multi-Level Caching

```go
type CacheManager struct {
    l1Cache *sync.Map           // In-memory cache
    l2Cache *redis.Client       // Redis cache
    db      *pgxkit.DB
}

func NewCacheManager(db *pgxkit.DB, redisClient *redis.Client) *CacheManager {
    return &CacheManager{
        l1Cache: &sync.Map{},
        l2Cache: redisClient,
        db:      db,
    }
}

func (cm *CacheManager) GetUser(ctx context.Context, userID int) (*User, error) {
    cacheKey := fmt.Sprintf("user:%d", userID)
    
    // L1 Cache (in-memory)
    if cached, found := cm.l1Cache.Load(cacheKey); found {
        return cached.(*User), nil
    }
    
    // L2 Cache (Redis)
    if cm.l2Cache != nil {
        cached, err := cm.l2Cache.Get(ctx, cacheKey).Result()
        if err == nil {
            var user User
            if err := json.Unmarshal([]byte(cached), &user); err == nil {
                // Store in L1 cache
                cm.l1Cache.Store(cacheKey, &user)
                return &user, nil
            }
        }
    }
    
    // Database query
    var user User
    err := cm.db.ReadQueryRow(ctx,
        "SELECT id, name, email FROM users WHERE id = $1",
        userID).Scan(&user.ID, &user.Name, &user.Email)
    if err != nil {
        return nil, err
    }
    
    // Store in caches
    cm.l1Cache.Store(cacheKey, &user)
    if cm.l2Cache != nil {
        if data, err := json.Marshal(user); err == nil {
            cm.l2Cache.Set(ctx, cacheKey, data, 5*time.Minute)
        }
    }
    
    return &user, nil
}
```

### Cache Invalidation

```go
func (cm *CacheManager) InvalidateUser(ctx context.Context, userID int) {
    cacheKey := fmt.Sprintf("user:%d", userID)
    
    // Remove from L1 cache
    cm.l1Cache.Delete(cacheKey)
    
    // Remove from L2 cache
    if cm.l2Cache != nil {
        cm.l2Cache.Del(ctx, cacheKey)
    }
}

// Automatic cache invalidation with hooks
func setupCacheInvalidation(db *pgxkit.DB, cache *CacheManager) {
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        if operationErr != nil {
            return nil // Don't invalidate on error
        }
        
        // Parse SQL to determine what to invalidate
        if isUserModification(sql) {
            // Extract user ID from args and invalidate
            if len(args) > 0 {
                if userID, ok := args[0].(int); ok {
                    cache.InvalidateUser(ctx, userID)
                }
            }
        }
        
        return nil
    })
}
```

## Monitoring and Profiling

### Performance Metrics Collection

```go
type PerformanceMetrics struct {
    QueryDuration    *prometheus.HistogramVec
    QueryCounter     *prometheus.CounterVec
    PoolUtilization  *prometheus.GaugeVec
    CacheHitRate     *prometheus.GaugeVec
}

func NewPerformanceMetrics() *PerformanceMetrics {
    return &PerformanceMetrics{
        QueryDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name: "pgxkit_query_duration_seconds",
                Help: "Query execution duration",
                Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0, 10.0},
            },
            []string{"operation", "table", "status"},
        ),
        QueryCounter: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "pgxkit_queries_total",
                Help: "Total number of queries",
            },
            []string{"operation", "table", "status"},
        ),
        PoolUtilization: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "pgxkit_pool_utilization",
                Help: "Connection pool utilization",
            },
            []string{"pool_type"},
        ),
        CacheHitRate: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "pgxkit_cache_hit_rate",
                Help: "Cache hit rate",
            },
            []string{"cache_level"},
        ),
    }
}

func setupPerformanceMonitoring(db *pgxkit.DB, metrics *PerformanceMetrics) {
    // Query performance monitoring
    db.AddHook(pgxkit.BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start := time.Now()
        return context.WithValue(ctx, "perf_start", start)
    })
    
    db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
        start, ok := ctx.Value("perf_start").(time.Time)
        if !ok {
            return nil
        }
        
        duration := time.Since(start)
        operation := extractOperation(sql)
        table := extractTable(sql)
        status := "success"
        if operationErr != nil {
            status = "error"
        }
        
        metrics.QueryDuration.WithLabelValues(operation, table, status).Observe(duration.Seconds())
        metrics.QueryCounter.WithLabelValues(operation, table, status).Inc()
        
        return nil
    })
}
```

## Golden Testing for Performance

### Automated Performance Regression Detection

```go
func TestQueryPerformance(t *testing.T) {
    testDB := setupTestDB(t)
    
    // Enable golden testing to capture EXPLAIN plans
    db := testDB.EnableGolden(t, "TestQueryPerformance")
    
    // Create test data
    createTestData(t, testDB, 10000)
    
    // Test critical queries
    t.Run("user_search_performance", func(t *testing.T) {
        // This query's EXPLAIN plan will be captured
        rows, err := db.Query(context.Background(), `
            SELECT u.id, u.name, u.email, COUNT(o.id) as order_count
            FROM users u
            LEFT JOIN orders o ON u.id = o.user_id
            WHERE u.name ILIKE $1
            GROUP BY u.id, u.name, u.email
            ORDER BY order_count DESC
            LIMIT 50
        `, "%john%")
        if err != nil {
            t.Fatal(err)
        }
        defer rows.Close()
        
        // Verify results
        var count int
        for rows.Next() {
            count++
        }
        
        if count == 0 {
            t.Error("Expected search results")
        }
    })
}
```

## Database Schema Optimization

### Index Optimization

```sql
-- Create indexes for common query patterns
CREATE INDEX CONCURRENTLY idx_users_email ON users(email);
CREATE INDEX CONCURRENTLY idx_users_active_created ON users(active, created_at DESC);
CREATE INDEX CONCURRENTLY idx_orders_user_created ON orders(user_id, created_at DESC);

-- Partial indexes for specific conditions
CREATE INDEX CONCURRENTLY idx_users_active ON users(id) WHERE active = true;
CREATE INDEX CONCURRENTLY idx_orders_recent ON orders(created_at DESC) 
    WHERE created_at > NOW() - INTERVAL '30 days';

-- Composite indexes for complex queries
CREATE INDEX CONCURRENTLY idx_users_search ON users 
    USING gin(to_tsvector('english', name || ' ' || email));
```

### Query Plan Analysis

```go
func analyzeQueryPlan(db *pgxkit.DB, sql string, args ...interface{}) {
    ctx := context.Background()
    
    // Get query plan
    explainSQL := "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) " + sql
    row := db.QueryRow(ctx, explainSQL, args...)
    
    var planJSON string
    err := row.Scan(&planJSON)
    if err != nil {
        log.Printf("Failed to get query plan: %v", err)
        return
    }
    
    // Parse and analyze plan
    var plan map[string]interface{}
    if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
        log.Printf("Failed to parse query plan: %v", err)
        return
    }
    
    // Extract key metrics
    if plans, ok := plan["Plan"].([]interface{}); ok && len(plans) > 0 {
        if planData, ok := plans[0].(map[string]interface{}); ok {
            executionTime := planData["Actual Total Time"]
            planningTime := plan["Planning Time"]
            
            log.Printf("Query: %s", sql)
            log.Printf("Execution Time: %v ms", executionTime)
            log.Printf("Planning Time: %v ms", planningTime)
            
            // Check for performance issues
            if execTime, ok := executionTime.(float64); ok && execTime > 1000 {
                log.Printf("WARNING: Slow query detected (%.2f ms)", execTime)
            }
        }
    }
}
```

## Application-Level Optimizations

### Connection Reuse

```go
type ConnectionManager struct {
    db *pgxkit.DB
}

func (cm *ConnectionManager) ExecuteInTransaction(ctx context.Context, fn func(tx pgx.Tx) error) error {
    tx, err := cm.db.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    
    if err := fn(tx); err != nil {
        return err
    }
    
    return tx.Commit(ctx)
}

// Batch multiple operations in a single transaction
func (cm *ConnectionManager) CreateUserWithProfile(ctx context.Context, user *User, profile *Profile) error {
    return cm.ExecuteInTransaction(ctx, func(tx pgx.Tx) error {
        // Insert user
        err := tx.QueryRow(ctx,
            "INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id",
            user.Name, user.Email).Scan(&user.ID)
        if err != nil {
            return err
        }
        
        // Insert profile
        _, err = tx.Exec(ctx,
            "INSERT INTO profiles (user_id, bio, avatar) VALUES ($1, $2, $3)",
            user.ID, profile.Bio, profile.Avatar)
        return err
    })
}
```

### Result Set Streaming

```go
func streamLargeResultSet(db *pgxkit.DB, processor func(*User) error) error {
    ctx := context.Background()
    
    rows, err := db.ReadQuery(ctx, `
        SELECT id, name, email 
        FROM users 
        ORDER BY id
    `)
    if err != nil {
        return err
    }
    defer rows.Close()
    
    for rows.Next() {
        var user User
        if err := rows.Scan(&user.ID, &user.Name, &user.Email); err != nil {
            return err
        }
        
        // Process each user immediately
        if err := processor(&user); err != nil {
            return err
        }
    }
    
    return rows.Err()
}
```

## Troubleshooting Performance Issues

### Common Performance Problems

1. **Connection Pool Exhaustion**
```go
// Symptoms: High latency, timeouts
// Solution: Increase pool size or fix connection leaks
func diagnosePoolExhaustion(db *pgxkit.DB) {
    stats := db.Stats()
    if stats.AcquiredConns() >= stats.MaxConns() {
        log.Printf("Pool exhausted: %d/%d connections", stats.AcquiredConns(), stats.MaxConns())
        // Check for connection leaks
        // Increase pool size if needed
    }
}
```

2. **Slow Queries**
```go
// Use query analyzer to identify problematic queries
func identifySlowQueries(analyzer *QueryAnalyzer) {
    slowQueries := analyzer.GetSlowQueries()
    for _, query := range slowQueries {
        avgDuration := query.TotalDuration / time.Duration(query.Count)
        log.Printf("Slow query: %s (avg: %v, count: %d)", 
            query.SQL, avgDuration, query.Count)
    }
}
```

3. **Memory Issues**
```go
// Monitor memory usage
func monitorMemoryUsage() {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    
    log.Printf("Memory Stats: Alloc=%d KB, TotalAlloc=%d KB, Sys=%d KB, NumGC=%d",
        bToKb(m.Alloc), bToKb(m.TotalAlloc), bToKb(m.Sys), m.NumGC)
}

func bToKb(b uint64) uint64 {
    return b / 1024
}
```

### Performance Debugging Checklist

- [ ] Check connection pool utilization
- [ ] Analyze slow query logs
- [ ] Verify index usage
- [ ] Monitor memory consumption
- [ ] Check for connection leaks
- [ ] Validate query patterns
- [ ] Review caching effectiveness
- [ ] Assess read/write split efficiency
- [ ] Monitor database server metrics
- [ ] Check network latency

### Performance Tuning Tips

1. **Query Optimization**
   - Use EXPLAIN ANALYZE to understand query plans
   - Add appropriate indexes
   - Avoid SELECT * queries
   - Use LIMIT for large result sets

2. **Connection Management**
   - Right-size connection pools
   - Monitor pool utilization
   - Use read/write splitting
   - Implement connection retry logic

3. **Caching Strategy**
   - Cache frequently accessed data
   - Use appropriate cache TTLs
   - Implement cache invalidation
   - Consider multi-level caching

4. **Application Design**
   - Batch operations when possible
   - Use transactions appropriately
   - Stream large result sets
   - Implement proper error handling

## See Also

- **[Getting Started](Getting-Started)** - Basic setup and configuration
- **[Examples](Examples)** - Practical code examples
- **[Production Guide](Production-Guide)** - Deployment and production considerations
- **[Testing Guide](Testing-Guide)** - Testing strategies and golden tests
- **[API Reference](API-Reference)** - Complete API documentation

---

**[← Back to Home](Home)**

*Following these performance optimization strategies will help you build high-performance applications with pgxkit while maintaining reliability and scalability.*

*Last updated: December 2024* 