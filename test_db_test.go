package pgxkit

import (
	"context"
	"os"
	"testing"
)

func TestNewTestDB(t *testing.T) {
	testDB := NewTestDB()

	if testDB == nil {
		t.Error("NewTestDB should not return nil")
		return
	}

	if testDB.DB == nil {
		t.Error("TestDB should wrap a valid DB instance")
	}

	// New pattern: TestDB should be unconnected initially
	if testDB.writePool != nil {
		t.Error("NewTestDB should return unconnected instance")
	}
}

func TestSetup(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return // Test was skipped
	}

	// Test Setup method
	err := testDB.Setup()
	if err != nil {
		t.Errorf("Setup should not return error: %v", err)
	}
}

func TestClean(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return // Test was skipped
	}

	// Test Clean method
	err := testDB.Clean()
	if err != nil {
		t.Errorf("Clean should not return error: %v", err)
	}
}

func TestEnableAssertPlan(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return // Test was skipped
	}

	// Test EnableAssertPlan method - no longer takes testing.T
	planDB := testDB.EnableAssertPlan("TestExample")

	if planDB == nil {
		t.Error("EnableAssertPlan should return a DB instance")
	}

	if planDB == testDB.DB {
		t.Error("EnableAssertPlan should return a new DB instance, not the same one")
	}
}

func TestAssertPlan(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return // Test was skipped
	}

	planDB := testDB.EnableAssertPlan("TestAssertPlan")

	// Execute a query to capture plan
	ctx := context.Background()
	rows, err := planDB.Query(ctx, "SELECT 1 as test_column")
	if err != nil {
		t.Fatalf("Query should not fail: %v", err)
	}
	defer rows.Close()

	// Process results
	var result int
	if rows.Next() {
		err = rows.Scan(&result)
		if err != nil {
			t.Fatalf("Scan should not fail: %v", err)
		}
	}

	if result != 1 {
		t.Errorf("Expected result 1, got %d", result)
	}

	// Test AssertPlan - this should create baseline on first run
	planDB.AssertPlan(t, "TestAssertPlan")

	// Clean up
	defer cleanupPlan("TestAssertPlan")
}

func TestAssertPlan_RerunIsStable(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}
	const name = "TestAssertPlan_RerunIsStable"
	defer cleanupPlan(name)
	_ = cleanupPlan(name)

	ctx := context.Background()

	p1 := testDB.EnableAssertPlan(name)
	rows, err := p1.Query(ctx, "SELECT 1")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	rows.Close()
	p1.AssertPlan(t, name)
	if t.Failed() {
		t.Fatalf("baseline run should pass")
	}

	p2 := testDB.EnableAssertPlan(name)
	rows, err = p2.Query(ctx, "SELECT 1")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	rows.Close()
	p2.AssertPlan(t, name)
}

func TestAssertPlan_OverwriteRegeneratesBaseline(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}
	const name = "TestAssertPlan_OverwriteRegeneratesBaseline"
	defer cleanupPlan(name)
	_ = cleanupPlan(name)

	ctx := context.Background()

	p1 := testDB.EnableAssertPlan(name)
	rows, err := p1.Query(ctx, "SELECT 1")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	rows.Close()
	p1.AssertPlan(t, name)
	if t.Failed() {
		t.Fatalf("baseline run should pass")
	}
	first, err := os.ReadFile(planPath(name))
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}

	old := *overwritePlan
	*overwritePlan = true
	defer func() { *overwritePlan = old }()

	p2 := testDB.EnableAssertPlan(name)
	rows, err = p2.Query(ctx, "SELECT 1 AS different_alias")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	rows.Close()
	p2.AssertPlan(t, name)
	if t.Failed() {
		t.Fatalf("overwrite run should pass")
	}
	second, err := os.ReadFile(planPath(name))
	if err != nil {
		t.Fatalf("read regenerated baseline: %v", err)
	}
	if string(first) == string(second) {
		t.Errorf("expected baseline to be regenerated under -overwrite-plan")
	}
}

func TestRequireDB(t *testing.T) {
	// This test depends on TEST_DATABASE_URL being set
	originalURL := os.Getenv("TEST_DATABASE_URL")

	// Test without TEST_DATABASE_URL (should skip)
	os.Unsetenv("TEST_DATABASE_URL")
	defer func() {
		if originalURL != "" {
			os.Setenv("TEST_DATABASE_URL", originalURL)
		}
	}()

	// Test the skip behavior in a subtest
	t.Run("should_skip_without_test_db", func(t *testing.T) {
		// This subtest will be skipped by RequireDB
		testDB := RequireDB(t)

		// This code should never execute because the test should be skipped
		if testDB != nil {
			t.Error("Test should have been skipped, but got a TestDB instance")
		}
	})
}

func TestPlanDML(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}

	ctx := context.Background()

	// Use a regular table (not TEMP) so it's visible across pool connections —
	// EnableAssertPlan returns a *DB sharing the pool, and DML may execute on a
	// different connection than the CREATE.
	_, err := testDB.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS plan_test_users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DROP TABLE IF EXISTS plan_test_users")
	})

	t.Run("insert", func(t *testing.T) {
		planDB := testDB.EnableAssertPlan("TestPlanDML_Insert")
		defer cleanupPlan("TestPlanDML_Insert")

		var id int
		err := planDB.QueryRow(ctx, `
			INSERT INTO plan_test_users (name, email)
			VALUES ($1, $2)
			RETURNING id
		`, "Test User", "test@example.com").Scan(&id)
		if err != nil {
			t.Fatalf("INSERT should not fail: %v", err)
		}

		if id == 0 {
			t.Error("Expected non-zero id from INSERT RETURNING")
		}

		planDB.AssertPlan(t, "TestPlanDML_Insert")
	})

	t.Run("update", func(t *testing.T) {
		planDB := testDB.EnableAssertPlan("TestPlanDML_Update")
		defer cleanupPlan("TestPlanDML_Update")

		_, err := planDB.Exec(ctx, `
			UPDATE plan_test_users
			SET email = $1
			WHERE name = $2
		`, "updated@example.com", "Test User")
		if err != nil {
			t.Fatalf("UPDATE should not fail: %v", err)
		}

		planDB.AssertPlan(t, "TestPlanDML_Update")
	})

	t.Run("delete", func(t *testing.T) {
		planDB := testDB.EnableAssertPlan("TestPlanDML_Delete")
		defer cleanupPlan("TestPlanDML_Delete")

		_, err := planDB.Exec(ctx, `
			DELETE FROM plan_test_users
			WHERE name = $1
		`, "Test User")
		if err != nil {
			t.Fatalf("DELETE should not fail: %v", err)
		}

		planDB.AssertPlan(t, "TestPlanDML_Delete")
	})
}

func TestPlanCTE(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}

	ctx := context.Background()

	// Regular table — see TestPlanDML for why TEMP doesn't work here.
	_, err := testDB.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS plan_cte_test (
			id SERIAL PRIMARY KEY,
			value INT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DROP TABLE IF EXISTS plan_cte_test")
	})

	t.Run("cte_select", func(t *testing.T) {
		planDB := testDB.EnableAssertPlan("TestPlanCTE_Select")
		defer cleanupPlan("TestPlanCTE_Select")

		rows, err := planDB.Query(ctx, `
			WITH numbered AS (
				SELECT id, value, ROW_NUMBER() OVER () as rn
				FROM plan_cte_test
			)
			SELECT * FROM numbered WHERE rn <= 10
		`)
		if err != nil {
			t.Fatalf("CTE SELECT should not fail: %v", err)
		}
		rows.Close()

		planDB.AssertPlan(t, "TestPlanCTE_Select")
	})

	t.Run("cte_insert", func(t *testing.T) {
		planDB := testDB.EnableAssertPlan("TestPlanCTE_Insert")
		defer cleanupPlan("TestPlanCTE_Insert")

		_, err := planDB.Exec(ctx, `
			WITH vals AS (
				SELECT generate_series(1, 5) as v
			)
			INSERT INTO plan_cte_test (value)
			SELECT v FROM vals
		`)
		if err != nil {
			t.Fatalf("CTE INSERT should not fail: %v", err)
		}

		planDB.AssertPlan(t, "TestPlanCTE_Insert")
	})
}

// Integration test that requires actual database connection
func TestTestDBIntegration(t *testing.T) {
	// Skip if no test database available
	testDB := RequireDB(t)
	if testDB == nil {
		return // Test was skipped
	}

	// Test Setup
	err := testDB.Setup()
	if err != nil {
		t.Fatalf("Setup should not fail: %v", err)
	}

	// Test EnableAssertPlan
	planDB := testDB.EnableAssertPlan("TestIntegration")

	// Execute a simple query that should capture EXPLAIN plan
	ctx := context.Background()
	rows, err := planDB.Query(ctx, "SELECT 1 as test_column")
	if err != nil {
		t.Fatalf("Query should not fail: %v", err)
	}
	defer rows.Close()

	// Process results
	var result int
	if rows.Next() {
		err = rows.Scan(&result)
		if err != nil {
			t.Fatalf("Scan should not fail: %v", err)
		}
	}

	if result != 1 {
		t.Errorf("Expected result 1, got %d", result)
	}

	// Test Clean
	err = testDB.Clean()
	if err != nil {
		t.Fatalf("Clean should not fail: %v", err)
	}

	// Test AssertPlan
	planDB.AssertPlan(t, "TestIntegration")

	// Clean up plan files
	defer cleanupPlan("TestIntegration")
}
