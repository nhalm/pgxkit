# Testing Best Practices with pgxkit

This guide covers effective testing strategies and best practices when using pgxkit in your applications.

## Table of Contents

1. [Testing Philosophy](#testing-philosophy)
2. [Test Environment Setup](#test-environment-setup)
3. [Unit Testing](#unit-testing)
4. [Integration Testing](#integration-testing)
5. [Golden Testing](#golden-testing)
6. [Testing Patterns](#testing-patterns)
7. [Test Data Management](#test-data-management)
8. [Performance Testing](#performance-testing)
9. [Error Testing](#error-testing)
10. [Best Practices](#best-practices)

## Testing Philosophy

pgxkit follows these testing principles:

- **Test database operations without mocking** - Use real database connections for reliable tests
- **Isolate test data** - Each test should have its own clean database state
- **Test performance regressions** - Use golden tests to catch query plan changes
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
    
    // Setup database schema and initial data
    err := testDB.Setup()
    if err != nil {
        t.Skip("Test database not available:", err)
    }
    
    // Clean up after test
    t.Cleanup(func() {
        testDB.Clean()
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

// CreateUser creates a test user
func (ts *TestSuite) CreateUser(t *testing.T, name, email string) int {
    var userID int
    err := ts.DB.QueryRow(ts.ctx, 
        "INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id",
        name, email).Scan(&userID)
    if err != nil {
        t.Fatal("Failed to create test user:", err)
    }
    return userID
}
```

## Unit Testing

### Repository Layer Testing

```go
// user_repository_test.go
package repository

import (
    "testing"
    
    "github.com/nhalm/pgxkit"
)

func TestUserRepository_Create(t *testing.T) {
    testDB := setupTestDB(t)
    repo := NewUserRepository(testDB.DB)
    
    user := &User{
        Name:  "John Doe",
        Email: "john@example.com",
    }
    
    err := repo.Create(context.Background(), user)
    if err != nil {
        t.Fatal("Failed to create user:", err)
    }
    
    // Verify user was created
    if user.ID == 0 {
        t.Error("Expected user ID to be set")
    }
    
    // Verify user exists in database
    var count int
    err = testDB.QueryRow(context.Background(), 
        "SELECT COUNT(*) FROM users WHERE email = $1", user.Email).Scan(&count)
    if err != nil {
        t.Fatal("Failed to verify user:", err)
    }
    
    if count != 1 {
        t.Errorf("Expected 1 user, got %d", count)
    }
}

func TestUserRepository_GetByID(t *testing.T) {
    testDB := setupTestDB(t)
    repo := NewUserRepository(testDB.DB)
    
    // Create test user
    userID := createTestUser(t, testDB, "Jane Doe", "jane@example.com")
    
    // Test retrieval
    user, err := repo.GetByID(context.Background(), userID)
    if err != nil {
        t.Fatal("Failed to get user:", err)
    }
    
    if user.ID != userID {
        t.Errorf("Expected user ID %d, got %d", userID, user.ID)
    }
    
    if user.Name != "Jane Doe" {
        t.Errorf("Expected name 'Jane Doe', got '%s'", user.Name)
    }
}

func TestUserRepository_GetByID_NotFound(t *testing.T) {
    testDB := setupTestDB(t)
    repo := NewUserRepository(testDB.DB)
    
    // Test non-existent user
    _, err := repo.GetByID(context.Background(), 999)
    
    // Should return NotFoundError
    var notFoundErr *pgxkit.NotFoundError
    if !errors.As(err, &notFoundErr) {
        t.Errorf("Expected NotFoundError, got %T", err)
    }
}
```

### Service Layer Testing

```go
// user_service_test.go
package service

import (
    "testing"
)

func TestUserService_CreateUser(t *testing.T) {
    testDB := setupTestDB(t)
    service := NewUserService(testDB.DB)
    
    tests := []struct {
        name    string
        input   CreateUserRequest
        wantErr bool
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
            name: "invalid email",
            input: CreateUserRequest{
                Name:  "John Doe",
                Email: "invalid-email",
            },
            wantErr: true,
        },
        {
            name: "duplicate email",
            input: CreateUserRequest{
                Name:  "Jane Doe",
                Email: "john@example.com", // Already exists
            },
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            user, err := service.CreateUser(context.Background(), tt.input)
            
            if tt.wantErr {
                if err == nil {
                    t.Error("Expected error, got nil")
                }
                return
            }
            
            if err != nil {
                t.Errorf("Unexpected error: %v", err)
                return
            }
            
            if user.ID == 0 {
                t.Error("Expected user ID to be set")
            }
        })
    }
}
```

## Integration Testing

### HTTP Handler Testing

```go
// user_handler_test.go
package handler

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestUserHandler_CreateUser(t *testing.T) {
    testDB := setupTestDB(t)
    handler := NewUserHandler(testDB.DB)
    
    requestBody := CreateUserRequest{
        Name:  "John Doe",
        Email: "john@example.com",
    }
    
    body, _ := json.Marshal(requestBody)
    req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBuffer(body))
    req.Header.Set("Content-Type", "application/json")
    
    w := httptest.NewRecorder()
    handler.CreateUser(w, req)
    
    if w.Code != http.StatusCreated {
        t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
    }
    
    var response CreateUserResponse
    err := json.NewDecoder(w.Body).Decode(&response)
    if err != nil {
        t.Fatal("Failed to decode response:", err)
    }
    
    if response.ID == 0 {
        t.Error("Expected user ID in response")
    }
}
```

### Full Application Testing

```go
// app_test.go
package main

import (
    "net/http"
    "testing"
)

func TestApp_UserWorkflow(t *testing.T) {
    // Setup test application
    app := setupTestApp(t)
    defer app.Shutdown()
    
    // Test complete user workflow
    t.Run("create user", func(t *testing.T) {
        resp := app.POST("/users", map[string]string{
            "name":  "John Doe",
            "email": "john@example.com",
        })
        
        if resp.StatusCode != http.StatusCreated {
            t.Errorf("Expected status %d, got %d", http.StatusCreated, resp.StatusCode)
        }
    })
    
    t.Run("get user", func(t *testing.T) {
        resp := app.GET("/users/1")
        
        if resp.StatusCode != http.StatusOK {
            t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
        }
    })
    
    t.Run("update user", func(t *testing.T) {
        resp := app.PUT("/users/1", map[string]string{
            "name": "John Smith",
        })
        
        if resp.StatusCode != http.StatusOK {
            t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
        }
    })
}
```

## Golden Testing

### Performance Regression Testing

```go
// user_repository_golden_test.go
package repository

import (
    "testing"
)

func TestUserRepository_GetRecentUsers_Performance(t *testing.T) {
    testDB := setupTestDB(t)
    
    // Enable golden testing for performance regression detection
    db := testDB.EnableGolden(t, "TestUserRepository_GetRecentUsers_Performance")
    
    repo := NewUserRepository(db)
    
    // Create test data
    createTestUsers(t, testDB, 1000)
    
    // This query will have its EXPLAIN plan captured
    users, err := repo.GetRecentUsers(context.Background(), 100)
    if err != nil {
        t.Fatal("Failed to get recent users:", err)
    }
    
    if len(users) == 0 {
        t.Error("Expected users to be returned")
    }
    
    // Additional complex query - will create separate golden file
    activeUsers, err := repo.GetActiveUsers(context.Background())
    if err != nil {
        t.Fatal("Failed to get active users:", err)
    }
    
    if len(activeUsers) == 0 {
        t.Error("Expected active users to be returned")
    }
}
```

### Query Plan Analysis

```go
func TestComplexQueries_Performance(t *testing.T) {
    testDB := setupTestDB(t)
    db := testDB.EnableGolden(t, "TestComplexQueries_Performance")
    
    // Test complex join query
    rows, err := db.Query(context.Background(), `
        SELECT u.id, u.name, COUNT(o.id) as order_count
        FROM users u
        LEFT JOIN orders o ON u.id = o.user_id
        WHERE u.active = true
        GROUP BY u.id, u.name
        HAVING COUNT(o.id) > 5
        ORDER BY order_count DESC
        LIMIT 50
    `)
    if err != nil {
        t.Fatal("Query failed:", err)
    }
    defer rows.Close()
    
    // Test subquery performance
    rows2, err := db.Query(context.Background(), `
        SELECT * FROM users
        WHERE id IN (
            SELECT user_id FROM orders
            WHERE created_at > NOW() - INTERVAL '30 days'
            GROUP BY user_id
            HAVING COUNT(*) > 10
        )
    `)
    if err != nil {
        t.Fatal("Subquery failed:", err)
    }
    defer rows2.Close()
    
    // Golden files will be created:
    // - testdata/golden/TestComplexQueries_Performance_query_1.json
    // - testdata/golden/TestComplexQueries_Performance_query_2.json
}
```

## Testing Patterns

### Table-Driven Tests

```go
func TestUserValidation(t *testing.T) {
    testDB := setupTestDB(t)
    service := NewUserService(testDB.DB)
    
    tests := []struct {
        name    string
        user    User
        wantErr string
    }{
        {
            name: "valid user",
            user: User{Name: "John Doe", Email: "john@example.com"},
            wantErr: "",
        },
        {
            name: "empty name",
            user: User{Name: "", Email: "john@example.com"},
            wantErr: "name is required",
        },
        {
            name: "invalid email",
            user: User{Name: "John Doe", Email: "invalid"},
            wantErr: "invalid email format",
        },
        {
            name: "long name",
            user: User{Name: strings.Repeat("a", 256), Email: "john@example.com"},
            wantErr: "name too long",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := service.ValidateUser(tt.user)
            
            if tt.wantErr == "" {
                if err != nil {
                    t.Errorf("Expected no error, got: %v", err)
                }
                return
            }
            
            if err == nil {
                t.Error("Expected error, got nil")
                return
            }
            
            if !strings.Contains(err.Error(), tt.wantErr) {
                t.Errorf("Expected error containing '%s', got: %v", tt.wantErr, err)
            }
        })
    }
}
```

### Parallel Testing

```go
func TestUserOperations_Parallel(t *testing.T) {
    tests := []struct {
        name string
        fn   func(t *testing.T)
    }{
        {"create", testCreateUser},
        {"update", testUpdateUser},
        {"delete", testDeleteUser},
        {"list", testListUsers},
    }
    
    for _, tt := range tests {
        tt := tt // Capture loop variable
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel() // Run tests in parallel
            tt.fn(t)
        })
    }
}

func testCreateUser(t *testing.T) {
    testDB := setupTestDB(t) // Each test gets its own DB
    service := NewUserService(testDB.DB)
    
    user, err := service.CreateUser(context.Background(), CreateUserRequest{
        Name:  "John Doe",
        Email: "john@example.com",
    })
    
    if err != nil {
        t.Fatal("Failed to create user:", err)
    }
    
    if user.ID == 0 {
        t.Error("Expected user ID to be set")
    }
}
```

## Test Data Management

### Test Fixtures

```go
// fixtures.go
package testdata

import (
    "context"
    "testing"
    
    "github.com/nhalm/pgxkit"
)

type UserFixture struct {
    ID    int
    Name  string
    Email string
}

func CreateUserFixtures(t *testing.T, db *pgxkit.TestDB) []UserFixture {
    fixtures := []UserFixture{
        {Name: "John Doe", Email: "john@example.com"},
        {Name: "Jane Smith", Email: "jane@example.com"},
        {Name: "Bob Johnson", Email: "bob@example.com"},
    }
    
    for i := range fixtures {
        err := db.QueryRow(context.Background(),
            "INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id",
            fixtures[i].Name, fixtures[i].Email).Scan(&fixtures[i].ID)
        if err != nil {
            t.Fatal("Failed to create user fixture:", err)
        }
    }
    
    return fixtures
}
```

### Test Data Builders

```go
// builders.go
package testdata

type UserBuilder struct {
    name  string
    email string
    active bool
}

func NewUserBuilder() *UserBuilder {
    return &UserBuilder{
        name:   "Test User",
        email:  "test@example.com",
        active: true,
    }
}

func (b *UserBuilder) WithName(name string) *UserBuilder {
    b.name = name
    return b
}

func (b *UserBuilder) WithEmail(email string) *UserBuilder {
    b.email = email
    return b
}

func (b *UserBuilder) Inactive() *UserBuilder {
    b.active = false
    return b
}

func (b *UserBuilder) Build() User {
    return User{
        Name:   b.name,
        Email:  b.email,
        Active: b.active,
    }
}

func (b *UserBuilder) Create(t *testing.T, db *pgxkit.TestDB) User {
    user := b.Build()
    
    err := db.QueryRow(context.Background(),
        "INSERT INTO users (name, email, active) VALUES ($1, $2, $3) RETURNING id",
        user.Name, user.Email, user.Active).Scan(&user.ID)
    if err != nil {
        t.Fatal("Failed to create user:", err)
    }
    
    return user
}

// Usage in tests
func TestUserService_GetActiveUsers(t *testing.T) {
    testDB := setupTestDB(t)
    service := NewUserService(testDB.DB)
    
    // Create test data using builders
    activeUser := NewUserBuilder().WithName("Active User").Create(t, testDB)
    inactiveUser := NewUserBuilder().WithName("Inactive User").Inactive().Create(t, testDB)
    
    users, err := service.GetActiveUsers(context.Background())
    if err != nil {
        t.Fatal("Failed to get active users:", err)
    }
    
    // Should only return active user
    if len(users) != 1 {
        t.Errorf("Expected 1 active user, got %d", len(users))
    }
    
    if users[0].ID != activeUser.ID {
        t.Errorf("Expected active user ID %d, got %d", activeUser.ID, users[0].ID)
    }
}
```

## Performance Testing

### Benchmark Tests

```go
// user_repository_bench_test.go
package repository

import (
    "context"
    "testing"
)

func BenchmarkUserRepository_GetByID(b *testing.B) {
    testDB := setupTestDB(b)
    repo := NewUserRepository(testDB.DB)
    
    // Create test user
    userID := createTestUser(b, testDB, "Benchmark User", "bench@example.com")
    
    ctx := context.Background()
    
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        _, err := repo.GetByID(ctx, userID)
        if err != nil {
            b.Fatal("Failed to get user:", err)
        }
    }
}

func BenchmarkUserRepository_GetRecentUsers(b *testing.B) {
    testDB := setupTestDB(b)
    repo := NewUserRepository(testDB.DB)
    
    // Create test data
    createTestUsers(b, testDB, 10000)
    
    ctx := context.Background()
    
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        users, err := repo.GetRecentUsers(ctx, 100)
        if err != nil {
            b.Fatal("Failed to get recent users:", err)
        }
        
        if len(users) == 0 {
            b.Error("Expected users to be returned")
        }
    }
}
```

### Load Testing

```go
func TestUserService_ConcurrentAccess(t *testing.T) {
    testDB := setupTestDB(t)
    service := NewUserService(testDB.DB)
    
    const numGoroutines = 100
    const operationsPerGoroutine = 10
    
    var wg sync.WaitGroup
    errors := make(chan error, numGoroutines*operationsPerGoroutine)
    
    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            
            for j := 0; j < operationsPerGoroutine; j++ {
                user, err := service.CreateUser(context.Background(), CreateUserRequest{
                    Name:  fmt.Sprintf("User %d-%d", workerID, j),
                    Email: fmt.Sprintf("user%d-%d@example.com", workerID, j),
                })
                
                if err != nil {
                    errors <- err
                    return
                }
                
                // Verify user was created
                _, err = service.GetUser(context.Background(), user.ID)
                if err != nil {
                    errors <- err
                    return
                }
            }
        }(i)
    }
    
    wg.Wait()
    close(errors)
    
    // Check for errors
    for err := range errors {
        t.Error("Concurrent operation failed:", err)
    }
}
```

## Error Testing

### Database Error Handling

```go
func TestUserRepository_HandleDatabaseErrors(t *testing.T) {
    testDB := setupTestDB(t)
    repo := NewUserRepository(testDB.DB)
    
    t.Run("connection error", func(t *testing.T) {
        // Close database connection to simulate error
        testDB.DB.Shutdown(context.Background())
        
        _, err := repo.GetByID(context.Background(), 1)
        if err == nil {
            t.Error("Expected error when database is closed")
        }
    })
    
    t.Run("constraint violation", func(t *testing.T) {
        testDB := setupTestDB(t)
        repo := NewUserRepository(testDB.DB)
        
        // Create user
        user := &User{Name: "John Doe", Email: "john@example.com"}
        err := repo.Create(context.Background(), user)
        if err != nil {
            t.Fatal("Failed to create user:", err)
        }
        
        // Try to create duplicate
        duplicate := &User{Name: "Jane Doe", Email: "john@example.com"}
        err = repo.Create(context.Background(), duplicate)
        
        // Should return ValidationError
        var validationErr *pgxkit.ValidationError
        if !errors.As(err, &validationErr) {
            t.Errorf("Expected ValidationError, got %T", err)
        }
    })
}
```

### Timeout Testing

```go
func TestUserService_Timeouts(t *testing.T) {
    testDB := setupTestDB(t)
    service := NewUserService(testDB.DB)
    
    // Create context with very short timeout
    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
    defer cancel()
    
    _, err := service.GetUsers(ctx)
    
    if err == nil {
        t.Error("Expected timeout error")
    }
    
    if !errors.Is(err, context.DeadlineExceeded) {
        t.Errorf("Expected context.DeadlineExceeded, got %v", err)
    }
}
```

## Best Practices

### Test Organization

```go
// Good: Organized test structure
func TestUserRepository(t *testing.T) {
    testDB := setupTestDB(t)
    repo := NewUserRepository(testDB.DB)
    
    t.Run("Create", func(t *testing.T) {
        t.Run("valid user", func(t *testing.T) {
            // Test implementation
        })
        
        t.Run("invalid email", func(t *testing.T) {
            // Test implementation
        })
    })
    
    t.Run("GetByID", func(t *testing.T) {
        t.Run("existing user", func(t *testing.T) {
            // Test implementation
        })
        
        t.Run("non-existent user", func(t *testing.T) {
            // Test implementation
        })
    })
}
```

### Test Assertions

```go
// Good: Clear, specific assertions
func TestUserService_CreateUser(t *testing.T) {
    testDB := setupTestDB(t)
    service := NewUserService(testDB.DB)
    
    user, err := service.CreateUser(context.Background(), CreateUserRequest{
        Name:  "John Doe",
        Email: "john@example.com",
    })
    
    // Check error first
    if err != nil {
        t.Fatal("Failed to create user:", err)
    }
    
    // Specific assertions
    if user.ID == 0 {
        t.Error("Expected user ID to be set")
    }
    
    if user.Name != "John Doe" {
        t.Errorf("Expected name 'John Doe', got '%s'", user.Name)
    }
    
    if user.Email != "john@example.com" {
        t.Errorf("Expected email 'john@example.com', got '%s'", user.Email)
    }
    
    if user.CreatedAt.IsZero() {
        t.Error("Expected CreatedAt to be set")
    }
}
```

### Test Documentation

```go
// Good: Well-documented test
func TestUserService_CreateUser_DuplicateEmail(t *testing.T) {
    // This test verifies that creating a user with an email that already
    // exists in the database returns a ValidationError with appropriate
    // error message and does not create a duplicate user.
    
    testDB := setupTestDB(t)
    service := NewUserService(testDB.DB)
    
    // Create initial user
    _, err := service.CreateUser(context.Background(), CreateUserRequest{
        Name:  "John Doe",
        Email: "john@example.com",
    })
    if err != nil {
        t.Fatal("Failed to create initial user:", err)
    }
    
    // Attempt to create duplicate
    _, err = service.CreateUser(context.Background(), CreateUserRequest{
        Name:  "Jane Doe",
        Email: "john@example.com", // Same email
    })
    
    // Verify error type and message
    var validationErr *pgxkit.ValidationError
    if !errors.As(err, &validationErr) {
        t.Fatalf("Expected ValidationError, got %T", err)
    }
    
    if validationErr.Field != "email" {
        t.Errorf("Expected field 'email', got '%s'", validationErr.Field)
    }
    
    if !strings.Contains(validationErr.Reason, "already exists") {
        t.Errorf("Expected reason to contain 'already exists', got '%s'", validationErr.Reason)
    }
}
```

## Summary

### Key Testing Principles

1. **Use real databases** - Don't mock database interactions
2. **Isolate test data** - Each test should have clean state
3. **Test error conditions** - Verify proper error handling
4. **Use golden tests** - Catch performance regressions
5. **Write clear assertions** - Make test failures easy to understand
6. **Keep tests fast** - Use efficient setup/teardown
7. **Test concurrency** - Verify thread safety
8. **Document complex tests** - Explain the purpose and expectations

### Testing Checklist

- [ ] Test database setup and teardown
- [ ] Unit tests for repository layer
- [ ] Integration tests for service layer
- [ ] End-to-end tests for handlers
- [ ] Golden tests for performance
- [ ] Error condition testing
- [ ] Concurrent access testing
- [ ] Benchmark tests for critical paths
- [ ] Test data management strategy
- [ ] CI/CD integration

Following these practices will help you build robust, reliable applications with pgxkit while maintaining fast, maintainable test suites. 