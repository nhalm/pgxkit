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

// TestDB is a testing utility that wraps DB with testing-specific functionality.
// It provides simple methods for test setup, cleanup, and golden test support.
// TestDB automatically manages test database connections and provides utilities
// for performance regression testing through golden tests.
type TestDB struct {
	*DB
}

// NewTestDB creates a new unconnected TestDB instance.
// Call Connect() to establish the database connection.
//
// Example:
//
//	func TestUserOperations(t *testing.T) {
//	    testDB := pgxkit.NewTestDB()
//	    err := testDB.Connect(context.Background(), "") // uses TEST_DATABASE_URL env var
//	    if err != nil {
//	        t.Skip("Test database not available")
//	    }
//	    defer testDB.Shutdown(context.Background())
//	    // ... test code
//	}
func NewTestDB() *TestDB {
	return &TestDB{DB: NewDB()}
}

// Setup prepares the database for testing.
// This method verifies the database connection and can be extended
// to seed data or perform other test setup tasks.
// Returns an error if the database is not available or not ready for testing.
//
// Example:
//
//	err := testDB.Setup()
//	if err != nil {
//	    t.Skip("Test database not available")
//	}
func (tdb *TestDB) Setup() error {
	// This method can be extended to seed data, etc.
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
func (tdb *TestDB) EnableGolden(testName string) *DB {
	// Create a new DB instance with the same pools
	goldenDB := &DB{
		readPool:  tdb.readPool,
		writePool: tdb.writePool,
		hooks:     newHooks(),
	}

	// Create golden test hook
	goldenHook := &goldenTestHook{
		testName:     testName,
		queryCounter: 0,
		mu:           sync.Mutex{},
		db:           goldenDB,
	}

	// Add the golden test hook to capture EXPLAIN plans
	goldenDB.AddHook(BeforeOperation, goldenHook.captureExplainPlan)

	return goldenDB
}

// goldenTestHook handles golden test functionality
type goldenTestHook struct {
	testName     string
	queryCounter int
	mu           sync.Mutex
	db           *DB
}

// QueryPlan represents a captured query and its EXPLAIN plan
type QueryPlan struct {
	Query       int                      `json:"query"`
	SQL         string                   `json:"sql"`
	Plan        []map[string]interface{} `json:"plan"`
	ExecutionMS float64                  `json:"execution_ms,omitempty"`
	PlanningMS  float64                  `json:"planning_ms,omitempty"`
}

// captureExplainPlan captures EXPLAIN (ANALYZE, BUFFERS) plans for queries
func (g *goldenTestHook) captureExplainPlan(ctx context.Context, sql string, args []interface{}, operationErr error) error {
	if g.db == nil {
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
		return nil
	}

	// Execute EXPLAIN query
	rows, err := g.db.writePool.Query(ctx, explainSQL, args...)
	if err != nil {
		// Silently skip if EXPLAIN fails
		return nil
	}
	defer rows.Close()

	// Read EXPLAIN result
	var explainResult string
	if rows.Next() {
		err = rows.Scan(&explainResult)
		if err != nil {
			return nil
		}
	}

	// Parse JSON to validate
	var explainData []map[string]interface{}
	if err := json.Unmarshal([]byte(explainResult), &explainData); err != nil {
		return nil
	}

	// Extract timing information for performance regression detection
	var executionTime, planningTime float64
	if len(explainData) > 0 {
		if planData, ok := explainData[0]["Plan"].(map[string]interface{}); ok {
			if execTime, ok := planData["Actual Total Time"].(float64); ok {
				executionTime = execTime
			}
		}
		if planTime, ok := explainData[0]["Planning Time"].(float64); ok {
			planningTime = planTime
		}
	}

	// Create query plan entry
	queryPlan := QueryPlan{
		Query:       currentQuery,
		SQL:         sql,
		Plan:        explainData,
		ExecutionMS: executionTime,
		PlanningMS:  planningTime,
	}

	// Append to golden file
	err = g.appendToGoldenFile(queryPlan)
	if err != nil {
		// Silently skip if file operations fail
		return nil
	}

	return nil
}

// appendToGoldenFile appends the query plan to the golden file
func (g *goldenTestHook) appendToGoldenFile(queryPlan QueryPlan) error {
	goldenFile := fmt.Sprintf("testdata/golden/%s.json", g.testName)

	// Create directory if it doesn't exist
	dir := filepath.Dir(goldenFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Read existing file or create empty array
	var existingPlans []QueryPlan
	if data, err := os.ReadFile(goldenFile); err == nil {
		json.Unmarshal(data, &existingPlans)
	}

	// Append new query plan
	existingPlans = append(existingPlans, queryPlan)

	// Write back to file
	data, err := json.MarshalIndent(existingPlans, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal query plans: %w", err)
	}

	err = os.WriteFile(goldenFile, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write golden file %s: %w", goldenFile, err)
	}

	return nil
}

// AssertGolden compares captured query plans with existing golden file
func (db *DB) AssertGolden(t *testing.T, testName string) {
	goldenFile := fmt.Sprintf("testdata/golden/%s.json", testName)

	// Read the golden file that was just created/updated
	data, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Errorf("Failed to read golden file %s: %v", goldenFile, err)
		return
	}

	var currentPlans []QueryPlan
	if err := json.Unmarshal(data, &currentPlans); err != nil {
		t.Errorf("Failed to parse golden file %s: %v", goldenFile, err)
		return
	}

	// Check if this is the first run (create baseline)
	baselineFile := goldenFile + ".baseline"
	if _, err := os.Stat(baselineFile); os.IsNotExist(err) {
		// First run - create baseline
		err = os.WriteFile(baselineFile, data, 0644)
		if err != nil {
			t.Errorf("Failed to create baseline file: %v", err)
			return
		}
		t.Logf("Created golden test baseline: %s", baselineFile)
		return
	}

	// Read baseline file
	baselineData, err := os.ReadFile(baselineFile)
	if err != nil {
		t.Errorf("Failed to read baseline file %s: %v", baselineFile, err)
		return
	}

	var baselinePlans []QueryPlan
	if err := json.Unmarshal(baselineData, &baselinePlans); err != nil {
		t.Errorf("Failed to parse baseline file %s: %v", baselineFile, err)
		return
	}

	// Compare plans
	if len(currentPlans) != len(baselinePlans) {
		t.Errorf("Query count mismatch: expected %d queries, got %d", len(baselinePlans), len(currentPlans))
		return
	}

	for i, current := range currentPlans {
		baseline := baselinePlans[i]

		// Compare SQL (should be identical)
		if current.SQL != baseline.SQL {
			t.Errorf("Query %d SQL mismatch:\nExpected: %s\nGot: %s", i+1, baseline.SQL, current.SQL)
			continue
		}

		// Compare plans (convert to JSON for comparison)
		currentPlanJSON, _ := json.Marshal(current.Plan)
		baselinePlanJSON, _ := json.Marshal(baseline.Plan)

		if string(currentPlanJSON) != string(baselinePlanJSON) {
			t.Errorf("Query %d plan regression detected:\nSQL: %s\nPlan changed from baseline", i+1, current.SQL)
			// TODO: Add detailed diff output
		}
	}
}

// RequireDB ensures a test database is available or skips the test.
// It creates a TestDB and connects using TEST_DATABASE_URL environment variable.
func RequireDB(t *testing.T) *TestDB {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping test")
		return nil
	}

	testDB := NewTestDB()
	ctx := context.Background()
	err := testDB.Connect(ctx, dsn)
	if err != nil {
		t.Skipf("Failed to connect to test database: %v", err)
		return nil
	}
	return testDB
}

// CleanupGolden removes all golden files for a test
func CleanupGolden(testName string) error {
	if testName == "" {
		return nil
	}

	files := []string{
		fmt.Sprintf("testdata/golden/%s.json", testName),
		fmt.Sprintf("testdata/golden/%s.json.baseline", testName),
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove golden file %s: %w", file, err)
		}
	}

	return nil
}
