package pgxkit

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type mockTx struct {
	queryFunc    func(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	queryRowFunc func(ctx context.Context, sql string, args ...interface{}) pgx.Row
	execFunc     func(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	commitFunc   func(ctx context.Context) error
	rollbackFunc func(ctx context.Context) error
}

func (m *mockTx) Begin(ctx context.Context) (pgx.Tx, error) { return nil, nil }
func (m *mockTx) Commit(ctx context.Context) error {
	if m.commitFunc != nil {
		return m.commitFunc(ctx)
	}
	return nil
}
func (m *mockTx) Rollback(ctx context.Context) error {
	if m.rollbackFunc != nil {
		return m.rollbackFunc(ctx)
	}
	return nil
}
func (m *mockTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (m *mockTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults { return nil }
func (m *mockTx) LargeObjects() pgx.LargeObjects                               { return pgx.LargeObjects{} }
func (m *mockTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (m *mockTx) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}
func (m *mockTx) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, sql, args...)
	}
	return nil, nil
}
func (m *mockTx) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return nil
}
func (m *mockTx) Conn() *pgx.Conn { return nil }

func TestTxQuery(t *testing.T) {
	db := NewDB()

	queryCalled := false
	var capturedSQL string
	var capturedArgs []interface{}

	mock := &mockTx{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
			queryCalled = true
			capturedSQL = sql
			capturedArgs = args
			return nil, nil
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	_, err := tx.Query(ctx, "SELECT * FROM users WHERE id = $1", 42)
	if err != nil {
		t.Errorf("Query returned unexpected error: %v", err)
	}

	if !queryCalled {
		t.Error("Query should have called underlying pgx.Tx.Query")
	}
	if capturedSQL != "SELECT * FROM users WHERE id = $1" {
		t.Errorf("Query passed wrong SQL: got %q", capturedSQL)
	}
	if len(capturedArgs) != 1 || capturedArgs[0] != 42 {
		t.Errorf("Query passed wrong args: got %v", capturedArgs)
	}
}

func TestTxQueryError(t *testing.T) {
	db := NewDB()
	expectedErr := errors.New("query failed")

	mock := &mockTx{
		queryFunc: func(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
			return nil, expectedErr
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	_, err := tx.Query(ctx, "SELECT 1")
	if err != expectedErr {
		t.Errorf("Query should return underlying error: got %v, want %v", err, expectedErr)
	}
}

type mockRow struct {
	scanFunc func(dest ...interface{}) error
}

func (m *mockRow) Scan(dest ...interface{}) error {
	if m.scanFunc != nil {
		return m.scanFunc(dest...)
	}
	return nil
}

func TestTxQueryRow(t *testing.T) {
	db := NewDB()

	queryRowCalled := false
	var capturedSQL string
	var capturedArgs []interface{}

	expectedRow := &mockRow{}
	mock := &mockTx{
		queryRowFunc: func(ctx context.Context, sql string, args ...interface{}) pgx.Row {
			queryRowCalled = true
			capturedSQL = sql
			capturedArgs = args
			return expectedRow
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	row := tx.QueryRow(ctx, "SELECT name FROM users WHERE id = $1", 123)

	if !queryRowCalled {
		t.Error("QueryRow should have called underlying pgx.Tx.QueryRow")
	}
	if capturedSQL != "SELECT name FROM users WHERE id = $1" {
		t.Errorf("QueryRow passed wrong SQL: got %q", capturedSQL)
	}
	if len(capturedArgs) != 1 || capturedArgs[0] != 123 {
		t.Errorf("QueryRow passed wrong args: got %v", capturedArgs)
	}
	if row != expectedRow {
		t.Error("QueryRow should return the row from underlying pgx.Tx")
	}
}

func TestTxExec(t *testing.T) {
	db := NewDB()

	execCalled := false
	var capturedSQL string
	var capturedArgs []interface{}

	expectedTag := pgconn.NewCommandTag("UPDATE 1")
	mock := &mockTx{
		execFunc: func(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
			execCalled = true
			capturedSQL = sql
			capturedArgs = args
			return expectedTag, nil
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	tag, err := tx.Exec(ctx, "UPDATE users SET name = $1 WHERE id = $2", "Alice", 42)
	if err != nil {
		t.Errorf("Exec returned unexpected error: %v", err)
	}

	if !execCalled {
		t.Error("Exec should have called underlying pgx.Tx.Exec")
	}
	if capturedSQL != "UPDATE users SET name = $1 WHERE id = $2" {
		t.Errorf("Exec passed wrong SQL: got %q", capturedSQL)
	}
	if len(capturedArgs) != 2 || capturedArgs[0] != "Alice" || capturedArgs[1] != 42 {
		t.Errorf("Exec passed wrong args: got %v", capturedArgs)
	}
	if tag != expectedTag {
		t.Errorf("Exec should return command tag from underlying pgx.Tx: got %v, want %v", tag, expectedTag)
	}
}

func TestTxExecError(t *testing.T) {
	db := NewDB()
	expectedErr := errors.New("exec failed")

	mock := &mockTx{
		execFunc: func(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, expectedErr
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	_, err := tx.Exec(ctx, "DELETE FROM users")
	if err != expectedErr {
		t.Errorf("Exec should return underlying error: got %v, want %v", err, expectedErr)
	}
}

func TestTxCommit(t *testing.T) {
	db := NewDB()

	commitCalled := false
	mock := &mockTx{
		commitFunc: func(ctx context.Context) error {
			commitCalled = true
			return nil
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	err := tx.Commit(ctx)
	if err != nil {
		t.Errorf("Commit returned unexpected error: %v", err)
	}

	if !commitCalled {
		t.Error("Commit should have called underlying pgx.Tx.Commit")
	}

	if !tx.finalized.Load() {
		t.Error("Commit should set finalized to true")
	}
}

func TestTxCommitError(t *testing.T) {
	db := NewDB()
	expectedErr := errors.New("commit failed")

	mock := &mockTx{
		commitFunc: func(ctx context.Context) error {
			return expectedErr
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	err := tx.Commit(ctx)
	if err != expectedErr {
		t.Errorf("Commit should return underlying error: got %v, want %v", err, expectedErr)
	}
}

func TestTxCommitHookExecution(t *testing.T) {
	db := NewDB()

	hookCalled := false
	var hookErr error
	db.hooks.addHook(AfterTransaction, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		hookCalled = true
		hookErr = operationErr
		return nil
	})

	mock := &mockTx{
		commitFunc: func(ctx context.Context) error {
			return nil
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	err := tx.Commit(ctx)
	if err != nil {
		t.Errorf("Commit returned unexpected error: %v", err)
	}

	if !hookCalled {
		t.Error("AfterTransaction hook should have been called")
	}

	if hookErr != nil {
		t.Errorf("AfterTransaction hook should receive nil error on success, got %v", hookErr)
	}
}

func TestTxCommitHookReceivesError(t *testing.T) {
	db := NewDB()
	expectedErr := errors.New("commit failed")

	hookCalled := false
	var hookErr error
	db.hooks.addHook(AfterTransaction, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		hookCalled = true
		hookErr = operationErr
		return nil
	})

	mock := &mockTx{
		commitFunc: func(ctx context.Context) error {
			return expectedErr
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	_ = tx.Commit(ctx)

	if !hookCalled {
		t.Error("AfterTransaction hook should have been called even on error")
	}

	if hookErr != expectedErr {
		t.Errorf("AfterTransaction hook should receive commit error: got %v, want %v", hookErr, expectedErr)
	}
}

func TestTxCommitFinalizationPreventsDoubleCommit(t *testing.T) {
	db := NewDB()

	commitCount := 0
	mock := &mockTx{
		commitFunc: func(ctx context.Context) error {
			commitCount++
			return nil
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	_ = tx.Commit(ctx)
	_ = tx.Commit(ctx)

	if commitCount != 1 {
		t.Errorf("Underlying commit should only be called once, got %d calls", commitCount)
	}
}

func TestTxRollback(t *testing.T) {
	db := NewDB()

	rollbackCalled := false
	mock := &mockTx{
		rollbackFunc: func(ctx context.Context) error {
			rollbackCalled = true
			return nil
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	err := tx.Rollback(ctx)
	if err != nil {
		t.Errorf("Rollback returned unexpected error: %v", err)
	}

	if !rollbackCalled {
		t.Error("Rollback should have called underlying pgx.Tx.Rollback")
	}

	if !tx.finalized.Load() {
		t.Error("Rollback should set finalized to true")
	}
}

func TestTxRollbackError(t *testing.T) {
	db := NewDB()
	expectedErr := errors.New("rollback failed")

	mock := &mockTx{
		rollbackFunc: func(ctx context.Context) error {
			return expectedErr
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	err := tx.Rollback(ctx)
	if err != expectedErr {
		t.Errorf("Rollback should return underlying error: got %v, want %v", err, expectedErr)
	}
}

func TestTxRollbackHookExecution(t *testing.T) {
	db := NewDB()

	hookCalled := false
	var hookErr error
	db.hooks.addHook(AfterTransaction, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		hookCalled = true
		hookErr = operationErr
		return nil
	})

	mock := &mockTx{
		rollbackFunc: func(ctx context.Context) error {
			return nil
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	err := tx.Rollback(ctx)
	if err != nil {
		t.Errorf("Rollback returned unexpected error: %v", err)
	}

	if !hookCalled {
		t.Error("AfterTransaction hook should have been called")
	}

	if hookErr != nil {
		t.Errorf("AfterTransaction hook should receive nil error on success, got %v", hookErr)
	}
}

func TestTxRollbackHookReceivesError(t *testing.T) {
	db := NewDB()
	expectedErr := errors.New("rollback failed")

	hookCalled := false
	var hookErr error
	db.hooks.addHook(AfterTransaction, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		hookCalled = true
		hookErr = operationErr
		return nil
	})

	mock := &mockTx{
		rollbackFunc: func(ctx context.Context) error {
			return expectedErr
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	_ = tx.Rollback(ctx)

	if !hookCalled {
		t.Error("AfterTransaction hook should have been called even on error")
	}

	if hookErr != expectedErr {
		t.Errorf("AfterTransaction hook should receive rollback error: got %v, want %v", hookErr, expectedErr)
	}
}

func TestTxDoubleRollbackSafety(t *testing.T) {
	db := NewDB()

	rollbackCount := 0
	mock := &mockTx{
		rollbackFunc: func(ctx context.Context) error {
			rollbackCount++
			return nil
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	err1 := tx.Rollback(ctx)
	err2 := tx.Rollback(ctx)

	if err1 != nil {
		t.Errorf("First rollback returned unexpected error: %v", err1)
	}
	if err2 != nil {
		t.Errorf("Second rollback should return nil (no-op), got: %v", err2)
	}

	if rollbackCount != 1 {
		t.Errorf("Underlying rollback should only be called once, got %d calls", rollbackCount)
	}
}

func TestTxDeferRollbackWithExplicitCommit(t *testing.T) {
	db := NewDB()

	commitCalled := false
	rollbackCalled := false
	mock := &mockTx{
		commitFunc: func(ctx context.Context) error {
			commitCalled = true
			return nil
		},
		rollbackFunc: func(ctx context.Context) error {
			rollbackCalled = true
			return nil
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	func() {
		defer tx.Rollback(ctx)
		_ = tx.Commit(ctx)
	}()

	if !commitCalled {
		t.Error("Commit should have been called")
	}
	if rollbackCalled {
		t.Error("Rollback should not have been called after Commit")
	}
	if !tx.finalized.Load() {
		t.Error("Transaction should be finalized")
	}
}

func TestTxCommitHookErrorPropagation(t *testing.T) {
	db := NewDB()
	hookErr := errors.New("hook failed")

	db.hooks.addHook(AfterTransaction, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		return hookErr
	})

	mock := &mockTx{
		commitFunc: func(ctx context.Context) error {
			return nil
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	err := tx.Commit(ctx)

	if !errors.Is(err, hookErr) {
		t.Errorf("Commit should return wrapped hook error: got %v, want error wrapping %v", err, hookErr)
	}
}

func TestTxRollbackHookErrorPropagation(t *testing.T) {
	db := NewDB()
	hookErr := errors.New("hook failed")

	db.hooks.addHook(AfterTransaction, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		return hookErr
	})

	mock := &mockTx{
		rollbackFunc: func(ctx context.Context) error {
			return nil
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	err := tx.Rollback(ctx)

	if !errors.Is(err, hookErr) {
		t.Errorf("Rollback should return wrapped hook error when rollback succeeds but hook fails: got %v, want error wrapping %v", err, hookErr)
	}
}

func TestTxCommitHookReceivesOperationType(t *testing.T) {
	db := NewDB()

	var capturedSQL string
	db.hooks.addHook(AfterTransaction, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		capturedSQL = sql
		return nil
	})

	mock := &mockTx{
		commitFunc: func(ctx context.Context) error {
			return nil
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	_ = tx.Commit(ctx)

	if capturedSQL != TxCommit {
		t.Errorf("AfterTransaction hook should receive TxCommit as sql parameter: got %q, want %q", capturedSQL, TxCommit)
	}
}

func TestTxRollbackHookReceivesOperationType(t *testing.T) {
	db := NewDB()

	var capturedSQL string
	db.hooks.addHook(AfterTransaction, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		capturedSQL = sql
		return nil
	})

	mock := &mockTx{
		rollbackFunc: func(ctx context.Context) error {
			return nil
		},
	}

	db.activeOps.Add(1)
	tx := &Tx{tx: mock, db: db}

	ctx := context.Background()
	_ = tx.Rollback(ctx)

	if capturedSQL != TxRollback {
		t.Errorf("AfterTransaction hook should receive TxRollback as sql parameter: got %q, want %q", capturedSQL, TxRollback)
	}
}
