package pgxkit

import (
	"database/sql"
	"fmt"
	"reflect"
	"time"

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

// assignReplay assigns src to dst, handling the common pgx-style destination
// kinds plus a reflect-based fallback for assignable types.
func assignReplay(dst, src any, colName string) error {
	if dst == nil {
		return fmt.Errorf("replayRows: nil destination for column %q", colName)
	}

	if scanner, ok := dst.(sql.Scanner); ok {
		return scanner.Scan(src)
	}

	switch d := dst.(type) {
	case *any:
		*d = src
		return nil
	case *string:
		s, err := toString(src)
		if err != nil {
			return scanErr(colName, src, dst, err)
		}
		*d = s
		return nil
	case *[]byte:
		b, ok := src.([]byte)
		if !ok {
			return scanErr(colName, src, dst, nil)
		}
		*d = b
		return nil
	case *bool:
		b, ok := src.(bool)
		if !ok {
			return scanErr(colName, src, dst, nil)
		}
		*d = b
		return nil
	case *time.Time:
		t, ok := src.(time.Time)
		if !ok {
			return scanErr(colName, src, dst, nil)
		}
		*d = t
		return nil
	case *float32:
		f, err := toFloat64(src)
		if err != nil {
			return scanErr(colName, src, dst, err)
		}
		*d = float32(f)
		return nil
	case *float64:
		f, err := toFloat64(src)
		if err != nil {
			return scanErr(colName, src, dst, err)
		}
		*d = f
		return nil
	case *int:
		n, err := toInt64(src)
		if err != nil {
			return scanErr(colName, src, dst, err)
		}
		*d = int(n)
		return nil
	case *int8:
		n, err := toInt64(src)
		if err != nil {
			return scanErr(colName, src, dst, err)
		}
		*d = int8(n)
		return nil
	case *int16:
		n, err := toInt64(src)
		if err != nil {
			return scanErr(colName, src, dst, err)
		}
		*d = int16(n)
		return nil
	case *int32:
		n, err := toInt64(src)
		if err != nil {
			return scanErr(colName, src, dst, err)
		}
		*d = int32(n)
		return nil
	case *int64:
		n, err := toInt64(src)
		if err != nil {
			return scanErr(colName, src, dst, err)
		}
		*d = n
		return nil
	case *uint:
		n, err := toUint64(src)
		if err != nil {
			return scanErr(colName, src, dst, err)
		}
		*d = uint(n)
		return nil
	case *uint8:
		n, err := toUint64(src)
		if err != nil {
			return scanErr(colName, src, dst, err)
		}
		*d = uint8(n)
		return nil
	case *uint16:
		n, err := toUint64(src)
		if err != nil {
			return scanErr(colName, src, dst, err)
		}
		*d = uint16(n)
		return nil
	case *uint32:
		n, err := toUint64(src)
		if err != nil {
			return scanErr(colName, src, dst, err)
		}
		*d = uint32(n)
		return nil
	case *uint64:
		n, err := toUint64(src)
		if err != nil {
			return scanErr(colName, src, dst, err)
		}
		*d = n
		return nil
	}

	dv := reflect.ValueOf(dst)
	if dv.Kind() != reflect.Ptr || dv.IsNil() {
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

func scanErr(colName string, src, dst any, cause error) error {
	if cause != nil {
		return fmt.Errorf("replayRows: cannot scan column %q (%T) into %T: %w", colName, src, dst, cause)
	}
	return fmt.Errorf("replayRows: cannot scan column %q (%T) into %T", colName, src, dst)
}

func toInt64(src any) (int64, error) {
	switch v := src.(type) {
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case uint:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint64:
		return int64(v), nil
	}
	return 0, fmt.Errorf("not an integer (%T)", src)
}

func toUint64(src any) (uint64, error) {
	switch v := src.(type) {
	case int:
		return uint64(v), nil
	case int8:
		return uint64(v), nil
	case int16:
		return uint64(v), nil
	case int32:
		return uint64(v), nil
	case int64:
		return uint64(v), nil
	case uint:
		return uint64(v), nil
	case uint8:
		return uint64(v), nil
	case uint16:
		return uint64(v), nil
	case uint32:
		return uint64(v), nil
	case uint64:
		return v, nil
	}
	return 0, fmt.Errorf("not an integer (%T)", src)
}

func toFloat64(src any) (float64, error) {
	switch v := src.(type) {
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	}
	if n, err := toInt64(src); err == nil {
		return float64(n), nil
	}
	return 0, fmt.Errorf("not a float (%T)", src)
}

func toString(src any) (string, error) {
	switch v := src.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	}
	return "", fmt.Errorf("not a string (%T)", src)
}
