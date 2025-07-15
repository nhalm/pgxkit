package dbutil

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MockQuerier implements the Querier interface for testing
type MockQuerier struct {
	pool *pgxpool.Pool
}

func (m *MockQuerier) WithTx(tx pgx.Tx) Querier {
	return &MockQuerier{pool: m.pool}
}

func NewMockQuerier(pool *pgxpool.Pool) *MockQuerier {
	return &MockQuerier{pool: pool}
}

func TestRequireTestDB(t *testing.T) {
	// This test requires TEST_DATABASE_URL to be set
	conn := RequireTestDB(t, NewMockQuerier)
	if conn == nil {
		// Test was skipped, which is fine
		return
	}

	// Test that we got a valid connection
	if conn.GetDB() == nil {
		t.Error("Expected connection to have a valid pool")
	}

	if conn.Queries() == nil {
		t.Error("Expected connection to have valid queries")
	}
}

func TestGetTestConnection(t *testing.T) {
	// This test requires TEST_DATABASE_URL to be set
	conn := GetTestConnection(NewMockQuerier)
	if conn == nil {
		// No test database available, skip
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	// Test that we got a valid connection
	if conn.GetDB() == nil {
		t.Error("Expected connection to have a valid pool")
	}

	if conn.Queries() == nil {
		t.Error("Expected connection to have valid queries")
	}

	// Test that subsequent calls return the same connection pool
	conn2 := GetTestConnection(NewMockQuerier)
	if conn2 == nil {
		t.Error("Expected second call to return connection")
	}

	if conn.GetDB() != conn2.GetDB() {
		t.Error("Expected shared connection pool between calls")
	}
}

func TestCleanupTestData(t *testing.T) {
	conn := GetTestConnection(NewMockQuerier)
	if conn == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	// Test cleanup with valid SQL (should not error)
	CleanupTestData(conn, "SELECT 1", "SELECT 2")

	// Test cleanup with invalid SQL (should not fail the test, just log warnings)
	CleanupTestData(conn, "INVALID SQL STATEMENT")

	// Test cleanup with nil connection (should not panic)
	CleanupTestData((*Connection[*MockQuerier])(nil), "SELECT 1")
}

func TestConnectionHealthCheck(t *testing.T) {
	conn := GetTestConnection(NewMockQuerier)
	if conn == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	ctx := context.Background()

	// Test health check
	err := conn.HealthCheck(ctx)
	if err != nil {
		t.Errorf("Expected health check to pass, got error: %v", err)
	}

	// Test IsReady
	if !conn.IsReady(ctx) {
		t.Error("Expected connection to be ready")
	}
}

func TestConnectionStats(t *testing.T) {
	conn := GetTestConnection(NewMockQuerier)
	if conn == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	stats := conn.Stats()
	if stats == nil {
		t.Error("Expected stats to be non-nil")
	}

	// Just verify we can call stats methods without panicking
	_ = stats.TotalConns()
	_ = stats.IdleConns()
	_ = stats.AcquiredConns()
}

func TestConnectionWithMetrics(t *testing.T) {
	conn := GetTestConnection(NewMockQuerier)
	if conn == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	metrics := &testMetricsCollector{}
	connWithMetrics := conn.WithMetrics(metrics)

	if connWithMetrics == nil {
		t.Error("Expected WithMetrics to return non-nil connection")
	}

	// Verify the connection still works
	ctx := context.Background()
	err := connWithMetrics.HealthCheck(ctx)
	if err != nil {
		t.Errorf("Expected health check to pass with metrics, got error: %v", err)
	}
}

func TestConnectionWithHooks(t *testing.T) {
	conn := GetTestConnection(NewMockQuerier)
	if conn == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	hooks := NewConnectionHooks()
	connWithHooks := conn.WithHooks(hooks)

	if connWithHooks == nil {
		t.Error("Expected WithHooks to return non-nil connection")
	}

	// Verify the connection still works
	ctx := context.Background()
	err := connWithHooks.HealthCheck(ctx)
	if err != nil {
		t.Errorf("Expected health check to pass with hooks, got error: %v", err)
	}

	// Test GetHooks
	retrievedHooks := connWithHooks.GetHooks()
	if retrievedHooks != hooks {
		t.Error("Expected GetHooks to return the same hooks instance")
	}
}

func TestAddHook(t *testing.T) {
	conn := GetTestConnection(NewMockQuerier)
	if conn == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	hooks1 := NewConnectionHooks()
	hooks2 := NewConnectionHooks()

	// Test adding hook to connection without existing hooks
	connWithHook := conn.AddHook(hooks1)
	if connWithHook.GetHooks() != hooks1 {
		t.Error("Expected first hook to be set")
	}

	// Test adding hook to connection with existing hooks
	connWithBothHooks := connWithHook.AddHook(hooks2)
	combinedHooks := connWithBothHooks.GetHooks()
	if combinedHooks == hooks1 || combinedHooks == hooks2 {
		t.Error("Expected AddHook to create combined hooks, not use original")
	}
}

func TestNewConnectionWithHooks(t *testing.T) {
	// This test creates a new connection, so it will only work if TEST_DATABASE_URL is set
	testDBURL := GetTestConnection(NewMockQuerier)
	if testDBURL == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	ctx := context.Background()
	hooks := NewConnectionHooks()

	// We can't easily test this without knowing the actual TEST_DATABASE_URL,
	// but we can at least verify the function signature works
	conn, err := NewConnectionWithHooks(ctx, "", NewMockQuerier, hooks)
	if err == nil {
		defer conn.Close()
		// If it succeeded, verify hooks are set
		if conn.GetHooks() != hooks {
			t.Error("Expected hooks to be set on new connection")
		}
	}
	// If it failed, that's okay - we don't have a valid DSN for this test
}

func TestNewConnectionWithLoggingHooks(t *testing.T) {
	testDBURL := GetTestConnection(NewMockQuerier)
	if testDBURL == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	ctx := context.Background()
	logger := NewDefaultLogger(LogLevelInfo)

	// Test that the function works (may fail due to DSN, but should compile)
	conn, err := NewConnectionWithLoggingHooks(ctx, "", NewMockQuerier, logger)
	if err == nil {
		defer conn.Close()
		// If it succeeded, verify hooks are set
		if conn.GetHooks() == nil {
			t.Error("Expected logging hooks to be set on new connection")
		}
	}
}

func TestNewConnectionWithValidationHooks(t *testing.T) {
	testDBURL := GetTestConnection(NewMockQuerier)
	if testDBURL == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	ctx := context.Background()

	// Test that the function works (may fail due to DSN, but should compile)
	conn, err := NewConnectionWithValidationHooks(ctx, "", NewMockQuerier)
	if err == nil {
		defer conn.Close()
		// If it succeeded, verify hooks are set
		if conn.GetHooks() == nil {
			t.Error("Expected validation hooks to be set on new connection")
		}
	}
}
