# Frequently Asked Questions

**[← Back to Home](Home)**

Common questions and answers about pgxkit usage, configuration, and troubleshooting.

## Table of Contents

1. [General Questions](#general-questions)
2. [Setup and Configuration](#setup-and-configuration)
3. [Performance and Optimization](#performance-and-optimization)
4. [Testing](#testing)
5. [Production Deployment](#production-deployment)
6. [Troubleshooting](#troubleshooting)
7. [Migration and Integration](#migration-and-integration)

## General Questions

### What is pgxkit and how is it different from other PostgreSQL libraries?

pgxkit is a **tool-agnostic** PostgreSQL toolkit that provides production-ready utilities while working with any PostgreSQL development approach. Unlike ORMs or query builders, pgxkit doesn't dictate how you write your queries - it works with:

- Raw pgx usage
- Code generation tools (sqlc, Skimatik, etc.)
- Any existing PostgreSQL workflow

**Key differences:**
- **Safety first** - Uses write pool by default, explicit read optimization
- **Extensible hooks** - Add logging, metrics, tracing without changing your queries
- **Golden testing** - Performance regression detection for your queries
- **Production features** - Retry logic, graceful shutdown, health checks built-in

### Should I use pgxkit instead of pgx directly?

pgxkit is built **on top of** pgx, not instead of it. You get all the benefits of pgx plus:

- Connection pool abstraction with read/write splitting
- Hook system for observability
- Testing utilities including golden tests
- Retry logic for transient failures
- Production-ready lifecycle management

If you're building a production application, pgxkit provides valuable utilities. If you're writing a simple script or tool, direct pgx might be sufficient.

### Is pgxkit compatible with code generation tools?

Yes! pgxkit is specifically designed to be tool-agnostic. It works seamlessly with:

- **sqlc** - Use pgxkit.DB directly or get underlying pools
- **Skimatik** - Pass connection pools to generated code
- **Custom code generation** - Any tool that accepts pgxpool.Pool
- **Raw SQL** - Use pgxkit methods directly

**Example with sqlc:**
```go
db := pgxkit.NewDB()
err := db.Connect(ctx, dsn)

// Use directly with sqlc
queries := sqlc.New(db)

// Or get specific pools
writeQueries := sqlc.New(db.WritePool())
readQueries := sqlc.New(db.ReadPool())
```

## Setup and Configuration

### How do I configure connection pools for my workload?

Pool sizing depends on your application characteristics:

**For CPU-bound workloads:**
```go
maxConns := runtime.NumCPU() * 2  // 1-2 connections per core
```

**For I/O-bound workloads:**
```go
maxConns := runtime.NumCPU() * 4  // 2-4 connections per core
```

**Configuration example:**
```go
dsn := fmt.Sprintf("%s?pool_max_conns=%d&pool_min_conns=%d", 
    pgxkit.GetDSN(), maxConns, maxConns/4)
```

Monitor pool utilization and adjust based on your metrics:
```go
stats := db.Stats()
utilization := float64(stats.AcquiredConns()) / float64(stats.MaxConns())
```

### When should I use read/write splitting?

Use read/write splitting when:

- You have read replicas available
- Your application is read-heavy (>70% read operations)
- You want to optimize read performance
- You have separate read and write database endpoints

**Don't use it if:**
- You only have a single database instance
- Your application is write-heavy
- You need strong consistency for all reads

**Setup:**
```go
err := db.ConnectReadWrite(ctx,
    "postgres://user:pass@read-replica:5432/db",  // Read pool
    "postgres://user:pass@primary:5432/db")       // Write pool
```

### How do I handle environment variables correctly?

pgxkit uses standard PostgreSQL environment variables:

```bash
# Required
export POSTGRES_HOST=localhost
export POSTGRES_USER=myuser
export POSTGRES_PASSWORD=mypassword
export POSTGRES_DB=mydb

# Optional (with defaults)
export POSTGRES_PORT=5432          # default: 5432
export POSTGRES_SSLMODE=require    # default: disable

# Connection pool settings
export POSTGRES_MAX_CONNS=20
export POSTGRES_MIN_CONNS=5
```

**Using in code:**
```go
// Uses environment variables automatically
err := db.Connect(ctx, "")

// Or build DSN explicitly
dsn := pgxkit.GetDSN()
```

## Performance and Optimization

### How do I optimize read performance?

1. **Use read methods when you have read/write splits:**
```go
// Instead of db.Query() (uses write pool)
rows, err := db.ReadQuery(ctx, "SELECT * FROM users")
row := db.ReadQueryRow(ctx, "SELECT name FROM users WHERE id = $1", id)
```

2. **Add appropriate database indexes:**
```sql
CREATE INDEX CONCURRENTLY idx_users_email ON users(email);
CREATE INDEX CONCURRENTLY idx_users_active_created ON users(active, created_at DESC);
```

3. **Use LIMIT for large result sets:**
```go
rows, err := db.ReadQuery(ctx, "SELECT * FROM users ORDER BY created_at DESC LIMIT 100")
```

4. **Implement caching for frequently accessed data** - See [Performance Guide](Performance-Guide) for detailed strategies.

### How do I monitor performance?

Use hooks to add metrics collection:

```go
db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    duration := time.Since(start) // start from context
    
    // Prometheus metrics
    queryDuration.WithLabelValues(operation, status).Observe(duration.Seconds())
    queryTotal.WithLabelValues(operation, status).Inc()
    
    // Log slow queries
    if duration > 100*time.Millisecond {
        log.Printf("Slow query: %s (took %v)", sql, duration)
    }
    
    return nil
})
```

Monitor connection pool utilization:
```go
stats := db.Stats()
utilization := float64(stats.AcquiredConns()) / float64(stats.MaxConns())
log.Printf("Pool utilization: %.2f%%", utilization*100)
```

### What's the best way to handle bulk operations?

Use batch operations for better performance:

**For moderate batch sizes (< 1000 records):**
```go
batch := &pgx.Batch{}
for _, user := range users {
    batch.Queue("INSERT INTO users (name, email) VALUES ($1, $2)", user.Name, user.Email)
}

results := tx.SendBatch(ctx, batch)
defer results.Close()
```

**For large datasets (1000+ records):**
```go
_, err := tx.CopyFrom(ctx,
    pgx.Identifier{"users"},
    []string{"name", "email"},
    pgx.CopyFromSlice(len(users), func(i int) ([]interface{}, error) {
        return []interface{}{users[i].Name, users[i].Email}, nil
    }),
)
```

## Testing

### How do I set up tests with pgxkit?

Use the testing utilities for clean test setup:

```go
func TestUserRepository(t *testing.T) {
    // Setup test database
    suite := NewTestSuite(t)
    repo := NewUserRepository(suite.DB)
    
    // Load test data
    suite.LoadFixtures(t, "users.sql")
    
    // Run your tests
    users, err := repo.GetActiveUsers(suite.ctx)
    require.NoError(t, err)
    assert.Len(t, users, 2)
}

type TestSuite struct {
    DB  *pgxkit.TestDB
    ctx context.Context
}

func NewTestSuite(t *testing.T) *TestSuite {
    return &TestSuite{
        DB:  setupTestDB(t),
        ctx: context.Background(),
    }
}
```

### What are golden tests and when should I use them?

Golden tests capture and compare query execution plans to detect performance regressions:

```go
func TestComplexQuery_Golden(t *testing.T) {
    testDB := setupTestDB(t)
    db := testDB.EnableGolden(t, "TestComplexQuery")
    
    // Query plan will be captured automatically
    rows, err := db.Query(ctx, `
        SELECT u.id, u.name, COUNT(o.id) as order_count
        FROM users u
        LEFT JOIN orders o ON u.id = o.user_id
        GROUP BY u.id, u.name
        ORDER BY order_count DESC
    `)
    require.NoError(t, err)
    defer rows.Close()
}
```

**Use golden tests for:**
- Critical queries that must maintain performance
- Complex queries with multiple joins
- Queries that use indexes heavily
- Performance regression detection in CI/CD

### How do I test error conditions?

Test various error scenarios to ensure robust error handling:

```go
func TestRepository_ErrorHandling(t *testing.T) {
    suite := NewTestSuite(t)
    repo := NewUserRepository(suite.DB)
    
    t.Run("duplicate_email", func(t *testing.T) {
        // Create user with duplicate email
        user1 := &User{Email: "test@example.com"}
        user2 := &User{Email: "test@example.com"}
        
        err := repo.CreateUser(suite.ctx, user1)
        require.NoError(t, err)
        
        err = repo.CreateUser(suite.ctx, user2)
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "duplicate")
    })
    
    t.Run("not_found", func(t *testing.T) {
        _, err := repo.GetUser(suite.ctx, 999)
        assert.Error(t, err)
        assert.True(t, errors.Is(err, pgx.ErrNoRows))
    })
}
```

## Production Deployment

### How do I implement graceful shutdown?

Use pgxkit's built-in graceful shutdown:

```go
func main() {
    db := pgxkit.NewDB()
    err := db.Connect(ctx, "")
    if err != nil {
        log.Fatal(err)
    }
    
    // Setup graceful shutdown
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    
    go func() {
        <-c
        log.Println("Shutting down...")
        
        // Give operations time to complete
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        
        if err := db.Shutdown(ctx); err != nil {
            log.Printf("Shutdown error: %v", err)
        }
        
        os.Exit(0)
    }()
    
    // Your application logic here
}
```

### How do I handle database migrations?

pgxkit doesn't include migration tools - use your preferred migration solution:

**With golang-migrate:**
```go
import "github.com/golang-migrate/migrate/v4"

func runMigrations() error {
    m, err := migrate.New(
        "file://migrations",
        pgxkit.GetDSN())
    if err != nil {
        return err
    }
    
    return m.Up()
}
```

**With custom migrations:**
```go
func applyMigrations(db *pgxkit.DB) error {
    migrations := []string{
        "CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT)",
        "ALTER TABLE users ADD COLUMN email TEXT",
    }
    
    for _, migration := range migrations {
        _, err := db.Exec(ctx, migration)
        if err != nil {
            return err
        }
    }
    return nil
}
```

### How do I set up health checks?

Implement health check endpoints for monitoring:

```go
func healthCheckHandler(db *pgxkit.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
        defer cancel()
        
        if err := db.HealthCheck(ctx); err != nil {
            w.WriteHeader(http.StatusServiceUnavailable)
            json.NewEncoder(w).Encode(map[string]string{
                "status": "unhealthy",
                "error":  err.Error(),
            })
            return
        }
        
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "healthy",
        })
    }
}
```

## Troubleshooting

### Connection pool exhausted - what should I do?

**Symptoms:**
- High latency
- Connection timeouts
- "failed to get connection" errors

**Diagnosis:**
```go
stats := db.Stats()
utilization := float64(stats.AcquiredConns()) / float64(stats.MaxConns())
log.Printf("Pool utilization: %.2f%%", utilization*100)

if utilization > 0.8 {
    log.Println("Pool utilization is high")
}
```

**Solutions:**
1. **Increase pool size** (if you have database capacity):
```go
dsn := fmt.Sprintf("%s?pool_max_conns=50", pgxkit.GetDSN())
```

2. **Fix connection leaks** - ensure you're closing rows and statements:
```go
rows, err := db.Query(ctx, sql)
if err != nil {
    return err
}
defer rows.Close() // Always close rows!
```

3. **Optimize slow queries** - they hold connections longer
4. **Implement connection timeouts**:
```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
```

### My queries are slow - how do I debug them?

1. **Enable query logging:**
```go
db.AddHook(pgxkit.AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
    if duration := getDuration(ctx); duration > 100*time.Millisecond {
        log.Printf("Slow query: %s (took %v)", sql, duration)
    }
    return nil
})
```

2. **Use EXPLAIN ANALYZE:**
```go
func analyzeQuery(db *pgxkit.DB, sql string, args ...interface{}) {
    explainSQL := "EXPLAIN (ANALYZE, BUFFERS) " + sql
    rows, err := db.Query(ctx, explainSQL, args...)
    if err != nil {
        log.Printf("Failed to explain query: %v", err)
        return
    }
    defer rows.Close()
    
    for rows.Next() {
        var line string
        rows.Scan(&line)
        log.Println(line)
    }
}
```

3. **Check for missing indexes:**
- Look for "Seq Scan" in query plans
- Add indexes for frequently queried columns
- Use partial indexes for filtered queries

### SSL connection issues

**Common SSL problems and solutions:**

1. **SSL required but not configured:**
```bash
# Set SSL mode in environment
export POSTGRES_SSLMODE=require
```

2. **Certificate verification issues:**
```bash
# For development (not production!)
export POSTGRES_SSLMODE=prefer
```

3. **Custom certificates:**
```go
dsn := fmt.Sprintf(
    "postgres://user:pass@host/db?sslmode=require&sslcert=%s&sslkey=%s&sslrootcert=%s",
    certFile, keyFile, rootCertFile)
```

4. **Verify SSL is working:**
```go
var sslInUse bool
err := db.QueryRow(ctx, "SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid()").Scan(&sslInUse)
if err != nil || !sslInUse {
    log.Println("WARNING: SSL not in use")
}
```

### Memory usage is high

**Potential causes and solutions:**

1. **Large result sets** - use LIMIT and pagination:
```go
rows, err := db.ReadQuery(ctx, "SELECT * FROM large_table ORDER BY id LIMIT 1000 OFFSET $1", offset)
```

2. **Connection leaks** - monitor pool statistics:
```go
go func() {
    ticker := time.NewTicker(30 * time.Second)
    for range ticker.C {
        stats := db.Stats()
        log.Printf("Active connections: %d/%d", stats.AcquiredConns(), stats.MaxConns())
    }
}()
```

3. **Result set streaming** for large datasets:
```go
func processLargeDataset(db *pgxkit.DB, processor func(*Record) error) error {
    rows, err := db.ReadQuery(ctx, "SELECT * FROM large_table ORDER BY id")
    if err != nil {
        return err
    }
    defer rows.Close()
    
    for rows.Next() {
        var record Record
        if err := rows.Scan(&record.ID, &record.Data); err != nil {
            return err
        }
        
        // Process immediately, don't accumulate in memory
        if err := processor(&record); err != nil {
            return err
        }
    }
    return rows.Err()
}
```

## Migration and Integration

### How do I migrate from database/sql?

1. **Replace sql.DB with pgxkit.DB:**
```go
// Before
db, err := sql.Open("postgres", dsn)

// After  
db := pgxkit.NewDB()
err := db.Connect(ctx, dsn)
```

2. **Update query methods:**
```go
// Before
rows, err := db.Query("SELECT * FROM users")

// After
rows, err := db.Query(ctx, "SELECT * FROM users")
```

3. **Add context to all operations:**
```go
// All pgxkit methods require context
ctx := context.Background()
rows, err := db.Query(ctx, "SELECT * FROM users")
```

4. **Update scanning (pgx uses different types):**
```go
// May need to adjust for pgx types
var id int64  // instead of int
var name string
err := rows.Scan(&id, &name)
```

### How do I integrate with existing code generation?

**With sqlc:**
```go
// In your sqlc yaml config, use pgxkit
gen:
  go:
    package: "myapp"
    out: "db"
    sql_package: "pgx/v5"

// In your code
db := pgxkit.NewDB()
err := db.Connect(ctx, dsn)

queries := myapp.New(db)  // sqlc generated code
```

**With existing repositories:**
```go
type UserRepository struct {
    db *pgxkit.DB  // Replace your existing db field
}

func NewUserRepository(db *pgxkit.DB) *UserRepository {
    return &UserRepository{db: db}
}

func (r *UserRepository) GetUser(ctx context.Context, id int) (*User, error) {
    var user User
    err := r.db.ReadQueryRow(ctx, "SELECT id, name FROM users WHERE id = $1", id).
        Scan(&user.ID, &user.Name)
    return &user, err
}
```

### Can I use pgxkit with ORMs?

pgxkit is designed to complement raw SQL approaches rather than ORMs. However, you can use it alongside ORMs:

```go
// Use pgxkit for performance-critical queries
criticalData, err := pgxkitDB.ReadQuery(ctx, complexOptimizedQuery)

// Use ORM for simpler operations
user := User{Name: "John"}
ormDB.Create(&user)
```

## Common Patterns

### Request-scoped database operations

```go
func getUserHandler(db *pgxkit.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Use request context for cancellation
        ctx := r.Context()
        
        userID, _ := strconv.Atoi(r.URL.Query().Get("id"))
        
        var user User
        err := db.ReadQueryRow(ctx, "SELECT id, name FROM users WHERE id = $1", userID).
            Scan(&user.ID, &user.Name)
        if err != nil {
            http.Error(w, "User not found", http.StatusNotFound)
            return
        }
        
        json.NewEncoder(w).Encode(user)
    }
}
```

### Background job processing

```go
func processJobs(db *pgxkit.DB) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        
        rows, err := db.Query(ctx, "SELECT id, data FROM jobs WHERE status = 'pending' LIMIT 10")
        if err != nil {
            log.Printf("Failed to fetch jobs: %v", err)
            cancel()
            continue
        }
        
        for rows.Next() {
            var job Job
            if err := rows.Scan(&job.ID, &job.Data); err != nil {
                log.Printf("Failed to scan job: %v", err)
                continue
            }
            
            go processJob(db, job)  // Process asynchronously
        }
        
        rows.Close()
        cancel()
    }
}
```

## See Also

- **[Getting Started](Getting-Started)** - Basic setup and usage
- **[Examples](Examples)** - Practical code examples
- **[Performance Guide](Performance-Guide)** - Optimization strategies
- **[Production Guide](Production-Guide)** - Deployment best practices
- **[Testing Guide](Testing-Guide)** - Testing strategies
- **[API Reference](API-Reference)** - Complete API documentation
- **[Contributing](Contributing)** - How to contribute

---

**[← Back to Home](Home)**

*If you have a question that's not covered here, please [open an issue](https://github.com/nhalm/pgxkit/issues) or start a [discussion](https://github.com/nhalm/pgxkit/discussions).*

*Last updated: December 2024* 