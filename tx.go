package pgxkit

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	TxCommit   = "TX:COMMIT"
	TxRollback = "TX:ROLLBACK"
)

var ErrTxFinalized = errors.New("transaction already finalized")

type finalizedRow struct{}

func (f *finalizedRow) Scan(dest ...any) error {
	return ErrTxFinalized
}

var _ Executor = (*Tx)(nil)

// Tx wraps a pgx.Tx to implement the Executor interface and provide
// transaction lifecycle management integrated with pgxkit's activeOps tracking
// and hook system.
type Tx struct {
	tx        pgx.Tx
	db        *DB
	finalized atomic.Bool
}

// Query executes a query within the transaction. Fires BeforeOperation /
// AfterOperation hooks on the parent DB.
func (t *Tx) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if t.finalized.Load() {
		return nil, ErrTxFinalized
	}
	if err := t.db.hooks.executeBeforeOperation(ctx, sql, args, pgconn.CommandTag{}, nil); err != nil {
		return nil, fmt.Errorf("before operation hook failed: %w", err)
	}
	rows, err := t.tx.Query(ctx, sql, args...)
	if hookErr := t.db.hooks.executeAfterOperation(ctx, sql, args, pgconn.CommandTag{}, err); hookErr != nil {
		if rows != nil {
			rows.Close()
		}
		if err == nil {
			return nil, fmt.Errorf("after operation hook failed: %w", hookErr)
		}
	}
	return rows, err
}

// QueryRow executes a query that returns a single row within the transaction.
// Fires BeforeOperation / AfterOperation hooks on the parent DB.
func (t *Tx) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	if t.finalized.Load() {
		return &finalizedRow{}
	}
	if err := t.db.hooks.executeBeforeOperation(ctx, sql, args, pgconn.CommandTag{}, nil); err != nil {
		return &shutdownRow{err: fmt.Errorf("before operation hook failed: %w", err)}
	}
	row := t.tx.QueryRow(ctx, sql, args...)
	if hookErr := t.db.hooks.executeAfterOperation(ctx, sql, args, pgconn.CommandTag{}, nil); hookErr != nil {
		return &shutdownRow{err: fmt.Errorf("after operation hook failed: %w", hookErr)}
	}
	return row
}

// Exec executes a statement within the transaction. Fires BeforeOperation /
// AfterOperation hooks on the parent DB; AfterOperation receives the command tag.
func (t *Tx) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	if t.finalized.Load() {
		return pgconn.CommandTag{}, ErrTxFinalized
	}
	if err := t.db.hooks.executeBeforeOperation(ctx, sql, args, pgconn.CommandTag{}, nil); err != nil {
		return pgconn.CommandTag{}, fmt.Errorf("before operation hook failed: %w", err)
	}
	tag, err := t.tx.Exec(ctx, sql, args...)
	if hookErr := t.db.hooks.executeAfterOperation(ctx, sql, args, tag, err); hookErr != nil {
		if err == nil {
			return tag, fmt.Errorf("after operation hook failed: %w", hookErr)
		}
	}
	return tag, err
}

// Commit commits the transaction and fires AfterTransaction. Atomic
// finalization makes "defer Rollback() + explicit Commit()" safe.
func (t *Tx) Commit(ctx context.Context) error {
	if !t.finalized.CompareAndSwap(false, true) {
		return nil
	}
	defer t.db.activeOps.Done()

	err := t.tx.Commit(ctx)
	hookErr := t.db.hooks.executeAfterTransaction(ctx, TxCommit, nil, pgconn.CommandTag{}, err)
	if hookErr != nil {
		if err != nil {
			return errors.Join(err, fmt.Errorf("after commit hook failed: %w", hookErr))
		}
		return fmt.Errorf("after commit hook failed: %w", hookErr)
	}
	return err
}

// Rollback rolls back the transaction and fires AfterTransaction. Atomic
// finalization makes "defer Rollback() + explicit Commit()" safe.
func (t *Tx) Rollback(ctx context.Context) error {
	if !t.finalized.CompareAndSwap(false, true) {
		return nil
	}
	defer t.db.activeOps.Done()

	err := t.tx.Rollback(ctx)
	hookErr := t.db.hooks.executeAfterTransaction(ctx, TxRollback, nil, pgconn.CommandTag{}, err)
	if hookErr != nil {
		if err != nil {
			return errors.Join(err, fmt.Errorf("after rollback hook failed: %w", hookErr))
		}
		return fmt.Errorf("after rollback hook failed: %w", hookErr)
	}
	return err
}

// Tx returns the underlying pgx.Tx for advanced use cases that require direct
// access to pgx transaction functionality.
func (t *Tx) Tx() pgx.Tx {
	return t.tx
}

// IsFinalized returns true if the transaction has been committed or rolled back.
func (t *Tx) IsFinalized() bool {
	return t.finalized.Load()
}
