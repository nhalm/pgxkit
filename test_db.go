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

	"github.com/jackc/pgx/v5/pgconn"
)

// TestDB is a testing utility that wraps DB with testing-specific functionality.
type TestDB struct {
	*DB
}

func NewTestDB() *TestDB {
	return &TestDB{DB: NewDB()}
}

func (tdb *TestDB) Setup() error {
	ctx := context.Background()
	if tdb.writePool == nil {
		return fmt.Errorf("no database pool available")
	}
	if err := tdb.writePool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}
	return nil
}

func (tdb *TestDB) Clean() error {
	ctx := context.Background()
	if tdb.writePool == nil {
		return nil
	}
	if err := tdb.writePool.Ping(ctx); err != nil {
		return fmt.Errorf("database connection lost during cleanup: %w", err)
	}
	return nil
}

// GoldenOption configures the assertGoldenHook installed by EnableGolden.
type GoldenOption func(*assertGoldenHook)

// WithGoldenNormalizer registers a custom normalizer that runs before the
// defaults (timestamps, UUIDs). Return ok=true to take over normalization for
// the value; ok=false to fall through.
func WithGoldenNormalizer(fn func(any) (any, bool)) GoldenOption {
	return func(h *assertGoldenHook) {
		h.normalizer.custom = append(h.normalizer.custom, fn)
	}
}

// assertGoldenHook accumulates a transcript of database events via the hook
// system. It mirrors assertPlanHook in shape and is the in-memory accumulator
// behind AssertGolden.
type assertGoldenHook struct {
	testName   string
	mu         sync.Mutex
	events     []transcriptEvent
	step       int
	normalizer *normalizer
}

func (h *assertGoldenHook) afterOp(_ context.Context, sql string, args []any, tag pgconn.CommandTag, err error) error {
	if err != nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.step++
	ev := transcriptEvent{
		Step:  h.step,
		Event: transcriptEventQuery,
		SQL:   sql,
		Args:  h.normalizer.normalizeArgs(args),
	}
	// pgx returns an empty tag for Query at AfterOperation time (rows haven't
	// streamed); the tag string only lands for Exec. Use that as the Exec signal.
	if tag.String() != "" {
		ra := tag.RowsAffected()
		ev.RowsAffected = &ra
	}
	h.events = append(h.events, ev)
	return nil
}

func (h *assertGoldenHook) beforeTx(_ context.Context, _ string, _ []any, _ pgconn.CommandTag, _ error) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.step++
	h.events = append(h.events, transcriptEvent{Step: h.step, Event: transcriptEventBegin})
	return nil
}

func (h *assertGoldenHook) afterTx(_ context.Context, sql string, _ []any, _ pgconn.CommandTag, _ error) error {
	var name string
	switch sql {
	case TxCommit:
		name = transcriptEventCommit
	case TxRollback:
		name = transcriptEventRollback
	default:
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.step++
	h.events = append(h.events, transcriptEvent{Step: h.step, Event: name})
	return nil
}

// EnableGolden returns a *DB that records database events (BEGIN, QUERY,
// COMMIT, ROLLBACK) for the test scenario via the hook system. Call
// AssertGolden after the scenario to compare against
// testdata/golden/<testName>.json.
func (tdb *TestDB) EnableGolden(testName string, opts ...GoldenOption) *DB {
	hook := &assertGoldenHook{testName: testName, normalizer: newNormalizer()}
	for _, opt := range opts {
		opt(hook)
	}
	goldenDB := &DB{
		readPool:   tdb.readPool,
		writePool:  tdb.writePool,
		hooks:      newHooks(),
		goldenHook: hook,
	}
	goldenDB.hooks.addHook(AfterOperation, hook.afterOp)
	goldenDB.hooks.addHook(BeforeTransaction, hook.beforeTx)
	goldenDB.hooks.addHook(AfterTransaction, hook.afterTx)
	return goldenDB
}

// AssertGolden compares the captured transcript against
// testdata/golden/<testName>.json. First run (or with -overwrite-golden) writes
// the baseline; later runs fail with a unified diff if it changes.
func (db *DB) AssertGolden(t *testing.T, testName string) {
	t.Helper()
	db.assertGolden(t, testName)
}

func (db *DB) assertGolden(t goldenT, testName string) {
	t.Helper()
	if db.goldenHook == nil {
		t.Errorf("AssertGolden called on a DB without an active golden hook; use TestDB.EnableGolden first")
		return
	}
	if db.goldenHook.testName != testName {
		t.Errorf("AssertGolden testName %q does not match hook testName %q", testName, db.goldenHook.testName)
		return
	}
	db.goldenHook.mu.Lock()
	events := append([]transcriptEvent(nil), db.goldenHook.events...)
	db.goldenHook.mu.Unlock()

	current, err := marshalEvents(events)
	if err != nil {
		t.Errorf("failed to marshal transcript: %v", err)
		return
	}
	assertBaseline(t, goldenPath(testName), current, "golden transcript", overwriteGolden != nil && *overwriteGolden)
}

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
// of each SELECT/INSERT/UPDATE/DELETE/WITH query into memory.
func (tdb *TestDB) EnableAssertPlan(testName string) *DB {
	planDB := &DB{
		readPool:  tdb.readPool,
		writePool: tdb.writePool,
		hooks:     newHooks(),
	}
	planHook := &assertPlanHook{testName: testName, db: planDB}
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

func (g *assertPlanHook) captureExplainPlan(ctx context.Context, sql string, args []interface{}, _ pgconn.CommandTag, _ error) error {
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
	return append(data, '\n'), nil
}

// AssertPlan compares the captured plans against testdata/plans/<testName>.json.
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
	assertBaseline(t, planPath(testName), current, "plan", overwritePlan != nil && *overwritePlan)
}

// RequireDB ensures a test database is available or skips the test.
func RequireDB(t *testing.T) *TestDB {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping test")
		return nil
	}
	testDB := NewTestDB()
	ctx := context.Background()
	if err := testDB.Connect(ctx, dsn); err != nil {
		t.Skipf("Failed to connect to test database: %v", err)
		return nil
	}
	return testDB
}

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
