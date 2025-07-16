package pgxkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestDB wraps the main DB with testing infrastructure and golden test support
type TestDB struct {
	*DB
	goldenEnabled bool
	testName      string
	t             *testing.T
	queryCounter  int
	mu            sync.Mutex
}

// NewTestDB creates a new TestDB instance with test database setup
func NewTestDB() *TestDB {
	return NewTestDBWithConfig(nil)
}

// NewTestDBWithConfig creates a new TestDB instance with custom configuration
func NewTestDBWithConfig(config *DBConfig) *TestDB {
	db := NewDB()

	// Apply configuration if provided
	if config != nil {
		if config.Hooks != nil {
			db.hooks = config.Hooks
		}
	}

	return &TestDB{
		DB:            db,
		goldenEnabled: false,
		queryCounter:  0,
	}
}

// ConnectToTestDB connects to the test database using TEST_DATABASE_URL
func (tdb *TestDB) ConnectToTestDB(ctx context.Context) error {
	testDBURL := os.Getenv("TEST_DATABASE_URL")
	if testDBURL == "" {
		return fmt.Errorf("TEST_DATABASE_URL environment variable not set")
	}

	return tdb.Connect(ctx, testDBURL)
}

// EnableExplainGolden enables automatic EXPLAIN plan capture for query plan regression testing
func (tdb *TestDB) EnableExplainGolden(t *testing.T, testName string) {
	tdb.mu.Lock()
	defer tdb.mu.Unlock()

	tdb.goldenEnabled = true
	tdb.testName = testName
	tdb.t = t
	tdb.queryCounter = 0

	// Add hook to capture EXPLAIN plans for all queries
	err := tdb.AddHook("BeforeOperation", tdb.captureExplainPlan)
	if err != nil {
		t.Fatalf("Failed to add golden test hook: %v", err)
	}
}

// captureExplainPlan captures EXPLAIN (ANALYZE, BUFFERS) plans for queries
func (tdb *TestDB) captureExplainPlan(ctx context.Context, sql string, args []interface{}, operationErr error) error {
	if !tdb.goldenEnabled || tdb.t == nil {
		return nil
	}

	// Skip EXPLAIN queries to avoid infinite recursion
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "EXPLAIN") {
		return nil
	}

	// Skip non-SELECT queries (EXPLAIN is most useful for SELECT)
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "SELECT") {
		return nil
	}

	tdb.mu.Lock()
	tdb.queryCounter++
	currentQuery := tdb.queryCounter
	tdb.mu.Unlock()

	// Create EXPLAIN query
	explainSQL := fmt.Sprintf("EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) %s", sql)

	// Check if we have a valid connection
	if tdb.writePool == nil {
		// No connection available, skip golden test capture
		return nil
	}

	// Execute EXPLAIN query
	rows, err := tdb.writePool.Query(ctx, explainSQL, args...)
	if err != nil {
		// Log error but don't fail the test - golden tests are optional
		tdb.t.Logf("Warning: Failed to capture EXPLAIN plan for query %d: %v", currentQuery, err)
		return nil
	}
	defer rows.Close()

	// Read EXPLAIN result
	var explainResult string
	if rows.Next() {
		err = rows.Scan(&explainResult)
		if err != nil {
			tdb.t.Logf("Warning: Failed to scan EXPLAIN result for query %d: %v", currentQuery, err)
			return nil
		}
	}

	// Parse JSON to validate and pretty-print
	var explainData interface{}
	if err := json.Unmarshal([]byte(explainResult), &explainData); err != nil {
		tdb.t.Logf("Warning: Failed to parse EXPLAIN JSON for query %d: %v", currentQuery, err)
		return nil
	}

	// Pretty-print JSON
	prettyJSON, err := json.MarshalIndent(explainData, "", "  ")
	if err != nil {
		tdb.t.Logf("Warning: Failed to marshal EXPLAIN JSON for query %d: %v", currentQuery, err)
		return nil
	}

	// Save to golden file
	goldenFile := fmt.Sprintf("testdata/golden/%s_query_%d.json", tdb.testName, currentQuery)
	err = tdb.saveGoldenFile(goldenFile, prettyJSON)
	if err != nil {
		tdb.t.Logf("Warning: Failed to save golden file for query %d: %v", currentQuery, err)
	}

	return nil
}

// saveGoldenFile saves the golden file, creating directories as needed
func (tdb *TestDB) saveGoldenFile(filename string, data []byte) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Check if file already exists
	if _, err := os.Stat(filename); err == nil {
		// File exists, compare with new data
		existingData, err := os.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("failed to read existing golden file %s: %w", filename, err)
		}

		// Compare JSON content (ignoring whitespace differences)
		var existing, new interface{}
		if err := json.Unmarshal(existingData, &existing); err != nil {
			return fmt.Errorf("failed to parse existing golden file %s: %w", filename, err)
		}
		if err := json.Unmarshal(data, &new); err != nil {
			return fmt.Errorf("failed to parse new golden data: %w", err)
		}

		// Convert back to JSON for comparison
		existingJSON, _ := json.Marshal(existing)
		newJSON, _ := json.Marshal(new)

		if string(existingJSON) != string(newJSON) {
			tdb.t.Errorf("Query plan regression detected in %s\nExpected plan differs from actual plan", filename)
			// TODO: Add detailed diff output
		}

		return nil
	}

	// File doesn't exist, create it
	err := os.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write golden file %s: %w", filename, err)
	}

	tdb.t.Logf("Created golden file: %s", filename)
	return nil
}

// RequireTestDBWithGolden ensures a test database is available or skips the test
func RequireTestDBWithGolden(t *testing.T) *TestDB {
	testDB := NewTestDB()

	ctx := context.Background()
	err := testDB.ConnectToTestDB(ctx)
	if err != nil {
		t.Skipf("TEST_DATABASE_URL not set or connection failed, skipping test: %v", err)
		return nil
	}

	return testDB
}

// CleanupGoldenFiles removes all golden files for a test
func (tdb *TestDB) CleanupGoldenFiles() error {
	if tdb.testName == "" {
		return nil
	}

	pattern := fmt.Sprintf("testdata/golden/%s_query_*.json", tdb.testName)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to find golden files: %w", err)
	}

	for _, file := range matches {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("failed to remove golden file %s: %w", file, err)
		}
	}

	return nil
}

// Close closes the test database connection and optionally cleans up golden files
func (tdb *TestDB) Close() error {
	return tdb.Shutdown(context.Background())
}
