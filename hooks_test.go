package dbutil

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestConnectionHooks(t *testing.T) {
	hooks := NewConnectionHooks()
	if hooks == nil {
		t.Fatal("Expected NewConnectionHooks to return non-nil")
	}

	// Test that hooks start empty
	err := hooks.ExecuteOnConnect(nil)
	if err != nil {
		t.Errorf("Expected no error from empty OnConnect hooks, got %v", err)
	}

	hooks.ExecuteOnDisconnect(nil)
	// No assertion needed, just ensure it doesn't panic

	err = hooks.ExecuteOnAcquire(context.Background(), nil)
	if err != nil {
		t.Errorf("Expected no error from empty OnAcquire hooks, got %v", err)
	}

	hooks.ExecuteOnRelease(nil)
	// No assertion needed, just ensure it doesn't panic
}

func TestAddOnConnectHooks(t *testing.T) {
	hooks := NewConnectionHooks()
	callCount := 0

	// Add first hook
	hooks.AddOnConnect(func(conn *pgx.Conn) error {
		callCount++
		return nil
	})

	// Add second hook
	hooks.AddOnConnect(func(conn *pgx.Conn) error {
		callCount += 10
		return nil
	})

	// Execute hooks
	err := hooks.ExecuteOnConnect(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if callCount != 11 {
		t.Errorf("Expected callCount to be 11, got %d", callCount)
	}
}

func TestOnConnectHookError(t *testing.T) {
	hooks := NewConnectionHooks()
	expectedErr := errors.New("connection failed")

	// Add hook that returns error
	hooks.AddOnConnect(func(conn *pgx.Conn) error {
		return expectedErr
	})

	// Add hook that should not be called due to error
	hooks.AddOnConnect(func(conn *pgx.Conn) error {
		t.Error("Second hook should not be called when first hook errors")
		return nil
	})

	err := hooks.ExecuteOnConnect(nil)
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestAddOnDisconnectHooks(t *testing.T) {
	hooks := NewConnectionHooks()
	callCount := 0

	// Add hooks
	hooks.AddOnDisconnect(func(conn *pgx.Conn) {
		callCount++
	})

	hooks.AddOnDisconnect(func(conn *pgx.Conn) {
		callCount += 10
	})

	// Execute hooks
	hooks.ExecuteOnDisconnect(nil)

	if callCount != 11 {
		t.Errorf("Expected callCount to be 11, got %d", callCount)
	}
}

func TestAddOnAcquireHooks(t *testing.T) {
	hooks := NewConnectionHooks()
	callCount := 0

	// Add hooks
	hooks.AddOnAcquire(func(ctx context.Context, conn *pgx.Conn) error {
		callCount++
		return nil
	})

	hooks.AddOnAcquire(func(ctx context.Context, conn *pgx.Conn) error {
		callCount += 10
		return nil
	})

	// Execute hooks
	err := hooks.ExecuteOnAcquire(context.Background(), nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if callCount != 11 {
		t.Errorf("Expected callCount to be 11, got %d", callCount)
	}
}

func TestOnAcquireHookError(t *testing.T) {
	hooks := NewConnectionHooks()
	expectedErr := errors.New("acquire failed")

	// Add hook that returns error
	hooks.AddOnAcquire(func(ctx context.Context, conn *pgx.Conn) error {
		return expectedErr
	})

	err := hooks.ExecuteOnAcquire(context.Background(), nil)
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestAddOnReleaseHooks(t *testing.T) {
	hooks := NewConnectionHooks()
	callCount := 0

	// Add hooks
	hooks.AddOnRelease(func(conn *pgx.Conn) {
		callCount++
	})

	hooks.AddOnRelease(func(conn *pgx.Conn) {
		callCount += 10
	})

	// Execute hooks
	hooks.ExecuteOnRelease(nil)

	if callCount != 11 {
		t.Errorf("Expected callCount to be 11, got %d", callCount)
	}
}

func TestCombineHooks(t *testing.T) {
	// Create first hook set
	hooks1 := NewConnectionHooks()
	callOrder := []string{}

	hooks1.AddOnConnect(func(conn *pgx.Conn) error {
		callOrder = append(callOrder, "hooks1-connect")
		return nil
	})

	hooks1.AddOnDisconnect(func(conn *pgx.Conn) {
		callOrder = append(callOrder, "hooks1-disconnect")
	})

	// Create second hook set
	hooks2 := NewConnectionHooks()
	hooks2.AddOnConnect(func(conn *pgx.Conn) error {
		callOrder = append(callOrder, "hooks2-connect")
		return nil
	})

	hooks2.AddOnDisconnect(func(conn *pgx.Conn) {
		callOrder = append(callOrder, "hooks2-disconnect")
	})

	// Combine hooks
	combined := CombineHooks(hooks1, hooks2)

	// Test that all hooks are executed
	err := combined.ExecuteOnConnect(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	combined.ExecuteOnDisconnect(nil)

	expectedOrder := []string{
		"hooks1-connect",
		"hooks2-connect",
		"hooks1-disconnect",
		"hooks2-disconnect",
	}

	if len(callOrder) != len(expectedOrder) {
		t.Errorf("Expected %d calls, got %d", len(expectedOrder), len(callOrder))
	}

	for i, expected := range expectedOrder {
		if i >= len(callOrder) || callOrder[i] != expected {
			t.Errorf("Expected call %d to be '%s', got '%s'", i, expected, callOrder[i])
		}
	}
}

func TestCombineHooksEmpty(t *testing.T) {
	// Test combining with no hooks
	combined := CombineHooks()
	if combined == nil {
		t.Error("Expected CombineHooks() to return non-nil even with no arguments")
	}

	// Should not panic
	err := combined.ExecuteOnConnect(nil)
	if err != nil {
		t.Errorf("Expected no error from empty combined hooks, got %v", err)
	}
}

func TestLoggingHook(t *testing.T) {
	// Mock logger that records log calls
	logCalls := []string{}
	mockLogger := &mockLogger{
		logFunc: func(ctx context.Context, level LogLevel, msg string, data map[string]interface{}) {
			logCalls = append(logCalls, msg)
		},
	}

	hooks := LoggingHook(mockLogger)
	if hooks == nil {
		t.Fatal("Expected LoggingHook to return non-nil")
	}

	// Test that hooks exist (we can't easily test execution without pgx.Conn)
	// But we can verify the hook was created successfully
	if len(logCalls) != 0 {
		t.Errorf("Expected no log calls yet, got %d", len(logCalls))
	}
}

func TestValidationHook(t *testing.T) {
	hooks := ValidationHook()
	if hooks == nil {
		t.Fatal("Expected ValidationHook to return non-nil")
	}

	// Test that we can create validation hooks without error
	// (Full testing would require a real database connection)
}

func TestSetupHook(t *testing.T) {
	hooks := SetupHook("SET timezone = 'UTC'")
	if hooks == nil {
		t.Fatal("Expected SetupHook to return non-nil")
	}

	// Test with empty SQL
	hooks = SetupHook("")
	if hooks == nil {
		t.Fatal("Expected SetupHook to return non-nil even with empty SQL")
	}
}

// Mock logger for testing
type mockLogger struct {
	logFunc func(ctx context.Context, level LogLevel, msg string, data map[string]interface{})
}

func (m *mockLogger) Log(ctx context.Context, level LogLevel, msg string, data map[string]interface{}) {
	if m.logFunc != nil {
		m.logFunc(ctx, level, msg, data)
	}
}
