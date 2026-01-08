package pgxkit

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestWithMaxRetries(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{"positive value", 5, 5},
		{"zero value", 0, 0},
		{"negative value ignored", -1, 3},
		{"large value", 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultRetryConfig()
			opt := WithMaxRetries(tt.input)
			opt(cfg)

			if cfg.maxRetries != tt.expected {
				t.Errorf("expected maxRetries=%d, got %d", tt.expected, cfg.maxRetries)
			}
		})
	}
}

func TestWithBaseDelay(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected time.Duration
	}{
		{"positive value", 500 * time.Millisecond, 500 * time.Millisecond},
		{"zero value ignored", 0, 100 * time.Millisecond},
		{"negative value ignored", -1 * time.Second, 100 * time.Millisecond},
		{"small value", 1 * time.Millisecond, 1 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultRetryConfig()
			opt := WithBaseDelay(tt.input)
			opt(cfg)

			if cfg.baseDelay != tt.expected {
				t.Errorf("expected baseDelay=%v, got %v", tt.expected, cfg.baseDelay)
			}
		})
	}
}

func TestWithMaxDelay(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected time.Duration
	}{
		{"positive value", 5 * time.Second, 5 * time.Second},
		{"zero value ignored", 0, 1 * time.Second},
		{"negative value ignored", -1 * time.Second, 1 * time.Second},
		{"large value", 1 * time.Minute, 1 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultRetryConfig()
			opt := WithMaxDelay(tt.input)
			opt(cfg)

			if cfg.maxDelay != tt.expected {
				t.Errorf("expected maxDelay=%v, got %v", tt.expected, cfg.maxDelay)
			}
		})
	}
}

func TestWithBackoffMultiplier(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"positive value", 3.0, 3.0},
		{"zero value ignored", 0, 2.0},
		{"negative value ignored", -1.5, 2.0},
		{"fractional value", 1.5, 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultRetryConfig()
			opt := WithBackoffMultiplier(tt.input)
			opt(cfg)

			if cfg.multiplier != tt.expected {
				t.Errorf("expected multiplier=%v, got %v", tt.expected, cfg.multiplier)
			}
		})
	}
}

func TestRetryOptionComposition(t *testing.T) {
	cfg := defaultRetryConfig()

	opts := []RetryOption{
		WithMaxRetries(10),
		WithBaseDelay(200 * time.Millisecond),
		WithMaxDelay(10 * time.Second),
		WithBackoffMultiplier(3.0),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.maxRetries != 10 {
		t.Errorf("expected maxRetries=10, got %d", cfg.maxRetries)
	}
	if cfg.baseDelay != 200*time.Millisecond {
		t.Errorf("expected baseDelay=200ms, got %v", cfg.baseDelay)
	}
	if cfg.maxDelay != 10*time.Second {
		t.Errorf("expected maxDelay=10s, got %v", cfg.maxDelay)
	}
	if cfg.multiplier != 3.0 {
		t.Errorf("expected multiplier=3.0, got %v", cfg.multiplier)
	}
}

func TestRetry_Success(t *testing.T) {
	result, err := Retry(context.Background(), func(ctx context.Context) (string, error) {
		return "success", nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got '%s'", result)
	}
}

func TestRetryOperation_SuccessNoRetry(t *testing.T) {
	var callCount int32
	err := RetryOperation(context.Background(), func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestRetryOperation_FailsThenSucceeds(t *testing.T) {
	var callCount int32
	err := RetryOperation(context.Background(), func(ctx context.Context) error {
		count := atomic.AddInt32(&callCount, 1)
		if count < 3 {
			return &net.OpError{Op: "dial", Err: errors.New("connection refused")}
		}
		return nil
	}, WithMaxRetries(5), WithBaseDelay(1*time.Millisecond))

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestRetryOperation_FailsAllAttempts(t *testing.T) {
	var callCount int32
	maxRetries := 3
	err := RetryOperation(context.Background(), func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	}, WithMaxRetries(maxRetries), WithBaseDelay(1*time.Millisecond))

	if err == nil {
		t.Error("expected error, got nil")
	}

	expectedCalls := int32(maxRetries + 1)
	if atomic.LoadInt32(&callCount) != expectedCalls {
		t.Errorf("expected %d calls, got %d", expectedCalls, callCount)
	}

	if !errors.Is(err, errors.Unwrap(err)) {
		expectedErrSubstring := "operation failed after"
		if err.Error()[:len(expectedErrSubstring)] != expectedErrSubstring {
			t.Errorf("expected error message to start with '%s', got '%s'", expectedErrSubstring, err.Error())
		}
	}
}

func TestRetryOperation_MaxRetriesRespected(t *testing.T) {
	tests := []struct {
		name          string
		maxRetries    int
		expectedCalls int32
	}{
		{"zero retries", 0, 1},
		{"one retry", 1, 2},
		{"five retries", 5, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var callCount int32
			_ = RetryOperation(context.Background(), func(ctx context.Context) error {
				atomic.AddInt32(&callCount, 1)
				return &net.OpError{Op: "dial", Err: errors.New("connection refused")}
			}, WithMaxRetries(tt.maxRetries), WithBaseDelay(1*time.Millisecond))

			if atomic.LoadInt32(&callCount) != tt.expectedCalls {
				t.Errorf("expected %d calls, got %d", tt.expectedCalls, callCount)
			}
		})
	}
}

func TestRetryOperation_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var callCount int32
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := RetryOperation(ctx, func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	}, WithMaxRetries(100), WithBaseDelay(5*time.Millisecond))

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}

	calls := atomic.LoadInt32(&callCount)
	if calls >= 100 {
		t.Errorf("expected fewer than 100 calls due to cancellation, got %d", calls)
	}
}

func TestRetryOperation_NonRetryableErrorStopsImmediately(t *testing.T) {
	var callCount int32
	err := RetryOperation(context.Background(), func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return pgx.ErrNoRows
	}, WithMaxRetries(5), WithBaseDelay(1*time.Millisecond))

	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("expected pgx.ErrNoRows, got %v", err)
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 call for non-retryable error, got %d", callCount)
	}
}

func TestRetryOperation_ExponentialBackoff(t *testing.T) {
	var timestamps []time.Time
	baseDelay := 10 * time.Millisecond

	_ = RetryOperation(context.Background(), func(ctx context.Context) error {
		timestamps = append(timestamps, time.Now())
		if len(timestamps) < 4 {
			return &net.OpError{Op: "dial", Err: errors.New("connection refused")}
		}
		return nil
	}, WithMaxRetries(5), WithBaseDelay(baseDelay), WithBackoffMultiplier(2.0))

	if len(timestamps) < 4 {
		t.Fatalf("expected at least 4 timestamps, got %d", len(timestamps))
	}

	delay1 := timestamps[1].Sub(timestamps[0])
	delay2 := timestamps[2].Sub(timestamps[1])
	delay3 := timestamps[3].Sub(timestamps[2])

	if delay2 < delay1 {
		t.Errorf("expected delay2 (%v) >= delay1 (%v)", delay2, delay1)
	}
	if delay3 < delay2 {
		t.Errorf("expected delay3 (%v) >= delay2 (%v)", delay3, delay2)
	}
}

func TestRetryOperation_MaxDelayRespected(t *testing.T) {
	var timestamps []time.Time
	baseDelay := 5 * time.Millisecond
	maxDelay := 10 * time.Millisecond

	_ = RetryOperation(context.Background(), func(ctx context.Context) error {
		timestamps = append(timestamps, time.Now())
		if len(timestamps) < 6 {
			return &net.OpError{Op: "dial", Err: errors.New("connection refused")}
		}
		return nil
	}, WithMaxRetries(10), WithBaseDelay(baseDelay), WithMaxDelay(maxDelay), WithBackoffMultiplier(2.0))

	if len(timestamps) < 5 {
		t.Fatalf("expected at least 5 timestamps, got %d", len(timestamps))
	}

	for i := 2; i < len(timestamps); i++ {
		delay := timestamps[i].Sub(timestamps[i-1])
		tolerance := 5 * time.Millisecond
		if delay > maxDelay+tolerance {
			t.Errorf("delay %d (%v) exceeded maxDelay (%v) + tolerance", i, delay, maxDelay)
		}
	}
}

func TestIsRetryableError_NilError(t *testing.T) {
	if IsRetryableError(nil) {
		t.Error("expected nil error to return false")
	}
}

func TestIsRetryableError_ContextErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"context.Canceled", context.Canceled, false},
		{"context.DeadlineExceeded", context.DeadlineExceeded, false},
		{"wrapped context.Canceled", errors.New("wrapped: " + context.Canceled.Error()), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsRetryableError_PostgreSQLRetryableCodes(t *testing.T) {
	retryableCodes := []struct {
		code string
		desc string
	}{
		{"08000", "connection_exception"},
		{"08003", "connection_does_not_exist"},
		{"08006", "connection_failure"},
		{"57P01", "admin_shutdown"},
		{"57P02", "crash_shutdown"},
		{"57P03", "cannot_connect_now"},
		{"40001", "serialization_failure"},
		{"40P01", "deadlock_detected"},
	}

	for _, tc := range retryableCodes {
		t.Run(tc.desc, func(t *testing.T) {
			err := &pgconn.PgError{Code: tc.code}
			if !IsRetryableError(err) {
				t.Errorf("expected code %s (%s) to be retryable", tc.code, tc.desc)
			}
		})
	}
}

func TestIsRetryableError_PostgreSQLNonRetryableCodes(t *testing.T) {
	nonRetryableCodes := []struct {
		code string
		desc string
	}{
		{"23505", "unique_violation"},
		{"23503", "foreign_key_violation"},
		{"42P01", "undefined_table"},
		{"42703", "undefined_column"},
		{"22P02", "invalid_text_representation"},
		{"42601", "syntax_error"},
		{"23502", "not_null_violation"},
	}

	for _, tc := range nonRetryableCodes {
		t.Run(tc.desc, func(t *testing.T) {
			err := &pgconn.PgError{Code: tc.code}
			if IsRetryableError(err) {
				t.Errorf("expected code %s (%s) to NOT be retryable", tc.code, tc.desc)
			}
		})
	}
}

func TestIsRetryableError_PgxSentinelErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"pgx.ErrNoRows", pgx.ErrNoRows, false},
		{"pgx.ErrTxClosed", pgx.ErrTxClosed, false},
		{"pgx.ErrTxCommitRollback", pgx.ErrTxCommitRollback, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

type testNetError struct {
	timeout   bool
	temporary bool
}

func (e *testNetError) Error() string   { return "test network error" }
func (e *testNetError) Timeout() bool   { return e.timeout }
func (e *testNetError) Temporary() bool { return e.temporary }

func TestIsRetryableError_NetworkErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			"net.Error interface",
			&testNetError{timeout: true, temporary: true},
			true,
		},
		{
			"net.OpError dial",
			&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
			true,
		},
		{
			"net.OpError read",
			&net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset by peer")},
			true,
		},
		{
			"wrapped net.OpError",
			errors.Join(errors.New("query failed"), &net.OpError{Op: "dial", Err: errors.New("no route to host")}),
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsRetryableError_NonRetryableErrors(t *testing.T) {
	nonRetryableErrors := []struct {
		name string
		err  error
	}{
		{"invalid query syntax", errors.New("invalid query syntax")},
		{"permission denied", errors.New("permission denied")},
		{"authentication failed", errors.New("authentication failed")},
		{"unknown database", errors.New("unknown database")},
		{"table does not exist", errors.New("table does not exist")},
	}

	for _, tc := range nonRetryableErrors {
		t.Run(tc.name, func(t *testing.T) {
			if IsRetryableError(tc.err) {
				t.Errorf("expected error '%s' to NOT be retryable", tc.err)
			}
		})
	}
}

func TestIsRetryableError_UnknownError(t *testing.T) {
	err := errors.New("some unknown error")
	if IsRetryableError(err) {
		t.Error("expected unknown error to return false")
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := defaultRetryConfig()

	if cfg.maxRetries != 3 {
		t.Errorf("expected default maxRetries=3, got %d", cfg.maxRetries)
	}
	if cfg.baseDelay != 100*time.Millisecond {
		t.Errorf("expected default baseDelay=100ms, got %v", cfg.baseDelay)
	}
	if cfg.maxDelay != 1*time.Second {
		t.Errorf("expected default maxDelay=1s, got %v", cfg.maxDelay)
	}
	if cfg.multiplier != 2.0 {
		t.Errorf("expected default multiplier=2.0, got %v", cfg.multiplier)
	}
}

func TestIsRetryableError_WrappedPgError(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "40001"}
	wrappedErr := errors.Join(errors.New("query failed"), pgErr)

	if !IsRetryableError(wrappedErr) {
		t.Error("expected wrapped PgError with retryable code to be retryable")
	}
}

func TestIsRetryableError_WrappedPgxErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"wrapped ErrNoRows", errors.Join(errors.New("query failed"), pgx.ErrNoRows), false},
		{"wrapped ErrTxClosed", errors.Join(errors.New("tx error"), pgx.ErrTxClosed), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestRetryOperation_DeadlockDetectedRetries(t *testing.T) {
	var callCount int32
	err := RetryOperation(context.Background(), func(ctx context.Context) error {
		count := atomic.AddInt32(&callCount, 1)
		if count < 3 {
			return &pgconn.PgError{Code: "40P01"}
		}
		return nil
	}, WithMaxRetries(5), WithBaseDelay(1*time.Millisecond))

	if err != nil {
		t.Errorf("expected no error after retries, got %v", err)
	}
	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("expected 3 calls (2 deadlocks then success), got %d", callCount)
	}
}

func TestRetryOperation_SerializationFailureRetries(t *testing.T) {
	var callCount int32
	err := RetryOperation(context.Background(), func(ctx context.Context) error {
		count := atomic.AddInt32(&callCount, 1)
		if count < 2 {
			return &pgconn.PgError{Code: "40001"}
		}
		return nil
	}, WithMaxRetries(5), WithBaseDelay(1*time.Millisecond))

	if err != nil {
		t.Errorf("expected no error after retry, got %v", err)
	}
	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestRetryOperation_UniqueViolationNoRetry(t *testing.T) {
	var callCount int32
	err := RetryOperation(context.Background(), func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return &pgconn.PgError{Code: "23505"}
	}, WithMaxRetries(5), WithBaseDelay(1*time.Millisecond))

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Errorf("expected PgError with code 23505, got %v", err)
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 call for non-retryable PgError, got %d", callCount)
	}
}

func TestRetryOperation_ContextDeadlineExceededNoRetry(t *testing.T) {
	var callCount int32
	err := RetryOperation(context.Background(), func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return context.DeadlineExceeded
	}, WithMaxRetries(5), WithBaseDelay(1*time.Millisecond))

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 call for context.DeadlineExceeded, got %d", callCount)
	}
}
