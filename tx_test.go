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
