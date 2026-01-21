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

// finalizedRow implements pgx.Row for queries on finalized transactions.
type finalizedRow struct{}

func (f *finalizedRow) Scan(dest ...any) error {
	return ErrTxFinalized
}

var _ Executor = (*Tx)(nil)

// Tx wraps a pgx.Tx to implement the Executor interface and provide
// transaction lifecycle management integrated with pgxkit's activeOps tracking.
type Tx struct {
	tx        pgx.Tx
	db        *DB
	finalized atomic.Bool
}

// Query executes a query within the transaction.
// Unlike DB.Query, this does not fire BeforeOperation/AfterOperation hooks.
func (t *Tx) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return t.tx.Query(ctx, sql, args...)
}

// QueryRow executes a query that returns a single row within the transaction.
// Unlike DB.QueryRow, this does not fire BeforeOperation/AfterOperation hooks.
func (t *Tx) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return t.tx.QueryRow(ctx, sql, args...)
}

// Exec executes a statement within the transaction.
// Unlike DB.Exec, this does not fire BeforeOperation/AfterOperation hooks.
func (t *Tx) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return t.tx.Exec(ctx, sql, args...)
}

// Commit commits the transaction.
// It fires the AfterTransaction hook and uses atomic finalization to ensure
// activeOps.Done() is called exactly once, making it safe for the
// "defer Rollback() + explicit Commit()" pattern.
func (t *Tx) Commit(ctx context.Context) error {
	if !t.finalized.CompareAndSwap(false, true) {
		return nil
	}
	defer t.db.activeOps.Done()

	err := t.tx.Commit(ctx)
	if hookErr := t.db.hooks.executeAfterTransaction(ctx, "", nil, err); hookErr != nil && err == nil {
		return fmt.Errorf("after commit hook failed: %w", hookErr)
	}
	return err
}

// Rollback rolls back the transaction.
// It fires the AfterTransaction hook and uses atomic finalization to ensure
// activeOps.Done() is called exactly once, making it safe for the
// "defer Rollback() + explicit Commit()" pattern.
func (t *Tx) Rollback(ctx context.Context) error {
	if !t.finalized.CompareAndSwap(false, true) {
		return nil
	}
	defer t.db.activeOps.Done()

	err := t.tx.Rollback(ctx)
	if hookErr := t.db.hooks.executeAfterTransaction(ctx, "", nil, err); hookErr != nil && err == nil {
		return fmt.Errorf("after rollback hook failed: %w", hookErr)
	}
	return err
}

// Tx returns the underlying pgx.Tx for advanced use cases
// that require direct access to pgx transaction functionality.
func (t *Tx) Tx() pgx.Tx {
	return t.tx
}
