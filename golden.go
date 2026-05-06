package pgxkit

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pmezard/go-difflib/difflib"
)

// goldenT captures the subset of *testing.T that assertBaseline drives, so unit
// tests can substitute a fake.
type goldenT interface {
	Helper()
	Errorf(format string, args ...any)
	Logf(format string, args ...any)
}

var overwriteGolden = flag.Bool("overwrite-golden", false, "regenerate testdata/golden baselines instead of asserting")

type transcriptEvent struct {
	Step         int    `json:"step"`
	Event        string `json:"event"`
	SQL          string `json:"sql,omitempty"`
	Args         []any  `json:"args,omitempty"`
	RowsAffected *int64 `json:"rows_affected,omitempty"`
}

const (
	transcriptEventBegin    = "BEGIN"
	transcriptEventCommit   = "COMMIT"
	transcriptEventRollback = "ROLLBACK"
	transcriptEventQuery    = "QUERY"
)

// normalizer replaces volatile arg values (timestamps, UUIDs) with stable
// placeholders so transcripts compare cleanly across runs. UUIDs use first-seen
// ordering so the same value gets the same placeholder.
type normalizer struct {
	uuids  map[string]int
	custom []func(any) (any, bool)
}

func newNormalizer() *normalizer {
	return &normalizer{uuids: make(map[string]int)}
}

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (n *normalizer) normalize(value any) any {
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
		return n.placeholderForUUID(uuid.UUID(v).String())
	case string:
		if uuidRegex.MatchString(v) {
			return n.placeholderForUUID(strings.ToLower(v))
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

func (n *normalizer) normalizeArgs(args []any) []any {
	if len(args) == 0 {
		return nil
	}
	out := make([]any, len(args))
	for i, a := range args {
		out[i] = n.normalize(a)
	}
	return out
}

func goldenPath(name string) string {
	return filepath.Join("testdata", "golden", name+".json")
}

func marshalEvents(events []transcriptEvent) ([]byte, error) {
	if events == nil {
		events = []transcriptEvent{}
	}
	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func writeBaseline(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create baseline directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write baseline file %s: %w", path, err)
	}
	return nil
}

func unifiedDiff(path string, baseline, current []byte) (string, bool) {
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

// assertBaseline writes current to path on first run or when overwrite is true,
// otherwise diffs against the existing baseline. kind labels the artifact in
// log/error messages (e.g. "golden transcript", "plan").
func assertBaseline(t goldenT, path string, current []byte, kind string, overwrite bool) {
	t.Helper()
	_, statErr := os.Stat(path)
	missing := os.IsNotExist(statErr)
	if missing || overwrite {
		if err := writeBaseline(path, current); err != nil {
			t.Errorf("%v", err)
			return
		}
		if missing {
			t.Logf("created %s baseline: %s", kind, path)
		} else {
			t.Logf("regenerated %s baseline: %s", kind, path)
		}
		return
	}
	baseline, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("failed to read %s file %s: %v", kind, path, err)
		return
	}
	diff, ok := unifiedDiff(path, baseline, current)
	if ok {
		return
	}
	t.Errorf("%s mismatch for %s\n%s", kind, path, diff)
}
