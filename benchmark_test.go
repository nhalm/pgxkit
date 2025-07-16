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
		db := NewDBWithPool(pool)

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
		db := NewDBWithPool(pool)
		db.AddHook(BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
			// Minimal hook that just returns
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
		db := NewDBWithPool(pool)
		db.AddHook(BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
			return nil
		})
		db.AddHook(AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
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
		db := NewDBWithPool(pool)
		// Add multiple hooks to test overhead scaling
		for i := 0; i < 5; i++ {
			db.AddHook(BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
				return nil
			})
			db.AddHook(AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
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
		db := NewDBWithPool(pool)

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
		db := NewDBWithPool(pool)

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
		db := NewDBWithPool(pool)

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

		db := NewReadWriteDB(readPool, writePool)

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

	db := NewDBWithPool(pool)

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
		dbWithHooks := NewDBWithPool(pool)
		dbWithHooks.AddHook(BeforeOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
			return nil
		})
		dbWithHooks.AddHook(AfterOperation, func(ctx context.Context, sql string, args []interface{}, operationErr error) error {
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

	// Test different hook complexities
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
			// Simulate logging overhead
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
			// Simulate timing overhead
			_ = time.Now()
			return nil
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = hookFunc(ctx, "SELECT 1", nil, nil)
		}
	})
}

// validatePerformanceRequirements checks if benchmarks meet the <1ms requirement
func validatePerformanceRequirements(b *testing.B, duration time.Duration) {
	// Convert to nanoseconds per operation
	nsPerOp := duration.Nanoseconds() / int64(b.N)

	// Check if under 1ms (1,000,000 ns)
	if nsPerOp > 1000000 {
		b.Errorf("Performance requirement not met: %d ns/op > 1ms", nsPerOp)
	}
}
