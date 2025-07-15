package dbutil

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Multiplier float64
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   1 * time.Second,
		Multiplier: 2.0,
	}
}

// RetryableConnection wraps a Connection with retry logic
type RetryableConnection[T Querier] struct {
	*Connection[T]
	retryConfig *RetryConfig
}

// WithRetry returns a new connection with retry logic enabled
func (c *Connection[T]) WithRetry(config *RetryConfig) *RetryableConnection[T] {
	if config == nil {
		config = DefaultRetryConfig()
	}
	return &RetryableConnection[T]{
		Connection:  c,
		retryConfig: config,
	}
}

// WithRetryableTransaction executes the given function within a database transaction with retry logic
func (rc *RetryableConnection[T]) WithRetryableTransaction(ctx context.Context, fn TransactionFunc[T]) error {
	return rc.retryOperation(ctx, func(ctx context.Context) error {
		return rc.Connection.WithTransaction(ctx, fn)
	})
}

// WithTimeout executes a function with a timeout and optional retry logic
func WithTimeout[T any](ctx context.Context, timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return fn(ctx)
}

// WithTimeoutAndRetry executes a function with timeout and retry logic
func WithTimeoutAndRetry[T any](ctx context.Context, timeout time.Duration, retryConfig *RetryConfig, fn func(context.Context) (T, error)) (T, error) {
	if retryConfig == nil {
		retryConfig = DefaultRetryConfig()
	}

	var result T
	err := retryOperation(ctx, retryConfig, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		var err error
		result, err = fn(ctx)
		return err
	})

	return result, err
}

// retryOperation retries an operation based on the retry configuration
func (rc *RetryableConnection[T]) retryOperation(ctx context.Context, operation func(context.Context) error) error {
	return retryOperation(ctx, rc.retryConfig, operation)
}

// retryOperation is the generic retry function
func retryOperation(ctx context.Context, config *RetryConfig, operation func(context.Context) error) error {
	var lastErr error
	delay := config.BaseDelay

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Apply exponential backoff
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				// Continue with retry
			}

			delay = time.Duration(float64(delay) * config.Multiplier)
		}

		err := operation(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			return err
		}

		// Don't retry on context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return fmt.Errorf("operation failed after %d attempts, last error: %w", config.MaxRetries+1, lastErr)
}

// isRetryableError determines if an error is worth retrying
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Context errors are not retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check for PostgreSQL connection errors
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// These are PostgreSQL error codes that might be retryable
		switch pgErr.Code {
		case "08000", // connection_exception
			"08003", // connection_does_not_exist
			"08006", // connection_failure
			"57P01", // admin_shutdown
			"57P02", // crash_shutdown
			"57P03": // cannot_connect_now
			return true
		case "40001", // serialization_failure
			"40P01": // deadlock_detected
			return true
		}
		return false
	}

	// Check for connection errors from pgx
	if errors.Is(err, pgx.ErrNoRows) {
		return false // Not retryable
	}

	// Check for network/connection errors
	if errors.Is(err, pgx.ErrTxClosed) || errors.Is(err, pgx.ErrTxCommitRollback) {
		return false // Not retryable
	}

	// Generic connection errors might be retryable
	errStr := err.Error()
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "connection timeout") ||
		strings.Contains(errStr, "network is unreachable") ||
		strings.Contains(errStr, "no route to host") {
		return true
	}

	return false
}
