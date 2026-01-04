package pgxkit

import (
	"context"
	"testing"
	"time"
)

// BenchmarkHookOverhead measures the performance impact of hooks
func BenchmarkHookOverhead(b *testing.B) {
	ctx := context.Background()
	pool := getTestPool()
	if pool == nil {
		b.Skip("TEST_DATABASE_URL not set, skipping benchmark")
	}

	b.Run("NoHooks", func(b *testing.B) {
		db := &DB{
			readPool:  pool,
			writePool: pool,
			hooks:     newHooks(),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rows, err := db.Query(ctx, "SELECT 1")
			if err != nil {
				b.Fatal(err)
			}
			rows.Close()
		}
	})

	b.Run("WithBeforeHook", func(b *testing.B) {
		db := &DB{
			readPool:  pool,
			writePool: pool,
			hooks:     newHooks(),
		}
		db.hooks.addHook(BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
			return nil
		})

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rows, err := db.Query(ctx, "SELECT 1")
			if err != nil {
				b.Fatal(err)
			}
			rows.Close()
		}
	})

	b.Run("WithBeforeAndAfterHooks", func(b *testing.B) {
		db := &DB{
			readPool:  pool,
			writePool: pool,
			hooks:     newHooks(),
		}
		db.hooks.addHook(BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
			return nil
		})
		db.hooks.addHook(AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
			return nil
		})

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rows, err := db.Query(ctx, "SELECT 1")
			if err != nil {
				b.Fatal(err)
			}
			rows.Close()
		}
	})

	b.Run("WithMultipleHooks", func(b *testing.B) {
		db := &DB{
			readPool:  pool,
			writePool: pool,
			hooks:     newHooks(),
		}
		for i := 0; i < 5; i++ {
			db.hooks.addHook(BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
				return nil
			})
			db.hooks.addHook(AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
				return nil
			})
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rows, err := db.Query(ctx, "SELECT 1")
			if err != nil {
				b.Fatal(err)
			}
			rows.Close()
		}
	})
}

// BenchmarkPoolOperations measures basic pool operation performance
func BenchmarkPoolOperations(b *testing.B) {
	ctx := context.Background()
	pool := getTestPool()
	if pool == nil {
		b.Skip("TEST_DATABASE_URL not set, skipping benchmark")
	}

	b.Run("Query", func(b *testing.B) {
		db := &DB{
			readPool:  pool,
			writePool: pool,
			hooks:     newHooks(),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rows, err := db.Query(ctx, "SELECT 1")
			if err != nil {
				b.Fatal(err)
			}
			rows.Close()
		}
	})

	b.Run("QueryRow", func(b *testing.B) {
		db := &DB{
			readPool:  pool,
			writePool: pool,
			hooks:     newHooks(),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var result int
			err := db.QueryRow(ctx, "SELECT 1").Scan(&result)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Exec", func(b *testing.B) {
		db := &DB{
			readPool:  pool,
			writePool: pool,
			hooks:     newHooks(),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.Exec(ctx, "SELECT 1")
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ReadQuery", func(b *testing.B) {
		readPool := getTestPool()
		writePool := getTestPool()
		if readPool == nil || writePool == nil {
			b.Skip("TEST_DATABASE_URL not set, skipping benchmark")
		}

		db := &DB{
			readPool:  readPool,
			writePool: writePool,
			hooks:     newHooks(),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rows, err := db.ReadQuery(ctx, "SELECT 1")
			if err != nil {
				b.Fatal(err)
			}
			rows.Close()
		}
	})
}

// BenchmarkConcurrentOperations tests concurrent performance
func BenchmarkConcurrentOperations(b *testing.B) {
	ctx := context.Background()
	pool := getTestPool()
	if pool == nil {
		b.Skip("TEST_DATABASE_URL not set, skipping benchmark")
	}

	db := &DB{
		readPool:  pool,
		writePool: pool,
		hooks:     newHooks(),
	}

	b.Run("Concurrent", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rows, err := db.Query(ctx, "SELECT 1")
				if err != nil {
					b.Fatal(err)
				}
				rows.Close()
			}
		})
	})

	b.Run("ConcurrentWithHooks", func(b *testing.B) {
		dbWithHooks := &DB{
			readPool:  pool,
			writePool: pool,
			hooks:     newHooks(),
		}
		dbWithHooks.hooks.addHook(BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
			return nil
		})
		dbWithHooks.hooks.addHook(AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
			return nil
		})

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rows, err := dbWithHooks.Query(ctx, "SELECT 1")
				if err != nil {
					b.Fatal(err)
				}
				rows.Close()
			}
		})
	})
}

// BenchmarkHookExecutionTime measures hook execution time specifically
func BenchmarkHookExecutionTime(b *testing.B) {
	ctx := context.Background()

	b.Run("EmptyHook", func(b *testing.B) {
		hookFunc := func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
			return nil
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = hookFunc(ctx, "SELECT 1", nil, nil)
		}
	})

	b.Run("LoggingHook", func(b *testing.B) {
		hookFunc := func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
			_ = sql
			_ = args
			return nil
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = hookFunc(ctx, "SELECT 1", []interface{}{1, "test"}, nil)
		}
	})

	b.Run("TimingHook", func(b *testing.B) {
		hookFunc := func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
			_ = time.Now()
			return nil
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = hookFunc(ctx, "SELECT 1", nil, nil)
		}
	})
}
