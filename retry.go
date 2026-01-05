package pgxkit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type retryConfig struct {
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	multiplier float64
}

func defaultRetryConfig() *retryConfig {
	return &retryConfig{
		maxRetries: 3,
		baseDelay:  100 * time.Millisecond,
		maxDelay:   1 * time.Second,
		multiplier: 2.0,
	}
}

// RetryOption configures retry behavior for operations.
type RetryOption func(*retryConfig)

// WithMaxRetries sets the maximum number of retry attempts.
func WithMaxRetries(n int) RetryOption {
	return func(c *retryConfig) {
		if n >= 0 {
			c.maxRetries = n
		}
	}
}

// WithBaseDelay sets the initial delay between retries.
func WithBaseDelay(d time.Duration) RetryOption {
	return func(c *retryConfig) {
		if d > 0 {
			c.baseDelay = d
		}
	}
}

// WithMaxDelay sets the maximum delay between retries.
func WithMaxDelay(d time.Duration) RetryOption {
	return func(c *retryConfig) {
		if d > 0 {
			c.maxDelay = d
		}
	}
}

// WithBackoffMultiplier sets the multiplier for exponential backoff.
func WithBackoffMultiplier(m float64) RetryOption {
	return func(c *retryConfig) {
		if m > 0 {
			c.multiplier = m
		}
	}
}

// WithTimeout executes a function with a timeout.
// This is a generic utility function that can be used with any operation.
//
// Example:
//
//	result, err := pgxkit.WithTimeout(ctx, 5*time.Second, func(ctx context.Context) (*User, error) {
//	    return getUserFromDatabase(ctx)
//	})
func WithTimeout[T any](ctx context.Context, timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return fn(ctx)
}

// WithTimeoutAndRetry executes a function with timeout and retry logic.
// This combines timeout handling with intelligent retry logic for transient failures.
//
// Example:
//
//	result, err := pgxkit.WithTimeoutAndRetry(ctx, 5*time.Second, func(ctx context.Context) (*User, error) {
//	    return getUserFromDatabase(ctx)
//	}, pgxkit.WithMaxRetries(5), pgxkit.WithBaseDelay(200*time.Millisecond))
func WithTimeoutAndRetry[T any](ctx context.Context, timeout time.Duration, fn func(context.Context) (T, error), opts ...RetryOption) (T, error) {
	var result T
	err := RetryOperation(ctx, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		var err error
		result, err = fn(ctx)
		return err
	}, opts...)

	return result, err
}

// RetryOperation executes an operation with configurable retry logic.
// It uses exponential backoff to avoid thundering herd problems.
//
// Example:
//
//	err := pgxkit.RetryOperation(ctx, func(ctx context.Context) error {
//	    return doSomething(ctx)
//	}, pgxkit.WithMaxRetries(5), pgxkit.WithMaxDelay(5*time.Second))
func RetryOperation(ctx context.Context, operation func(context.Context) error, opts ...RetryOption) error {
	cfg := defaultRetryConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	var lastErr error
	delay := cfg.baseDelay

	for attempt := 0; attempt <= cfg.maxRetries; attempt++ {
		if attempt > 0 {
			if delay > cfg.maxDelay {
				delay = cfg.maxDelay
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}

			delay = time.Duration(float64(delay) * cfg.multiplier)
		}

		err := operation(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		if !IsRetryableError(err) {
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return fmt.Errorf("operation failed after %d attempts, last error: %w", cfg.maxRetries+1, lastErr)
}

// IsRetryableError determines if an error is worth retrying
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
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

	if errors.Is(err, pgx.ErrNoRows) {
		return false
	}

	if errors.Is(err, pgx.ErrTxClosed) || errors.Is(err, pgx.ErrTxCommitRollback) {
		return false
	}

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
