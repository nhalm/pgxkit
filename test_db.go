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
// It provides simple methods for test setup, cleanup, and plan-regression
// assertion support. TestDB automatically manages test database connections
// and offers utilities for catching query-plan changes between runs.
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

// Clean performs cleanup operations after a test completes.
// It verifies the database connection is still active and can be extended
// to truncate tables or reset test data. Returns nil if no pool is configured.
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

// EnableAssertPlan returns a new DB instance configured to capture query
// plans for plan-regression testing. For each eligible query, it issues an
// EXPLAIN (FORMAT JSON, COSTS OFF) and records the structural plan so it
// can be diffed against a baseline. The testName is used to name the plan
// file under testdata/plans. Use AssertPlan after test execution to compare
// captured plans against the baseline.
//
// Example:
//
//	planDB := testDB.EnableAssertPlan("user_queries")
//	// run queries using planDB
//	planDB.AssertPlan(t, "user_queries")
func (tdb *TestDB) EnableAssertPlan(testName string) *DB {
	planDB := &DB{
		readPool:  tdb.readPool,
		writePool: tdb.writePool,
		hooks:     newHooks(),
	}

	planHook := &assertPlanHook{
		testName:     testName,
		queryCounter: 0,
		mu:           sync.Mutex{},
		db:           planDB,
	}

	planDB.hooks.addHook(BeforeOperation, planHook.captureExplainPlan)

	return planDB
}

// assertPlanHook handles plan-regression assertion functionality.
type assertPlanHook struct {
	testName     string
	queryCounter int
	mu           sync.Mutex
	db           *DB
}

// QueryPlan represents a captured structural query plan.
// It stores the SQL statement and the JSON plan output for use in
// plan-regression comparisons.
type QueryPlan struct {
	Query int                      `json:"query"`
	SQL   string                   `json:"sql"`
	Plan  []map[string]interface{} `json:"plan"`
}

// captureExplainPlan captures structural EXPLAIN plans for queries.
func (g *assertPlanHook) captureExplainPlan(ctx context.Context, sql string, args []interface{}, operationErr error) error {
	if g.db == nil {
		return nil
	}

	upperSQL := strings.ToUpper(strings.TrimSpace(sql))

	if strings.HasPrefix(upperSQL, "EXPLAIN") {
		return nil
	}

	if !strings.HasPrefix(upperSQL, "SELECT") &&
		!strings.HasPrefix(upperSQL, "INSERT") &&
		!strings.HasPrefix(upperSQL, "UPDATE") &&
		!strings.HasPrefix(upperSQL, "DELETE") &&
		!strings.HasPrefix(upperSQL, "WITH") {
		return nil
	}

	g.mu.Lock()
	g.queryCounter++
	currentQuery := g.queryCounter
	g.mu.Unlock()

	explainSQL := fmt.Sprintf("EXPLAIN (FORMAT JSON, COSTS OFF) %s", sql)

	if g.db.writePool == nil {
		return nil
	}

	var explainResult string

	rows, err := g.db.writePool.Query(ctx, explainSQL, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	if rows.Next() {
		err = rows.Scan(&explainResult)
		if err != nil {
			return nil
		}
	}
	if rows.Err() != nil {
		return nil
	}

	var explainData []map[string]interface{}
	if err := json.Unmarshal([]byte(explainResult), &explainData); err != nil {
		return nil
	}

	queryPlan := QueryPlan{
		Query: currentQuery,
		SQL:   sql,
		Plan:  explainData,
	}

	err = g.appendToPlanFile(queryPlan)
	if err != nil {
		return nil
	}

	return nil
}

// appendToPlanFile appends the query plan to the plan file.
func (g *assertPlanHook) appendToPlanFile(queryPlan QueryPlan) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	planFile := fmt.Sprintf("testdata/plans/%s.json", g.testName)

	dir := filepath.Dir(planFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	var existingPlans []QueryPlan
	if data, err := os.ReadFile(planFile); err == nil {
		if len(data) > 0 {
			if err := json.Unmarshal(data, &existingPlans); err != nil {
				return fmt.Errorf("failed to parse existing plan file %s: %w", planFile, err)
			}
		}
	}

	existingPlans = append(existingPlans, queryPlan)

	data, err := json.MarshalIndent(existingPlans, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal query plans: %w", err)
	}

	err = os.WriteFile(planFile, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write plan file %s: %w", planFile, err)
	}

	return nil
}

// AssertPlan compares captured query plans against a baseline file.
// On first run, it creates a baseline file from the current plan output.
// On subsequent runs, it compares the current plans against the baseline
// and reports test failures for any query count, SQL, or plan changes.
func (db *DB) AssertPlan(t *testing.T, testName string) {
	planFile := fmt.Sprintf("testdata/plans/%s.json", testName)

	data, err := os.ReadFile(planFile)
	if err != nil {
		t.Errorf("Failed to read plan file %s: %v", planFile, err)
		return
	}

	var currentPlans []QueryPlan
	if err := json.Unmarshal(data, &currentPlans); err != nil {
		t.Errorf("Failed to parse plan file %s: %v", planFile, err)
		return
	}

	baselineFile := planFile + ".baseline"
	if _, err := os.Stat(baselineFile); os.IsNotExist(err) {
		err = os.WriteFile(baselineFile, data, 0644)
		if err != nil {
			t.Errorf("Failed to create baseline file: %v", err)
			return
		}
		t.Logf("Created plan baseline: %s", baselineFile)
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

// CleanupPlan removes all plan-regression files for the specified test name.
// This includes both the captured query plan file and its baseline file
// from the testdata/plans directory.
func CleanupPlan(testName string) error {
	if testName == "" {
		return nil
	}

	files := []string{
		fmt.Sprintf("testdata/plans/%s.json", testName),
		fmt.Sprintf("testdata/plans/%s.json.baseline", testName),
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove plan file %s: %w", file, err)
		}
	}

	return nil
}
