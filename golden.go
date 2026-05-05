package pgxkit

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pmezard/go-difflib/difflib"
)

// goldenT captures the subset of *testing.T that assertGolden needs. Splitting
// it out lets us drive assertGolden from unit tests with a fake.
type goldenT interface {
	Helper()
	Errorf(format string, args ...any)
	Logf(format string, args ...any)
}

// overwriteGolden, when set, regenerates testdata/golden baselines instead of
// asserting them. The flag is registered at package scope so it can be passed
// as `go test -overwrite-golden` from any test invocation that links pgxkit.
var overwriteGolden = flag.Bool("overwrite-golden", false, "regenerate testdata/golden baselines instead of asserting")

// transcriptEvent is one entry in a recorded transcript. omitempty keeps each
// event lean — non-applicable fields don't appear in the serialized JSON.
type transcriptEvent struct {
	Step         int              `json:"step"`
	Event        string           `json:"event"`
	SQL          string           `json:"sql,omitempty"`
	Args         []any            `json:"args,omitempty"`
	Rows         []map[string]any `json:"rows,omitempty"`
	RowsAffected *int64           `json:"rows_affected,omitempty"`
}

// Event names used in the transcript.
const (
	transcriptEventBegin    = "BEGIN"
	transcriptEventCommit   = "COMMIT"
	transcriptEventRollback = "ROLLBACK"
	transcriptEventQuery    = "QUERY"
)

// transcriptRecorder collects a sequential transcript of database events for a
// single golden test scenario. It is shared between a *DB returned by
// EnableGolden and any *Tx that DB starts so a multi-statement transaction
// records as one ordered stream.
type transcriptRecorder struct {
	mu         sync.Mutex
	testName   string
	events     []transcriptEvent
	step       int
	normalizer *normalizer
}

// GoldenOption configures a transcript recorder created by EnableGolden.
type GoldenOption func(*transcriptRecorder)

// WithGoldenNormalizer registers a custom normalizer that runs before pgxkit's
// defaults. Return ok=true to take over normalization for the value; return
// ok=false to fall through to the next custom normalizer or the defaults.
func WithGoldenNormalizer(fn func(any) (any, bool)) GoldenOption {
	return func(r *transcriptRecorder) {
		r.normalizer.custom = append(r.normalizer.custom, fn)
	}
}

func newTranscriptRecorder(testName string, opts ...GoldenOption) *transcriptRecorder {
	r := &transcriptRecorder{
		testName:   testName,
		normalizer: newNormalizer(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// nextStep returns the next 1-based step number. Caller must hold r.mu.
func (r *transcriptRecorder) nextStep() int {
	r.step++
	return r.step
}

func (r *transcriptRecorder) recordBegin() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, transcriptEvent{
		Step:  r.nextStep(),
		Event: transcriptEventBegin,
	})
}

func (r *transcriptRecorder) recordCommit(_ error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, transcriptEvent{
		Step:  r.nextStep(),
		Event: transcriptEventCommit,
	})
}

func (r *transcriptRecorder) recordRollback(_ error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, transcriptEvent{
		Step:  r.nextStep(),
		Event: transcriptEventRollback,
	})
}

// recordQuery captures a SELECT-shaped result with materialized rows.
// rows are already-normalized column→value maps in field order.
func (r *transcriptRecorder) recordQuery(sql string, args []any, rows []map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	normalizedArgs := r.normalizeArgs(args)
	if rows == nil {
		rows = []map[string]any{}
	}
	r.events = append(r.events, transcriptEvent{
		Step:  r.nextStep(),
		Event: transcriptEventQuery,
		SQL:   sql,
		Args:  normalizedArgs,
		Rows:  rows,
	})
}

// recordExec captures a command-tag result (no rows; rows_affected only).
func (r *transcriptRecorder) recordExec(sql string, args []any, rowsAffected int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	normalizedArgs := r.normalizeArgs(args)
	ra := rowsAffected
	r.events = append(r.events, transcriptEvent{
		Step:         r.nextStep(),
		Event:        transcriptEventQuery,
		SQL:          sql,
		Args:         normalizedArgs,
		RowsAffected: &ra,
	})
}

// normalizeArgs normalizes positional query args. Caller must hold r.mu.
// Args have no column hint, so int normalization (which keys off column name)
// does not trigger here — only timestamps and UUIDs are replaced.
func (r *transcriptRecorder) normalizeArgs(args []any) []any {
	if len(args) == 0 {
		return nil
	}
	out := make([]any, len(args))
	for i, a := range args {
		out[i] = r.normalizer.normalize(a, "")
	}
	return out
}

// normalizeRow normalizes one decoded row by column name. Caller must hold r.mu.
func (r *transcriptRecorder) normalizeRow(columnNames []string, values []any) map[string]any {
	out := make(map[string]any, len(columnNames))
	for i, name := range columnNames {
		var v any
		if i < len(values) {
			v = values[i]
		}
		out[name] = r.normalizer.normalize(v, name)
	}
	return out
}

// normalizer replaces volatile values (timestamps, UUIDs, sequence IDs) with
// stable placeholders so transcripts compare cleanly across runs. UUIDs and
// IDs use first-seen ordering within the scenario so the same value in two
// places gets the same placeholder.
type normalizer struct {
	uuids  map[string]int
	ids    map[string]int
	custom []func(any) (any, bool)
}

func newNormalizer() *normalizer {
	return &normalizer{
		uuids: make(map[string]int),
		ids:   make(map[string]int),
	}
}

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// normalize returns a placeholder for volatile values. columnHint is the
// column name (for row values) or "" (for query args). Custom normalizers run
// before defaults so users can override.
func (n *normalizer) normalize(value any, columnHint string) any {
	for _, fn := range n.custom {
		if replaced, ok := fn(value); ok {
			return replaced
		}
	}

	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case time.Time:
		return "<TIMESTAMP>"
	case *time.Time:
		if v == nil {
			return nil
		}
		return "<TIMESTAMP>"
	case uuid.UUID:
		return n.placeholderForUUID(v.String())
	case [16]byte:
		u := uuid.UUID(v)
		return n.placeholderForUUID(u.String())
	case string:
		if uuidRegex.MatchString(v) {
			return n.placeholderForUUID(strings.ToLower(v))
		}
	}

	if isIDColumn(columnHint) {
		if key, ok := intKey(value); ok {
			return n.placeholderForID(key)
		}
	}

	return value
}

func (n *normalizer) placeholderForUUID(canonical string) string {
	canonical = strings.ToLower(canonical)
	if idx, ok := n.uuids[canonical]; ok {
		return fmt.Sprintf("<UUID:%d>", idx)
	}
	idx := len(n.uuids) + 1
	n.uuids[canonical] = idx
	return fmt.Sprintf("<UUID:%d>", idx)
}

func (n *normalizer) placeholderForID(key string) string {
	if idx, ok := n.ids[key]; ok {
		return fmt.Sprintf("<ID:%d>", idx)
	}
	idx := len(n.ids) + 1
	n.ids[key] = idx
	return fmt.Sprintf("<ID:%d>", idx)
}

// isIDColumn reports whether a column name indicates a sequence-style ID
// suitable for normalization. Hint is empty for positional args.
func isIDColumn(hint string) bool {
	if hint == "" {
		return false
	}
	if hint == "id" {
		return true
	}
	return strings.HasSuffix(hint, "_id")
}

// intKey returns a stable string key for any signed/unsigned integer value
// so that int and int64 with the same numeric value collapse to one entry.
func intKey(value any) (string, bool) {
	switch v := value.(type) {
	case int:
		return strconv.FormatInt(int64(v), 10), true
	case int8:
		return strconv.FormatInt(int64(v), 10), true
	case int16:
		return strconv.FormatInt(int64(v), 10), true
	case int32:
		return strconv.FormatInt(int64(v), 10), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case uint:
		return strconv.FormatUint(uint64(v), 10), true
	case uint8:
		return strconv.FormatUint(uint64(v), 10), true
	case uint16:
		return strconv.FormatUint(uint64(v), 10), true
	case uint32:
		return strconv.FormatUint(uint64(v), 10), true
	case uint64:
		return strconv.FormatUint(v, 10), true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10), true
	}
	return "", false
}

// goldenPath returns the on-disk path for a named transcript baseline.
func goldenPath(name string) string {
	return filepath.Join("testdata", "golden", name+".json")
}

// marshalEvents renders the recorder's events as canonical pretty-printed JSON
// with a trailing newline. Empty event slices serialize as `[]`, not `null`.
func marshalEvents(events []transcriptEvent) ([]byte, error) {
	if events == nil {
		events = []transcriptEvent{}
	}
	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	return data, nil
}

// writeGolden writes pretty-printed transcript bytes, creating the directory.
func writeGolden(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create golden directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write golden file %s: %w", path, err)
	}
	return nil
}

// compareGolden compares baseline to current bytes. If they match, ok is true
// and diff is empty. Otherwise diff is a unified diff string. Extracted as a
// pure function so it can be unit tested without a *testing.T.
func compareGolden(path string, baseline, current []byte) (string, bool) {
	if bytes.Equal(baseline, current) {
		return "", true
	}
	udiff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(baseline)),
		B:        difflib.SplitLines(string(current)),
		FromFile: path + " (baseline)",
		ToFile:   path + " (current)",
		Context:  3,
	}
	out, err := difflib.GetUnifiedDiffString(udiff)
	if err != nil {
		return fmt.Sprintf("(failed to render diff: %v)", err), false
	}
	return out, false
}

// assertGolden marshals the recorder's transcript and compares it against the
// baseline file, writing the baseline on first run or when -overwrite-golden
// is set.
func assertGolden(t goldenT, recorder *transcriptRecorder) {
	t.Helper()

	recorder.mu.Lock()
	events := append([]transcriptEvent(nil), recorder.events...)
	name := recorder.testName
	recorder.mu.Unlock()

	current, err := marshalEvents(events)
	if err != nil {
		t.Errorf("failed to marshal transcript: %v", err)
		return
	}

	path := goldenPath(name)
	_, statErr := os.Stat(path)
	missing := os.IsNotExist(statErr)

	if missing || (overwriteGolden != nil && *overwriteGolden) {
		if err := writeGolden(path, current); err != nil {
			t.Errorf("%v", err)
			return
		}
		if missing {
			t.Logf("created golden baseline: %s", path)
		} else {
			t.Logf("regenerated golden baseline: %s", path)
		}
		return
	}

	baseline, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("failed to read golden file %s: %v", path, err)
		return
	}

	diff, ok := compareGolden(path, baseline, current)
	if ok {
		return
	}
	t.Errorf("golden transcript mismatch for %s\n%s", path, diff)
}
