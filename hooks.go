package dbutil

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
)

// ConnectionHooks manages connection lifecycle hooks
type ConnectionHooks struct {
	mu           sync.RWMutex
	onConnect    []func(*pgx.Conn) error
	onDisconnect []func(*pgx.Conn)
	onAcquire    []func(context.Context, *pgx.Conn) error
	onRelease    []func(*pgx.Conn)
}

// NewConnectionHooks creates a new connection hooks manager
func NewConnectionHooks() *ConnectionHooks {
	return &ConnectionHooks{
		onConnect:    make([]func(*pgx.Conn) error, 0),
		onDisconnect: make([]func(*pgx.Conn), 0),
		onAcquire:    make([]func(context.Context, *pgx.Conn) error, 0),
		onRelease:    make([]func(*pgx.Conn), 0),
	}
}

// AddOnConnect adds a callback that will be called when a new connection is established
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

// LoggingHook creates a hook that logs connection events
func LoggingHook(logger Logger) *ConnectionHooks {
	hooks := NewConnectionHooks()

	hooks.AddOnConnect(func(conn *pgx.Conn) error {
		logger.Log(context.Background(), LogLevelInfo, "database connection established", map[string]interface{}{
			"pid": conn.PgConn().PID(),
		})
		return nil
	})

	hooks.AddOnDisconnect(func(conn *pgx.Conn) {
		logger.Log(context.Background(), LogLevelInfo, "database connection closed", map[string]interface{}{
			"pid": conn.PgConn().PID(),
		})
	})

	hooks.AddOnAcquire(func(ctx context.Context, conn *pgx.Conn) error {
		logger.Log(ctx, LogLevelDebug, "connection acquired from pool", map[string]interface{}{
			"pid": conn.PgConn().PID(),
		})
		return nil
	})

	hooks.AddOnRelease(func(conn *pgx.Conn) {
		logger.Log(context.Background(), LogLevelDebug, "connection released to pool", map[string]interface{}{
			"pid": conn.PgConn().PID(),
		})
	})

	return hooks
}

// MetricsHook creates a hook that records connection metrics.
// Note: Duration tracking for acquire/release is not implemented in hooks as it requires
// pool-level instrumentation. Use Connection.WithMetrics() for comprehensive metrics.
func MetricsHook(metrics MetricsCollector) *ConnectionHooks {
	hooks := NewConnectionHooks()

	if metrics != nil {
		hooks.AddOnAcquire(func(ctx context.Context, conn *pgx.Conn) error {
			// Record connection acquisition (duration tracking requires pool-level instrumentation)
			metrics.RecordConnectionAcquired(0)
			return nil
		})

		hooks.AddOnRelease(func(conn *pgx.Conn) {
			// Record connection release (duration tracking requires pool-level instrumentation)
			metrics.RecordConnectionReleased(0)
		})
	}

	return hooks
}

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
