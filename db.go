package pgxkit

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB represents a database connection with read/write pool abstraction
type DB struct {
	readPool  *pgxpool.Pool
	writePool *pgxpool.Pool
	hooks     *Hooks
	mu        sync.RWMutex
	shutdown  bool
}

// NewDB creates a new DB instance with a single pool (same pool for read/write)
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{
		readPool:  pool,
		writePool: pool,
		hooks:     NewHooks(),
	}
}

// NewReadWriteDB creates a new DB instance with separate read and write pools
func NewReadWriteDB(readPool, writePool *pgxpool.Pool) *DB {
	return &DB{
		readPool:  readPool,
		writePool: writePool,
		hooks:     NewHooks(),
	}
}

// Query executes a query using the write pool (safe by default)
func (db *DB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return db.executeQuery(ctx, db.writePool, sql, args...)
}

// QueryRow executes a query that returns a single row using the write pool
func (db *DB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return db.executeQueryRow(ctx, db.writePool, sql, args...)
}

// Exec executes a statement using the write pool
func (db *DB) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return db.executeExec(ctx, db.writePool, sql, args...)
}

// ReadQuery executes a query using the read pool (explicit optimization)
func (db *DB) ReadQuery(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return db.executeQuery(ctx, db.readPool, sql, args...)
}

// ReadQueryRow executes a query that returns a single row using the read pool
func (db *DB) ReadQueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return db.executeQueryRow(ctx, db.readPool, sql, args...)
}

// BeginTx starts a transaction using the write pool
func (db *DB) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) {
	db.mu.RLock()
	if db.shutdown {
		db.mu.RUnlock()
		return nil, fmt.Errorf("database is shutting down")
	}
	db.mu.RUnlock()

	// Execute BeforeTransaction hooks
	if err := db.hooks.ExecuteBeforeTransaction(ctx, "", nil, nil); err != nil {
		return nil, fmt.Errorf("before transaction hook failed: %w", err)
	}

	tx, err := db.writePool.BeginTx(ctx, txOptions)

	// Execute AfterTransaction hooks
	hookErr := db.hooks.ExecuteAfterTransaction(ctx, "", nil, err)
	if hookErr != nil && err == nil {
		// If transaction succeeded but hook failed, rollback
		if tx != nil {
			tx.Rollback(ctx)
		}
		return nil, fmt.Errorf("after transaction hook failed: %w", hookErr)
	}

	return tx, err
}

// AddHook adds a hook to the database
func (db *DB) AddHook(hookType string, hookFunc HookFunc) error {
	return db.hooks.AddHook(hookType, hookFunc)
}

// AddConnectionHook adds a connection-level hook
func (db *DB) AddConnectionHook(hookType string, hookFunc interface{}) error {
	return db.hooks.AddConnectionHook(hookType, hookFunc)
}

// Shutdown gracefully shuts down the database connections
func (db *DB) Shutdown(ctx context.Context) error {
	db.mu.Lock()
	if db.shutdown {
		db.mu.Unlock()
		return nil
	}
	db.shutdown = true
	db.mu.Unlock()

	// Execute shutdown hooks
	if err := db.hooks.ExecuteOnShutdown(ctx, "", nil, nil); err != nil {
		return fmt.Errorf("shutdown hook failed: %w", err)
	}

	// Close pools - handle nil pools gracefully
	if db.readPool != nil && db.readPool != db.writePool {
		db.readPool.Close()
	}
	if db.writePool != nil {
		db.writePool.Close()
	}

	return nil
}

// Stats returns statistics for the write pool
func (db *DB) Stats() *pgxpool.Stat {
	if db.writePool == nil {
		return nil
	}
	return db.writePool.Stat()
}

// ReadStats returns statistics for the read pool
func (db *DB) ReadStats() *pgxpool.Stat {
	if db.readPool == nil {
		return nil
	}
	return db.readPool.Stat()
}

// WriteStats returns statistics for the write pool
func (db *DB) WriteStats() *pgxpool.Stat {
	if db.writePool == nil {
		return nil
	}
	return db.writePool.Stat()
}

// Internal execution methods that handle hooks

func (db *DB) executeQuery(ctx context.Context, pool *pgxpool.Pool, sql string, args ...interface{}) (pgx.Rows, error) {
	db.mu.RLock()
	if db.shutdown {
		db.mu.RUnlock()
		return nil, fmt.Errorf("database is shutting down")
	}
	db.mu.RUnlock()

	// Execute BeforeOperation hooks
	if err := db.hooks.ExecuteBeforeOperation(ctx, sql, args, nil); err != nil {
		return nil, fmt.Errorf("before operation hook failed: %w", err)
	}

	rows, err := pool.Query(ctx, sql, args...)

	// Execute AfterOperation hooks
	if hookErr := db.hooks.ExecuteAfterOperation(ctx, sql, args, err); hookErr != nil {
		if rows != nil {
			rows.Close()
		}
		if err == nil {
			return nil, fmt.Errorf("after operation hook failed: %w", hookErr)
		}
	}

	return rows, err
}

func (db *DB) executeQueryRow(ctx context.Context, pool *pgxpool.Pool, sql string, args ...interface{}) pgx.Row {
	db.mu.RLock()
	if db.shutdown {
		db.mu.RUnlock()
		return &shutdownRow{err: fmt.Errorf("database is shutting down")}
	}
	db.mu.RUnlock()

	// Execute BeforeOperation hooks
	if err := db.hooks.ExecuteBeforeOperation(ctx, sql, args, nil); err != nil {
		return &shutdownRow{err: fmt.Errorf("before operation hook failed: %w", err)}
	}

	row := pool.QueryRow(ctx, sql, args...)

	// Execute AfterOperation hooks - for QueryRow we can't easily get the error
	// so we pass nil as the operation error
	if hookErr := db.hooks.ExecuteAfterOperation(ctx, sql, args, nil); hookErr != nil {
		return &shutdownRow{err: fmt.Errorf("after operation hook failed: %w", hookErr)}
	}

	return row
}

func (db *DB) executeExec(ctx context.Context, pool *pgxpool.Pool, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	db.mu.RLock()
	if db.shutdown {
		db.mu.RUnlock()
		return pgconn.CommandTag{}, fmt.Errorf("database is shutting down")
	}
	db.mu.RUnlock()

	// Execute BeforeOperation hooks
	if err := db.hooks.ExecuteBeforeOperation(ctx, sql, args, nil); err != nil {
		return pgconn.CommandTag{}, fmt.Errorf("before operation hook failed: %w", err)
	}

	tag, err := pool.Exec(ctx, sql, args...)

	// Execute AfterOperation hooks
	if hookErr := db.hooks.ExecuteAfterOperation(ctx, sql, args, err); hookErr != nil {
		if err == nil {
			return tag, fmt.Errorf("after operation hook failed: %w", hookErr)
		}
	}

	return tag, err
}

// shutdownRow implements pgx.Row for shutdown scenarios
type shutdownRow struct {
	err error
}

func (r *shutdownRow) Scan(dest ...interface{}) error {
	return r.err
}
