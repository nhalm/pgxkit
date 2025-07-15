package dbutil

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestNewNotFoundError(t *testing.T) {
	// Test with string identifier
	err := NewNotFoundError("User", "123")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	var notFoundErr *NotFoundError
	if !errors.As(err, &notFoundErr) {
		t.Fatalf("Expected *NotFoundError, got %T", err)
	}

	if notFoundErr.Entity != "User" {
		t.Errorf("Expected entity 'User', got '%s'", notFoundErr.Entity)
	}

	if notFoundErr.Identifier != "123" {
		t.Errorf("Expected identifier '123', got '%s'", notFoundErr.Identifier)
	}

	expectedMsg := "User not found: 123"
	if notFoundErr.Error() != expectedMsg {
		t.Errorf("Expected message '%s', got '%s'", expectedMsg, notFoundErr.Error())
	}

	// Test with UUID identifier
	id := uuid.New()
	err = NewNotFoundError("Order", id)
	if !errors.As(err, &notFoundErr) {
		t.Fatalf("Expected *NotFoundError, got %T", err)
	}

	expectedMsg = "Order not found: " + id.String()
	if notFoundErr.Error() != expectedMsg {
		t.Errorf("Expected message '%s', got '%s'", expectedMsg, notFoundErr.Error())
	}
}

func TestNewValidationError(t *testing.T) {
	originalErr := errors.New("original error")

	err := NewValidationError("User", "create", "email", "invalid format", originalErr)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Expected *ValidationError, got %T", err)
	}

	if validationErr.Entity != "User" {
		t.Errorf("Expected entity 'User', got '%s'", validationErr.Entity)
	}

	if validationErr.Operation != "create" {
		t.Errorf("Expected operation 'create', got '%s'", validationErr.Operation)
	}

	if validationErr.Field != "email" {
		t.Errorf("Expected field 'email', got '%s'", validationErr.Field)
	}

	if validationErr.Reason != "invalid format" {
		t.Errorf("Expected reason 'invalid format', got '%s'", validationErr.Reason)
	}

	if validationErr.Err != originalErr {
		t.Errorf("Expected original error to be preserved")
	}

	expectedMsg := "validation failed for User create: email (invalid format): original error"
	if validationErr.Error() != expectedMsg {
		t.Errorf("Expected message '%s', got '%s'", expectedMsg, validationErr.Error())
	}

	// Test Unwrap
	if validationErr.Unwrap() != originalErr {
		t.Errorf("Expected Unwrap to return original error")
	}

	// Test with nil original error
	err = NewValidationError("User", "create", "email", "invalid format", nil)
	if !errors.As(err, &validationErr) {
		t.Fatalf("Expected *ValidationError, got %T", err)
	}
	if validationErr.Unwrap() != nil {
		t.Errorf("Expected Unwrap to return nil when no original error")
	}

	// Test error message without original error
	expectedMsgNoErr := "validation failed for User create: email (invalid format)"
	if validationErr.Error() != expectedMsgNoErr {
		t.Errorf("Expected message '%s' for nil error, got '%s'", expectedMsgNoErr, validationErr.Error())
	}
}

func TestNewDatabaseError(t *testing.T) {
	originalErr := errors.New("connection failed")

	err := NewDatabaseError("User", "query", originalErr)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	var dbErr *DatabaseError
	if !errors.As(err, &dbErr) {
		t.Fatalf("Expected *DatabaseError, got %T", err)
	}

	if dbErr.Entity != "User" {
		t.Errorf("Expected entity 'User', got '%s'", dbErr.Entity)
	}

	if dbErr.Operation != "query" {
		t.Errorf("Expected operation 'query', got '%s'", dbErr.Operation)
	}

	if dbErr.Err != originalErr {
		t.Errorf("Expected original error to be preserved")
	}

	expectedMsg := "failed to query User: connection failed"
	if dbErr.Error() != expectedMsg {
		t.Errorf("Expected message '%s', got '%s'", expectedMsg, dbErr.Error())
	}

	// Test Unwrap
	if dbErr.Unwrap() != originalErr {
		t.Errorf("Expected Unwrap to return original error")
	}
}

func TestErrorTypeDetection(t *testing.T) {
	// Test that we can distinguish between error types using errors.As
	notFoundErr := NewNotFoundError("User", "123")
	validationErr := NewValidationError("User", "create", "email", "invalid", nil)
	dbErr := NewDatabaseError("User", "query", errors.New("failed"))

	// Test NotFoundError detection
	var nfErr *NotFoundError
	if !errors.As(notFoundErr, &nfErr) {
		t.Error("Expected errors.As to detect NotFoundError")
	}
	if errors.As(validationErr, &nfErr) {
		t.Error("Expected errors.As to NOT detect NotFoundError in ValidationError")
	}

	// Test ValidationError detection
	var valErr *ValidationError
	if !errors.As(validationErr, &valErr) {
		t.Error("Expected errors.As to detect ValidationError")
	}
	if errors.As(notFoundErr, &valErr) {
		t.Error("Expected errors.As to NOT detect ValidationError in NotFoundError")
	}

	// Test DatabaseError detection
	var dErr *DatabaseError
	if !errors.As(dbErr, &dErr) {
		t.Error("Expected errors.As to detect DatabaseError")
	}
	if errors.As(notFoundErr, &dErr) {
		t.Error("Expected errors.As to NOT detect DatabaseError in NotFoundError")
	}
}
