package pgxkit

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HookType represents the type of hook for operation-level hooks.
// These hooks are executed during database operations and provide extensibility
// for logging, tracing, metrics, circuit breakers, and other cross-cutting concerns.
type HookType int

const (
	// BeforeOperation is called before any query/exec operation.
	// The operationErr parameter will always be nil.
	BeforeOperation HookType = iota

	// AfterOperation is called after any query/exec operation.
	// The operationErr parameter contains the result of the operation.
	AfterOperation

	// BeforeTransaction is called before starting a transaction.
	// The operationErr parameter will always be nil.
	BeforeTransaction

	// AfterTransaction is called after a transaction completes.
	// The operationErr parameter contains the result of the transaction.
	AfterTransaction

	// OnShutdown is called during graceful shutdown.
	// The sql and args parameters will be empty, operationErr will be nil.
	OnShutdown
)

// HookFunc is the universal hook function signature for operation-level hooks.
// All operation-level hooks use this signature for consistency and simplicity.
//
// Parameters:
//   - ctx: The context for the operation
//   - sql: The SQL statement being executed (empty for shutdown hooks)
//   - args: The arguments for the SQL statement (nil for shutdown hooks)
//   - operationErr: The error from the operation (nil for before hooks)
//
// The hook should return an error if it wants to abort the operation.
// For after hooks, returning an error will not affect the original operation result.
type HookFunc func(ctx context.Context, sql string, args []interface{}, operationErr error) error

// hooks manages both operation-level and connection-level hooks
type hooks struct {
	mu sync.RWMutex

	// Operation-level hooks
	beforeOperation   []HookFunc
	afterOperation    []HookFunc
	beforeTransaction []HookFunc
	afterTransaction  []HookFunc
	onShutdown        []HookFunc

	// Connection-level hooks (pgx native signatures)
	connectionHooks *ConnectionHooks
}

// newHooks creates a new hooks manager
func newHooks() *hooks {
	return &hooks{
		beforeOperation:   make([]HookFunc, 0),
		afterOperation:    make([]HookFunc, 0),
		beforeTransaction: make([]HookFunc, 0),
		afterTransaction:  make([]HookFunc, 0),
		onShutdown:        make([]HookFunc, 0),
		connectionHooks:   NewConnectionHooks(),
	}
}

// AddHook adds an operation-level hook
func (h *hooks) addHook(hookType HookType, hookFunc HookFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch hookType {
	case BeforeOperation:
		h.beforeOperation = append(h.beforeOperation, hookFunc)
	case AfterOperation:
		h.afterOperation = append(h.afterOperation, hookFunc)
	case BeforeTransaction:
		h.beforeTransaction = append(h.beforeTransaction, hookFunc)
	case AfterTransaction:
		h.afterTransaction = append(h.afterTransaction, hookFunc)
	case OnShutdown:
		h.onShutdown = append(h.onShutdown, hookFunc)
	}
}

// AddConnectionHook adds a connection-level hook
func (h *hooks) addConnectionHook(hookType string, hookFunc interface{}) error {
	switch hookType {
	case "OnConnect":
		if fn, ok := hookFunc.(func(*pgx.Conn) error); ok {
			h.connectionHooks.AddOnConnect(fn)
			return nil
		}
		return fmt.Errorf("OnConnect hook must be of type func(*pgx.Conn) error")
	case "OnDisconnect":
		if fn, ok := hookFunc.(func(*pgx.Conn)); ok {
			h.connectionHooks.AddOnDisconnect(fn)
			return nil
		}
		return fmt.Errorf("OnDisconnect hook must be of type func(*pgx.Conn)")
	case "OnAcquire":
		if fn, ok := hookFunc.(func(context.Context, *pgx.Conn) error); ok {
			h.connectionHooks.AddOnAcquire(fn)
			return nil
		}
		return fmt.Errorf("OnAcquire hook must be of type func(context.Context, *pgx.Conn) error")
	case "OnRelease":
		if fn, ok := hookFunc.(func(*pgx.Conn)); ok {
			h.connectionHooks.AddOnRelease(fn)
			return nil
		}
		return fmt.Errorf("OnRelease hook must be of type func(*pgx.Conn)")
	default:
		return fmt.Errorf("unknown connection hook type: %s", hookType)
	}
}

// ExecuteBeforeOperation executes all BeforeOperation hooks
func (h *hooks) executeBeforeOperation(ctx context.Context, sql string, args []interface{}, operationErr error) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, hook := range h.beforeOperation {
		if err := hook(ctx, sql, args, operationErr); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteAfterOperation executes all AfterOperation hooks
func (h *hooks) executeAfterOperation(ctx context.Context, sql string, args []interface{}, operationErr error) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, hook := range h.afterOperation {
		if err := hook(ctx, sql, args, operationErr); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteBeforeTransaction executes all BeforeTransaction hooks
func (h *hooks) executeBeforeTransaction(ctx context.Context, sql string, args []interface{}, operationErr error) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, hook := range h.beforeTransaction {
		if err := hook(ctx, sql, args, operationErr); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteAfterTransaction executes all AfterTransaction hooks
func (h *hooks) executeAfterTransaction(ctx context.Context, sql string, args []interface{}, operationErr error) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, hook := range h.afterTransaction {
		if err := hook(ctx, sql, args, operationErr); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteOnShutdown executes all OnShutdown hooks
func (h *hooks) executeOnShutdown(ctx context.Context, sql string, args []interface{}, operationErr error) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, hook := range h.onShutdown {
		if err := hook(ctx, sql, args, operationErr); err != nil {
			return err
		}
	}
	return nil
}

// ConnectionHooks manages connection lifecycle hooks.
// These hooks are integrated with pgx's connection lifecycle and are useful
// for connection setup, validation, and cleanup. They use pgx's native function signatures.
type ConnectionHooks struct {
	mu           sync.RWMutex
	onConnect    []func(*pgx.Conn) error
	onDisconnect []func(*pgx.Conn)
	onAcquire    []func(context.Context, *pgx.Conn) error
	onRelease    []func(*pgx.Conn)
}

// NewConnectionHooks creates a new connection hooks manager.
// This is used internally by the DB type but can also be used directly
// for advanced connection pool configuration.
//
// Example:
//
//	hooks := pgxkit.NewConnectionHooks()
//	hooks.AddOnConnect(func(conn *pgx.Conn) error {
//	    log.Println("New connection established")
//	    return nil
//	})
func NewConnectionHooks() *ConnectionHooks {
	return &ConnectionHooks{
		onConnect:    make([]func(*pgx.Conn) error, 0),
		onDisconnect: make([]func(*pgx.Conn), 0),
		onAcquire:    make([]func(context.Context, *pgx.Conn) error, 0),
		onRelease:    make([]func(*pgx.Conn), 0),
	}
}

// AddOnConnect adds a callback that will be called when a new connection is established.
// This is useful for connection initialization, setting session variables, or validation.
// If the callback returns an error, the connection will be closed.
//
// Example:
//
//	hooks.AddOnConnect(func(conn *pgx.Conn) error {
//	    _, err := conn.Exec(context.Background(), "SET application_name = 'myapp'")
//	    return err
//	})
func (h *ConnectionHooks) AddOnConnect(fn func(*pgx.Conn) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onConnect = append(h.onConnect, fn)
}

// AddOnDisconnect adds a callback that will be called when a connection is closed
func (h *ConnectionHooks) AddOnDisconnect(fn func(*pgx.Conn)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onDisconnect = append(h.onDisconnect, fn)
}

// AddOnAcquire adds a callback that will be called when a connection is acquired from the pool
func (h *ConnectionHooks) AddOnAcquire(fn func(context.Context, *pgx.Conn) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onAcquire = append(h.onAcquire, fn)
}

// AddOnRelease adds a callback that will be called when a connection is released back to the pool
func (h *ConnectionHooks) AddOnRelease(fn func(*pgx.Conn)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onRelease = append(h.onRelease, fn)
}

// ExecuteOnConnect executes all OnConnect callbacks
func (h *ConnectionHooks) ExecuteOnConnect(conn *pgx.Conn) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, fn := range h.onConnect {
		if err := fn(conn); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteOnDisconnect executes all OnDisconnect callbacks
func (h *ConnectionHooks) ExecuteOnDisconnect(conn *pgx.Conn) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, fn := range h.onDisconnect {
		fn(conn)
	}
}

// ExecuteOnAcquire executes all OnAcquire callbacks
func (h *ConnectionHooks) ExecuteOnAcquire(ctx context.Context, conn *pgx.Conn) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, fn := range h.onAcquire {
		if err := fn(ctx, conn); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteOnRelease executes all OnRelease callbacks
func (h *ConnectionHooks) ExecuteOnRelease(conn *pgx.Conn) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, fn := range h.onRelease {
		fn(conn)
	}
}

// Common hook functions for typical use cases

// ValidationHook creates a hook that validates connections
func ValidationHook() *ConnectionHooks {
	hooks := NewConnectionHooks()

	hooks.AddOnConnect(func(conn *pgx.Conn) error {
		// Validate connection by running a simple query
		_, err := conn.Exec(context.Background(), "SELECT 1")
		return err
	})

	hooks.AddOnAcquire(func(ctx context.Context, conn *pgx.Conn) error {
		// Validate connection is still alive
		return conn.Ping(ctx)
	})

	return hooks
}

// SetupHook creates a hook that sets up connection-specific settings
func SetupHook(setupSQL string) *ConnectionHooks {
	hooks := NewConnectionHooks()

	hooks.AddOnConnect(func(conn *pgx.Conn) error {
		if setupSQL != "" {
			_, err := conn.Exec(context.Background(), setupSQL)
			return err
		}
		return nil
	})

	return hooks
}

// CombineHooks combines multiple hook managers into one
func CombineHooks(hooksList ...*ConnectionHooks) *ConnectionHooks {
	combined := NewConnectionHooks()

	for _, hooks := range hooksList {
		hooks.mu.RLock()

		for _, fn := range hooks.onConnect {
			combined.AddOnConnect(fn)
		}

		for _, fn := range hooks.onDisconnect {
			combined.AddOnDisconnect(fn)
		}

		for _, fn := range hooks.onAcquire {
			combined.AddOnAcquire(fn)
		}

		for _, fn := range hooks.onRelease {
			combined.AddOnRelease(fn)
		}

		hooks.mu.RUnlock()
	}

	return combined
}

// ConfigurePool configures a pgxpool.Config with the connection hooks
// This allows the hooks to be properly integrated with the pool lifecycle
func (h *hooks) configurePool(config *pgxpool.Config) {
	h.connectionHooks.ConfigurePool(config)
}

// ConfigurePool configures a pgxpool.Config with the connection hooks
// This integrates the hooks with the actual pool lifecycle events
func (ch *ConnectionHooks) ConfigurePool(config *pgxpool.Config) {
	// Store original callbacks if they exist
	originalAfterConnect := config.AfterConnect
	originalBeforeClose := config.BeforeClose

	// Set up AfterConnect hook that combines original callback with our hooks
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		// Execute original callback first
		if originalAfterConnect != nil {
			if err := originalAfterConnect(ctx, conn); err != nil {
				return err
			}
		}

		// Execute our OnConnect hooks
		if err := ch.ExecuteOnConnect(conn); err != nil {
			return err
		}

		// Execute our OnAcquire hooks
		return ch.ExecuteOnAcquire(ctx, conn)
	}

	// Set up BeforeClose hook that combines original callback with our hooks
	config.BeforeClose = func(conn *pgx.Conn) {
		// Execute our OnDisconnect hooks first
		ch.ExecuteOnDisconnect(conn)

		// Execute our OnRelease hooks
		ch.ExecuteOnRelease(conn)

		// Execute original callback last
		if originalBeforeClose != nil {
			originalBeforeClose(conn)
		}
	}
}
