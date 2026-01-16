package pgxkit

import (
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
