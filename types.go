package pgxkit

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Type conversion utilities for pgx types
// These functions provide seamless conversion between Go types and pgx types,
// handling null values appropriately. Use these instead of manual type conversions.

// =============================================================================
// TEXT / STRING CONVERSIONS
// =============================================================================

// ToPgxText converts a string pointer to pgtype.Text.
// If the input is nil, returns an invalid pgtype.Text (NULL in database).
func ToPgxText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// FromPgxText converts a pgtype.Text to a string pointer.
// If the pgtype.Text is invalid (NULL), returns nil.
func FromPgxText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

// ToPgxTextFromString converts a string value to pgtype.Text.
// Use this when you have a string value instead of a pointer.
func ToPgxTextFromString(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

// FromPgxTextToString converts a pgtype.Text to a string value.
// If the pgtype.Text is invalid (NULL), returns empty string.
func FromPgxTextToString(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

// =============================================================================
// INTEGER CONVERSIONS
// =============================================================================

// ToPgxInt8 converts an int64 pointer to pgtype.Int8.
// If the input is nil, returns an invalid pgtype.Int8 (NULL in database).
func ToPgxInt8(i *int64) pgtype.Int8 {
	if i == nil {
		return pgtype.Int8{Valid: false}
	}
	return pgtype.Int8{Int64: *i, Valid: true}
}

// FromPgxInt8 converts a pgtype.Int8 to an int64 pointer.
// If the pgtype.Int8 is invalid (NULL), returns nil.
func FromPgxInt8(i pgtype.Int8) *int64 {
	if !i.Valid {
		return nil
	}
	return &i.Int64
}

// ToPgxInt4 converts an int32 pointer to pgtype.Int4.
// If the input is nil, returns an invalid pgtype.Int4 (NULL in database).
func ToPgxInt4(i *int32) pgtype.Int4 {
	if i == nil {
		return pgtype.Int4{Valid: false}
	}
	return pgtype.Int4{Int32: *i, Valid: true}
}

// FromPgxInt4 converts a pgtype.Int4 to an int32 pointer.
// If the pgtype.Int4 is invalid (NULL), returns nil.
func FromPgxInt4(i pgtype.Int4) *int32 {
	if !i.Valid {
		return nil
	}
	return &i.Int32
}

// ToPgxInt4FromInt converts an int pointer to pgtype.Int4.
// If the input is nil, returns an invalid pgtype.Int4 (NULL in database).
func ToPgxInt4FromInt(i *int) pgtype.Int4 {
	if i == nil {
		return pgtype.Int4{Valid: false}
	}
	return pgtype.Int4{Int32: int32(*i), Valid: true}
}

// FromPgxInt4ToInt converts a pgtype.Int4 to an int pointer.
// If the pgtype.Int4 is invalid (NULL), returns nil.
func FromPgxInt4ToInt(i pgtype.Int4) *int {
	if !i.Valid {
		return nil
	}
	result := int(i.Int32)
	return &result
}

// ToPgxInt2 converts an int16 pointer to pgtype.Int2.
// If the input is nil, returns an invalid pgtype.Int2 (NULL in database).
func ToPgxInt2(i *int16) pgtype.Int2 {
	if i == nil {
		return pgtype.Int2{Valid: false}
	}
	return pgtype.Int2{Int16: *i, Valid: true}
}

// FromPgxInt2 converts a pgtype.Int2 to an int16 pointer.
// If the pgtype.Int2 is invalid (NULL), returns nil.
func FromPgxInt2(i pgtype.Int2) *int16 {
	if !i.Valid {
		return nil
	}
	return &i.Int16
}

// =============================================================================
// BOOLEAN CONVERSIONS
// =============================================================================

// ToPgxBool converts a bool pointer to pgtype.Bool.
// If the input is nil, returns an invalid pgtype.Bool (NULL in database).
func ToPgxBool(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{Valid: false}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

// FromPgxBool converts a pgtype.Bool to a bool pointer.
// If the pgtype.Bool is invalid (NULL), returns nil.
func FromPgxBool(b pgtype.Bool) *bool {
	if !b.Valid {
		return nil
	}
	return &b.Bool
}

// ToPgxBoolFromBool converts a bool value to pgtype.Bool.
// Use this when you have a bool value instead of a pointer.
func ToPgxBoolFromBool(b bool) pgtype.Bool {
	return pgtype.Bool{Bool: b, Valid: true}
}

// FromPgxBoolToBool converts a pgtype.Bool to a bool value.
// If the pgtype.Bool is invalid (NULL), returns false.
func FromPgxBoolToBool(b pgtype.Bool) bool {
	if !b.Valid {
		return false
	}
	return b.Bool
}

// =============================================================================
// FLOAT / NUMERIC CONVERSIONS
// =============================================================================

// ToPgxFloat8 converts a float64 pointer to pgtype.Float8.
// If the input is nil, returns an invalid pgtype.Float8 (NULL in database).
func ToPgxFloat8(f *float64) pgtype.Float8 {
	if f == nil {
		return pgtype.Float8{Valid: false}
	}
	return pgtype.Float8{Float64: *f, Valid: true}
}

// FromPgxFloat8 converts a pgtype.Float8 to a float64 pointer.
// If the pgtype.Float8 is invalid (NULL), returns nil.
func FromPgxFloat8(f pgtype.Float8) *float64 {
	if !f.Valid {
		return nil
	}
	return &f.Float64
}

// ToPgxFloat4 converts a float32 pointer to pgtype.Float4.
// If the input is nil, returns an invalid pgtype.Float4 (NULL in database).
func ToPgxFloat4(f *float32) pgtype.Float4 {
	if f == nil {
		return pgtype.Float4{Valid: false}
	}
	return pgtype.Float4{Float32: *f, Valid: true}
}

// FromPgxFloat4 converts a pgtype.Float4 to a float32 pointer.
// If the pgtype.Float4 is invalid (NULL), returns nil.
func FromPgxFloat4(f pgtype.Float4) *float32 {
	if !f.Valid {
		return nil
	}
	return &f.Float32
}

// ToPgxNumeric converts a float64 pointer to pgtype.Numeric.
// If the input is nil, returns an invalid pgtype.Numeric (NULL in database).
// Uses 6 decimal places as standard precision.
func ToPgxNumeric(f *float64) pgtype.Numeric {
	if f == nil {
		return pgtype.Numeric{Valid: false}
	}
	// Convert float64 to string first, then scan (use 6 decimal places as standard)
	strVal := fmt.Sprintf("%.6f", *f)
	var num pgtype.Numeric
	if err := num.Scan(strVal); err != nil {
		return pgtype.Numeric{Valid: false}
	}
	return num
}

// FromPgxNumeric converts a pgtype.Numeric to a float64 pointer.
// If the pgtype.Numeric is invalid (NULL), returns nil.
func FromPgxNumeric(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f64, err := n.Float64Value()
	if err != nil {
		return nil
	}
	result := f64.Float64
	return &result
}

// Legacy aliases for backward compatibility
// TODO: Consider deprecating these in favor of the new names
var (
	ToPgxNumericFromFloat64Ptr = ToPgxNumeric
	FromPgxNumericPtr          = FromPgxNumeric
)

// =============================================================================
// UUID CONVERSIONS
// =============================================================================

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

// ToPgxUUIDFromPtr converts a uuid.UUID pointer to pgtype.UUID.
// If the input is nil, returns an invalid pgtype.UUID (NULL in database).
func ToPgxUUIDFromPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{Valid: false}
	}
	return ToPgxUUID(*id)
}

// FromPgxUUIDToPtr converts a pgtype.UUID to a uuid.UUID pointer.
// If the pgtype.UUID is invalid (NULL), returns nil.
func FromPgxUUIDToPtr(pgxID pgtype.UUID) *uuid.UUID {
	if !pgxID.Valid {
		return nil
	}
	id := FromPgxUUID(pgxID)
	if id == uuid.Nil {
		return nil
	}
	return &id
}

// =============================================================================
// TIME / TIMESTAMP CONVERSIONS
// =============================================================================

// ToPgxTimestamp converts a time.Time pointer to pgtype.Timestamp.
// If the input is nil, returns an invalid pgtype.Timestamp (NULL in database).
func ToPgxTimestamp(t *time.Time) pgtype.Timestamp {
	if t == nil {
		return pgtype.Timestamp{Valid: false}
	}
	return pgtype.Timestamp{Time: *t, Valid: true}
}

// FromPgxTimestamp converts a pgtype.Timestamp to a time.Time pointer.
// If the pgtype.Timestamp is invalid (NULL), returns nil.
func FromPgxTimestamp(t pgtype.Timestamp) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

// ToPgxTimestamptz converts a time.Time pointer to pgtype.Timestamptz.
// If the input is nil, returns an invalid pgtype.Timestamptz (NULL in database).
func ToPgxTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// FromPgxTimestamptz converts a pgtype.Timestamptz to a time.Time value.
// If the pgtype.Timestamptz is invalid (NULL), returns zero time.
func FromPgxTimestamptz(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}

// FromPgxTimestamptzPtr converts a pgtype.Timestamptz to a time.Time pointer.
// If the pgtype.Timestamptz is invalid (NULL), returns nil.
func FromPgxTimestamptzPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

// ToPgxDate converts a time.Time pointer to pgtype.Date.
// If the input is nil, returns an invalid pgtype.Date (NULL in database).
func ToPgxDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

// FromPgxDate converts a pgtype.Date to a time.Time pointer.
// If the pgtype.Date is invalid (NULL), returns nil.
func FromPgxDate(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	return &d.Time
}

// ToPgxTime converts a time.Time pointer to pgtype.Time.
// If the input is nil, returns an invalid pgtype.Time (NULL in database).
func ToPgxTime(t *time.Time) pgtype.Time {
	if t == nil {
		return pgtype.Time{Valid: false}
	}
	// Convert time.Time to microseconds since midnight
	midnight := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	microseconds := t.Sub(midnight).Microseconds()
	return pgtype.Time{Microseconds: microseconds, Valid: true}
}

// FromPgxTime converts a pgtype.Time to a time.Time pointer.
// If the pgtype.Time is invalid (NULL), returns nil.
// The returned time will be on the current date with the time component.
func FromPgxTime(t pgtype.Time) *time.Time {
	if !t.Valid {
		return nil
	}
	// Create a time on today's date with the time component
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	result := midnight.Add(time.Duration(t.Microseconds) * time.Microsecond)
	return &result
}

// =============================================================================
// JSON CONVERSIONS
// =============================================================================

// Note: JSON and JSONB types are not available in pgtype package
// For JSON support, use []byte or string types with manual marshaling/unmarshaling

// =============================================================================
// ARRAY CONVERSIONS
// =============================================================================

// ToPgxTextArray converts a string slice to pgtype.Array[pgtype.Text].
// If the input is nil, returns an invalid array (NULL in database).
func ToPgxTextArray(s []string) pgtype.Array[pgtype.Text] {
	if s == nil {
		return pgtype.Array[pgtype.Text]{Valid: false}
	}

	elements := make([]pgtype.Text, len(s))
	for i, str := range s {
		elements[i] = pgtype.Text{String: str, Valid: true}
	}

	return pgtype.Array[pgtype.Text]{Elements: elements, Valid: true}
}

// FromPgxTextArray converts a pgtype.Array[pgtype.Text] to a string slice.
// If the array is invalid (NULL), returns nil.
func FromPgxTextArray(a pgtype.Array[pgtype.Text]) []string {
	if !a.Valid {
		return nil
	}

	result := make([]string, len(a.Elements))
	for i, elem := range a.Elements {
		if elem.Valid {
			result[i] = elem.String
		}
		// Invalid elements become empty strings
	}

	return result
}

// ToPgxInt8Array converts an int64 slice to pgtype.Array[pgtype.Int8].
// If the input is nil, returns an invalid array (NULL in database).
func ToPgxInt8Array(s []int64) pgtype.Array[pgtype.Int8] {
	if s == nil {
		return pgtype.Array[pgtype.Int8]{Valid: false}
	}

	elements := make([]pgtype.Int8, len(s))
	for i, val := range s {
		elements[i] = pgtype.Int8{Int64: val, Valid: true}
	}

	return pgtype.Array[pgtype.Int8]{Elements: elements, Valid: true}
}

// FromPgxInt8Array converts a pgtype.Array[pgtype.Int8] to an int64 slice.
// If the array is invalid (NULL), returns nil.
func FromPgxInt8Array(a pgtype.Array[pgtype.Int8]) []int64 {
	if !a.Valid {
		return nil
	}

	result := make([]int64, len(a.Elements))
	for i, elem := range a.Elements {
		if elem.Valid {
			result[i] = elem.Int64
		}
		// Invalid elements become 0
	}

	return result
}

// =============================================================================
// BYTES CONVERSIONS
// =============================================================================

// Note: Bytea type is not available in pgtype package
// For bytea support, use []byte directly with pgx scan/value interfaces
