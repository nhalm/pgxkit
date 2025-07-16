package pgxkit

import (
	"context"
	"testing"
)

func TestRequireTestPool(t *testing.T) {
	// This test requires TEST_DATABASE_URL to be set
	pool := requireTestPool(t)
	if pool == nil {
		// Test was skipped, which is fine
		return
	}
}
func TestGetTestPool(t *testing.T) {
	// This test requires TEST_DATABASE_URL to be set
	pool := getTestPool()
	if pool == nil {
		// No test database available, skip
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	// Test that subsequent calls return the same pool
	pool2 := getTestPool()
	if pool2 == nil {
		t.Error("Expected second call to return pool")
	}

	if pool != pool2 {
		t.Error("Expected shared pool between calls")
	}
}

func TestCleanupTestData(t *testing.T) {
	// Test cleanup with valid SQL (should not error)
	CleanupTestData("SELECT 1", "SELECT 2")

	// Test cleanup with invalid SQL (should not fail the test, just log warnings)
	CleanupTestData("INVALID SQL STATEMENT")

	// Test cleanup when no test database is available (should not panic)
	// This is tested implicitly - if TEST_DATABASE_URL is not set, it just returns
}

func TestPoolHealthCheck(t *testing.T) {
	pool := getTestPool()
	if pool == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	ctx := context.Background()

	// Test pool ping (health check)
	err := pool.Ping(ctx)
	if err != nil {
		t.Errorf("Expected pool ping to pass, got error: %v", err)
	}

	// Test pool stats (readiness check)
	stats := pool.Stat()
	if stats == nil {
		t.Error("Expected pool stats to be available")
	}
}
