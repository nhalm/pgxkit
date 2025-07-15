package dbutil

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// --- Helper functions for pgx types ---
// These functions provide seamless conversion between Go types and pgx types,
// handling null values appropriately. Use these instead of manual type conversions.

// ToPgxText converts a string pointer to pgtype.Text.
// If the input is nil, returns an invalid pgtype.Text (NULL in database).
func ToPgxText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// ToPgxInt8 converts an int64 pointer to pgtype.Int8.
// If the input is nil, returns an invalid pgtype.Int8 (NULL in database).
func ToPgxInt8(i *int64) pgtype.Int8 {
	if i == nil {
		return pgtype.Int8{Valid: false}
	}
	return pgtype.Int8{Int64: *i, Valid: true}
}

// ToPgxUUID converts a uuid.UUID to pgtype.UUID.
func ToPgxUUID(id uuid.UUID) pgtype.UUID {
	var pgxID pgtype.UUID
	_ = pgxID.Scan(id.String()) // Error intentionally ignored as uuid.UUID is always valid
	return pgxID
}

// FromPgxUUID converts a pgtype.UUID to uuid.UUID.
// Returns uuid.Nil if the pgtype.UUID is invalid or cannot be parsed.
func FromPgxUUID(pgxID pgtype.UUID) uuid.UUID {
	if !pgxID.Valid {
		return uuid.Nil
	}
	id, err := uuid.Parse(pgxID.String())
	if err != nil {
		return uuid.Nil
	}
	return id
}

// FromPgxText converts a pgtype.Text to a string pointer.
// If the pgtype.Text is invalid (NULL), returns nil.
func FromPgxText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func FromPgxInt8(i pgtype.Int8) *int64 {
	if !i.Valid {
		return nil
	}
	return &i.Int64
}

func FromPgxTimestamptz(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time
}

func ToPgxTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func FromPgxTimestamptzPtr(val pgtype.Timestamptz) *time.Time {
	if !val.Valid {
		return nil
	}
	return &val.Time
}

func FromPgxTextToString(val pgtype.Text) string {
	if !val.Valid {
		return ""
	}
	return val.String
}

func FromPgxInt4(val pgtype.Int4) *int {
	if !val.Valid {
		return nil
	}
	result := int(val.Int32)
	return &result
}

func ToPgxInt4FromInt(val *int) pgtype.Int4 {
	if val == nil {
		return pgtype.Int4{Valid: false}
	}
	return pgtype.Int4{Int32: int32(*val), Valid: true}
}

// ToPgxBool converts a *bool to pgtype.Bool
func ToPgxBool(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{Valid: false}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

// FromPgxBool converts pgtype.Bool to *bool
func FromPgxBool(b pgtype.Bool) *bool {
	if !b.Valid {
		return nil
	}
	return &b.Bool
}

// ToPgxNumericFromFloat64Ptr converts *float64 to pgtype.Numeric with configurable precision
func ToPgxNumericFromFloat64Ptr(val *float64) pgtype.Numeric {
	if val == nil {
		return pgtype.Numeric{Valid: false}
	}
	// Convert float64 to string first, then scan (use 6 decimal places as standard)
	strVal := fmt.Sprintf("%.6f", *val)
	var num pgtype.Numeric
	if err := num.Scan(strVal); err != nil {
		return pgtype.Numeric{Valid: false}
	}
	return num
}

// FromPgxNumericPtr converts pgtype.Numeric to *float64
func FromPgxNumericPtr(val pgtype.Numeric) *float64 {
	if !val.Valid {
		return nil
	}
	f64, err := val.Float64Value()
	if err != nil {
		return nil
	}
	result := f64.Float64
	return &result
}
