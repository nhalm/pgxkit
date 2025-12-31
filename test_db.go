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
	ctx := context.Background()

	if tdb.writePool == nil {
		return fmt.Errorf("no database pool available")
	}

	err := tdb.writePool.Ping(ctx)
	if err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	return nil
}

// Clean cleans the database after the test
func (tdb *TestDB) Clean() error {
	ctx := context.Background()

	if tdb.writePool == nil {
		return nil
	}

	err := tdb.writePool.Ping(ctx)
	if err != nil {
		return fmt.Errorf("database connection lost during cleanup: %w", err)
	}

	return nil
}

// EnableGolden returns a new DB with golden test hooks added
func (tdb *TestDB) EnableGolden(testName string) *DB {
	goldenDB := &DB{
		readPool:  tdb.readPool,
		writePool: tdb.writePool,
		hooks:     newHooks(),
	}

	goldenHook := &goldenTestHook{
		testName:     testName,
		queryCounter: 0,
		mu:           sync.Mutex{},
		db:           goldenDB,
	}

	goldenDB.hooks.addHook(BeforeOperation, goldenHook.captureExplainPlan)

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

	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "EXPLAIN") {
		return nil
	}

	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "SELECT") {
		return nil
	}

	g.mu.Lock()
	g.queryCounter++
	currentQuery := g.queryCounter
	g.mu.Unlock()

	explainSQL := fmt.Sprintf("EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) %s", sql)

	if g.db.writePool == nil {
		return nil
	}

	rows, err := g.db.writePool.Query(ctx, explainSQL, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var explainResult string
	if rows.Next() {
		err = rows.Scan(&explainResult)
		if err != nil {
			return nil
		}
	}

	var explainData []map[string]interface{}
	if err := json.Unmarshal([]byte(explainResult), &explainData); err != nil {
		return nil
	}

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

	queryPlan := QueryPlan{
		Query:       currentQuery,
		SQL:         sql,
		Plan:        explainData,
		ExecutionMS: executionTime,
		PlanningMS:  planningTime,
	}

	err = g.appendToGoldenFile(queryPlan)
	if err != nil {
		return nil
	}

	return nil
}

// appendToGoldenFile appends the query plan to the golden file
func (g *goldenTestHook) appendToGoldenFile(queryPlan QueryPlan) error {
	goldenFile := fmt.Sprintf("testdata/golden/%s.json", g.testName)

	dir := filepath.Dir(goldenFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	var existingPlans []QueryPlan
	if data, err := os.ReadFile(goldenFile); err == nil {
		json.Unmarshal(data, &existingPlans)
	}

	existingPlans = append(existingPlans, queryPlan)

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

	baselineFile := goldenFile + ".baseline"
	if _, err := os.Stat(baselineFile); os.IsNotExist(err) {
		err = os.WriteFile(baselineFile, data, 0644)
		if err != nil {
			t.Errorf("Failed to create baseline file: %v", err)
			return
		}
		t.Logf("Created golden test baseline: %s", baselineFile)
		return
	}

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

	if len(currentPlans) != len(baselinePlans) {
		t.Errorf("Query count mismatch: expected %d queries, got %d", len(baselinePlans), len(currentPlans))
		return
	}

	for i, current := range currentPlans {
		baseline := baselinePlans[i]

		if current.SQL != baseline.SQL {
			t.Errorf("Query %d SQL mismatch:\nExpected: %s\nGot: %s", i+1, baseline.SQL, current.SQL)
			continue
		}

		currentPlanJSON, _ := json.Marshal(current.Plan)
		baselinePlanJSON, _ := json.Marshal(baseline.Plan)

		if string(currentPlanJSON) != string(baselinePlanJSON) {
			t.Errorf("Query %d plan regression detected:\nSQL: %s\nPlan changed from baseline", i+1, current.SQL)
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
