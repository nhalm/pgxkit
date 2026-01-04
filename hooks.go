package pgxkit

import (
	"context"
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
	connectionHooks *connectionHooks
}

// newHooks creates a new hooks manager
func newHooks() *hooks {
	return &hooks{
		beforeOperation:   make([]HookFunc, 0),
		afterOperation:    make([]HookFunc, 0),
		beforeTransaction: make([]HookFunc, 0),
		afterTransaction:  make([]HookFunc, 0),
		onShutdown:        make([]HookFunc, 0),
		connectionHooks:   newConnectionHooks(),
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

// connectionHooks manages connection lifecycle hooks.
// These hooks are integrated with pgx's connection lifecycle and are useful
// for connection setup, validation, and cleanup. They use pgx's native function signatures.
type connectionHooks struct {
	mu           sync.RWMutex
	onConnect    []func(*pgx.Conn) error
	onDisconnect []func(*pgx.Conn)
	onAcquire    []func(context.Context, *pgx.Conn) error
	onRelease    []func(*pgx.Conn)
}

// newConnectionHooks creates a new connection hooks manager.
func newConnectionHooks() *connectionHooks {
	return &connectionHooks{
		onConnect:    make([]func(*pgx.Conn) error, 0),
		onDisconnect: make([]func(*pgx.Conn), 0),
		onAcquire:    make([]func(context.Context, *pgx.Conn) error, 0),
		onRelease:    make([]func(*pgx.Conn), 0),
	}
}

// addOnConnect adds a callback that will be called when a new connection is established.
func (h *connectionHooks) addOnConnect(fn func(*pgx.Conn) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onConnect = append(h.onConnect, fn)
}

// addOnDisconnect adds a callback that will be called when a connection is closed
func (h *connectionHooks) addOnDisconnect(fn func(*pgx.Conn)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onDisconnect = append(h.onDisconnect, fn)
}

// addOnAcquire adds a callback that will be called when a connection is acquired from the pool
func (h *connectionHooks) addOnAcquire(fn func(context.Context, *pgx.Conn) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onAcquire = append(h.onAcquire, fn)
}

// addOnRelease adds a callback that will be called when a connection is released back to the pool
func (h *connectionHooks) addOnRelease(fn func(*pgx.Conn)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onRelease = append(h.onRelease, fn)
}

// executeOnConnect executes all OnConnect callbacks
func (h *connectionHooks) executeOnConnect(conn *pgx.Conn) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, fn := range h.onConnect {
		if err := fn(conn); err != nil {
			return err
		}
	}
	return nil
}

// executeOnDisconnect executes all OnDisconnect callbacks
func (h *connectionHooks) executeOnDisconnect(conn *pgx.Conn) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, fn := range h.onDisconnect {
		fn(conn)
	}
}

// executeOnAcquire executes all OnAcquire callbacks
func (h *connectionHooks) executeOnAcquire(ctx context.Context, conn *pgx.Conn) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, fn := range h.onAcquire {
		if err := fn(ctx, conn); err != nil {
			return err
		}
	}
	return nil
}

// executeOnRelease executes all OnRelease callbacks
func (h *connectionHooks) executeOnRelease(conn *pgx.Conn) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, fn := range h.onRelease {
		fn(conn)
	}
}

// validationHook creates a hook that validates connections
func validationHook() *connectionHooks {
	hooks := newConnectionHooks()

	hooks.addOnConnect(func(conn *pgx.Conn) error {
		_, err := conn.Exec(context.Background(), "SELECT 1")
		return err
	})

	hooks.addOnAcquire(func(ctx context.Context, conn *pgx.Conn) error {
		return conn.Ping(ctx)
	})

	return hooks
}

// setupHook creates a hook that sets up connection-specific settings
func setupHook(setupSQL string) *connectionHooks {
	hooks := newConnectionHooks()

	hooks.addOnConnect(func(conn *pgx.Conn) error {
		if setupSQL != "" {
			_, err := conn.Exec(context.Background(), setupSQL)
			return err
		}
		return nil
	})

	return hooks
}

// combineHooks combines multiple hook managers into one
func combineHooks(hooksList ...*connectionHooks) *connectionHooks {
	combined := newConnectionHooks()

	for _, hooks := range hooksList {
		hooks.mu.RLock()

		for _, fn := range hooks.onConnect {
			combined.addOnConnect(fn)
		}

		for _, fn := range hooks.onDisconnect {
			combined.addOnDisconnect(fn)
		}

		for _, fn := range hooks.onAcquire {
			combined.addOnAcquire(fn)
		}

		for _, fn := range hooks.onRelease {
			combined.addOnRelease(fn)
		}

		hooks.mu.RUnlock()
	}

	return combined
}

// configurePool configures a pgxpool.Config with the connection hooks
func (h *hooks) configurePool(config *pgxpool.Config) {
	h.connectionHooks.configurePool(config)
}

// configurePool configures a pgxpool.Config with the connection hooks
func (ch *connectionHooks) configurePool(config *pgxpool.Config) {
	originalAfterConnect := config.AfterConnect
	originalBeforeClose := config.BeforeClose
	originalPrepareConn := config.PrepareConn
	originalAfterRelease := config.AfterRelease

	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		if originalAfterConnect != nil {
			if err := originalAfterConnect(ctx, conn); err != nil {
				return err
			}
		}
		return ch.executeOnConnect(conn)
	}

	config.BeforeClose = func(conn *pgx.Conn) {
		ch.executeOnDisconnect(conn)
		if originalBeforeClose != nil {
			originalBeforeClose(conn)
		}
	}

	config.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
		if originalPrepareConn != nil {
			ok, err := originalPrepareConn(ctx, conn)
			if !ok || err != nil {
				return ok, err
			}
		}
		if err := ch.executeOnAcquire(ctx, conn); err != nil {
			return false, nil
		}
		return true, nil
	}

	config.AfterRelease = func(conn *pgx.Conn) bool {
		ch.executeOnRelease(conn)
		if originalAfterRelease != nil {
			return originalAfterRelease(conn)
		}
		return true
	}
}
