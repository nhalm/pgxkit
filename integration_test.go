package pgxkit

import (
	"context"
	"testing"
)

func TestRequireTestPool(t *testing.T) {
	// This test requires TEST_DATABASE_URL to be set
	pool := RequireTestPool(t)
	if pool == nil {
		// Test was skipped, which is fine
		return
	}

	// Test that we got a valid pool
	if pool == nil {
		t.Error("Expected pool to be non-nil")
	}
}

func TestGetTestPool(t *testing.T) {
	// This test requires TEST_DATABASE_URL to be set
	pool := GetTestPool()
	if pool == nil {
		// No test database available, skip
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	// Test that we got a valid pool
	if pool == nil {
		t.Error("Expected pool to be non-nil")
	}

	// Test that subsequent calls return the same pool
	pool2 := GetTestPool()
	if pool2 == nil {
		t.Error("Expected second call to return pool")
	}

	if pool != pool2 {
		t.Error("Expected shared pool between calls")
	}
}

func TestCleanupTestData(t *testing.T) {
	pool := GetTestPool()
	if pool == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	// Test cleanup with valid SQL (should not error)
	CleanupTestData(pool, "SELECT 1", "SELECT 2")

	// Test cleanup with invalid SQL (should not fail the test, just log warnings)
	CleanupTestData(pool, "INVALID SQL STATEMENT")

	// Test cleanup with nil pool (should not panic)
	CleanupTestData(nil, "SELECT 1")
}

func TestPoolHealthCheck(t *testing.T) {
	pool := GetTestPool()
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
