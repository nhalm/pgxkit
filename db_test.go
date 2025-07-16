package pgxkit

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewDB(t *testing.T) {
	// Test creating unconnected DB
	db := NewDB()
	if db == nil {
		t.Error("NewDB should not return nil")
		return
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
		return
	}

	// Expected behavior - we pass nil pools, so they should be nil

	if db.hooks == nil {
		t.Error("NewReadWriteDB should initialize hooks")
	}

	if db.shutdown {
		t.Error("NewReadWriteDB should not be in shutdown state")
	}
}

func TestDBHooks(t *testing.T) {
	db := NewDB()

	// Test adding operation-level hooks
	hookCalled := false
	testHook := func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		hookCalled = true
		return nil
	}

	db.AddHook(BeforeOperation, testHook)

	// Test hook execution
	err := db.hooks.ExecuteBeforeOperation(context.Background(), "SELECT 1", nil, nil)
	if err != nil {
		t.Errorf("ExecuteBeforeOperation should not return error: %v", err)
	}

	if !hookCalled {
		t.Error("Hook should have been called")
	}
}

func TestDBConnectionHooks(t *testing.T) {
	db := NewDB()

	// Test adding connection-level hooks
	connectCalled := false
	testConnectHook := func(conn *pgx.Conn) error {
		connectCalled = true
		return nil
	}

	err := db.AddConnectionHook("OnConnect", testConnectHook)
	if err != nil {
		t.Errorf("AddConnectionHook should not return error: %v", err)
	}

	// Test invalid connection hook type
	err = db.AddConnectionHook("InvalidHook", testConnectHook)
	if err == nil {
		t.Error("AddConnectionHook should return error for invalid hook type")
	}

	// Test hook execution
	err = db.hooks.connectionHooks.ExecuteOnConnect(nil)
	if err != nil {
		t.Errorf("ExecuteOnConnect should not return error: %v", err)
	}

	if !connectCalled {
		t.Error("Connect hook should have been called")
	}
}

func TestHooksConfigurePool(t *testing.T) {
	hooks := NewHooks()

	// Add some connection hooks
	connectCalled := false
	disconnectCalled := false
	acquireCalled := false
	releaseCalled := false

	err := hooks.AddConnectionHook("OnConnect", func(conn *pgx.Conn) error {
		connectCalled = true
		return nil
	})
	if err != nil {
		t.Errorf("AddConnectionHook should not return error: %v", err)
	}

	err = hooks.AddConnectionHook("OnDisconnect", func(conn *pgx.Conn) {
		disconnectCalled = true
	})
	if err != nil {
		t.Errorf("AddConnectionHook should not return error: %v", err)
	}

	err = hooks.AddConnectionHook("OnAcquire", func(ctx context.Context, conn *pgx.Conn) error {
		acquireCalled = true
		return nil
	})
	if err != nil {
		t.Errorf("AddConnectionHook should not return error: %v", err)
	}

	err = hooks.AddConnectionHook("OnRelease", func(conn *pgx.Conn) {
		releaseCalled = true
	})
	if err != nil {
		t.Errorf("AddConnectionHook should not return error: %v", err)
	}

	// Create a mock pool config
	config := &pgxpool.Config{}

	// Configure the pool with hooks
	hooks.ConfigurePool(config)

	// Verify that the config now has the hook callbacks
	if config.AfterConnect == nil {
		t.Error("Expected AfterConnect to be set")
	}

	if config.BeforeClose == nil {
		t.Error("Expected BeforeClose to be set")
	}

	// Test that the hooks are actually called
	ctx := context.Background()
	err = config.AfterConnect(ctx, nil)
	if err != nil {
		t.Errorf("AfterConnect should not return error: %v", err)
	}

	if !connectCalled {
		t.Error("OnConnect hook should have been called")
	}

	if !acquireCalled {
		t.Error("OnAcquire hook should have been called")
	}

	config.BeforeClose(nil)

	if !disconnectCalled {
		t.Error("OnDisconnect hook should have been called")
	}

	if !releaseCalled {
		t.Error("OnRelease hook should have been called")
	}
}

func TestHooksConfigurePoolWithExistingCallbacks(t *testing.T) {
	hooks := NewHooks()

	// Add a connection hook
	hookCalled := false
	err := hooks.AddConnectionHook("OnConnect", func(conn *pgx.Conn) error {
		hookCalled = true
		return nil
	})
	if err != nil {
		t.Errorf("AddConnectionHook should not return error: %v", err)
	}

	// Create a mock pool config with existing callbacks
	originalConnectCalled := false
	originalCloseCalled := false
	config := &pgxpool.Config{
		AfterConnect: func(ctx context.Context, conn *pgx.Conn) error {
			originalConnectCalled = true
			return nil
		},
		BeforeClose: func(conn *pgx.Conn) {
			originalCloseCalled = true
		},
	}

	// Configure the pool with hooks
	hooks.ConfigurePool(config)

	// Test that both original and hook callbacks are called
	ctx := context.Background()
	err = config.AfterConnect(ctx, nil)
	if err != nil {
		t.Errorf("AfterConnect should not return error: %v", err)
	}

	if !originalConnectCalled {
		t.Error("Original AfterConnect callback should have been called")
	}

	if !hookCalled {
		t.Error("Hook callback should have been called")
	}

	config.BeforeClose(nil)

	if !originalCloseCalled {
		t.Error("Original BeforeClose callback should have been called")
	}
}

func TestDBShutdown(t *testing.T) {
	db := NewDB()

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
	db := NewDB()

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
