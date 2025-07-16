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

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestDB is just an embedded DB with 3 simple methods
type TestDB struct {
	*DB
}

// NewTestDB creates a new TestDB instance with the provided pool
func NewTestDB(pool *pgxpool.Pool) *TestDB {
	return &TestDB{DB: NewDBWithPool(pool)}
}

// Setup prepares the database for testing
func (tdb *TestDB) Setup() error {
	// This method can be extended to run migrations, seed data, etc.
	// For now, it's a placeholder that ensures the database is ready
	ctx := context.Background()

	// Verify connection is working
	if tdb.writePool == nil {
		return fmt.Errorf("no database pool available")
	}

	// Test connection
	err := tdb.writePool.Ping(ctx)
	if err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	return nil
}

// Clean cleans the database after the test
func (tdb *TestDB) Clean() error {
	// This method can be extended to truncate tables, reset sequences, etc.
	// For now, it's a placeholder for cleanup operations
	ctx := context.Background()

	if tdb.writePool == nil {
		return nil // No connection to clean
	}

	// Example cleanup operations (can be customized per project)
	// _, err := tdb.writePool.Exec(ctx, "TRUNCATE TABLE users CASCADE")
	// if err != nil {
	//     return fmt.Errorf("failed to clean users table: %w", err)
	// }

	// For now, just verify the connection is still valid
	err := tdb.writePool.Ping(ctx)
	if err != nil {
		return fmt.Errorf("database connection lost during cleanup: %w", err)
	}

	return nil
}

// EnableGolden returns a new DB with golden test hooks added
func (tdb *TestDB) EnableGolden(t *testing.T, testName string) *DB {
	// Create a new DB instance with the same pools
	goldenDB := &DB{
		readPool:  tdb.readPool,
		writePool: tdb.writePool,
		hooks:     NewHooks(),
	}

	// Create golden test hook with access to the DB
	goldenHook := &goldenTestHook{
		t:            t,
		testName:     testName,
		queryCounter: 0,
		mu:           sync.Mutex{},
		db:           goldenDB,
	}

	// Add the golden test hook to capture EXPLAIN plans
	err := goldenDB.AddHook("BeforeOperation", goldenHook.captureExplainPlan)
	if err != nil {
		t.Fatalf("Failed to add golden test hook: %v", err)
	}

	return goldenDB
}

// goldenTestHook handles golden test functionality
type goldenTestHook struct {
	t            *testing.T
	testName     string
	queryCounter int
	mu           sync.Mutex
	db           *DB
}

// captureExplainPlan captures EXPLAIN (ANALYZE, BUFFERS) plans for queries
func (g *goldenTestHook) captureExplainPlan(ctx context.Context, sql string, args []interface{}, operationErr error) error {
	if g.t == nil || g.db == nil {
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

	g.mu.Lock()
	g.queryCounter++
	currentQuery := g.queryCounter
	g.mu.Unlock()

	// Create EXPLAIN query
	explainSQL := fmt.Sprintf("EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) %s", sql)

	// Check if we have a valid connection
	if g.db.writePool == nil {
		// No connection available, skip golden test capture
		g.t.Logf("Warning: No database connection available for golden test capture")
		return nil
	}

	// Execute EXPLAIN query
	rows, err := g.db.writePool.Query(ctx, explainSQL, args...)
	if err != nil {
		// Log error but don't fail the test - golden tests are optional
		g.t.Logf("Warning: Failed to capture EXPLAIN plan for query %d: %v", currentQuery, err)
		return nil
	}
	defer rows.Close()

	// Read EXPLAIN result
	var explainResult string
	if rows.Next() {
		err = rows.Scan(&explainResult)
		if err != nil {
			g.t.Logf("Warning: Failed to scan EXPLAIN result for query %d: %v", currentQuery, err)
			return nil
		}
	}

	// Parse JSON to validate and pretty-print
	var explainData interface{}
	if err := json.Unmarshal([]byte(explainResult), &explainData); err != nil {
		g.t.Logf("Warning: Failed to parse EXPLAIN JSON for query %d: %v", currentQuery, err)
		return nil
	}

	// Pretty-print JSON
	prettyJSON, err := json.MarshalIndent(explainData, "", "  ")
	if err != nil {
		g.t.Logf("Warning: Failed to marshal EXPLAIN JSON for query %d: %v", currentQuery, err)
		return nil
	}

	// Save to golden file
	goldenFile := fmt.Sprintf("testdata/golden/%s_query_%d.json", g.testName, currentQuery)
	err = g.saveGoldenFile(goldenFile, prettyJSON)
	if err != nil {
		g.t.Logf("Warning: Failed to save golden file for query %d: %v", currentQuery, err)
	}

	return nil
}

// saveGoldenFile saves the golden file, creating directories as needed
func (g *goldenTestHook) saveGoldenFile(filename string, data []byte) error {
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
			g.t.Errorf("Query plan regression detected in %s\nExpected plan differs from actual plan", filename)
			// TODO: Add detailed diff output
		}

		return nil
	}

	// File doesn't exist, create it
	err := os.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write golden file %s: %w", filename, err)
	}

	g.t.Logf("Created golden file: %s", filename)
	return nil
}

// RequireDB ensures a test database is available or skips the test
func RequireDB(t *testing.T) *TestDB {
	pool := GetTestPool()
	if pool == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping test")
		return nil
	}

	testDB := NewTestDB(pool)
	return testDB
}

// CleanupGolden removes all golden files for a test
func CleanupGolden(testName string) error {
	if testName == "" {
		return nil
	}

	pattern := fmt.Sprintf("testdata/golden/%s_query_*.json", testName)
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
