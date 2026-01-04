package pgxkit

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestConnectionHooks(t *testing.T) {
	hooks := newConnectionHooks()
	if hooks == nil {
		t.Fatal("Expected newConnectionHooks to return non-nil")
	}

	err := hooks.executeOnConnect(nil)
	if err != nil {
		t.Errorf("Expected no error from empty OnConnect hooks, got %v", err)
	}

	hooks.executeOnDisconnect(nil)

	err = hooks.executeOnAcquire(context.Background(), nil)
	if err != nil {
		t.Errorf("Expected no error from empty OnAcquire hooks, got %v", err)
	}

	hooks.executeOnRelease(nil)
}

func TestAddOnConnectHooks(t *testing.T) {
	hooks := newConnectionHooks()
	callCount := 0

	hooks.addOnConnect(func(conn *pgx.Conn) error {
		callCount++
		return nil
	})

	hooks.addOnConnect(func(conn *pgx.Conn) error {
		callCount += 10
		return nil
	})

	err := hooks.executeOnConnect(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if callCount != 11 {
		t.Errorf("Expected callCount to be 11, got %d", callCount)
	}
}

func TestOnConnectHookError(t *testing.T) {
	hooks := newConnectionHooks()
	expectedErr := errors.New("connection failed")

	hooks.addOnConnect(func(conn *pgx.Conn) error {
		return expectedErr
	})

	hooks.addOnConnect(func(conn *pgx.Conn) error {
		t.Error("Second hook should not be called when first hook errors")
		return nil
	})

	err := hooks.executeOnConnect(nil)
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestAddOnDisconnectHooks(t *testing.T) {
	hooks := newConnectionHooks()
	callCount := 0

	hooks.addOnDisconnect(func(conn *pgx.Conn) {
		callCount++
	})

	hooks.addOnDisconnect(func(conn *pgx.Conn) {
		callCount += 10
	})

	hooks.executeOnDisconnect(nil)

	if callCount != 11 {
		t.Errorf("Expected callCount to be 11, got %d", callCount)
	}
}

func TestAddOnAcquireHooks(t *testing.T) {
	hooks := newConnectionHooks()
	callCount := 0

	hooks.addOnAcquire(func(ctx context.Context, conn *pgx.Conn) error {
		callCount++
		return nil
	})

	hooks.addOnAcquire(func(ctx context.Context, conn *pgx.Conn) error {
		callCount += 10
		return nil
	})

	err := hooks.executeOnAcquire(context.Background(), nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if callCount != 11 {
		t.Errorf("Expected callCount to be 11, got %d", callCount)
	}
}

func TestOnAcquireHookError(t *testing.T) {
	hooks := newConnectionHooks()
	expectedErr := errors.New("acquire failed")

	hooks.addOnAcquire(func(ctx context.Context, conn *pgx.Conn) error {
		return expectedErr
	})

	err := hooks.executeOnAcquire(context.Background(), nil)
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestAddOnReleaseHooks(t *testing.T) {
	hooks := newConnectionHooks()
	callCount := 0

	hooks.addOnRelease(func(conn *pgx.Conn) {
		callCount++
	})

	hooks.addOnRelease(func(conn *pgx.Conn) {
		callCount += 10
	})

	hooks.executeOnRelease(nil)

	if callCount != 11 {
		t.Errorf("Expected callCount to be 11, got %d", callCount)
	}
}

func TestCombineHooks(t *testing.T) {
	hooks1 := newConnectionHooks()
	callOrder := []string{}

	hooks1.addOnConnect(func(conn *pgx.Conn) error {
		callOrder = append(callOrder, "hooks1-connect")
		return nil
	})

	hooks1.addOnDisconnect(func(conn *pgx.Conn) {
		callOrder = append(callOrder, "hooks1-disconnect")
	})

	hooks2 := newConnectionHooks()
	hooks2.addOnConnect(func(conn *pgx.Conn) error {
		callOrder = append(callOrder, "hooks2-connect")
		return nil
	})

	hooks2.addOnDisconnect(func(conn *pgx.Conn) {
		callOrder = append(callOrder, "hooks2-disconnect")
	})

	combined := combineHooks(hooks1, hooks2)

	err := combined.executeOnConnect(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	combined.executeOnDisconnect(nil)

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
	combined := combineHooks()
	if combined == nil {
		t.Error("Expected combineHooks() to return non-nil even with no arguments")
	}

	err := combined.executeOnConnect(nil)
	if err != nil {
		t.Errorf("Expected no error from empty combined hooks, got %v", err)
	}
}

func TestValidationHook(t *testing.T) {
	hooks := validationHook()
	if hooks == nil {
		t.Fatal("Expected validationHook to return non-nil")
	}
}

func TestSetupHook(t *testing.T) {
	hooks := setupHook("SET timezone = 'UTC'")
	if hooks == nil {
		t.Fatal("Expected setupHook to return non-nil")
	}

	hooks = setupHook("")
	if hooks == nil {
		t.Fatal("Expected setupHook to return non-nil even with empty SQL")
	}
}
