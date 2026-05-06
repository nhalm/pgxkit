package pgxkit

import (
	"context"
	"encoding/json"
	"flag"
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

// EnableGolden returns a new DB instance that records every database event
// (BEGIN, QUERY, COMMIT, ROLLBACK) the test scenario produces, including
// SQL, normalized args, and materialized result rows. Call AssertGolden
// after the scenario to compare the transcript against
// testdata/golden/<testName>.json. Pass GoldenOption values (for example
// WithGoldenNormalizer) to extend or override the default normalization.
//
// Example:
//
//	golden := testDB.EnableGolden("TestCreateOrder")
//	// run the code under test using golden as the DB
//	golden.AssertGolden(t, "TestCreateOrder")
func (tdb *TestDB) EnableGolden(testName string, opts ...GoldenOption) *DB {
	return &DB{
		readPool:  tdb.readPool,
		writePool: tdb.writePool,
		hooks:     newHooks(),
		recorder:  newTranscriptRecorder(testName, opts...),
	}
}

// AssertGolden compares the captured transcript against
// testdata/golden/<testName>.json. On the first run (or with -overwrite-golden)
// it writes the baseline and logs that fact. On subsequent runs it fails the
// test with a unified diff if the transcript has changed. testName must match
// the name passed to EnableGolden.
func (db *DB) AssertGolden(t *testing.T, testName string) {
	t.Helper()
	db.assertGolden(t, testName)
}

// assertGolden is the implementation behind AssertGolden, taking the smaller
// goldenT interface so it can be exercised by capturing fakes in tests.
func (db *DB) assertGolden(t goldenT, testName string) {
	t.Helper()
	if db.recorder == nil {
		t.Errorf("AssertGolden called on a DB without an active golden recorder; use TestDB.EnableGolden first")
		return
	}
	if db.recorder.testName != testName {
		t.Errorf("AssertGolden testName %q does not match recorder testName %q", testName, db.recorder.testName)
		return
	}
	assertGolden(t, db.recorder)
}

// cleanupGolden removes the golden transcript file for the named scenario.
// Used internally by pgxkit's own tests so generated baselines don't pollute
// the repo. Not part of the public API — end users should let baselines
// persist across runs (that's the whole point of golden testing) and use
// the -overwrite-golden flag to regenerate, or `rm` the file directly to
// invalidate it.
func cleanupGolden(testName string) error {
	if testName == "" {
		return nil
	}
	path := goldenPath(testName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove golden file %s: %w", path, err)
	}
	return nil
}

var overwritePlan = flag.Bool("overwrite-plan", false, "regenerate testdata/plans baselines instead of asserting")

// EnableAssertPlan returns a *DB that captures the structural EXPLAIN plan
// of each SELECT/INSERT/UPDATE/DELETE/WITH query into memory. Call AssertPlan
// to compare against testdata/plans/<testName>.json.
func (tdb *TestDB) EnableAssertPlan(testName string) *DB {
	planDB := &DB{
		readPool:  tdb.readPool,
		writePool: tdb.writePool,
		hooks:     newHooks(),
	}

	planHook := &assertPlanHook{
		testName: testName,
		db:       planDB,
	}
	planDB.planHook = planHook
	planDB.hooks.addHook(BeforeOperation, planHook.captureExplainPlan)

	return planDB
}

type assertPlanHook struct {
	testName string
	mu       sync.Mutex
	plans    []QueryPlan
	db       *DB
}

// QueryPlan is one captured structural query plan.
type QueryPlan struct {
	Query int                      `json:"query"`
	SQL   string                   `json:"sql"`
	Plan  []map[string]interface{} `json:"plan"`
}

func (g *assertPlanHook) captureExplainPlan(ctx context.Context, sql string, args []interface{}, operationErr error) error {
	if g.db == nil || g.db.writePool == nil {
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

	explainSQL := fmt.Sprintf("EXPLAIN (FORMAT JSON, COSTS OFF) %s", sql)

	var explainResult string
	rows, err := g.db.writePool.Query(ctx, explainSQL, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(&explainResult); err != nil {
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

	g.mu.Lock()
	g.plans = append(g.plans, QueryPlan{
		Query: len(g.plans) + 1,
		SQL:   sql,
		Plan:  explainData,
	})
	g.mu.Unlock()

	return nil
}

func planPath(name string) string {
	return filepath.Join("testdata", "plans", name+".json")
}

func marshalPlans(plans []QueryPlan) ([]byte, error) {
	if plans == nil {
		plans = []QueryPlan{}
	}
	data, err := json.MarshalIndent(plans, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	return data, nil
}

// AssertPlan compares the captured plans against testdata/plans/<testName>.json,
// writing the baseline on first run or with -overwrite-plan, and failing with
// a unified diff on mismatch. testName must match EnableAssertPlan's.
func (db *DB) AssertPlan(t *testing.T, testName string) {
	t.Helper()
	db.assertPlan(t, testName)
}

func (db *DB) assertPlan(t goldenT, testName string) {
	t.Helper()
	if db.planHook == nil {
		t.Errorf("AssertPlan called on a DB without an active plan hook; use TestDB.EnableAssertPlan first")
		return
	}
	if db.planHook.testName != testName {
		t.Errorf("AssertPlan testName %q does not match hook testName %q", testName, db.planHook.testName)
		return
	}

	db.planHook.mu.Lock()
	plans := append([]QueryPlan(nil), db.planHook.plans...)
	db.planHook.mu.Unlock()

	current, err := marshalPlans(plans)
	if err != nil {
		t.Errorf("failed to marshal plans: %v", err)
		return
	}

	path := planPath(testName)
	_, statErr := os.Stat(path)
	missing := os.IsNotExist(statErr)

	if missing || (overwritePlan != nil && *overwritePlan) {
		if err := writeBaseline(path, current); err != nil {
			t.Errorf("%v", err)
			return
		}
		if missing {
			t.Logf("created plan baseline: %s", path)
		} else {
			t.Logf("regenerated plan baseline: %s", path)
		}
		return
	}

	baseline, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("failed to read plan file %s: %v", path, err)
		return
	}

	diff, ok := unifiedDiff(path, baseline, current)
	if ok {
		return
	}
	t.Errorf("plan regression for %s\n%s", path, diff)
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

// cleanupPlan removes a plan baseline file. Internal helper for pgxkit tests.
func cleanupPlan(testName string) error {
	if testName == "" {
		return nil
	}
	path := planPath(testName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plan file %s: %w", path, err)
	}
	return nil
}
