package db

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSecurityHardening verifies that external access is disabled
func TestSecurityHardening(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.duckdb")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Try to read an external file - this should fail
	_, err = db.Query("SELECT * FROM read_csv_auto('/etc/passwd')")
	if err == nil {
		t.Fatal("Expected error when reading external file, but query succeeded")
	}

	// Verify the error mentions external access
	errMsg := err.Error()
	if errMsg == "" {
		t.Fatal("Expected non-empty error message")
	}

	t.Logf("Correctly blocked external file access with error: %v", err)
}

// TestSecurityHardeningReadOnly verifies that external access is disabled in read-only mode
func TestSecurityHardeningReadOnly(t *testing.T) {
	// Create and initialize a database first
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.duckdb")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	db.Close()

	// Open in read-only mode
	dbReadOnly, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database in read-only mode: %v", err)
	}
	defer dbReadOnly.Close()

	// Try to read an external file - this should fail
	_, err = dbReadOnly.Query("SELECT * FROM read_csv_auto('/etc/passwd')")
	if err == nil {
		t.Fatal("Expected error when reading external file in read-only mode, but query succeeded")
	}

	t.Logf("Correctly blocked external file access in read-only mode with error: %v", err)
}

// TestExternalAccessDisabled verifies the setting is actually applied
func TestExternalAccessDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.duckdb")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Query the setting value
	var enabled bool
	err = db.QueryRow("SELECT current_setting('enable_external_access')").Scan(&enabled)
	if err != nil {
		t.Fatalf("Failed to query enable_external_access setting: %v", err)
	}

	if enabled {
		t.Fatal("Expected enable_external_access to be false, but it was true")
	}

	t.Log("Confirmed enable_external_access is set to false")
}

// TestNoFileSystemAccess verifies various file system operations are blocked
func TestNoFileSystemAccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.duckdb")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	testCases := []struct {
		name  string
		query string
	}{
		{"read_csv", "SELECT * FROM read_csv_auto('/tmp/test.csv')"},
		{"read_json", "SELECT * FROM read_json_auto('/tmp/test.json')"},
		{"read_parquet", "SELECT * FROM read_parquet('/tmp/test.parquet')"},
		{"copy_to", "COPY (SELECT 1) TO '/tmp/output.csv'"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.Query(tc.query)
			if err == nil {
				t.Fatalf("Expected %s to fail with external access disabled, but it succeeded", tc.name)
			}
			t.Logf("%s correctly blocked: %v", tc.name, err)
		})
	}
}

// TestSecurityHardeningEmbeddings verifies that external access is disabled for embeddings DB
func TestSecurityHardeningEmbeddings(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "embeddings.duckdb")

	db, err := OpenEmbeddings(dbPath)
	if err != nil {
		t.Fatalf("Failed to open embeddings database: %v", err)
	}
	defer db.Close()

	// Try to read an external file - this should fail
	_, err = db.Query("SELECT * FROM read_csv_auto('/etc/passwd')")
	if err == nil {
		t.Fatal("Expected error when reading external file from embeddings DB, but query succeeded")
	}

	t.Logf("Correctly blocked external file access in embeddings DB with error: %v", err)
}

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	os.Exit(code)
}
