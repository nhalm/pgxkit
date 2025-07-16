package pgxkit

import (
	"context"
	"log"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// Shared test database pool for all integration tests
	testDBPool *pgxpool.Pool
	testDBOnce sync.Once
)

// GetTestPool returns a shared test database pool, initializing it once
func GetTestPool() *pgxpool.Pool {
	testDBOnce.Do(func() {
		testDBPool = initTestDatabasePool()
	})
	return testDBPool
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

// RequireTestPool ensures a test database pool is available or skips the test
func RequireTestPool(t *testing.T) *pgxpool.Pool {
	pool := GetTestPool()
	if pool == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}
	return pool
}

// CleanupTestData executes cleanup SQL statements on a pool
func CleanupTestData(pool *pgxpool.Pool, sqlStatements ...string) {
	if pool == nil {
		return
	}

	ctx := context.Background()
	for _, sql := range sqlStatements {
		_, err := pool.Exec(ctx, sql)
		if err != nil {
			log.Printf("Warning: Failed to cleanup test data with SQL '%s': %v", sql, err)
		}
	}
}
