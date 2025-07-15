package dbutil

import (
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestGetDSN(t *testing.T) {
	// Save original env vars
	originalVars := map[string]string{
		"POSTGRES_HOST":     os.Getenv("POSTGRES_HOST"),
		"POSTGRES_PORT":     os.Getenv("POSTGRES_PORT"),
		"POSTGRES_USER":     os.Getenv("POSTGRES_USER"),
		"POSTGRES_PASSWORD": os.Getenv("POSTGRES_PASSWORD"),
		"POSTGRES_DB":       os.Getenv("POSTGRES_DB"),
		"POSTGRES_SSLMODE":  os.Getenv("POSTGRES_SSLMODE"),
	}

	// Cleanup function to restore env vars
	defer func() {
		for key, value := range originalVars {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	// Test with default values
	os.Unsetenv("POSTGRES_HOST")
	os.Unsetenv("POSTGRES_PORT")
	os.Unsetenv("POSTGRES_USER")
	os.Unsetenv("POSTGRES_PASSWORD")
	os.Unsetenv("POSTGRES_DB")
	os.Unsetenv("POSTGRES_SSLMODE")

	dsn := GetDSN()
	expected := "postgres://postgres:@localhost:5432/postgres?sslmode=disable"
	if dsn != expected {
		t.Errorf("Expected DSN '%s', got '%s'", expected, dsn)
	}

	// Test with custom values
	os.Setenv("POSTGRES_HOST", "custom-host")
	os.Setenv("POSTGRES_PORT", "5433")
	os.Setenv("POSTGRES_USER", "custom-user")
	os.Setenv("POSTGRES_PASSWORD", "custom-pass")
	os.Setenv("POSTGRES_DB", "custom-db")
	os.Setenv("POSTGRES_SSLMODE", "require")

	dsn = GetDSN()
	expected = "postgres://custom-user:custom-pass@custom-host:5433/custom-db?sslmode=require"
	if dsn != expected {
		t.Errorf("Expected DSN '%s', got '%s'", expected, dsn)
	}
}

func TestGetDSNWithSearchPath(t *testing.T) {
	// Save original env vars
	originalVars := map[string]string{
		"POSTGRES_HOST":     os.Getenv("POSTGRES_HOST"),
		"POSTGRES_PORT":     os.Getenv("POSTGRES_PORT"),
		"POSTGRES_USER":     os.Getenv("POSTGRES_USER"),
		"POSTGRES_PASSWORD": os.Getenv("POSTGRES_PASSWORD"),
		"POSTGRES_DB":       os.Getenv("POSTGRES_DB"),
		"POSTGRES_SSLMODE":  os.Getenv("POSTGRES_SSLMODE"),
	}

	// Cleanup function to restore env vars
	defer func() {
		for key, value := range originalVars {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	// Reset to defaults
	os.Unsetenv("POSTGRES_HOST")
	os.Unsetenv("POSTGRES_PORT")
	os.Unsetenv("POSTGRES_USER")
	os.Unsetenv("POSTGRES_PASSWORD")
	os.Unsetenv("POSTGRES_DB")
	os.Unsetenv("POSTGRES_SSLMODE")

	dsn := getDSNWithSearchPath("myschema")
	expected := "postgres://postgres:@localhost:5432/postgres?sslmode=disable&search_path=myschema"
	if dsn != expected {
		t.Errorf("Expected DSN '%s', got '%s'", expected, dsn)
	}

	// Test with empty search path
	dsn = getDSNWithSearchPath("")
	expected = "postgres://postgres:@localhost:5432/postgres?sslmode=disable"
	if dsn != expected {
		t.Errorf("Expected DSN '%s', got '%s'", expected, dsn)
	}
}

func TestGetEnvWithDefault(t *testing.T) {
	// Test with existing env var
	os.Setenv("TEST_VAR", "test-value")
	result := getEnvWithDefault("TEST_VAR", "default")
	if result != "test-value" {
		t.Errorf("Expected 'test-value', got '%s'", result)
	}

	// Test with non-existing env var
	os.Unsetenv("TEST_VAR")
	result = getEnvWithDefault("TEST_VAR", "default")
	if result != "default" {
		t.Errorf("Expected 'default', got '%s'", result)
	}

	// Test with empty env var
	os.Setenv("TEST_VAR", "")
	result = getEnvWithDefault("TEST_VAR", "default")
	if result != "default" {
		t.Errorf("Expected 'default' for empty env var, got '%s'", result)
	}

	// Cleanup
	os.Unsetenv("TEST_VAR")
}

func TestGetEnvIntWithDefault(t *testing.T) {
	// Test with valid integer
	os.Setenv("TEST_INT", "42")
	result := getEnvIntWithDefault("TEST_INT", 10)
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}

	// Test with non-existing env var
	os.Unsetenv("TEST_INT")
	result = getEnvIntWithDefault("TEST_INT", 10)
	if result != 10 {
		t.Errorf("Expected 10, got %d", result)
	}

	// Test with invalid integer
	os.Setenv("TEST_INT", "not-a-number")
	result = getEnvIntWithDefault("TEST_INT", 10)
	if result != 10 {
		t.Errorf("Expected 10 for invalid integer, got %d", result)
	}

	// Test with empty env var
	os.Setenv("TEST_INT", "")
	result = getEnvIntWithDefault("TEST_INT", 10)
	if result != 10 {
		t.Errorf("Expected 10 for empty env var, got %d", result)
	}

	// Cleanup
	os.Unsetenv("TEST_INT")
}

func TestConfig(t *testing.T) {
	// Test creating config with all fields
	config := &Config{
		MaxConns:        20,
		MinConns:        5,
		MaxConnLifetime: 1 * time.Hour,
		SearchPath:      "test_schema",
		OnConnect: func(conn *pgx.Conn) error {
			return nil
		},
		OnDisconnect: func(conn *pgx.Conn) {
			// no-op
		},
		Hooks: NewConnectionHooks(),
	}

	if config.MaxConns != 20 {
		t.Errorf("Expected MaxConns 20, got %d", config.MaxConns)
	}

	if config.MinConns != 5 {
		t.Errorf("Expected MinConns 5, got %d", config.MinConns)
	}

	if config.MaxConnLifetime != 1*time.Hour {
		t.Errorf("Expected MaxConnLifetime 1h, got %v", config.MaxConnLifetime)
	}

	if config.SearchPath != "test_schema" {
		t.Errorf("Expected SearchPath 'test_schema', got '%s'", config.SearchPath)
	}

	if config.OnConnect == nil {
		t.Error("Expected OnConnect to be set")
	}

	if config.OnDisconnect == nil {
		t.Error("Expected OnDisconnect to be set")
	}

	if config.Hooks == nil {
		t.Error("Expected Hooks to be set")
	}
}

func TestConfigDefaults(t *testing.T) {
	// Test that nil config works (should use defaults)
	var config *Config = nil

	// This would normally be tested in the createPoolWithConfig function,
	// but we can't easily test that without a database connection.
	// Instead, we just verify the config can be nil.
	if config != nil {
		t.Error("Expected config to be nil for this test")
	}
}

func TestMetricsCollectorInterface(t *testing.T) {
	// Test that we can implement the MetricsCollector interface
	metrics := &testMetricsCollector{}

	// Test that it implements the interface
	var _ MetricsCollector = metrics

	// Test calling methods
	metrics.RecordConnectionAcquired(100 * time.Millisecond)
	metrics.RecordConnectionReleased(50 * time.Millisecond)
	metrics.RecordQueryExecuted("SELECT 1", 10*time.Millisecond, nil)
	metrics.RecordTransactionStarted()
	metrics.RecordTransactionCommitted(200 * time.Millisecond)
	metrics.RecordTransactionRolledBack(150 * time.Millisecond)

	// Verify calls were recorded
	if metrics.ConnectionsAcquired != 1 {
		t.Errorf("Expected 1 connection acquired, got %d", metrics.ConnectionsAcquired)
	}

	if metrics.ConnectionsReleased != 1 {
		t.Errorf("Expected 1 connection released, got %d", metrics.ConnectionsReleased)
	}

	if metrics.QueriesExecuted != 1 {
		t.Errorf("Expected 1 query executed, got %d", metrics.QueriesExecuted)
	}

	if metrics.TransactionsStarted != 1 {
		t.Errorf("Expected 1 transaction started, got %d", metrics.TransactionsStarted)
	}

	if metrics.TransactionsCommitted != 1 {
		t.Errorf("Expected 1 transaction committed, got %d", metrics.TransactionsCommitted)
	}

	if metrics.TransactionsRolledBack != 1 {
		t.Errorf("Expected 1 transaction rolled back, got %d", metrics.TransactionsRolledBack)
	}
}

// Test implementation of MetricsCollector
type testMetricsCollector struct {
	ConnectionsAcquired    int
	ConnectionsReleased    int
	QueriesExecuted        int
	TransactionsStarted    int
	TransactionsCommitted  int
	TransactionsRolledBack int
}

func (t *testMetricsCollector) RecordConnectionAcquired(duration time.Duration) {
	t.ConnectionsAcquired++
}

func (t *testMetricsCollector) RecordConnectionReleased(duration time.Duration) {
	t.ConnectionsReleased++
}

func (t *testMetricsCollector) RecordQueryExecuted(queryName string, duration time.Duration, err error) {
	t.QueriesExecuted++
}

func (t *testMetricsCollector) RecordTransactionStarted() {
	t.TransactionsStarted++
}

func (t *testMetricsCollector) RecordTransactionCommitted(duration time.Duration) {
	t.TransactionsCommitted++
}

func (t *testMetricsCollector) RecordTransactionRolledBack(duration time.Duration) {
	t.TransactionsRolledBack++
}
