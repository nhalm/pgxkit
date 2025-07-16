package pgxkit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewTestDB(t *testing.T) {
	pool := GetTestPool()
	if pool == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping test")
		return
	}

	testDB := NewTestDB(pool)

	if testDB == nil {
		t.Error("NewTestDB should not return nil")
		return
	}

	if testDB.DB == nil {
		t.Error("TestDB should wrap a valid DB instance")
	}
}

func TestSetup(t *testing.T) {
	pool := GetTestPool()
	if pool == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping test")
		return
	}

	testDB := NewTestDB(pool)

	// Test Setup method
	err := testDB.Setup()
	if err != nil {
		t.Errorf("Setup should not return error: %v", err)
	}
}

func TestClean(t *testing.T) {
	pool := GetTestPool()
	if pool == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping test")
		return
	}

	testDB := NewTestDB(pool)

	// Test Clean method
	err := testDB.Clean()
	if err != nil {
		t.Errorf("Clean should not return error: %v", err)
	}
}

func TestEnableGolden(t *testing.T) {
	pool := GetTestPool()
	if pool == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping test")
		return
	}

	testDB := NewTestDB(pool)

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
	pool := GetTestPool()
	if pool == nil {
		t.Skip("TEST_DATABASE_URL not set, skipping test")
		return
	}

	testDB := NewTestDB(pool)
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
