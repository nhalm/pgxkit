package pgxkit

import (
	"context"
	"os"
	"path/filepath"
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

func TestEnableGolden(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return // Test was skipped
	}

	// Test EnableGolden method - no longer takes testing.T
	goldenDB := testDB.EnableGolden("TestExample")

	if goldenDB == nil {
		t.Error("EnableGolden should return a DB instance")
	}

	if goldenDB == testDB.DB {
		t.Error("EnableGolden should return a new DB instance, not the same one")
	}
}

func TestAssertGolden(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return // Test was skipped
	}

	goldenDB := testDB.EnableGolden("TestAssertGolden")

	// Execute a query to capture plan
	ctx := context.Background()
	rows, err := goldenDB.Query(ctx, "SELECT 1 as test_column")
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

	// Test AssertGolden - this should create baseline on first run
	goldenDB.AssertGolden(t, "TestAssertGolden")

	// Clean up
	defer CleanupGolden("TestAssertGolden")
}

func TestCleanupGolden(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Create some test golden files using the new naming pattern
	goldenFiles := []string{
		filepath.Join(tempDir, "testdata", "golden", "TestCleanup.json"),
		filepath.Join(tempDir, "testdata", "golden", "TestCleanup.json.baseline"),
	}

	for _, file := range goldenFiles {
		err := os.MkdirAll(filepath.Dir(file), 0755)
		if err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		err = os.WriteFile(file, []byte(`[{"query": 1, "sql": "SELECT 1", "plan": []}]`), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Change to temp directory for cleanup
	originalDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalDir)

	// Test cleanup
	err := CleanupGolden("TestCleanup")
	if err != nil {
		t.Errorf("CleanupGolden should not return error: %v", err)
	}

	// Verify files were removed
	for _, file := range goldenFiles {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Errorf("Golden file should have been removed: %s", file)
		}
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

func TestGoldenDML(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}

	ctx := context.Background()

	// Create a temporary table for DML testing
	_, err := testDB.Exec(ctx, `
		CREATE TEMP TABLE golden_test_users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create temp table: %v", err)
	}

	t.Run("insert", func(t *testing.T) {
		goldenDB := testDB.EnableGolden("TestGoldenDML_Insert")
		defer CleanupGolden("TestGoldenDML_Insert")

		var id int
		err := goldenDB.QueryRow(ctx, `
			INSERT INTO golden_test_users (name, email)
			VALUES ($1, $2)
			RETURNING id
		`, "Test User", "test@example.com").Scan(&id)
		if err != nil {
			t.Fatalf("INSERT should not fail: %v", err)
		}

		if id == 0 {
			t.Error("Expected non-zero id from INSERT RETURNING")
		}

		goldenDB.AssertGolden(t, "TestGoldenDML_Insert")
	})

	t.Run("update", func(t *testing.T) {
		goldenDB := testDB.EnableGolden("TestGoldenDML_Update")
		defer CleanupGolden("TestGoldenDML_Update")

		_, err := goldenDB.Exec(ctx, `
			UPDATE golden_test_users
			SET email = $1
			WHERE name = $2
		`, "updated@example.com", "Test User")
		if err != nil {
			t.Fatalf("UPDATE should not fail: %v", err)
		}

		goldenDB.AssertGolden(t, "TestGoldenDML_Update")
	})

	t.Run("delete", func(t *testing.T) {
		goldenDB := testDB.EnableGolden("TestGoldenDML_Delete")
		defer CleanupGolden("TestGoldenDML_Delete")

		_, err := goldenDB.Exec(ctx, `
			DELETE FROM golden_test_users
			WHERE name = $1
		`, "Test User")
		if err != nil {
			t.Fatalf("DELETE should not fail: %v", err)
		}

		goldenDB.AssertGolden(t, "TestGoldenDML_Delete")
	})
}

func TestGoldenCTE(t *testing.T) {
	testDB := RequireDB(t)
	if testDB == nil {
		return
	}

	ctx := context.Background()

	// Create temp table
	_, err := testDB.Exec(ctx, `
		CREATE TEMP TABLE golden_cte_test (
			id SERIAL PRIMARY KEY,
			value INT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create temp table: %v", err)
	}

	t.Run("cte_select", func(t *testing.T) {
		goldenDB := testDB.EnableGolden("TestGoldenCTE_Select")
		defer CleanupGolden("TestGoldenCTE_Select")

		rows, err := goldenDB.Query(ctx, `
			WITH numbered AS (
				SELECT id, value, ROW_NUMBER() OVER () as rn
				FROM golden_cte_test
			)
			SELECT * FROM numbered WHERE rn <= 10
		`)
		if err != nil {
			t.Fatalf("CTE SELECT should not fail: %v", err)
		}
		rows.Close()

		goldenDB.AssertGolden(t, "TestGoldenCTE_Select")
	})

	t.Run("cte_insert", func(t *testing.T) {
		goldenDB := testDB.EnableGolden("TestGoldenCTE_Insert")
		defer CleanupGolden("TestGoldenCTE_Insert")

		_, err := goldenDB.Exec(ctx, `
			WITH vals AS (
				SELECT generate_series(1, 5) as v
			)
			INSERT INTO golden_cte_test (value)
			SELECT v FROM vals
		`)
		if err != nil {
			t.Fatalf("CTE INSERT should not fail: %v", err)
		}

		goldenDB.AssertGolden(t, "TestGoldenCTE_Insert")
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

	// Test EnableGolden
	goldenDB := testDB.EnableGolden("TestIntegration")

	// Execute a simple query that should capture EXPLAIN plan
	ctx := context.Background()
	rows, err := goldenDB.Query(ctx, "SELECT 1 as test_column")
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

	// Test AssertGolden
	goldenDB.AssertGolden(t, "TestIntegration")

	// Clean up golden files
	defer CleanupGolden("TestIntegration")
}
