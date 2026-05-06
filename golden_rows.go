package pgxkit

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// captureRowsForGolden materializes a live pgx.Rows into a replayRows the
// caller can iterate, while also recording a normalized snapshot of the rows
// on the recorder. Raw wire bytes plus the connection's pgx type map are
// retained so that replay Scan calls go through pgx's normal decode path —
// destinations like *uuid.UUID, *pgtype.UUID, custom sql.Scanners, jsonb,
// and arrays round-trip identically to live rows. The original pgx.Rows is
// consumed and closed.
func captureRowsForGolden(rows pgx.Rows, recorder *transcriptRecorder, sql string, args []any) (*replayRows, error) {
	defer rows.Close()

	fields := append([]pgconn.FieldDescription(nil), rows.FieldDescriptions()...)
	columnNames := make([]string, len(fields))
	for i, f := range fields {
		columnNames[i] = f.Name
	}

	var typeMap *pgtype.Map
	if conn := rows.Conn(); conn != nil {
		typeMap = conn.TypeMap()
	}

	var capturedValues [][]any
	var capturedRaw [][][]byte
	var normalized []map[string]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("failed to read row values for golden capture: %w", err)
		}
		copyValues := make([]any, len(values))
		copy(copyValues, values)
		capturedValues = append(capturedValues, copyValues)
		normalized = append(normalized, recorder.normalizeRow(columnNames, copyValues))

		// pgx reuses the RawValues buffer between Next() calls, so each column
		// must be copied to survive past iteration.
		raw := rows.RawValues()
		copyRaw := make([][]byte, len(raw))
		for i, b := range raw {
			if b == nil {
				continue
			}
			cp := make([]byte, len(b))
			copy(cp, b)
			copyRaw[i] = cp
		}
		capturedRaw = append(capturedRaw, copyRaw)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	recorder.recordQuery(sql, args, normalized)
	return newReplayRows(fields, capturedValues, capturedRaw, typeMap, nil), nil
}

// replayRows materializes a query result so the recorder can capture it while
// still letting the caller iterate normally. Raw wire bytes from the original
// rows are decoded through the captured pgx TypeMap on each Scan, mirroring
// live pgx behavior and preserving the consumer's destination types.
//
// Documented limitations:
//   - Conn returns nil; replayRows is not bound to a live connection.
type replayRows struct {
	fields    []pgconn.FieldDescription
	values    [][]any
	rawValues [][][]byte
	typeMap   *pgtype.Map
	cursor    int
	closed    bool
	err       error
}

func newReplayRows(fields []pgconn.FieldDescription, values [][]any, raw [][][]byte, typeMap *pgtype.Map, err error) *replayRows {
	return &replayRows{
		fields:    fields,
		values:    values,
		rawValues: raw,
		typeMap:   typeMap,
		cursor:    -1,
		err:       err,
	}
}

func (r *replayRows) Next() bool {
	if r.closed || r.err != nil {
		return false
	}
	r.cursor++
	return r.cursor < len(r.values)
}

func (r *replayRows) Values() ([]any, error) {
	if r.cursor < 0 || r.cursor >= len(r.values) {
		return nil, fmt.Errorf("replayRows: Values called outside iteration")
	}
	row := r.values[r.cursor]
	out := make([]any, len(row))
	copy(out, row)
	return out, nil
}

func (r *replayRows) Scan(dest ...any) error {
	if r.cursor < 0 || r.cursor >= len(r.rawValues) {
		return fmt.Errorf("replayRows: Scan called outside iteration")
	}
	row := r.rawValues[r.cursor]
	if len(dest) != len(row) {
		return fmt.Errorf("replayRows: Scan got %d destinations, row has %d columns", len(dest), len(row))
	}
	if r.typeMap == nil {
		return fmt.Errorf("replayRows: no pgx type map available for replay scan")
	}
	for i, d := range dest {
		f := r.fields[i]
		if err := r.typeMap.Scan(f.DataTypeOID, f.Format, row[i], d); err != nil {
			return fmt.Errorf("replayRows: cannot scan column %q: %w", f.Name, err)
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

// RawValues returns the captured wire bytes for the current row, or nil if
// called outside iteration.
func (r *replayRows) RawValues() [][]byte {
	if r.cursor < 0 || r.cursor >= len(r.rawValues) {
		return nil
	}
	return r.rawValues[r.cursor]
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
