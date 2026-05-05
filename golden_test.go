package pgxkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// withGoldenSchema creates a regular (non-temp) table for golden testing and
// returns its name. It registers a Cleanup to drop the table. Regular tables
// are required because the *DB returned by EnableGolden shares the underlying
// pool and DML may run on a different connection than the CREATE.
func withGoldenSchema(t *testing.T, testDB *TestDB, table string) {
	t.Helper()
	ctx := context.Background()
	_, err := testDB.Exec(ctx, "CREATE TABLE IF NOT EXISTS "+table+" ("+
		"id SERIAL PRIMARY KEY, "+
		"name TEXT NOT NULL, "+
		"created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())")
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DROP TABLE IF EXISTS "+table)
	})
	_, err = testDB.Exec(ctx, "TRUNCATE "+table+" RESTART IDENTITY")
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func goldenFileExists(name string) bool {
	_, err := os.Stat(goldenPath(name))
	return err == nil
}

// readGoldenFile returns the raw bytes of the named golden file or fails.
func readGoldenFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(goldenPath(name))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	return data
}

func TestGolden_FirstRunCreatesBaseline(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}
	withGoldenSchema(t, testDB, "golden_first_run")
	const name = "TestGolden_FirstRunCreatesBaseline"
	defer CleanupGolden(name)
	_ = CleanupGolden(name)

	ctx := context.Background()
	g := testDB.EnableGolden(name)

	_, err := g.Exec(ctx, "INSERT INTO golden_first_run (name) VALUES ($1)", "alpha")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	g.AssertGolden(t, name)
	if t.Failed() {
		return
	}
	if !goldenFileExists(name) {
		t.Fatalf("expected golden file to be created at %s", goldenPath(name))
	}
}

func TestGolden_SecondRunNoDiff(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}
	withGoldenSchema(t, testDB, "golden_second_run")
	const name = "TestGolden_SecondRunNoDiff"
	defer CleanupGolden(name)
	_ = CleanupGolden(name)

	ctx := context.Background()
	for run := 0; run < 2; run++ {
		_, _ = testDB.Exec(ctx, "TRUNCATE golden_second_run RESTART IDENTITY")
		g := testDB.EnableGolden(name)
		_, err := g.Exec(ctx, "INSERT INTO golden_second_run (name) VALUES ($1)", "alpha")
		if err != nil {
			t.Fatalf("insert run %d: %v", run, err)
		}
		_, err = g.Exec(ctx, "INSERT INTO golden_second_run (name) VALUES ($1)", "beta")
		if err != nil {
			t.Fatalf("insert run %d: %v", run, err)
		}
		g.AssertGolden(t, name)
		if t.Failed() {
			t.Fatalf("run %d should not fail", run)
		}
	}
}

func TestGolden_FailsOnExtraQuery(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}
	withGoldenSchema(t, testDB, "golden_extra")
	const name = "TestGolden_FailsOnExtraQuery"
	defer CleanupGolden(name)
	_ = CleanupGolden(name)

	ctx := context.Background()

	g1 := testDB.EnableGolden(name)
	if _, err := g1.Exec(ctx, "INSERT INTO golden_extra (name) VALUES ($1)", "a"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	g1.AssertGolden(t, name)
	if t.Failed() {
		t.Fatalf("baseline run should pass")
	}

	g2 := testDB.EnableGolden(name)
	if _, err := g2.Exec(ctx, "INSERT INTO golden_extra (name) VALUES ($1)", "a"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := g2.Exec(ctx, "INSERT INTO golden_extra (name) VALUES ($1)", "b"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	mt := &capturingT{}
	assertGolden(mt, g2.recorder)
	if !mt.failed {
		t.Errorf("expected mismatch failure for extra query")
	}
	if !strings.Contains(mt.errorMsg, "golden transcript mismatch") {
		t.Errorf("expected diff message, got: %s", mt.errorMsg)
	}
}

func TestGolden_FailsOnDifferentArgs(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}
	withGoldenSchema(t, testDB, "golden_diff_args")
	const name = "TestGolden_FailsOnDifferentArgs"
	defer CleanupGolden(name)
	_ = CleanupGolden(name)

	ctx := context.Background()

	g1 := testDB.EnableGolden(name)
	_, _ = g1.Exec(ctx, "INSERT INTO golden_diff_args (name) VALUES ($1)", "alpha")
	g1.AssertGolden(t, name)
	if t.Failed() {
		t.Fatalf("baseline run should pass")
	}

	g2 := testDB.EnableGolden(name)
	_, _ = g2.Exec(ctx, "INSERT INTO golden_diff_args (name) VALUES ($1)", "DIFFERENT")
	mt := &capturingT{}
	assertGolden(mt, g2.recorder)
	if !mt.failed {
		t.Errorf("expected mismatch failure on differing args")
	}
}

func TestGolden_FailsOnDifferentRow(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}
	withGoldenSchema(t, testDB, "golden_diff_row")
	const name = "TestGolden_FailsOnDifferentRow"
	defer CleanupGolden(name)
	_ = CleanupGolden(name)

	ctx := context.Background()

	if _, err := testDB.Exec(ctx, "INSERT INTO golden_diff_row (name) VALUES ('first')"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	g1 := testDB.EnableGolden(name)
	rows, err := g1.Query(ctx, "SELECT name FROM golden_diff_row ORDER BY id")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
	}
	rows.Close()
	g1.AssertGolden(t, name)
	if t.Failed() {
		t.Fatalf("baseline run should pass")
	}

	if _, err := testDB.Exec(ctx, "UPDATE golden_diff_row SET name = 'changed' WHERE name = 'first'"); err != nil {
		t.Fatalf("update: %v", err)
	}

	g2 := testDB.EnableGolden(name)
	rows, err = g2.Query(ctx, "SELECT name FROM golden_diff_row ORDER BY id")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
	}
	rows.Close()

	mt := &capturingT{}
	assertGolden(mt, g2.recorder)
	if !mt.failed {
		t.Errorf("expected mismatch on differing row")
	}
}

func TestGolden_FailsOnMissingExec(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}
	withGoldenSchema(t, testDB, "golden_missing")
	const name = "TestGolden_FailsOnMissingExec"
	defer CleanupGolden(name)
	_ = CleanupGolden(name)

	ctx := context.Background()

	g1 := testDB.EnableGolden(name)
	_, _ = g1.Exec(ctx, "INSERT INTO golden_missing (name) VALUES ($1)", "a")
	_, _ = g1.Exec(ctx, "INSERT INTO golden_missing (name) VALUES ($1)", "b")
	g1.AssertGolden(t, name)
	if t.Failed() {
		t.Fatalf("baseline run should pass")
	}

	g2 := testDB.EnableGolden(name)
	_, _ = g2.Exec(ctx, "INSERT INTO golden_missing (name) VALUES ($1)", "a")
	mt := &capturingT{}
	assertGolden(mt, g2.recorder)
	if !mt.failed {
		t.Errorf("expected mismatch on missing exec")
	}
}

func TestGolden_FailsCommitToRollback(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}
	withGoldenSchema(t, testDB, "golden_commit_rollback")
	const name = "TestGolden_FailsCommitToRollback"
	defer CleanupGolden(name)
	_ = CleanupGolden(name)

	ctx := context.Background()

	g1 := testDB.EnableGolden(name)
	tx, err := g1.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	_, _ = tx.Exec(ctx, "INSERT INTO golden_commit_rollback (name) VALUES ($1)", "x")
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	g1.AssertGolden(t, name)
	if t.Failed() {
		t.Fatalf("baseline run should pass")
	}

	g2 := testDB.EnableGolden(name)
	tx2, err := g2.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	_, _ = tx2.Exec(ctx, "INSERT INTO golden_commit_rollback (name) VALUES ($1)", "x")
	if err := tx2.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	mt := &capturingT{}
	assertGolden(mt, g2.recorder)
	if !mt.failed {
		t.Errorf("expected mismatch when COMMIT becomes ROLLBACK")
	}
}

func TestGolden_TransactionPreservesOrder(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}
	withGoldenSchema(t, testDB, "golden_tx_order")
	const name = "TestGolden_TransactionPreservesOrder"
	defer CleanupGolden(name)
	_ = CleanupGolden(name)

	ctx := context.Background()
	g := testDB.EnableGolden(name)

	tx, err := g.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO golden_tx_order (name) VALUES ($1)", "first"); err != nil {
		t.Fatalf("exec1: %v", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO golden_tx_order (name) VALUES ($1)", "second"); err != nil {
		t.Fatalf("exec2: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	g.AssertGolden(t, name)
	if t.Failed() {
		return
	}

	data := readGoldenFile(t, name)
	var events []map[string]any
	if err := json.Unmarshal(data, &events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d: %s", len(events), data)
	}
	want := []string{"BEGIN", "QUERY", "QUERY", "COMMIT"}
	for i, w := range want {
		if got := events[i]["event"]; got != w {
			t.Errorf("event[%d] = %v, want %s", i, got, w)
		}
	}
}

func TestGolden_NormalizesUUIDsTimestampsAndIDs(t *testing.T) {
	r := newTranscriptRecorder("normalize-test")

	now := time.Now()
	u := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	r.recordQuery("SELECT 1",
		[]any{u, now, "22222222-2222-2222-2222-222222222222"},
		[]map[string]any{
			r.normalizeRow([]string{"id", "user_id", "name", "created_at"},
				[]any{int64(7), int64(7), "Alice", now}),
			r.normalizeRow([]string{"id", "user_id", "name", "created_at"},
				[]any{int64(8), int64(7), "Bob", now}),
		})

	if len(r.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(r.events))
	}
	ev := r.events[0]
	if ev.Args[0] != "<UUID:1>" {
		t.Errorf("args[0] = %v, want <UUID:1>", ev.Args[0])
	}
	if ev.Args[1] != "<TIMESTAMP>" {
		t.Errorf("args[1] = %v, want <TIMESTAMP>", ev.Args[1])
	}
	if ev.Args[2] != "<UUID:2>" {
		t.Errorf("args[2] = %v, want <UUID:2>", ev.Args[2])
	}
	if ev.Rows[0]["id"] != "<ID:1>" {
		t.Errorf("rows[0].id = %v, want <ID:1>", ev.Rows[0]["id"])
	}
	if ev.Rows[0]["user_id"] != "<ID:1>" {
		t.Errorf("rows[0].user_id = %v, want <ID:1> (same numeric value)", ev.Rows[0]["user_id"])
	}
	if ev.Rows[0]["name"] != "Alice" {
		t.Errorf("rows[0].name should not be normalized: %v", ev.Rows[0]["name"])
	}
	if ev.Rows[0]["created_at"] != "<TIMESTAMP>" {
		t.Errorf("rows[0].created_at = %v, want <TIMESTAMP>", ev.Rows[0]["created_at"])
	}
	if ev.Rows[1]["id"] != "<ID:2>" {
		t.Errorf("rows[1].id = %v, want <ID:2>", ev.Rows[1]["id"])
	}
	if ev.Rows[1]["user_id"] != "<ID:1>" {
		t.Errorf("rows[1].user_id = %v, want <ID:1>", ev.Rows[1]["user_id"])
	}
}

func TestGolden_CustomNormalizerRunsBeforeDefaults(t *testing.T) {
	r := newTranscriptRecorder("custom",
		WithGoldenNormalizer(func(v any) (any, bool) {
			if s, ok := v.(string); ok && strings.HasPrefix(s, "ord_") {
				return "<ORDER>", true
			}
			return nil, false
		}),
		WithGoldenNormalizer(func(v any) (any, bool) {
			if t, ok := v.(time.Time); ok {
				_ = t
				return "<CUSTOM-TIME>", true
			}
			return nil, false
		}),
	)

	now := time.Now()
	out := r.normalizer.normalize(now, "")
	if out != "<CUSTOM-TIME>" {
		t.Errorf("custom time normalizer should run before default: got %v", out)
	}
	out = r.normalizer.normalize("ord_abc", "")
	if out != "<ORDER>" {
		t.Errorf("custom string normalizer should run: got %v", out)
	}
	out = r.normalizer.normalize("plain", "")
	if out != "plain" {
		t.Errorf("non-matching value should pass through: got %v", out)
	}
}

func TestGolden_DDLAndSetRecordedAsQuery(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}
	const name = "TestGolden_DDLAndSet"
	const table = "golden_ddl_set"
	defer CleanupGolden(name)
	_ = CleanupGolden(name)
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DROP TABLE IF EXISTS "+table)
	})

	ctx := context.Background()
	g := testDB.EnableGolden(name)

	if _, err := g.Exec(ctx, "CREATE TABLE IF NOT EXISTS "+table+" (id INT)"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := g.Exec(ctx, "SET LOCAL search_path TO public"); err != nil {
		t.Fatalf("set: %v", err)
	}
	g.AssertGolden(t, name)
	if t.Failed() {
		return
	}

	data := readGoldenFile(t, name)
	var events []map[string]any
	if err := json.Unmarshal(data, &events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	for i, ev := range events {
		if ev["event"] != "QUERY" {
			t.Errorf("event[%d] = %v, want QUERY", i, ev["event"])
		}
	}
}

func TestGolden_OverwriteFlagRegeneratesBaseline(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}
	withGoldenSchema(t, testDB, "golden_overwrite")
	const name = "TestGolden_OverwriteFlag"
	defer CleanupGolden(name)
	_ = CleanupGolden(name)

	ctx := context.Background()

	g1 := testDB.EnableGolden(name)
	_, _ = g1.Exec(ctx, "INSERT INTO golden_overwrite (name) VALUES ($1)", "alpha")
	g1.AssertGolden(t, name)
	if t.Failed() {
		t.Fatalf("baseline run should pass")
	}
	first := readGoldenFile(t, name)

	old := *overwriteGolden
	*overwriteGolden = true
	defer func() { *overwriteGolden = old }()

	g2 := testDB.EnableGolden(name)
	_, _ = g2.Exec(ctx, "INSERT INTO golden_overwrite (name) VALUES ($1)", "beta")
	_, _ = g2.Exec(ctx, "INSERT INTO golden_overwrite (name) VALUES ($1)", "gamma")
	g2.AssertGolden(t, name)
	if t.Failed() {
		t.Fatalf("overwrite run should pass")
	}
	second := readGoldenFile(t, name)
	if string(first) == string(second) {
		t.Errorf("expected baseline to be regenerated under -overwrite-golden")
	}
}

func TestGolden_AssertWithoutEnableFails(t *testing.T) {
	db := &DB{hooks: newHooks()}
	mt := &capturingT{}
	db.assertGolden(mt, "any")
	if !mt.failed {
		t.Errorf("expected error when calling AssertGolden without EnableGolden")
	}
	if !strings.Contains(mt.errorMsg, "active golden recorder") {
		t.Errorf("unexpected message: %s", mt.errorMsg)
	}
}

func TestGolden_NameMismatchFails(t *testing.T) {
	db := &DB{hooks: newHooks(), recorder: newTranscriptRecorder("expected")}
	mt := &capturingT{}
	db.assertGolden(mt, "different")
	if !mt.failed {
		t.Errorf("expected error when AssertGolden name does not match recorder name")
	}
	if !strings.Contains(mt.errorMsg, "does not match recorder testName") {
		t.Errorf("unexpected message: %s", mt.errorMsg)
	}
}

func TestGolden_CleanupRemovesFile(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}
	withGoldenSchema(t, testDB, "golden_cleanup")
	const name = "TestGolden_CleanupRemovesFile"
	_ = CleanupGolden(name)

	ctx := context.Background()
	g := testDB.EnableGolden(name)
	_, _ = g.Exec(ctx, "INSERT INTO golden_cleanup (name) VALUES ($1)", "x")
	g.AssertGolden(t, name)
	if !goldenFileExists(name) {
		t.Fatalf("expected golden file to exist before cleanup")
	}
	if err := CleanupGolden(name); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if goldenFileExists(name) {
		t.Errorf("expected golden file to be removed after CleanupGolden")
	}
	if err := CleanupGolden(name); err != nil {
		t.Errorf("CleanupGolden should be idempotent: %v", err)
	}
}

func TestGolden_QueryRowReturnsErrNoRowsOnEmpty(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}
	withGoldenSchema(t, testDB, "golden_no_rows")
	const name = "TestGolden_QueryRowEmpty"
	defer CleanupGolden(name)
	_ = CleanupGolden(name)

	ctx := context.Background()
	g := testDB.EnableGolden(name)

	var n int
	err := g.QueryRow(ctx, "SELECT id FROM golden_no_rows WHERE id = $1", 999).Scan(&n)
	if err == nil {
		t.Fatalf("expected error scanning empty result")
	}
	if err != pgx.ErrNoRows {
		t.Errorf("expected pgx.ErrNoRows, got %v", err)
	}
	g.AssertGolden(t, name)
}

// capturingT mimics enough of *testing.T for assertGolden to drive into
// without polluting the real test result.
type capturingT struct {
	failed   bool
	errorMsg string
	logs     []string
}

func (c *capturingT) Errorf(format string, args ...any) {
	c.failed = true
	c.errorMsg = fmt.Sprintf(format, args...)
}

func (c *capturingT) Logf(format string, args ...any) {
	c.logs = append(c.logs, fmt.Sprintf(format, args...))
}

func (c *capturingT) Helper() {}
