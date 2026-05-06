package pgxkit

import (
	"database/sql"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// captureRowsForGolden materializes a live pgx.Rows into a replayRows the
// caller can iterate, while also recording a normalized snapshot of the rows
// on the recorder. The original pgx.Rows is consumed and closed.
func captureRowsForGolden(rows pgx.Rows, recorder *transcriptRecorder, sql string, args []any) (*replayRows, error) {
	defer rows.Close()

	fields := append([]pgconn.FieldDescription(nil), rows.FieldDescriptions()...)
	columnNames := make([]string, len(fields))
	for i, f := range fields {
		columnNames[i] = f.Name
	}

	var captured [][]any
	var normalized []map[string]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("failed to read row values for golden capture: %w", err)
		}
		copyValues := make([]any, len(values))
		copy(copyValues, values)
		captured = append(captured, copyValues)
		normalized = append(normalized, recorder.normalizeRow(columnNames, copyValues))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	recorder.recordQuery(sql, args, normalized)
	return newReplayRows(fields, captured, nil), nil
}

// replayRows materializes a query result so the recorder can capture it while
// still letting the caller iterate normally. Rows are decoded once via pgx
// (rows.Values()) and then replayed through the pgx.Rows interface.
//
// Documented limitations:
//   - RawValues returns nil; the underlying pgx wire bytes are not retained.
//   - Conn returns nil; replayRows is not bound to a live connection.
type replayRows struct {
	fields []pgconn.FieldDescription
	rows   [][]any
	cursor int
	closed bool
	err    error
}

func newReplayRows(fields []pgconn.FieldDescription, rows [][]any, err error) *replayRows {
	return &replayRows{
		fields: fields,
		rows:   rows,
		cursor: -1,
		err:    err,
	}
}

func (r *replayRows) Next() bool {
	if r.closed || r.err != nil {
		return false
	}
	r.cursor++
	return r.cursor < len(r.rows)
}

func (r *replayRows) Values() ([]any, error) {
	if r.cursor < 0 || r.cursor >= len(r.rows) {
		return nil, fmt.Errorf("replayRows: Values called outside iteration")
	}
	row := r.rows[r.cursor]
	out := make([]any, len(row))
	copy(out, row)
	return out, nil
}

func (r *replayRows) Scan(dest ...any) error {
	if r.cursor < 0 || r.cursor >= len(r.rows) {
		return fmt.Errorf("replayRows: Scan called outside iteration")
	}
	row := r.rows[r.cursor]
	if len(dest) != len(row) {
		return fmt.Errorf("replayRows: Scan got %d destinations, row has %d columns", len(dest), len(row))
	}
	for i := range dest {
		colName := ""
		if i < len(r.fields) {
			colName = r.fields[i].Name
		}
		if err := assignReplay(dest[i], row[i], colName); err != nil {
			return err
		}
	}
	return nil
}

func (r *replayRows) FieldDescriptions() []pgconn.FieldDescription {
	return r.fields
}

func (r *replayRows) Err() error {
	return r.err
}

func (r *replayRows) Close() {
	r.closed = true
}

func (r *replayRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

// Conn returns nil — replayRows is not bound to a live connection.
func (r *replayRows) Conn() *pgx.Conn {
	return nil
}

// RawValues returns nil — the underlying wire bytes are not retained after
// the original rows.Values() decode.
func (r *replayRows) RawValues() [][]byte {
	return nil
}

// replayRow wraps the first row of a captured result for QueryRow callers.
// On empty results, Scan returns pgx.ErrNoRows.
type replayRow struct {
	rows *replayRows
}

func (r *replayRow) Scan(dest ...any) error {
	if r.rows.err != nil {
		return r.rows.err
	}
	if !r.rows.Next() {
		return pgx.ErrNoRows
	}
	defer r.rows.Close()
	return r.rows.Scan(dest...)
}

// assignReplay assigns src to dst for the test-only replay path. sql.Scanner
// destinations get the raw decoded value; everything else goes through
// reflect-based assign-or-convert. Numeric width/sign conversions follow Go's
// normal conversion rules (silent truncation/overflow), same as a hand-rolled
// type switch.
func assignReplay(dst, src any, colName string) error {
	if dst == nil {
		return fmt.Errorf("replayRows: nil destination for column %q", colName)
	}
	if scanner, ok := dst.(sql.Scanner); ok {
		return scanner.Scan(src)
	}
	if d, ok := dst.(*any); ok {
		*d = src
		return nil
	}

	dv := reflect.ValueOf(dst)
	if dv.Kind() != reflect.Pointer || dv.IsNil() {
		return fmt.Errorf("replayRows: destination for column %q is not a non-nil pointer", colName)
	}
	target := dv.Elem()
	if src == nil {
		target.Set(reflect.Zero(target.Type()))
		return nil
	}
	sv := reflect.ValueOf(src)
	if sv.Type().AssignableTo(target.Type()) {
		target.Set(sv)
		return nil
	}
	if sv.Type().ConvertibleTo(target.Type()) {
		target.Set(sv.Convert(target.Type()))
		return nil
	}
	return fmt.Errorf("replayRows: cannot scan column %q (%T) into %T", colName, src, dst)
}
