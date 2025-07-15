package dbutil

import (
	"context"
	"log"
	"os"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// Shared test database connection for all integration tests
	testDBPool *pgxpool.Pool
	testDBOnce sync.Once
)

// GetTestConnection returns a shared test database connection, initializing it once
// This is a generic function that must be called with the appropriate type parameter
func GetTestConnection[T Querier](newQueriesFunc func(*pgxpool.Pool) T) *Connection[T] {
	testDBOnce.Do(func() {
		testDBPool = initTestDatabasePool()
	})

	if testDBPool == nil {
		return nil
	}

	return &Connection[T]{
		pool:    testDBPool,
		queries: newQueriesFunc(testDBPool),
		metrics: nil,
	}
}

// initTestDatabasePool sets up the test database pool once
func initTestDatabasePool() *pgxpool.Pool {
	// Get test database URL from environment
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		log.Printf("TEST_DATABASE_URL not set, integration tests will be skipped")
		return nil
	}

	ctx := context.Background()
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatalf("Failed to parse test database URL: %v", err)
	}

	// Set test-specific pool configuration
	config.MaxConns = 5
	config.MinConns = 1

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Fatalf("Failed to connect to test database: %v", err)
	}

	// Verify connection works
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Failed to ping test database: %v", err)
	}

	log.Printf("Test database pool initialized successfully")
	return pool
}

// CleanupTestData executes cleanup SQL statements
// This is a generic cleanup utility that takes SQL statements as parameters
func CleanupTestData[T Querier](conn *Connection[T], sqlStatements ...string) {
	if conn == nil {
		return
	}

	ctx := context.Background()
	pool := conn.GetDB()

	for _, sql := range sqlStatements {
		_, err := pool.Exec(ctx, sql)
		if err != nil {
			log.Printf("Warning: Failed to cleanup test data with SQL '%s': %v", sql, err)
		}
	}
}

// RequireTestDB ensures a test database is available or skips the test
func RequireTestDB[T Querier](t TestingT, newQueriesFunc func(*pgxpool.Pool) T) *Connection[T] {
	conn := GetTestConnection(newQueriesFunc)
	if conn == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}
	return conn
}

// TestingT is an interface that matches both *testing.T and *testing.B
type TestingT interface {
	Skip(args ...interface{})
	Logf(format string, args ...interface{})
}
