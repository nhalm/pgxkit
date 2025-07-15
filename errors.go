package dbutil

import "fmt"

// Database error types - these are generic errors that can be used by any repository.
// These errors provide consistent error handling across database operations and can be
// used with errors.As() for type-safe error handling.

// NotFoundError represents when a requested entity is not found in the database.
// This error should be used instead of returning pgx.ErrNoRows directly.
type NotFoundError struct {
	Entity     string
	Identifier interface{}
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found: %v", e.Entity, e.Identifier)
}

// ValidationError represents validation failures that occur before database operations.
// Use this for input validation, constraint violations, or business rule failures.
type ValidationError struct {
	Entity    string
	Operation string
	Field     string
	Reason    string
	Err       error
}

func (e *ValidationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("validation failed for %s %s: %s (%s): %v", e.Entity, e.Operation, e.Field, e.Reason, e.Err)
	}
	return fmt.Sprintf("validation failed for %s %s: %s (%s)", e.Entity, e.Operation, e.Field, e.Reason)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}

// DatabaseError represents database operation failures such as connection errors,
// constraint violations, or other database-specific errors.
type DatabaseError struct {
	Entity    string
	Operation string // "create", "update", "delete", "query"
	Err       error
}

func (e *DatabaseError) Error() string {
	return fmt.Sprintf("failed to %s %s: %v", e.Operation, e.Entity, e.Err)
}

func (e *DatabaseError) Unwrap() error {
	return e.Err
}

// Error constructor functions for common cases.
// These functions provide a consistent way to create structured database errors.

// NewNotFoundError creates a new NotFoundError with the given entity and identifier.
func NewNotFoundError(entity string, identifier interface{}) *NotFoundError {
	return &NotFoundError{
		Entity:     entity,
		Identifier: identifier,
	}
}

// NewValidationError creates a new ValidationError with the given parameters.
func NewValidationError(entity, operation, field, reason string, err error) *ValidationError {
	return &ValidationError{
		Entity:    entity,
		Operation: operation,
		Field:     field,
		Reason:    reason,
		Err:       err,
	}
}

// NewDatabaseError creates a new DatabaseError with the given entity, operation, and underlying error.
func NewDatabaseError(entity, operation string, err error) *DatabaseError {
	return &DatabaseError{
		Entity:    entity,
		Operation: operation,
		Err:       err,
	}
}
