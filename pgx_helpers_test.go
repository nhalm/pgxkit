package dbutil

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestToPgxText(t *testing.T) {
	// Test with valid string
	str := "hello"
	result := ToPgxText(&str)
	if !result.Valid || result.String != "hello" {
		t.Errorf("Expected valid text 'hello', got valid=%v, string=%v", result.Valid, result.String)
	}

	// Test with nil
	result = ToPgxText(nil)
	if result.Valid {
		t.Errorf("Expected invalid text for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxText(t *testing.T) {
	// Test with valid pgtype.Text
	pgText := pgtype.Text{String: "hello", Valid: true}
	result := FromPgxText(pgText)
	if result == nil || *result != "hello" {
		t.Errorf("Expected 'hello', got %v", result)
	}

	// Test with invalid pgtype.Text
	pgText = pgtype.Text{Valid: false}
	result = FromPgxText(pgText)
	if result != nil {
		t.Errorf("Expected nil for invalid text, got %v", result)
	}
}

func TestFromPgxTextToString(t *testing.T) {
	// Test with valid pgtype.Text
	pgText := pgtype.Text{String: "hello", Valid: true}
	result := FromPgxTextToString(pgText)
	if result != "hello" {
		t.Errorf("Expected 'hello', got %v", result)
	}

	// Test with invalid pgtype.Text
	pgText = pgtype.Text{Valid: false}
	result = FromPgxTextToString(pgText)
	if result != "" {
		t.Errorf("Expected empty string for invalid text, got %v", result)
	}
}

func TestToPgxInt8(t *testing.T) {
	// Test with valid int64
	val := int64(42)
	result := ToPgxInt8(&val)
	if !result.Valid || result.Int64 != 42 {
		t.Errorf("Expected valid int64 42, got valid=%v, int64=%v", result.Valid, result.Int64)
	}

	// Test with nil
	result = ToPgxInt8(nil)
	if result.Valid {
		t.Errorf("Expected invalid int64 for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxInt8(t *testing.T) {
	// Test with valid pgtype.Int8
	pgInt := pgtype.Int8{Int64: 42, Valid: true}
	result := FromPgxInt8(pgInt)
	if result == nil || *result != 42 {
		t.Errorf("Expected 42, got %v", result)
	}

	// Test with invalid pgtype.Int8
	pgInt = pgtype.Int8{Valid: false}
	result = FromPgxInt8(pgInt)
	if result != nil {
		t.Errorf("Expected nil for invalid int8, got %v", result)
	}
}

func TestToPgxInt4FromInt(t *testing.T) {
	// Test with valid int
	val := 42
	result := ToPgxInt4FromInt(&val)
	if !result.Valid || result.Int32 != 42 {
		t.Errorf("Expected valid int32 42, got valid=%v, int32=%v", result.Valid, result.Int32)
	}

	// Test with nil
	result = ToPgxInt4FromInt(nil)
	if result.Valid {
		t.Errorf("Expected invalid int32 for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxInt4(t *testing.T) {
	// Test with valid pgtype.Int4
	pgInt := pgtype.Int4{Int32: 42, Valid: true}
	result := FromPgxInt4(pgInt)
	if result == nil || *result != 42 {
		t.Errorf("Expected 42, got %v", result)
	}

	// Test with invalid pgtype.Int4
	pgInt = pgtype.Int4{Valid: false}
	result = FromPgxInt4(pgInt)
	if result != nil {
		t.Errorf("Expected nil for invalid int4, got %v", result)
	}
}

func TestToPgxBool(t *testing.T) {
	// Test with valid bool
	val := true
	result := ToPgxBool(&val)
	if !result.Valid || !result.Bool {
		t.Errorf("Expected valid bool true, got valid=%v, bool=%v", result.Valid, result.Bool)
	}

	// Test with false
	val = false
	result = ToPgxBool(&val)
	if !result.Valid || result.Bool {
		t.Errorf("Expected valid bool false, got valid=%v, bool=%v", result.Valid, result.Bool)
	}

	// Test with nil
	result = ToPgxBool(nil)
	if result.Valid {
		t.Errorf("Expected invalid bool for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxBool(t *testing.T) {
	// Test with valid pgtype.Bool
	pgBool := pgtype.Bool{Bool: true, Valid: true}
	result := FromPgxBool(pgBool)
	if result == nil || *result != true {
		t.Errorf("Expected true, got %v", result)
	}

	// Test with invalid pgtype.Bool
	pgBool = pgtype.Bool{Valid: false}
	result = FromPgxBool(pgBool)
	if result != nil {
		t.Errorf("Expected nil for invalid bool, got %v", result)
	}
}

func TestToPgxUUID(t *testing.T) {
	// Test with valid UUID
	id := uuid.New()
	result := ToPgxUUID(id)
	if !result.Valid {
		t.Errorf("Expected valid UUID, got valid=%v", result.Valid)
	}

	// Convert back to verify
	converted := FromPgxUUID(result)
	if converted != id {
		t.Errorf("Expected %v, got %v", id, converted)
	}
}

func TestFromPgxUUID(t *testing.T) {
	// Test with valid pgtype.UUID
	id := uuid.New()
	var pgUUID pgtype.UUID
	_ = pgUUID.Scan(id.String())

	result := FromPgxUUID(pgUUID)
	if result != id {
		t.Errorf("Expected %v, got %v", id, result)
	}
}

func TestToPgxTimestamptz(t *testing.T) {
	// Test with valid time
	now := time.Now()
	result := ToPgxTimestamptz(&now)
	if !result.Valid || !result.Time.Equal(now) {
		t.Errorf("Expected valid time %v, got valid=%v, time=%v", now, result.Valid, result.Time)
	}

	// Test with nil
	result = ToPgxTimestamptz(nil)
	if result.Valid {
		t.Errorf("Expected invalid time for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxTimestamptz(t *testing.T) {
	// Test with valid pgtype.Timestamptz
	now := time.Now()
	pgTime := pgtype.Timestamptz{Time: now, Valid: true}
	result := FromPgxTimestamptz(pgTime)
	if !result.Equal(now) {
		t.Errorf("Expected %v, got %v", now, result)
	}

	// Test with invalid pgtype.Timestamptz
	pgTime = pgtype.Timestamptz{Valid: false}
	result = FromPgxTimestamptz(pgTime)
	if !result.IsZero() {
		t.Errorf("Expected zero time for invalid timestamptz, got %v", result)
	}
}

func TestFromPgxTimestamptzPtr(t *testing.T) {
	// Test with valid pgtype.Timestamptz
	now := time.Now()
	pgTime := pgtype.Timestamptz{Time: now, Valid: true}
	result := FromPgxTimestamptzPtr(pgTime)
	if result == nil || !result.Equal(now) {
		t.Errorf("Expected %v, got %v", now, result)
	}

	// Test with invalid pgtype.Timestamptz
	pgTime = pgtype.Timestamptz{Valid: false}
	result = FromPgxTimestamptzPtr(pgTime)
	if result != nil {
		t.Errorf("Expected nil for invalid timestamptz, got %v", result)
	}
}

func TestToPgxNumericFromFloat64Ptr(t *testing.T) {
	// Test with valid float64
	val := 123.456
	result := ToPgxNumericFromFloat64Ptr(&val)
	if !result.Valid {
		t.Errorf("Expected valid numeric, got valid=%v", result.Valid)
	}

	// Convert back to verify (approximately)
	converted := FromPgxNumericPtr(result)
	if converted == nil || *converted < 123.0 || *converted > 124.0 {
		t.Errorf("Expected approximately 123.456, got %v", converted)
	}

	// Test with nil
	result = ToPgxNumericFromFloat64Ptr(nil)
	if result.Valid {
		t.Errorf("Expected invalid numeric for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxNumericPtr(t *testing.T) {
	// Test with valid pgtype.Numeric
	var pgNum pgtype.Numeric
	_ = pgNum.Scan("123.456")

	result := FromPgxNumericPtr(pgNum)
	if result == nil || *result < 123.0 || *result > 124.0 {
		t.Errorf("Expected approximately 123.456, got %v", result)
	}

	// Test with invalid pgtype.Numeric
	pgNum = pgtype.Numeric{Valid: false}
	result = FromPgxNumericPtr(pgNum)
	if result != nil {
		t.Errorf("Expected nil for invalid numeric, got %v", result)
	}
}
