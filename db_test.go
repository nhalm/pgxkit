package pgxkit

import (
	"context"
	"testing"
)

func TestNewDB(t *testing.T) {
	// Test with nil pool - should not panic
	db := NewDB(nil)
	if db == nil {
		t.Error("NewDB should not return nil")
	}

	if db.readPool != db.writePool {
		t.Error("NewDB should use the same pool for read and write")
	}

	if db.hooks == nil {
		t.Error("NewDB should initialize hooks")
	}

	if db.shutdown {
		t.Error("NewDB should not be in shutdown state")
	}
}

func TestNewReadWriteDB(t *testing.T) {
	// Test with nil pools - should not panic
	db := NewReadWriteDB(nil, nil)
	if db == nil {
		t.Error("NewReadWriteDB should not return nil")
	}

	if db.readPool != nil || db.writePool != nil {
		// This is expected behavior - we pass nil pools
	}

	if db.hooks == nil {
		t.Error("NewReadWriteDB should initialize hooks")
	}

	if db.shutdown {
		t.Error("NewReadWriteDB should not be in shutdown state")
	}
}

func TestDBHooks(t *testing.T) {
	db := NewDB(nil)

	// Test adding operation-level hooks
	hookCalled := false
	testHook := func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		hookCalled = true
		return nil
	}

	err := db.AddHook("BeforeOperation", testHook)
	if err != nil {
		t.Errorf("AddHook should not return error: %v", err)
	}

	// Test invalid hook type
	err = db.AddHook("InvalidHook", testHook)
	if err == nil {
		t.Error("AddHook should return error for invalid hook type")
	}

	// Test hook execution
	err = db.hooks.ExecuteBeforeOperation(context.Background(), "SELECT 1", nil, nil)
	if err != nil {
		t.Errorf("ExecuteBeforeOperation should not return error: %v", err)
	}

	if !hookCalled {
		t.Error("Hook should have been called")
	}
}

func TestDBShutdown(t *testing.T) {
	db := NewDB(nil)

	// Test shutdown
	err := db.Shutdown(context.Background())
	if err != nil {
		t.Errorf("Shutdown should not return error: %v", err)
	}

	// Test that DB is marked as shutdown
	if !db.shutdown {
		t.Error("DB should be marked as shutdown")
	}

	// Test that subsequent operations fail
	_, err = db.Query(context.Background(), "SELECT 1")
	if err == nil {
		t.Error("Query should fail after shutdown")
	}
}

func TestDBStats(t *testing.T) {
	db := NewDB(nil)

	// These should not panic even with nil pools
	writeStats := db.Stats()
	readStats := db.ReadStats()
	writeStatsAgain := db.WriteStats()

	// For single pool with nil, all should be nil
	if writeStats != nil || readStats != nil || writeStatsAgain != nil {
		t.Error("Stats should be nil when pools are nil")
	}
}

func TestDBReadWriteStats(t *testing.T) {
	db := NewReadWriteDB(nil, nil)

	// These should not panic even with nil pools
	writeStats := db.Stats()
	readStats := db.ReadStats()
	writeStatsAgain := db.WriteStats()

	// For nil pools, all should be nil
	if writeStats != nil || readStats != nil || writeStatsAgain != nil {
		t.Error("Stats should be nil when pools are nil")
	}
}
