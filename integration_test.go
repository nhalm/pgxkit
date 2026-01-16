package pgxkit

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestRequireTestPool(t *testing.T) {
	// This test requires TEST_DATABASE_URL to be set
	pool := requireTestPool(t)
	if pool == nil {
		// Test was skipped, which is fine
		return
	}
}
func TestGetTestPool(t *testing.T) {
	// This test requires TEST_DATABASE_URL to be set
	pool := getTestPool()
	if pool == nil {
		// No test database available, skip
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	// Test that subsequent calls return the same pool
	pool2 := getTestPool()
	if pool2 == nil {
		t.Error("Expected second call to return pool")
	}

	if pool != pool2 {
		t.Error("Expected shared pool between calls")
	}
}

func TestCleanupTestData(t *testing.T) {
	// Test cleanup with valid SQL (should not error)
	CleanupTestData("SELECT 1", "SELECT 2")

	// Test cleanup with invalid SQL (should not fail the test, just log warnings)
	CleanupTestData("INVALID SQL STATEMENT")

	// Test cleanup when no test database is available (should not panic)
	// This is tested implicitly - if TEST_DATABASE_URL is not set, it just returns
}

func TestPoolHealthCheck(t *testing.T) {
	pool := getTestPool()
	if pool == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
		return
	}

	ctx := context.Background()

	// Test pool ping (health check)
	err := pool.Ping(ctx)
	if err != nil {
		t.Errorf("Expected pool ping to pass, got error: %v", err)
	}

	// Test pool stats (readiness check)
	stats := pool.Stat()
	if stats == nil {
		t.Error("Expected pool stats to be available")
	}
}

func TestTransactionCommitFlow(t *testing.T) {
	pool := requireTestPool(t)
	ctx := context.Background()

	db := NewDB()
	db.readPool = pool
	db.writePool = pool

	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS tx_test_commit (id SERIAL PRIMARY KEY, value TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}
	defer CleanupTestData("DROP TABLE IF EXISTS tx_test_commit")

	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO tx_test_commit (value) VALUES ($1)`, "test_value")
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("Insert in transaction failed: %v", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	var value string
	err = pool.QueryRow(ctx, `SELECT value FROM tx_test_commit WHERE value = $1`, "test_value").Scan(&value)
	if err != nil {
		t.Fatalf("Failed to verify committed data: %v", err)
	}
	if value != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", value)
	}
}

func TestTransactionRollbackOnError(t *testing.T) {
	pool := requireTestPool(t)
	ctx := context.Background()

	db := NewDB()
	db.readPool = pool
	db.writePool = pool

	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS tx_test_rollback (id SERIAL PRIMARY KEY, value TEXT NOT NULL)`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}
	defer CleanupTestData("DROP TABLE IF EXISTS tx_test_rollback")

	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `INSERT INTO tx_test_rollback (value) VALUES ($1)`, "before_error")
	if err != nil {
		t.Fatalf("First insert failed: %v", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO tx_test_rollback (value) VALUES ($1)`, nil)
	if err == nil {
		t.Fatal("Expected error on NULL insert into NOT NULL column")
	}

	err = tx.Rollback(ctx)
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	var count int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM tx_test_rollback`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count rows: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 rows after rollback, got %d", count)
	}
}

func TestGracefulShutdownWaitsForTransaction(t *testing.T) {
	pool := requireTestPool(t)
	ctx := context.Background()

	db := NewDB()
	db.readPool = pool
	db.writePool = pool

	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS tx_test_shutdown (id SERIAL PRIMARY KEY, value TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}
	defer CleanupTestData("DROP TABLE IF EXISTS tx_test_shutdown")

	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	shutdownComplete := make(chan struct{})
	go func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		db.Shutdown(shutdownCtx)
		close(shutdownComplete)
	}()

	select {
	case <-shutdownComplete:
		t.Fatal("Shutdown completed before transaction finished")
	case <-time.After(100 * time.Millisecond):
	}

	_, err = tx.Exec(ctx, `INSERT INTO tx_test_shutdown (value) VALUES ($1)`, "during_shutdown")
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("Insert during shutdown wait failed: %v", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	select {
	case <-shutdownComplete:
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not complete after transaction finished")
	}

	var value string
	err = pool.QueryRow(ctx, `SELECT value FROM tx_test_shutdown WHERE value = $1`, "during_shutdown").Scan(&value)
	if err != nil {
		t.Fatalf("Failed to verify committed data: %v", err)
	}
	if value != "during_shutdown" {
		t.Errorf("Expected 'during_shutdown', got '%s'", value)
	}
}

func TestTxEscapeHatch(t *testing.T) {
	pool := requireTestPool(t)
	ctx := context.Background()

	db := NewDB()
	db.readPool = pool
	db.writePool = pool

	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS tx_test_escape_hatch (id SERIAL PRIMARY KEY, value TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}
	defer CleanupTestData("DROP TABLE IF EXISTS tx_test_escape_hatch")

	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}
	defer tx.Rollback(ctx)

	// Get the underlying pgx.Tx via escape hatch
	rawTx := tx.Tx()
	if rawTx == nil {
		t.Fatal("Tx() returned nil")
	}

	// Execute query using the raw pgx.Tx
	_, err = rawTx.Exec(ctx, `INSERT INTO tx_test_escape_hatch (value) VALUES ($1)`, "escape_hatch_value")
	if err != nil {
		t.Fatalf("Exec via raw pgx.Tx failed: %v", err)
	}

	// Query using the raw pgx.Tx
	var value string
	err = rawTx.QueryRow(ctx, `SELECT value FROM tx_test_escape_hatch WHERE value = $1`, "escape_hatch_value").Scan(&value)
	if err != nil {
		t.Fatalf("QueryRow via raw pgx.Tx failed: %v", err)
	}
	if value != "escape_hatch_value" {
		t.Errorf("Expected 'escape_hatch_value', got '%s'", value)
	}

	// Commit using the wrapper to ensure proper cleanup
	err = tx.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify data was committed
	var count int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM tx_test_escape_hatch WHERE value = $1`, "escape_hatch_value").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify committed data: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 row, got %d", count)
	}
}
