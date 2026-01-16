package pgxkit

import (
	"context"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
)

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
