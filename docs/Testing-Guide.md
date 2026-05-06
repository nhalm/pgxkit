# Testing Best Practices with pgxkit

**[← Back to Home](Home)**

This guide covers effective testing strategies and best practices when using pgxkit in your applications.

## Table of Contents

1. [Testing Philosophy](#testing-philosophy)
2. [Test Environment Setup](#test-environment-setup)
3. [Unit Testing](#unit-testing)
4. [Integration Testing](#integration-testing)
5. [Plan-Regression Testing](#plan-regression-testing)
6. [Golden Transcript Testing](#golden-transcript-testing)
7. [Testing Patterns](#testing-patterns)
8. [Test Data Management](#test-data-management)
9. [Performance Testing](#performance-testing)
10. [Error Testing](#error-testing)
11. [Best Practices](#best-practices)

## Testing Philosophy

pgxkit follows these testing principles:

- **Test database operations without mocking** - Use real database connections for reliable tests
- **Isolate test data** - Each test should have its own clean database state
- **Test performance regressions** - Use plan-regression tests to catch query plan changes
- **Test error conditions** - Verify proper error handling and recovery
- **Keep tests fast** - Use efficient setup/teardown and parallel execution

## Test Environment Setup

### Database Configuration

Set up a dedicated test database:

```bash
# Test database environment variables
export TEST_POSTGRES_HOST=localhost
export TEST_POSTGRES_PORT=5432
export TEST_POSTGRES_USER=test_user
export TEST_POSTGRES_PASSWORD=test_password
export TEST_POSTGRES_DB=test_db
export TEST_POSTGRES_SSLMODE=disable
```

### Test Database Initialization

```go
package main

import (
    "context"
    "testing"

    "github.com/nhalm/pgxkit"
)

func setupTestDB(t *testing.T) *pgxkit.TestDB {
    // Create test database connection
    testDB := pgxkit.NewTestDB()

    // Connect to test database (uses TEST_DATABASE_URL env var)
    ctx := context.Background()
    err := testDB.Connect(ctx, "")
    if err != nil {
        t.Skip("Test database not available:", err)
    }

    // Setup database schema and initial data
    err = testDB.Setup()
    if err != nil {
        t.Skip("Test database setup failed:", err)
    }

    // Clean up after test
    t.Cleanup(func() {
        testDB.Clean()
        testDB.Shutdown(ctx)
    })

    return testDB
}
```

### Test Suite Structure

```go
// test_helper.go
package myapp

import (
    "context"
    "testing"

    "github.com/nhalm/pgxkit"
)

// TestSuite provides common test utilities
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

// loadFixtureSQL executes SQL from a file for test data setup.
// Implement your own fixture loading logic based on your project needs.
func (ts *TestSuite) loadFixtureSQL(t *testing.T, sql string) {
    _, err := ts.DB.Exec(ts.ctx, sql)
    if err != nil {
        t.Fatalf("Failed to execute fixture SQL: %v", err)
    }
}

func (ts *TestSuite) AssertRowCount(t *testing.T, table string, expected int) {
    var count int
    err := ts.DB.QueryRow(ts.ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
    if err != nil {
        t.Fatalf("Failed to count rows in %s: %v", table, err)
    }

    if count != expected {
        t.Errorf("Expected %d rows in %s, got %d", expected, table, count)
    }
}
```

## Unit Testing

### Repository Testing

```go
func TestUserRepository_CreateUser(t *testing.T) {
    suite := NewTestSuite(t)
    repo := NewUserRepository(suite.DB)

    user := &User{
        Name:  "John Doe",
        Email: "john@example.com",
    }

    // Test user creation
    err := repo.CreateUser(suite.ctx, user)
    require.NoError(t, err)
    require.NotZero(t, user.ID)

    // Verify user was created
    retrieved, err := repo.GetUser(suite.ctx, user.ID)
    require.NoError(t, err)
    assert.Equal(t, user.Name, retrieved.Name)
    assert.Equal(t, user.Email, retrieved.Email)
}

func TestUserRepository_GetUser_NotFound(t *testing.T) {
    suite := NewTestSuite(t)
    repo := NewUserRepository(suite.DB)

    // Test getting non-existent user
    _, err := repo.GetUser(suite.ctx, 999)
    assert.Error(t, err)
    assert.True(t, errors.Is(err, pgx.ErrNoRows))
}

func TestUserRepository_UpdateUser(t *testing.T) {
    suite := NewTestSuite(t)
    repo := NewUserRepository(suite.DB)

    // Create test data directly with SQL
    _, err := suite.DB.Exec(suite.ctx, `
        INSERT INTO users (id, name, email) VALUES (1, 'Original Name', 'original@example.com')
        ON CONFLICT (id) DO NOTHING
    `)
    require.NoError(t, err)

    // Update existing user
    user := &User{
        ID:    1,
        Name:  "Updated Name",
        Email: "updated@example.com",
    }

    err = repo.UpdateUser(suite.ctx, user)
    require.NoError(t, err)

    // Verify update
    retrieved, err := repo.GetUser(suite.ctx, 1)
    require.NoError(t, err)
    assert.Equal(t, user.Name, retrieved.Name)
    assert.Equal(t, user.Email, retrieved.Email)
}
```

### Service Layer Testing

```go
func TestUserService_CreateUser(t *testing.T) {
    suite := NewTestSuite(t)
    service := NewUserService(suite.DB)

    tests := []struct {
        name    string
        input   CreateUserRequest
        wantErr bool
        errMsg  string
    }{
        {
            name: "valid user",
            input: CreateUserRequest{
                Name:  "John Doe",
                Email: "john@example.com",
            },
            wantErr: false,
        },
        {
            name: "duplicate email",
            input: CreateUserRequest{
                Name:  "Jane Doe",
                Email: "john@example.com", // Same email as above
            },
            wantErr: true,
            errMsg:  "email already exists",
        },
        {
            name: "invalid email",
            input: CreateUserRequest{
                Name:  "Invalid User",
                Email: "not-an-email",
            },
            wantErr: true,
            errMsg:  "invalid email format",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            user, err := service.CreateUser(suite.ctx, tt.input)

            if tt.wantErr {
                assert.Error(t, err)
                if tt.errMsg != "" {
                    assert.Contains(t, err.Error(), tt.errMsg)
                }
                assert.Nil(t, user)
            } else {
                assert.NoError(t, err)
                assert.NotNil(t, user)
                assert.NotZero(t, user.ID)
                assert.Equal(t, tt.input.Name, user.Name)
                assert.Equal(t, tt.input.Email, user.Email)
            }
        })
    }
}
```

## Integration Testing

### API Endpoint Testing

```go
func TestUserHandler_CreateUser(t *testing.T) {
    suite := NewTestSuite(t)
    handler := NewUserHandler(suite.DB)

    tests := []struct {
        name           string
        requestBody    string
        expectedStatus int
        checkResponse  func(t *testing.T, resp *http.Response, body []byte)
    }{
        {
            name: "successful creation",
            requestBody: `{
                "name": "John Doe",
                "email": "john@example.com"
            }`,
            expectedStatus: http.StatusCreated,
            checkResponse: func(t *testing.T, resp *http.Response, body []byte) {
                var user User
                err := json.Unmarshal(body, &user)
                require.NoError(t, err)
                assert.NotZero(t, user.ID)
                assert.Equal(t, "John Doe", user.Name)
                assert.Equal(t, "john@example.com", user.Email)
            },
        },
        {
            name: "invalid json",
            requestBody: `{
                "name": "John Doe",
                "email":
            }`,
            expectedStatus: http.StatusBadRequest,
            checkResponse: func(t *testing.T, resp *http.Response, body []byte) {
                assert.Contains(t, string(body), "invalid JSON")
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req := httptest.NewRequest(http.MethodPost, "/users",
                strings.NewReader(tt.requestBody))
            req.Header.Set("Content-Type", "application/json")

            w := httptest.NewRecorder()
            handler.CreateUser(w, req)

            resp := w.Result()
            body, err := io.ReadAll(resp.Body)
            require.NoError(t, err)

            assert.Equal(t, tt.expectedStatus, resp.StatusCode)

            if tt.checkResponse != nil {
                tt.checkResponse(t, resp, body)
            }
        })
    }
}
```

### Transaction Testing

```go
func TestUserService_CreateUserWithProfile(t *testing.T) {
    suite := NewTestSuite(t)
    service := NewUserService(suite.DB)

    t.Run("successful transaction", func(t *testing.T) {
        req := CreateUserWithProfileRequest{
            User: CreateUserRequest{
                Name:  "John Doe",
                Email: "john@example.com",
            },
            Profile: CreateProfileRequest{
                Bio:    "Software developer",
                Avatar: "https://example.com/avatar.jpg",
            },
        }

        result, err := service.CreateUserWithProfile(suite.ctx, req)
        require.NoError(t, err)

        // Verify user was created
        user, err := service.GetUser(suite.ctx, result.UserID)
        require.NoError(t, err)
        assert.Equal(t, req.User.Name, user.Name)

        // Verify profile was created
        profile, err := service.GetProfile(suite.ctx, result.UserID)
        require.NoError(t, err)
        assert.Equal(t, req.Profile.Bio, profile.Bio)
    })

    t.Run("transaction rollback on error", func(t *testing.T) {
        // Create user first to cause duplicate email error
        _, err := suite.DB.Exec(suite.ctx, `
            INSERT INTO users (id, name, email, active) VALUES
            (100, 'Existing User', 'existing@example.com', true)
            ON CONFLICT (id) DO NOTHING
        `)
        require.NoError(t, err)

        req := CreateUserWithProfileRequest{
            User: CreateUserRequest{
                Name:  "Jane Doe",
                Email: "existing@example.com", // Already exists
            },
            Profile: CreateProfileRequest{
                Bio: "Should not be created",
            },
        }

        _, err = service.CreateUserWithProfile(suite.ctx, req)
        require.Error(t, err)

        // Verify no partial data was created
        suite.AssertRowCount(t, "users", 1)    // Only fixture user
        suite.AssertRowCount(t, "profiles", 0) // No profiles created
    })
}
```

## Plan-Regression Testing

Plan-regression testing captures the structural query plan via `EXPLAIN (FORMAT JSON, COSTS OFF)` and asserts it is unchanged across runs. It catches plan shape changes such as seq-scan to index-scan, nested-loop to hash-join, a new sort node, or a different join order. It does NOT assert anything about query result rows — assert those yourself in the test body. Because the captured form is plan-only (no `ANALYZE`), the underlying statement is not executed during plan capture, so there are no side effects.

### Query Plan Testing

```go
func TestUserQueries_Plan(t *testing.T) {
    testDB := setupTestDB(t)

    // Enable plan-regression testing
    db := testDB.EnableAssertPlan("TestUserQueries_Plan")

    // Load test data with manual SQL
    _, err := testDB.Exec(context.Background(), `
        INSERT INTO users (id, name, email, active) VALUES
        (1, 'John Doe', 'john@example.com', true),
        (2, 'Jane Smith', 'jane@example.com', true),
        (3, 'Bob Johnson', 'bob@example.com', false)
        ON CONFLICT (id) DO NOTHING;

        INSERT INTO orders (id, user_id, total, created_at) VALUES
        (1, 1, 99.99, NOW() - INTERVAL '3 days'),
        (2, 1, 149.99, NOW() - INTERVAL '2 days'),
        (3, 2, 75.50, NOW() - INTERVAL '1 day')
        ON CONFLICT (id) DO NOTHING;
    `)
    require.NoError(t, err)

    t.Run("complex_user_query", func(t *testing.T) {
        // This query's structural EXPLAIN plan will be captured and asserted
        rows, err := db.Query(context.Background(), `
            SELECT
                u.id,
                u.name,
                u.email,
                COUNT(o.id) as order_count,
                COALESCE(SUM(o.total), 0) as total_spent
            FROM users u
            LEFT JOIN orders o ON u.id = o.user_id
            WHERE u.active = true
            GROUP BY u.id, u.name, u.email
            HAVING COUNT(o.id) > 0
            ORDER BY total_spent DESC
            LIMIT 10
        `)
        require.NoError(t, err)
        defer rows.Close()

        var users []UserSummary
        for rows.Next() {
            var user UserSummary
            err := rows.Scan(&user.ID, &user.Name, &user.Email,
                &user.OrderCount, &user.TotalSpent)
            require.NoError(t, err)
            users = append(users, user)
        }

        // Verify results make sense
        require.True(t, len(users) > 0)

        // The plan-regression test asserts the structural query plan is
        // unchanged. It does not compare result rows or measure execution
        // time - assert results yourself if needed.
    })

    t.Run("user_search_query", func(t *testing.T) {
        // Test search functionality
        rows, err := db.Query(context.Background(), `
            SELECT id, name, email
            FROM users
            WHERE
                active = true AND
                (name ILIKE $1 OR email ILIKE $1)
            ORDER BY name
            LIMIT 20
        `, "%john%")
        require.NoError(t, err)
        defer rows.Close()

        var users []User
        for rows.Next() {
            var user User
            err := rows.Scan(&user.ID, &user.Name, &user.Email)
            require.NoError(t, err)
            users = append(users, user)
        }

        // The plan-regression test will assert the structural plan of this search is unchanged
        assert.True(t, len(users) >= 0)
    })

    t.Run("insert_with_returning", func(t *testing.T) {
        // DML queries are also captured. EXPLAIN without ANALYZE plans the
        // statement without executing it, so plan capture has no side effects.
        var userID int
        err := db.QueryRow(context.Background(), `
            INSERT INTO users (name, email, active)
            VALUES ($1, $2, true)
            RETURNING id
        `, "Test User", "test@example.com").Scan(&userID)
        require.NoError(t, err)
    })

    t.Run("update_query", func(t *testing.T) {
        // UPDATE query plans are captured
        _, err := db.Exec(context.Background(), `
            UPDATE users
            SET last_login = NOW()
            WHERE active = true AND last_login < NOW() - INTERVAL '30 days'
        `)
        require.NoError(t, err)
    })

    db.AssertPlan(t, "TestUserQueries_Plan")
}
```

### Performance Regression Testing

```go
func TestPerformanceRegression(t *testing.T) {
    testDB := setupTestDB(t)
    db := testDB.EnableAssertPlan("TestPerformanceRegression")

    // Create large dataset for performance testing
    _, err := testDB.Exec(context.Background(), `
        INSERT INTO users (name, email, active, created_at)
        SELECT
            'User ' || i,
            'user' || i || '@example.com',
            true,
            NOW() - (i || ' minutes')::interval
        FROM generate_series(1, 10000) AS i
        ON CONFLICT DO NOTHING
    `)
    require.NoError(t, err)

    t.Run("pagination_performance", func(t *testing.T) {
        start := time.Now()

        rows, err := db.Query(context.Background(), `
            SELECT id, name, email, created_at
            FROM users
            WHERE active = true
            ORDER BY created_at DESC
            LIMIT 50 OFFSET 1000
        `)
        require.NoError(t, err)
        defer rows.Close()

        var users []User
        for rows.Next() {
            var user User
            err := rows.Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt)
            require.NoError(t, err)
            users = append(users, user)
        }

        duration := time.Since(start)

        // Assert reasonable performance
        assert.True(t, duration < 100*time.Millisecond,
            "Query took too long: %v", duration)
        assert.Len(t, users, 50)

        // The plan-regression test will track:
        // - Structural EXPLAIN plan (FORMAT JSON, COSTS OFF)
        // - Plan-shape changes such as seq-scan vs index-scan,
        //   nested-loop vs hash-join, new sort nodes, or join-order changes
        //
        // Wall-clock duration is asserted explicitly above; the
        // plan-regression test does not measure execution time.
    })

    db.AssertPlan(t, "TestPerformanceRegression")
}
```

## Golden Transcript Testing

Golden transcript testing is a behavioral assertion: it captures the full sequence of database events a scenario produces — `BEGIN`, every `Query`/`Exec` with its SQL, normalized args, and materialized result rows, and the closing `COMMIT` or `ROLLBACK` — and asserts on subsequent runs that the transcript hasn't changed. It catches an extra UPDATE, a missing INSERT, a different argument, a different returned row, or a `COMMIT` becoming a `ROLLBACK`.

This complements [Plan-Regression Testing](#plan-regression-testing): plan-regression catches changes to query plan *shape*, golden transcripts catch changes to *behavior*. Use one or the other per scenario — `EnableGolden` and `EnableAssertPlan` each return a fresh `*DB`, so they don't compose on the same instance.

### Basic Usage

```go
func TestCreateOrder(t *testing.T) {
    testDB := pgxkit.RequireDB(t)

    golden := testDB.EnableGolden("TestCreateOrder")

    // run the code under test using golden as the DB
    _, err := golden.Exec(context.Background(),
        "INSERT INTO orders (total) VALUES ($1)", 100)
    require.NoError(t, err)

    golden.AssertGolden(t, "TestCreateOrder")
}
```

On the first run, the test writes `testdata/golden/TestCreateOrder.json` and logs that a baseline was created. On subsequent runs, it compares the recorded transcript against that file and fails with a unified diff on any mismatch.

### What the Transcript Captures

Every event pgxkit observes ends up in the transcript, in order:

- `BEGIN`, `COMMIT`, `ROLLBACK` for transactions started via `BeginTx`.
- `QUERY` events for every `Query`, `QueryRow`, and `Exec` call — including DDL (`CREATE TABLE`, `ALTER`, etc.) and `SET` statements. There is no SQL-prefix filter.
- For `Query` and `QueryRow`, materialized result rows as `column → value` maps.
- For `Exec`, the `rows_affected` integer from the command tag.

Caller iteration of `pgx.Rows` still works normally — pgxkit reads the rows once for the transcript, then replays them through a wrapper.

### Refreshing the Baseline

When you intentionally change behavior (a new column in a returning clause, an extra normalization step, a deliberate query reorder), regenerate the baseline:

```bash
go test -overwrite-golden -run TestCreateOrder
```

The flag is package-scoped on pgxkit, so any test binary that links pgxkit accepts it.

### Normalization Defaults

To keep transcripts stable across runs, volatile values are replaced with placeholders before the transcript is written or compared:

- `time.Time` values → `"<TIMESTAMP>"`.
- UUIDs (`uuid.UUID`, `[16]byte`, or canonical UUID strings) → `"<UUID:1>"`, `"<UUID:2>"`, ... assigned in first-seen order, so the same UUID gets the same placeholder wherever it appears.
- Integer columns whose name is `id` or ends in `_id` → `"<ID:1>"`, `"<ID:2>"`, ... also first-seen by numeric value (so `id=7` and `user_id=7` collapse to the same placeholder).

Args have no column hint, so integer normalization does not trigger there; timestamps and UUIDs do.

### Custom Normalization

Add domain-specific normalizers via `WithGoldenNormalizer`. Custom normalizers run **before** defaults, so you can override `time.Time` handling, redact sensitive fields, or canonicalize ordering of map-shaped values:

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

Return `(replacement, true)` to take over normalization for the value, or `(nil, false)` to fall through to the next custom normalizer or to the defaults.

## Testing Patterns

### Table-Driven Tests

```go
func TestUserValidation(t *testing.T) {
    tests := []struct {
        name    string
        user    User
        wantErr bool
        errMsg  string
    }{
        {
            name: "valid user",
            user: User{
                Name:  "John Doe",
                Email: "john@example.com",
            },
            wantErr: false,
        },
        {
            name: "empty name",
            user: User{
                Name:  "",
                Email: "john@example.com",
            },
            wantErr: true,
            errMsg:  "name is required",
        },
        {
            name: "invalid email",
            user: User{
                Name:  "John Doe",
                Email: "not-an-email",
            },
            wantErr: true,
            errMsg:  "invalid email",
        },
        {
            name: "email too long",
            user: User{
                Name:  "John Doe",
                Email: strings.Repeat("a", 250) + "@example.com",
            },
            wantErr: true,
            errMsg:  "email too long",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateUser(&tt.user)

            if tt.wantErr {
                assert.Error(t, err)
                if tt.errMsg != "" {
                    assert.Contains(t, err.Error(), tt.errMsg)
                }
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Parallel Testing

```go
func TestUserRepository_Parallel(t *testing.T) {
    t.Run("parallel_operations", func(t *testing.T) {
        tests := []struct {
            name string
            test func(t *testing.T)
        }{
            {
                name: "create_user",
                test: func(t *testing.T) {
                    suite := NewTestSuite(t)
                    repo := NewUserRepository(suite.DB)

                    user := &User{
                        Name:  "Parallel User 1",
                        Email: "parallel1@example.com",
                    }

                    err := repo.CreateUser(suite.ctx, user)
                    require.NoError(t, err)
                },
            },
            {
                name: "query_users",
                test: func(t *testing.T) {
                    suite := NewTestSuite(t)
                    repo := NewUserRepository(suite.DB)

                    // Set up test data
                    _, err := suite.DB.Exec(suite.ctx, `
                        INSERT INTO users (name, email, active) VALUES
                        ('Test User', 'test@example.com', true)
                        ON CONFLICT DO NOTHING
                    `)
                    require.NoError(t, err)

                    users, err := repo.GetAllUsers(suite.ctx)
                    require.NoError(t, err)
                    assert.True(t, len(users) > 0)
                },
            },
        }

        for _, tt := range tests {
            tt := tt // Capture loop variable
            t.Run(tt.name, func(t *testing.T) {
                t.Parallel() // Run tests in parallel
                tt.test(t)
            })
        }
    })
}
```

## Test Data Management

### Fixture Files

Store SQL fixtures in your project and load them manually:

```sql
-- fixtures/users.sql
INSERT INTO users (id, name, email, active, created_at) VALUES
(1, 'John Doe', 'john@example.com', true, NOW() - INTERVAL '10 days'),
(2, 'Jane Smith', 'jane@example.com', true, NOW() - INTERVAL '9 days'),
(3, 'Bob Johnson', 'bob@example.com', false, NOW() - INTERVAL '8 days')
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    email = EXCLUDED.email,
    active = EXCLUDED.active;

-- fixtures/orders.sql
INSERT INTO orders (id, user_id, total, created_at) VALUES
(1, 1, 99.99, NOW() - INTERVAL '3 days'),
(2, 1, 149.99, NOW() - INTERVAL '2 days'),
(3, 2, 75.50, NOW() - INTERVAL '1 day')
ON CONFLICT (id) DO UPDATE SET
    user_id = EXCLUDED.user_id,
    total = EXCLUDED.total;
```

```go
// Helper to load fixture files
func loadFixture(t *testing.T, db *pgxkit.TestDB, filename string) {
    ctx := context.Background()
    data, err := os.ReadFile(filepath.Join("fixtures", filename))
    if err != nil {
        t.Fatalf("Failed to read fixture %s: %v", filename, err)
    }

    _, err = db.Exec(ctx, string(data))
    if err != nil {
        t.Fatalf("Failed to execute fixture %s: %v", filename, err)
    }
}

// Test with fixtures
func TestOrderRepository_GetUserOrders(t *testing.T) {
    suite := NewTestSuite(t)
    loadFixture(t, suite.DB, "users.sql")
    loadFixture(t, suite.DB, "orders.sql")
    repo := NewOrderRepository(suite.DB)

    orders, err := repo.GetUserOrders(suite.ctx, 1)
    require.NoError(t, err)
    assert.Len(t, orders, 2) // User 1 has 2 orders

    // Verify order details
    assert.Equal(t, 99.99, orders[0].Total)
    assert.Equal(t, 149.99, orders[1].Total)
}
```

### Factory Pattern

```go
type UserFactory struct {
    db *pgxkit.TestDB
}

func NewUserFactory(db *pgxkit.TestDB) *UserFactory {
    return &UserFactory{db: db}
}

func (f *UserFactory) Create(ctx context.Context, overrides ...func(*User)) *User {
    user := &User{
        Name:   "Test User",
        Email:  fmt.Sprintf("test%d@example.com", time.Now().UnixNano()),
        Active: true,
    }

    // Apply overrides
    for _, override := range overrides {
        override(user)
    }

    // Save to database
    err := f.db.QueryRow(ctx, `
        INSERT INTO users (name, email, active)
        VALUES ($1, $2, $3)
        RETURNING id, created_at
    `, user.Name, user.Email, user.Active).Scan(&user.ID, &user.CreatedAt)

    if err != nil {
        panic(fmt.Sprintf("Failed to create test user: %v", err))
    }

    return user
}

// Usage in tests
func TestUserService_UpdateUser(t *testing.T) {
    suite := NewTestSuite(t)
    factory := NewUserFactory(suite.DB)
    service := NewUserService(suite.DB)

    // Create test user with specific attributes
    user := factory.Create(suite.ctx, func(u *User) {
        u.Name = "Original Name"
        u.Email = "original@example.com"
    })

    // Test update
    user.Name = "Updated Name"
    err := service.UpdateUser(suite.ctx, user)
    require.NoError(t, err)

    // Verify update
    updated, err := service.GetUser(suite.ctx, user.ID)
    require.NoError(t, err)
    assert.Equal(t, "Updated Name", updated.Name)
}
```

## Performance Testing

### Benchmark Tests

```go
func BenchmarkUserRepository_GetUser(b *testing.B) {
    testDB := setupBenchmarkDB(b)
    repo := NewUserRepository(testDB)
    ctx := context.Background()

    // Create test user
    user := &User{
        Name:  "Benchmark User",
        Email: "benchmark@example.com",
    }
    err := repo.CreateUser(ctx, user)
    if err != nil {
        b.Fatal(err)
    }

    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            _, err := repo.GetUser(ctx, user.ID)
            if err != nil {
                b.Error(err)
            }
        }
    })
}

func BenchmarkUserRepository_CreateUser(b *testing.B) {
    testDB := setupBenchmarkDB(b)
    repo := NewUserRepository(testDB)
    ctx := context.Background()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        user := &User{
            Name:  fmt.Sprintf("User %d", i),
            Email: fmt.Sprintf("user%d@example.com", i),
        }

        err := repo.CreateUser(ctx, user)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func setupBenchmarkDB(b *testing.B) *pgxkit.TestDB {
    testDB := pgxkit.NewTestDB()
    ctx := context.Background()
    err := testDB.Connect(ctx, "")
    if err != nil {
        b.Skip("Test database not available:", err)
    }

    err = testDB.Setup()
    if err != nil {
        b.Skip("Test database setup failed:", err)
    }

    b.Cleanup(func() {
        testDB.Clean()
        testDB.Shutdown(ctx)
    })

    return testDB
}
```

### Load Testing

```go
func TestConcurrentUserCreation(t *testing.T) {
    suite := NewTestSuite(t)
    service := NewUserService(suite.DB)

    const numUsers = 100
    const numWorkers = 10

    userChan := make(chan CreateUserRequest, numUsers)
    resultChan := make(chan error, numUsers)

    // Start workers
    for i := 0; i < numWorkers; i++ {
        go func() {
            for req := range userChan {
                _, err := service.CreateUser(suite.ctx, req)
                resultChan <- err
            }
        }()
    }

    // Send work
    go func() {
        defer close(userChan)
        for i := 0; i < numUsers; i++ {
            userChan <- CreateUserRequest{
                Name:  fmt.Sprintf("User %d", i),
                Email: fmt.Sprintf("user%d@example.com", i),
            }
        }
    }()

    // Collect results
    var errors []error
    for i := 0; i < numUsers; i++ {
        if err := <-resultChan; err != nil {
            errors = append(errors, err)
        }
    }

    // Verify results
    if len(errors) > 0 {
        t.Errorf("Got %d errors out of %d operations: %v",
            len(errors), numUsers, errors[0])
    }

    // Verify all users were created
    suite.AssertRowCount(t, "users", numUsers)
}
```

## Error Testing

### Connection Error Testing

```go
func TestDatabaseConnection_Errors(t *testing.T) {
    t.Run("connection_timeout", func(t *testing.T) {
        // Use invalid host to trigger timeout
        dsn := "postgres://user:pass@invalid-host:5432/db?connect_timeout=1"

        db := pgxkit.NewDB()
        err := db.Connect(context.Background(), dsn)

        assert.Error(t, err)
        assert.Contains(t, err.Error(), "connection")
    })

    t.Run("invalid_credentials", func(t *testing.T) {
        dsn := "postgres://invalid:invalid@localhost:5432/testdb"

        db := pgxkit.NewDB()
        err := db.Connect(context.Background(), dsn)

        assert.Error(t, err)
        // Don't check specific error message as it varies
    })

    t.Run("database_not_found", func(t *testing.T) {
        dsn := "postgres://user:pass@localhost:5432/nonexistent_db"

        db := pgxkit.NewDB()
        err := db.Connect(context.Background(), dsn)

        assert.Error(t, err)
    })
}
```

### Query Error Testing

```go
func TestRepository_QueryErrors(t *testing.T) {
    suite := NewTestSuite(t)
    repo := NewUserRepository(suite.DB)

    t.Run("syntax_error", func(t *testing.T) {
        // Intentional SQL syntax error
        _, err := suite.DB.Query(suite.ctx, "SELECT * FORM users") // FORM instead of FROM
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "syntax")
    })

    t.Run("constraint_violation", func(t *testing.T) {
        // Create user with unique email constraint violation
        user1 := &User{
            Name:  "User 1",
            Email: "duplicate@example.com",
        }
        err := repo.CreateUser(suite.ctx, user1)
        require.NoError(t, err)

        // Try to create another user with same email
        user2 := &User{
            Name:  "User 2",
            Email: "duplicate@example.com",
        }
        err = repo.CreateUser(suite.ctx, user2)
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "duplicate")
    })

    t.Run("not_found", func(t *testing.T) {
        _, err := repo.GetUser(suite.ctx, 999999)
        assert.Error(t, err)
        assert.True(t, errors.Is(err, pgx.ErrNoRows))
    })
}
```

## Best Practices

### Test Organization

```go
// Group related tests
func TestUserRepository(t *testing.T) {
    t.Run("CreateUser", func(t *testing.T) {
        // Create user tests
    })

    t.Run("GetUser", func(t *testing.T) {
        // Get user tests
    })

    t.Run("UpdateUser", func(t *testing.T) {
        // Update user tests
    })

    t.Run("DeleteUser", func(t *testing.T) {
        // Delete user tests
    })
}
```

### Test Naming

```go
// Good: Descriptive test names
func TestUserRepository_CreateUser_WithValidData_ReturnsUser(t *testing.T) {}
func TestUserRepository_CreateUser_WithDuplicateEmail_ReturnsError(t *testing.T) {}
func TestUserRepository_GetUser_WithNonExistentID_ReturnsNotFound(t *testing.T) {}

// Bad: Vague test names
func TestCreateUser(t *testing.T) {}
func TestGetUser(t *testing.T) {}
```

### Assertion Strategy

```go
func TestUserCreation(t *testing.T) {
    // Use require for critical assertions that should stop the test
    user, err := service.CreateUser(ctx, req)
    require.NoError(t, err)
    require.NotNil(t, user)

    // Use assert for additional checks
    assert.NotZero(t, user.ID)
    assert.Equal(t, req.Name, user.Name)
    assert.Equal(t, req.Email, user.Email)
    assert.NotZero(t, user.CreatedAt)
}
```

### Test Data Isolation

```go
// Good: Each test creates its own data
func TestUserService_GetActiveUsers(t *testing.T) {
    suite := NewTestSuite(t)

    // Create specific test data
    activeUser := createTestUser(suite, "active@example.com", true)
    inactiveUser := createTestUser(suite, "inactive@example.com", false)

    users, err := service.GetActiveUsers(suite.ctx)
    require.NoError(t, err)

    // Should only contain active user
    assert.Len(t, users, 1)
    assert.Equal(t, activeUser.ID, users[0].ID)
}

// Bad: Relying on shared test data
func TestUserService_GetActiveUsers_Bad(t *testing.T) {
    // Assumes specific data exists from fixtures
    users, err := service.GetActiveUsers(ctx)
    require.NoError(t, err)
    assert.Len(t, users, 2) // Fragile - depends on fixture data
}
```

### Testing Checklist

- [ ] Test database setup and teardown
- [ ] Unit tests for business logic
- [ ] Integration tests for database operations
- [ ] Plan-regression tests for query plan shape
- [ ] Golden transcript tests for behavioral stability
- [ ] Error condition testing
- [ ] Concurrent operation testing
- [ ] Benchmark tests for critical paths
- [ ] Test data isolation
- [ ] Proper assertion strategy
- [ ] Meaningful test names

## See Also

- **[Getting Started](Getting-Started)** - Basic setup and configuration
- **[Examples](Examples)** - Practical code examples
- **[Performance Guide](Performance-Guide)** - Performance optimization
- **[Production Guide](Production-Guide)** - Deployment best practices
- **[API Reference](API-Reference)** - Complete API documentation

---

**[← Back to Home](Home)**

*Following these testing practices will help you build reliable, maintainable applications with confidence in your database operations.*
