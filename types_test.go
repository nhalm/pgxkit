package pgxkit

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// =============================================================================
// TEXT / STRING TESTS
// =============================================================================

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

func TestToPgxTextFromString(t *testing.T) {
	result := ToPgxTextFromString("hello")
	if !result.Valid || result.String != "hello" {
		t.Errorf("Expected valid text 'hello', got valid=%v, string=%v", result.Valid, result.String)
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

// =============================================================================
// INTEGER TESTS
// =============================================================================

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

func TestToPgxInt4(t *testing.T) {
	// Test with valid int32
	val := int32(42)
	result := ToPgxInt4(&val)
	if !result.Valid || result.Int32 != 42 {
		t.Errorf("Expected valid int32 42, got valid=%v, int32=%v", result.Valid, result.Int32)
	}

	// Test with nil
	result = ToPgxInt4(nil)
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

func TestFromPgxInt4ToInt(t *testing.T) {
	// Test with valid pgtype.Int4
	pgInt := pgtype.Int4{Int32: 42, Valid: true}
	result := FromPgxInt4ToInt(pgInt)
	if result == nil || *result != 42 {
		t.Errorf("Expected 42, got %v", result)
	}

	// Test with invalid pgtype.Int4
	pgInt = pgtype.Int4{Valid: false}
	result = FromPgxInt4ToInt(pgInt)
	if result != nil {
		t.Errorf("Expected nil for invalid int4, got %v", result)
	}
}

func TestToPgxInt2(t *testing.T) {
	// Test with valid int16
	val := int16(42)
	result := ToPgxInt2(&val)
	if !result.Valid || result.Int16 != 42 {
		t.Errorf("Expected valid int16 42, got valid=%v, int16=%v", result.Valid, result.Int16)
	}

	// Test with nil
	result = ToPgxInt2(nil)
	if result.Valid {
		t.Errorf("Expected invalid int16 for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxInt2(t *testing.T) {
	// Test with valid pgtype.Int2
	pgInt := pgtype.Int2{Int16: 42, Valid: true}
	result := FromPgxInt2(pgInt)
	if result == nil || *result != 42 {
		t.Errorf("Expected 42, got %v", result)
	}

	// Test with invalid pgtype.Int2
	pgInt = pgtype.Int2{Valid: false}
	result = FromPgxInt2(pgInt)
	if result != nil {
		t.Errorf("Expected nil for invalid int2, got %v", result)
	}
}

// =============================================================================
// BOOLEAN TESTS
// =============================================================================

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

func TestToPgxBoolFromBool(t *testing.T) {
	result := ToPgxBoolFromBool(true)
	if !result.Valid || !result.Bool {
		t.Errorf("Expected valid bool true, got valid=%v, bool=%v", result.Valid, result.Bool)
	}

	result = ToPgxBoolFromBool(false)
	if !result.Valid || result.Bool {
		t.Errorf("Expected valid bool false, got valid=%v, bool=%v", result.Valid, result.Bool)
	}
}

func TestFromPgxBoolToBool(t *testing.T) {
	// Test with valid pgtype.Bool
	pgBool := pgtype.Bool{Bool: true, Valid: true}
	result := FromPgxBoolToBool(pgBool)
	if result != true {
		t.Errorf("Expected true, got %v", result)
	}

	// Test with invalid pgtype.Bool
	pgBool = pgtype.Bool{Valid: false}
	result = FromPgxBoolToBool(pgBool)
	if result != false {
		t.Errorf("Expected false for invalid bool, got %v", result)
	}
}

// =============================================================================
// FLOAT / NUMERIC TESTS
// =============================================================================

func TestToPgxFloat8(t *testing.T) {
	// Test with valid float64
	val := 123.456
	result := ToPgxFloat8(&val)
	if !result.Valid || result.Float64 != 123.456 {
		t.Errorf("Expected valid float64 123.456, got valid=%v, float64=%v", result.Valid, result.Float64)
	}

	// Test with nil
	result = ToPgxFloat8(nil)
	if result.Valid {
		t.Errorf("Expected invalid float64 for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxFloat8(t *testing.T) {
	// Test with valid pgtype.Float8
	pgFloat := pgtype.Float8{Float64: 123.456, Valid: true}
	result := FromPgxFloat8(pgFloat)
	if result == nil || *result != 123.456 {
		t.Errorf("Expected 123.456, got %v", result)
	}

	// Test with invalid pgtype.Float8
	pgFloat = pgtype.Float8{Valid: false}
	result = FromPgxFloat8(pgFloat)
	if result != nil {
		t.Errorf("Expected nil for invalid float8, got %v", result)
	}
}

func TestToPgxFloat4(t *testing.T) {
	// Test with valid float32
	val := float32(123.456)
	result := ToPgxFloat4(&val)
	if !result.Valid || result.Float32 != float32(123.456) {
		t.Errorf("Expected valid float32 123.456, got valid=%v, float32=%v", result.Valid, result.Float32)
	}

	// Test with nil
	result = ToPgxFloat4(nil)
	if result.Valid {
		t.Errorf("Expected invalid float32 for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxFloat4(t *testing.T) {
	// Test with valid pgtype.Float4
	pgFloat := pgtype.Float4{Float32: float32(123.456), Valid: true}
	result := FromPgxFloat4(pgFloat)
	if result == nil || *result != float32(123.456) {
		t.Errorf("Expected 123.456, got %v", result)
	}

	// Test with invalid pgtype.Float4
	pgFloat = pgtype.Float4{Valid: false}
	result = FromPgxFloat4(pgFloat)
	if result != nil {
		t.Errorf("Expected nil for invalid float4, got %v", result)
	}
}

func TestToPgxNumeric(t *testing.T) {
	// Test with valid float64
	val := 123.456
	result := ToPgxNumeric(&val)
	if !result.Valid {
		t.Errorf("Expected valid numeric, got valid=%v", result.Valid)
	}

	// Convert back to verify (approximately)
	converted := FromPgxNumeric(result)
	if converted == nil || *converted < 123.0 || *converted > 124.0 {
		t.Errorf("Expected approximately 123.456, got %v", converted)
	}

	// Test with nil
	result = ToPgxNumeric(nil)
	if result.Valid {
		t.Errorf("Expected invalid numeric for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxNumeric(t *testing.T) {
	// Test with valid pgtype.Numeric
	var pgNum pgtype.Numeric
	_ = pgNum.Scan("123.456")

	result := FromPgxNumeric(pgNum)
	if result == nil || *result < 123.0 || *result > 124.0 {
		t.Errorf("Expected approximately 123.456, got %v", result)
	}

	// Test with invalid pgtype.Numeric
	pgNum = pgtype.Numeric{Valid: false}
	result = FromPgxNumeric(pgNum)
	if result != nil {
		t.Errorf("Expected nil for invalid numeric, got %v", result)
	}
}

// =============================================================================
// UUID TESTS
// =============================================================================

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

	// Test with invalid pgtype.UUID
	pgUUID = pgtype.UUID{Valid: false}
	result = FromPgxUUID(pgUUID)
	if result != uuid.Nil {
		t.Errorf("Expected uuid.Nil for invalid UUID, got %v", result)
	}
}

func TestToPgxUUIDFromPtr(t *testing.T) {
	// Test with valid UUID pointer
	id := uuid.New()
	result := ToPgxUUIDFromPtr(&id)
	if !result.Valid {
		t.Errorf("Expected valid UUID, got valid=%v", result.Valid)
	}

	// Test with nil
	result = ToPgxUUIDFromPtr(nil)
	if result.Valid {
		t.Errorf("Expected invalid UUID for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxUUIDToPtr(t *testing.T) {
	// Test with valid pgtype.UUID
	id := uuid.New()
	var pgUUID pgtype.UUID
	_ = pgUUID.Scan(id.String())

	result := FromPgxUUIDToPtr(pgUUID)
	if result == nil || *result != id {
		t.Errorf("Expected %v, got %v", id, result)
	}

	// Test with invalid pgtype.UUID
	pgUUID = pgtype.UUID{Valid: false}
	result = FromPgxUUIDToPtr(pgUUID)
	if result != nil {
		t.Errorf("Expected nil for invalid UUID, got %v", result)
	}
}

// =============================================================================
// TIME / TIMESTAMP TESTS
// =============================================================================

func TestToPgxTimestamp(t *testing.T) {
	// Test with valid time
	now := time.Now()
	result := ToPgxTimestamp(&now)
	if !result.Valid || !result.Time.Equal(now) {
		t.Errorf("Expected valid time %v, got valid=%v, time=%v", now, result.Valid, result.Time)
	}

	// Test with nil
	result = ToPgxTimestamp(nil)
	if result.Valid {
		t.Errorf("Expected invalid time for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxTimestamp(t *testing.T) {
	// Test with valid pgtype.Timestamp
	now := time.Now()
	pgTime := pgtype.Timestamp{Time: now, Valid: true}
	result := FromPgxTimestamp(pgTime)
	if result == nil || !result.Equal(now) {
		t.Errorf("Expected %v, got %v", now, result)
	}

	// Test with invalid pgtype.Timestamp
	pgTime = pgtype.Timestamp{Valid: false}
	result = FromPgxTimestamp(pgTime)
	if result != nil {
		t.Errorf("Expected nil for invalid timestamp, got %v", result)
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

func TestToPgxDate(t *testing.T) {
	// Test with valid time
	now := time.Now()
	result := ToPgxDate(&now)
	if !result.Valid || !result.Time.Equal(now) {
		t.Errorf("Expected valid date %v, got valid=%v, time=%v", now, result.Valid, result.Time)
	}

	// Test with nil
	result = ToPgxDate(nil)
	if result.Valid {
		t.Errorf("Expected invalid date for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxDate(t *testing.T) {
	// Test with valid pgtype.Date
	now := time.Now()
	pgDate := pgtype.Date{Time: now, Valid: true}
	result := FromPgxDate(pgDate)
	if result == nil || !result.Equal(now) {
		t.Errorf("Expected %v, got %v", now, result)
	}

	// Test with invalid pgtype.Date
	pgDate = pgtype.Date{Valid: false}
	result = FromPgxDate(pgDate)
	if result != nil {
		t.Errorf("Expected nil for invalid date, got %v", result)
	}
}

// =============================================================================
// JSON TESTS
// =============================================================================

// Note: JSON and JSONB types are not available in pgtype package
// Tests removed - use []byte or string types with manual marshaling/unmarshaling

// =============================================================================
// ARRAY TESTS
// =============================================================================

func TestToPgxTextArray(t *testing.T) {
	// Test with valid string slice
	data := []string{"hello", "world"}
	result := ToPgxTextArray(data)
	if !result.Valid || len(result.Elements) != 2 {
		t.Errorf("Expected valid array with 2 elements, got valid=%v, len=%v", result.Valid, len(result.Elements))
	}
	if result.Elements[0].String != "hello" || result.Elements[1].String != "world" {
		t.Errorf("Expected [hello, world], got [%v, %v]", result.Elements[0].String, result.Elements[1].String)
	}

	// Test with nil
	result = ToPgxTextArray(nil)
	if result.Valid {
		t.Errorf("Expected invalid array for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxTextArray(t *testing.T) {
	// Test with valid pgtype.Array[pgtype.Text]
	elements := []pgtype.Text{
		{String: "hello", Valid: true},
		{String: "world", Valid: true},
	}
	pgArray := pgtype.Array[pgtype.Text]{Elements: elements, Valid: true}
	result := FromPgxTextArray(pgArray)
	if len(result) != 2 || result[0] != "hello" || result[1] != "world" {
		t.Errorf("Expected [hello, world], got %v", result)
	}

	// Test with invalid pgtype.Array
	pgArray = pgtype.Array[pgtype.Text]{Valid: false}
	result = FromPgxTextArray(pgArray)
	if result != nil {
		t.Errorf("Expected nil for invalid array, got %v", result)
	}
}

func TestToPgxInt8Array(t *testing.T) {
	// Test with valid int64 slice
	data := []int64{1, 2, 3}
	result := ToPgxInt8Array(data)
	if !result.Valid || len(result.Elements) != 3 {
		t.Errorf("Expected valid array with 3 elements, got valid=%v, len=%v", result.Valid, len(result.Elements))
	}
	if result.Elements[0].Int64 != 1 || result.Elements[1].Int64 != 2 || result.Elements[2].Int64 != 3 {
		t.Errorf("Expected [1, 2, 3], got [%v, %v, %v]", result.Elements[0].Int64, result.Elements[1].Int64, result.Elements[2].Int64)
	}

	// Test with nil
	result = ToPgxInt8Array(nil)
	if result.Valid {
		t.Errorf("Expected invalid array for nil, got valid=%v", result.Valid)
	}
}

func TestFromPgxInt8Array(t *testing.T) {
	// Test with valid pgtype.Array[pgtype.Int8]
	elements := []pgtype.Int8{
		{Int64: 1, Valid: true},
		{Int64: 2, Valid: true},
		{Int64: 3, Valid: true},
	}
	pgArray := pgtype.Array[pgtype.Int8]{Elements: elements, Valid: true}
	result := FromPgxInt8Array(pgArray)
	if len(result) != 3 || result[0] != 1 || result[1] != 2 || result[2] != 3 {
		t.Errorf("Expected [1, 2, 3], got %v", result)
	}

	// Test with invalid pgtype.Array
	pgArray = pgtype.Array[pgtype.Int8]{Valid: false}
	result = FromPgxInt8Array(pgArray)
	if result != nil {
		t.Errorf("Expected nil for invalid array, got %v", result)
	}
}

// =============================================================================
// BYTES TESTS
// =============================================================================

// Note: Bytea type is not available in pgtype package
// Tests removed - use []byte directly with pgx scan/value interfaces


