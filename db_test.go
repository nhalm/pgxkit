package pgxkit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewDB(t *testing.T) {
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
	db := NewReadWriteDB(nil, nil)
	if db == nil {
		t.Error("NewReadWriteDB should not return nil")
		return
	}

	if db.hooks == nil {
		t.Error("NewReadWriteDB should initialize hooks")
	}

	if db.shutdown {
		t.Error("NewReadWriteDB should not be in shutdown state")
	}
}

func TestConnectOptions(t *testing.T) {
	cfg := newConnectConfig()

	WithMaxConns(25)(cfg)
	if cfg.maxConns != 25 {
		t.Errorf("WithMaxConns: expected 25, got %d", cfg.maxConns)
	}

	WithMinConns(5)(cfg)
	if cfg.minConns != 5 {
		t.Errorf("WithMinConns: expected 5, got %d", cfg.minConns)
	}

	WithMaxConnLifetime(time.Hour)(cfg)
	if cfg.maxConnLifetime != time.Hour {
		t.Errorf("WithMaxConnLifetime: expected %v, got %v", time.Hour, cfg.maxConnLifetime)
	}

	WithMaxConnIdleTime(10 * time.Minute)(cfg)
	if cfg.maxConnIdleTime != 10*time.Minute {
		t.Errorf("WithMaxConnIdleTime: expected %v, got %v", 10*time.Minute, cfg.maxConnIdleTime)
	}
}

func TestConnectOptionsValidation(t *testing.T) {
	tests := []struct {
		name     string
		apply    func(*connectConfig)
		validate func(*connectConfig) bool
		desc     string
	}{
		{
			name: "WithMaxConns valid",
			apply: func(c *connectConfig) {
				WithMaxConns(10)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.maxConns == 10
			},
			desc: "valid maxConns should be set",
		},
		{
			name: "WithMaxConns zero ignored",
			apply: func(c *connectConfig) {
				WithMaxConns(10)(c)
				WithMaxConns(0)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.maxConns == 10
			},
			desc: "zero maxConns should be ignored",
		},
		{
			name: "WithMaxConns negative ignored",
			apply: func(c *connectConfig) {
				WithMaxConns(10)(c)
				WithMaxConns(-5)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.maxConns == 10
			},
			desc: "negative maxConns should be ignored",
		},
		{
			name: "WithMinConns valid",
			apply: func(c *connectConfig) {
				WithMinConns(5)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.minConns == 5
			},
			desc: "valid minConns should be set",
		},
		{
			name: "WithMinConns zero valid",
			apply: func(c *connectConfig) {
				WithMinConns(5)(c)
				WithMinConns(0)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.minConns == 0
			},
			desc: "zero minConns should be valid",
		},
		{
			name: "WithMinConns negative ignored",
			apply: func(c *connectConfig) {
				WithMinConns(5)(c)
				WithMinConns(-1)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.minConns == 5
			},
			desc: "negative minConns should be ignored",
		},
		{
			name: "WithMaxConnLifetime valid",
			apply: func(c *connectConfig) {
				WithMaxConnLifetime(time.Hour)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.maxConnLifetime == time.Hour
			},
			desc: "valid maxConnLifetime should be set",
		},
		{
			name: "WithMaxConnLifetime zero ignored",
			apply: func(c *connectConfig) {
				WithMaxConnLifetime(time.Hour)(c)
				WithMaxConnLifetime(0)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.maxConnLifetime == time.Hour
			},
			desc: "zero maxConnLifetime should be ignored",
		},
		{
			name: "WithMaxConnLifetime negative ignored",
			apply: func(c *connectConfig) {
				WithMaxConnLifetime(time.Hour)(c)
				WithMaxConnLifetime(-time.Second)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.maxConnLifetime == time.Hour
			},
			desc: "negative maxConnLifetime should be ignored",
		},
		{
			name: "WithMaxConnIdleTime valid",
			apply: func(c *connectConfig) {
				WithMaxConnIdleTime(30 * time.Minute)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.maxConnIdleTime == 30*time.Minute
			},
			desc: "valid maxConnIdleTime should be set",
		},
		{
			name: "WithMaxConnIdleTime zero ignored",
			apply: func(c *connectConfig) {
				WithMaxConnIdleTime(30 * time.Minute)(c)
				WithMaxConnIdleTime(0)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.maxConnIdleTime == 30*time.Minute
			},
			desc: "zero maxConnIdleTime should be ignored",
		},
		{
			name: "WithMaxConnIdleTime negative ignored",
			apply: func(c *connectConfig) {
				WithMaxConnIdleTime(30 * time.Minute)(c)
				WithMaxConnIdleTime(-time.Minute)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.maxConnIdleTime == 30*time.Minute
			},
			desc: "negative maxConnIdleTime should be ignored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConnectConfig()
			tt.apply(cfg)
			if !tt.validate(cfg) {
				t.Errorf("%s", tt.desc)
			}
		})
	}
}

func TestReadWritePoolOptions(t *testing.T) {
	cfg := newConnectConfig()

	WithReadMaxConns(50)(cfg)
	if cfg.readMaxConns != 50 {
		t.Errorf("WithReadMaxConns: expected 50, got %d", cfg.readMaxConns)
	}

	WithReadMinConns(10)(cfg)
	if cfg.readMinConns != 10 {
		t.Errorf("WithReadMinConns: expected 10, got %d", cfg.readMinConns)
	}

	WithWriteMaxConns(25)(cfg)
	if cfg.writeMaxConns != 25 {
		t.Errorf("WithWriteMaxConns: expected 25, got %d", cfg.writeMaxConns)
	}

	WithWriteMinConns(5)(cfg)
	if cfg.writeMinConns != 5 {
		t.Errorf("WithWriteMinConns: expected 5, got %d", cfg.writeMinConns)
	}
}

func TestReadWritePoolOptionsValidation(t *testing.T) {
	tests := []struct {
		name     string
		apply    func(*connectConfig)
		validate func(*connectConfig) bool
		desc     string
	}{
		{
			name: "WithReadMaxConns valid",
			apply: func(c *connectConfig) {
				WithReadMaxConns(50)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.readMaxConns == 50
			},
			desc: "valid readMaxConns should be set",
		},
		{
			name: "WithReadMaxConns zero ignored",
			apply: func(c *connectConfig) {
				WithReadMaxConns(50)(c)
				WithReadMaxConns(0)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.readMaxConns == 50
			},
			desc: "zero readMaxConns should be ignored",
		},
		{
			name: "WithReadMaxConns negative ignored",
			apply: func(c *connectConfig) {
				WithReadMaxConns(50)(c)
				WithReadMaxConns(-10)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.readMaxConns == 50
			},
			desc: "negative readMaxConns should be ignored",
		},
		{
			name: "WithReadMinConns valid",
			apply: func(c *connectConfig) {
				WithReadMinConns(10)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.readMinConns == 10
			},
			desc: "valid readMinConns should be set",
		},
		{
			name: "WithReadMinConns zero valid",
			apply: func(c *connectConfig) {
				WithReadMinConns(10)(c)
				WithReadMinConns(0)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.readMinConns == 0
			},
			desc: "zero readMinConns should be valid",
		},
		{
			name: "WithReadMinConns negative ignored",
			apply: func(c *connectConfig) {
				WithReadMinConns(10)(c)
				WithReadMinConns(-1)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.readMinConns == 10
			},
			desc: "negative readMinConns should be ignored",
		},
		{
			name: "WithWriteMaxConns valid",
			apply: func(c *connectConfig) {
				WithWriteMaxConns(25)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.writeMaxConns == 25
			},
			desc: "valid writeMaxConns should be set",
		},
		{
			name: "WithWriteMaxConns zero ignored",
			apply: func(c *connectConfig) {
				WithWriteMaxConns(25)(c)
				WithWriteMaxConns(0)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.writeMaxConns == 25
			},
			desc: "zero writeMaxConns should be ignored",
		},
		{
			name: "WithWriteMaxConns negative ignored",
			apply: func(c *connectConfig) {
				WithWriteMaxConns(25)(c)
				WithWriteMaxConns(-5)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.writeMaxConns == 25
			},
			desc: "negative writeMaxConns should be ignored",
		},
		{
			name: "WithWriteMinConns valid",
			apply: func(c *connectConfig) {
				WithWriteMinConns(5)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.writeMinConns == 5
			},
			desc: "valid writeMinConns should be set",
		},
		{
			name: "WithWriteMinConns zero valid",
			apply: func(c *connectConfig) {
				WithWriteMinConns(5)(c)
				WithWriteMinConns(0)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.writeMinConns == 0
			},
			desc: "zero writeMinConns should be valid",
		},
		{
			name: "WithWriteMinConns negative ignored",
			apply: func(c *connectConfig) {
				WithWriteMinConns(5)(c)
				WithWriteMinConns(-1)(c)
			},
			validate: func(c *connectConfig) bool {
				return c.writeMinConns == 5
			},
			desc: "negative writeMinConns should be ignored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConnectConfig()
			tt.apply(cfg)
			if !tt.validate(cfg) {
				t.Errorf("%s", tt.desc)
			}
		})
	}
}

func TestReadWritePoolOptionsFallback(t *testing.T) {
	t.Run("read specific overrides general", func(t *testing.T) {
		cfg := newConnectConfig()
		WithMaxConns(10)(cfg)
		WithReadMaxConns(50)(cfg)

		readMaxConns := cfg.maxConns
		if cfg.readMaxConns > 0 {
			readMaxConns = cfg.readMaxConns
		}

		if readMaxConns != 50 {
			t.Errorf("expected readMaxConns to be 50 (specific), got %d", readMaxConns)
		}
	})

	t.Run("write specific overrides general", func(t *testing.T) {
		cfg := newConnectConfig()
		WithMaxConns(10)(cfg)
		WithWriteMaxConns(25)(cfg)

		writeMaxConns := cfg.maxConns
		if cfg.writeMaxConns > 0 {
			writeMaxConns = cfg.writeMaxConns
		}

		if writeMaxConns != 25 {
			t.Errorf("expected writeMaxConns to be 25 (specific), got %d", writeMaxConns)
		}
	})

	t.Run("general applies when specific not set", func(t *testing.T) {
		cfg := newConnectConfig()
		WithMaxConns(10)(cfg)

		readMaxConns := cfg.maxConns
		if cfg.readMaxConns > 0 {
			readMaxConns = cfg.readMaxConns
		}

		writeMaxConns := cfg.maxConns
		if cfg.writeMaxConns > 0 {
			writeMaxConns = cfg.writeMaxConns
		}

		if readMaxConns != 10 {
			t.Errorf("expected readMaxConns to fall back to general (10), got %d", readMaxConns)
		}
		if writeMaxConns != 10 {
			t.Errorf("expected writeMaxConns to fall back to general (10), got %d", writeMaxConns)
		}
	})

	t.Run("different values for read and write", func(t *testing.T) {
		cfg := newConnectConfig()
		WithMaxConns(10)(cfg)
		WithReadMaxConns(100)(cfg)
		WithWriteMaxConns(20)(cfg)
		WithReadMinConns(5)(cfg)
		WithWriteMinConns(2)(cfg)

		readMaxConns := cfg.maxConns
		if cfg.readMaxConns > 0 {
			readMaxConns = cfg.readMaxConns
		}

		writeMaxConns := cfg.maxConns
		if cfg.writeMaxConns > 0 {
			writeMaxConns = cfg.writeMaxConns
		}

		readMinConns := cfg.minConns
		if cfg.readMinConns > 0 {
			readMinConns = cfg.readMinConns
		}

		writeMinConns := cfg.minConns
		if cfg.writeMinConns > 0 {
			writeMinConns = cfg.writeMinConns
		}

		if readMaxConns != 100 {
			t.Errorf("expected readMaxConns 100, got %d", readMaxConns)
		}
		if writeMaxConns != 20 {
			t.Errorf("expected writeMaxConns 20, got %d", writeMaxConns)
		}
		if readMinConns != 5 {
			t.Errorf("expected readMinConns 5, got %d", readMinConns)
		}
		if writeMinConns != 2 {
			t.Errorf("expected writeMinConns 2, got %d", writeMinConns)
		}
	})
}

func TestOperationHookOptions(t *testing.T) {
	cfg := newConnectConfig()

	beforeOpCalled := false
	afterOpCalled := false
	beforeTxCalled := false
	afterTxCalled := false
	shutdownCalled := false

	WithBeforeOperation(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		beforeOpCalled = true
		return nil
	})(cfg)

	WithAfterOperation(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		afterOpCalled = true
		return nil
	})(cfg)

	WithBeforeTransaction(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		beforeTxCalled = true
		return nil
	})(cfg)

	WithAfterTransaction(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		afterTxCalled = true
		return nil
	})(cfg)

	WithOnShutdown(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
		shutdownCalled = true
		return nil
	})(cfg)

	ctx := context.Background()
	cfg.hooks.executeBeforeOperation(ctx, "", nil, nil)
	cfg.hooks.executeAfterOperation(ctx, "", nil, nil)
	cfg.hooks.executeBeforeTransaction(ctx, "", nil, nil)
	cfg.hooks.executeAfterTransaction(ctx, "", nil, nil)
	cfg.hooks.executeOnShutdown(ctx, "", nil, nil)

	if !beforeOpCalled {
		t.Error("WithBeforeOperation hook should have been called")
	}
	if !afterOpCalled {
		t.Error("WithAfterOperation hook should have been called")
	}
	if !beforeTxCalled {
		t.Error("WithBeforeTransaction hook should have been called")
	}
	if !afterTxCalled {
		t.Error("WithAfterTransaction hook should have been called")
	}
	if !shutdownCalled {
		t.Error("WithOnShutdown hook should have been called")
	}
}

func TestConnectionHookOptions(t *testing.T) {
	cfg := newConnectConfig()

	connectCalled := false
	disconnectCalled := false
	acquireCalled := false
	releaseCalled := false

	WithOnConnect(func(conn *pgx.Conn) error {
		connectCalled = true
		return nil
	})(cfg)

	WithOnDisconnect(func(conn *pgx.Conn) {
		disconnectCalled = true
	})(cfg)

	WithOnAcquire(func(ctx context.Context, conn *pgx.Conn) error {
		acquireCalled = true
		return nil
	})(cfg)

	WithOnRelease(func(conn *pgx.Conn) {
		releaseCalled = true
	})(cfg)

	ctx := context.Background()
	cfg.hooks.connectionHooks.ExecuteOnConnect(nil)
	cfg.hooks.connectionHooks.ExecuteOnDisconnect(nil)
	cfg.hooks.connectionHooks.ExecuteOnAcquire(ctx, nil)
	cfg.hooks.connectionHooks.ExecuteOnRelease(nil)

	if !connectCalled {
		t.Error("WithOnConnect hook should have been called")
	}
	if !disconnectCalled {
		t.Error("WithOnDisconnect hook should have been called")
	}
	if !acquireCalled {
		t.Error("WithOnAcquire hook should have been called")
	}
	if !releaseCalled {
		t.Error("WithOnRelease hook should have been called")
	}
}

func TestHooksConfigurePool(t *testing.T) {
	cfg := newConnectConfig()

	connectCalled := false
	disconnectCalled := false
	acquireCalled := false
	releaseCalled := false

	WithOnConnect(func(conn *pgx.Conn) error {
		connectCalled = true
		return nil
	})(cfg)

	WithOnDisconnect(func(conn *pgx.Conn) {
		disconnectCalled = true
	})(cfg)

	WithOnAcquire(func(ctx context.Context, conn *pgx.Conn) error {
		acquireCalled = true
		return nil
	})(cfg)

	WithOnRelease(func(conn *pgx.Conn) {
		releaseCalled = true
	})(cfg)

	poolConfig := &pgxpool.Config{}
	cfg.hooks.configurePool(poolConfig)

	if poolConfig.AfterConnect == nil {
		t.Error("Expected AfterConnect to be set")
	}
	if poolConfig.BeforeClose == nil {
		t.Error("Expected BeforeClose to be set")
	}
	if poolConfig.PrepareConn == nil {
		t.Error("Expected PrepareConn to be set")
	}
	if poolConfig.AfterRelease == nil {
		t.Error("Expected AfterRelease to be set")
	}

	ctx := context.Background()

	// OnConnect via AfterConnect (called once on connection creation)
	err := poolConfig.AfterConnect(ctx, nil)
	if err != nil {
		t.Errorf("AfterConnect should not return error: %v", err)
	}
	if !connectCalled {
		t.Error("OnConnect hook should have been called")
	}

	// OnAcquire via PrepareConn (called every checkout)
	ok, err := poolConfig.PrepareConn(ctx, nil)
	if !ok || err != nil {
		t.Errorf("PrepareConn should return (true, nil), got (%v, %v)", ok, err)
	}
	if !acquireCalled {
		t.Error("OnAcquire hook should have been called")
	}

	// OnRelease via AfterRelease (called every return)
	ok = poolConfig.AfterRelease(nil)
	if !ok {
		t.Error("AfterRelease should return true")
	}
	if !releaseCalled {
		t.Error("OnRelease hook should have been called")
	}

	// OnDisconnect via BeforeClose (called once on connection destruction)
	poolConfig.BeforeClose(nil)
	if !disconnectCalled {
		t.Error("OnDisconnect hook should have been called")
	}
}

func TestHooksConfigurePoolWithExistingCallbacks(t *testing.T) {
	cfg := newConnectConfig()

	hookCalled := false
	WithOnConnect(func(conn *pgx.Conn) error {
		hookCalled = true
		return nil
	})(cfg)

	originalConnectCalled := false
	originalCloseCalled := false
	poolConfig := &pgxpool.Config{
		AfterConnect: func(ctx context.Context, conn *pgx.Conn) error {
			originalConnectCalled = true
			return nil
		},
		BeforeClose: func(conn *pgx.Conn) {
			originalCloseCalled = true
		},
	}

	cfg.hooks.configurePool(poolConfig)

	ctx := context.Background()
	err := poolConfig.AfterConnect(ctx, nil)
	if err != nil {
		t.Errorf("AfterConnect should not return error: %v", err)
	}

	if !originalConnectCalled {
		t.Error("Original AfterConnect callback should have been called")
	}

	if !hookCalled {
		t.Error("Hook callback should have been called")
	}

	poolConfig.BeforeClose(nil)

	if !originalCloseCalled {
		t.Error("Original BeforeClose callback should have been called")
	}
}

func TestDBShutdown(t *testing.T) {
	db := NewDB()

	err := db.Shutdown(context.Background())
	if err != nil {
		t.Errorf("Shutdown should not return error: %v", err)
	}

	if !db.shutdown {
		t.Error("DB should be marked as shutdown")
	}

	_, err = db.Query(context.Background(), "SELECT 1")
	if err == nil {
		t.Error("Query should fail after shutdown")
	}
}

func TestDBStats(t *testing.T) {
	db := NewDB()

	writeStats := db.Stats()
	readStats := db.ReadStats()
	writeStatsAgain := db.WriteStats()

	if writeStats != nil || readStats != nil || writeStatsAgain != nil {
		t.Error("Stats should be nil when pools are nil")
	}
}

func TestDBReadWriteStats(t *testing.T) {
	db := NewReadWriteDB(nil, nil)

	writeStats := db.Stats()
	readStats := db.ReadStats()
	writeStatsAgain := db.WriteStats()

	if writeStats != nil || readStats != nil || writeStatsAgain != nil {
		t.Error("Stats should be nil when pools are nil")
	}
}

// Race condition tests - run with `go test -race` to detect data races

func TestConcurrentHookExecution(t *testing.T) {
	hooks := newHooks()
	var counter atomic.Int64

	// Add multiple hooks
	for i := 0; i < 10; i++ {
		hooks.addHook(BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
			counter.Add(1)
			return nil
		})
	}

	// Execute hooks concurrently from multiple goroutines
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			_ = hooks.executeBeforeOperation(ctx, "SELECT 1", nil, nil)
		}()
	}
	wg.Wait()

	if counter.Load() == 0 {
		t.Error("Hooks should have been executed")
	}
}

func TestConcurrentOptionApplication(t *testing.T) {
	// Apply options concurrently to separate configs (no shared state)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cfg := newConnectConfig()
			WithMaxConns(int32(n))(cfg)
			WithMinConns(int32(n / 2))(cfg)
			WithBeforeOperation(func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
				return nil
			})(cfg)
		}(i)
	}
	wg.Wait()
}

func TestConcurrentConnectionHookAddAndExecute(t *testing.T) {
	ch := NewConnectionHooks()
	var counter atomic.Int64

	// Concurrently add hooks and execute them
	var wg sync.WaitGroup

	// Writers - add hooks
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch.AddOnConnect(func(conn *pgx.Conn) error {
				counter.Add(1)
				return nil
			})
		}()
	}

	// Readers - execute hooks
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ch.ExecuteOnConnect(nil)
		}()
	}

	wg.Wait()
}

func TestConcurrentDBMethodAccess(t *testing.T) {
	db := NewDB()

	// Concurrent reads of various DB state
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = db.Stats()
			_ = db.ReadStats()
			_ = db.IsReady(context.Background())
		}()
	}
	wg.Wait()
}
