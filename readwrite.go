package dbutil

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReadWriteConnection represents a database connection with separate read and write pools
type ReadWriteConnection[T Querier] struct {
	readPool     *pgxpool.Pool
	writePool    *pgxpool.Pool
	readQueries  T
	writeQueries T
	metrics      MetricsCollector
	hooks        *ConnectionHooks
}

// NewReadWriteConnection creates a new connection with separate read and write pools
func NewReadWriteConnection[T Querier](ctx context.Context, readDSN, writeDSN string, newQueriesFunc func(*pgxpool.Pool) T) (*ReadWriteConnection[T], error) {
	return NewReadWriteConnectionWithConfig(ctx, readDSN, writeDSN, newQueriesFunc, nil, nil)
}

// NewReadWriteConnectionWithConfig creates a new read/write connection with configuration options
func NewReadWriteConnectionWithConfig[T Querier](ctx context.Context, readDSN, writeDSN string, newQueriesFunc func(*pgxpool.Pool) T, readConfig, writeConfig *Config) (*ReadWriteConnection[T], error) {
	// Create read pool
	readPool, err := createPoolWithConfig(ctx, readDSN, readConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create read pool: %w", err)
	}

	// Create write pool
	writePool, err := createPoolWithConfig(ctx, writeDSN, writeConfig)
	if err != nil {
		readPool.Close()
		return nil, fmt.Errorf("failed to create write pool: %w", err)
	}

	return &ReadWriteConnection[T]{
		readPool:     readPool,
		writePool:    writePool,
		readQueries:  newQueriesFunc(readPool),
		writeQueries: newQueriesFunc(writePool),
		metrics:      nil,
		hooks:        nil,
	}, nil
}

// ReadQueries returns the queries instance for read operations
func (rw *ReadWriteConnection[T]) ReadQueries() T {
	return rw.readQueries
}

// WriteQueries returns the queries instance for write operations
func (rw *ReadWriteConnection[T]) WriteQueries() T {
	return rw.writeQueries
}

// ReadDB returns the underlying read pool
func (rw *ReadWriteConnection[T]) ReadDB() *pgxpool.Pool {
	return rw.readPool
}

// WriteDB returns the underlying write pool
func (rw *ReadWriteConnection[T]) WriteDB() *pgxpool.Pool {
	return rw.writePool
}

// HealthCheck performs health checks on both read and write connections
func (rw *ReadWriteConnection[T]) HealthCheck(ctx context.Context) error {
	if err := rw.readPool.Ping(ctx); err != nil {
		return fmt.Errorf("read pool health check failed: %w", err)
	}
	if err := rw.writePool.Ping(ctx); err != nil {
		return fmt.Errorf("write pool health check failed: %w", err)
	}
	return nil
}

// IsReady checks if both read and write connections are ready
func (rw *ReadWriteConnection[T]) IsReady(ctx context.Context) bool {
	return rw.HealthCheck(ctx) == nil
}

// ReadStats returns statistics for the read connection pool
func (rw *ReadWriteConnection[T]) ReadStats() *pgxpool.Stat {
	return rw.readPool.Stat()
}

// WriteStats returns statistics for the write connection pool
func (rw *ReadWriteConnection[T]) WriteStats() *pgxpool.Stat {
	return rw.writePool.Stat()
}

// WithMetrics returns a new read/write connection with metrics collection enabled
func (rw *ReadWriteConnection[T]) WithMetrics(metrics MetricsCollector) *ReadWriteConnection[T] {
	return &ReadWriteConnection[T]{
		readPool:     rw.readPool,
		writePool:    rw.writePool,
		readQueries:  rw.readQueries,
		writeQueries: rw.writeQueries,
		metrics:      metrics,
		hooks:        rw.hooks,
	}
}

// WithHooks returns a new read/write connection with hooks enabled
func (rw *ReadWriteConnection[T]) WithHooks(hooks *ConnectionHooks) *ReadWriteConnection[T] {
	return &ReadWriteConnection[T]{
		readPool:     rw.readPool,
		writePool:    rw.writePool,
		readQueries:  rw.readQueries,
		writeQueries: rw.writeQueries,
		metrics:      rw.metrics,
		hooks:        hooks,
	}
}

// WithTransaction executes the given function within a database transaction on the write pool
func (rw *ReadWriteConnection[T]) WithTransaction(ctx context.Context, fn TransactionFunc[T]) error {
	tx, err := rw.writePool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			if !errors.Is(rollbackErr, pgx.ErrTxClosed) {
				_ = rollbackErr // Explicitly ignore for linter
			}
		}
	}()

	// Use sqlc's built-in WithTx method on our cached write queries
	txQuerier := rw.writeQueries.WithTx(tx)

	// Execute the function with the transaction querier
	if err := fn(ctx, txQuerier.(T)); err != nil {
		return err // Transaction will be rolled back by defer
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// BeginTransaction starts a new transaction on the write pool
func (rw *ReadWriteConnection[T]) BeginTransaction(ctx context.Context) (pgx.Tx, T, error) {
	tx, err := rw.writePool.Begin(ctx)
	if err != nil {
		return nil, *new(T), fmt.Errorf("failed to begin transaction: %w", err)
	}

	txQueries := rw.writeQueries.WithTx(tx)
	return tx, txQueries.(T), nil
}

// WithRetry returns a new read/write connection with retry logic enabled
func (rw *ReadWriteConnection[T]) WithRetry(config *RetryConfig) *RetryableReadWriteConnection[T] {
	if config == nil {
		config = DefaultRetryConfig()
	}
	return &RetryableReadWriteConnection[T]{
		ReadWriteConnection: rw,
		retryConfig:         config,
	}
}

// Close closes both read and write pools
func (rw *ReadWriteConnection[T]) Close() {
	rw.readPool.Close()
	rw.writePool.Close()
}

// RetryableReadWriteConnection wraps a ReadWriteConnection with retry logic
type RetryableReadWriteConnection[T Querier] struct {
	*ReadWriteConnection[T]
	retryConfig *RetryConfig
}

// WithRetryableTransaction executes the given function within a database transaction with retry logic
func (rrc *RetryableReadWriteConnection[T]) WithRetryableTransaction(ctx context.Context, fn TransactionFunc[T]) error {
	return retryOperation(ctx, rrc.retryConfig, func(ctx context.Context) error {
		return rrc.ReadWriteConnection.WithTransaction(ctx, fn)
	})
}
