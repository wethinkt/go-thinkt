package db

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCopyOnReadFallback(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.duckdb")

	// Open a read-write connection (simulates the watcher holding the lock)
	writer, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open write connection: %v", err)
	}
	defer writer.Close()

	// Insert some test data and force a checkpoint so it's in the main file
	_, err = writer.Exec("INSERT INTO projects (id, path, name, source) VALUES ('p1', '/test', 'Test', 'claude')")
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}
	if _, err := writer.Exec("CHECKPOINT"); err != nil {
		t.Fatalf("Failed to checkpoint: %v", err)
	}

	// Open read-only while writer is holding the lock — should succeed via copy fallback
	reader, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly should succeed via copy fallback, got: %v", err)
	}
	defer reader.Close()

	// Verify the copy contains the checkpointed data
	var name string
	err = reader.QueryRow("SELECT name FROM projects WHERE id = 'p1'").Scan(&name)
	if err != nil {
		t.Fatalf("Failed to query from copy: %v", err)
	}
	if name != "Test" {
		t.Fatalf("Expected name 'Test', got %q", name)
	}

	// Verify the reader was opened via copy (tempDir should be set)
	if reader.tempDir == "" {
		t.Fatal("Expected tempDir to be set for copy-on-read fallback")
	}

	// Verify security hardening on the copy
	var enabled bool
	err = reader.QueryRow("SELECT current_setting('enable_external_access')").Scan(&enabled)
	if err != nil {
		t.Fatalf("Failed to query security setting on copy: %v", err)
	}
	if enabled {
		t.Fatal("Expected enable_external_access to be false on copy")
	}
}

func TestCopyOnReadCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.duckdb")

	// Open a read-write connection
	writer, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open write connection: %v", err)
	}
	defer writer.Close()

	// Open read-only (will use copy fallback)
	reader, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly should succeed via copy fallback, got: %v", err)
	}

	tempDir := reader.tempDir
	if tempDir == "" {
		t.Fatal("Expected tempDir to be set")
	}

	// Close the reader — temp dir should be cleaned up
	reader.Close()

	// Verify temp dir was removed
	if _, err := filepath.Glob(filepath.Join(tempDir, "*")); err == nil {
		// Check if directory itself exists
		if exists(tempDir) {
			t.Fatalf("Expected temp dir %s to be removed after Close", tempDir)
		}
	}
}

func TestReadOnlyWithoutWriter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.duckdb")

	// Create and initialize a database, then close it
	writer, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open: %v", err)
	}
	_, err = writer.Exec("INSERT INTO projects (id, path, name, source) VALUES ('p1', '/test', 'Test', 'claude')")
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}
	writer.Close()

	// Open read-only without any writer — should succeed directly (no copy)
	reader, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly should succeed directly: %v", err)
	}
	defer reader.Close()

	// Verify no temp dir was created
	if reader.tempDir != "" {
		t.Fatal("Expected no tempDir when no lock conflict")
	}

	// Verify data is accessible
	var name string
	err = reader.QueryRow("SELECT name FROM projects WHERE id = 'p1'").Scan(&name)
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if name != "Test" {
		t.Fatalf("Expected 'Test', got %q", name)
	}
}

func TestIsLockError(t *testing.T) {
	tests := []struct {
		err    error
		expect bool
	}{
		{nil, false},
		{fmt.Errorf("some other error"), false},
		{fmt.Errorf("Conflicting lock is held in /path/to/db"), true},
		{fmt.Errorf("wrapped: Conflicting lock"), true},
	}
	for _, tc := range tests {
		got := isLockError(tc.err)
		if got != tc.expect {
			t.Errorf("isLockError(%v) = %v, want %v", tc.err, got, tc.expect)
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// TestCopyOnReadStress verifies that copying the main database file while a
// writer is actively inserting and checkpointing does not produce a corrupt
// copy. The writer goroutine inserts rows and checkpoints in a tight loop.
// Concurrently, multiple reader goroutines call OpenReadOnly (which triggers
// the copy fallback) and verify the copy opens without error and returns
// consistent data — i.e. the projects table exists and every row has a
// non-empty name.
func TestCopyOnReadStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "stress.duckdb")

	// Open writer and seed initial data + checkpoint so the main file has
	// at least the schema.
	writer, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open write connection: %v", err)
	}
	defer writer.Close()

	_, err = writer.Exec("INSERT INTO projects (id, path, name, source) VALUES ('seed', '/seed', 'Seed', 'claude')")
	if err != nil {
		t.Fatalf("Failed to seed data: %v", err)
	}
	if _, err := writer.Exec("CHECKPOINT"); err != nil {
		t.Fatalf("Failed to initial checkpoint: %v", err)
	}

	// Writer goroutine: insert rows and checkpoint in a tight loop.
	var stop atomic.Bool
	var writerWg sync.WaitGroup
	writerWg.Add(1)
	go func() {
		defer writerWg.Done()
		i := 0
		for !stop.Load() {
			id := fmt.Sprintf("p%d", i)
			_, err := writer.Exec(
				"INSERT INTO projects (id, path, name, source) VALUES (?, ?, ?, 'claude')",
				id, "/path/"+id, "Project "+id,
			)
			if err != nil {
				// INSERT errors are non-fatal for the stress test
				continue
			}
			writer.Exec("CHECKPOINT")
			i++
		}
	}()

	// Reader goroutines: repeatedly open read-only copies and verify integrity.
	const numReaders = 8
	const readsPerReader = 20
	var readerWg sync.WaitGroup
	var failures atomic.Int64

	for r := 0; r < numReaders; r++ {
		readerWg.Add(1)
		go func(readerID int) {
			defer readerWg.Done()
			for i := 0; i < readsPerReader; i++ {
				reader, err := OpenReadOnly(dbPath)
				if err != nil {
					t.Errorf("reader %d iter %d: OpenReadOnly failed: %v", readerID, i, err)
					failures.Add(1)
					continue
				}

				// Verify the copy is queryable: projects table must exist
				// and every row must have a non-empty name.
				rows, err := reader.Query("SELECT id, name FROM projects")
				if err != nil {
					t.Errorf("reader %d iter %d: query failed: %v", readerID, i, err)
					failures.Add(1)
					reader.Close()
					continue
				}

				rowCount := 0
				for rows.Next() {
					var id, name string
					if err := rows.Scan(&id, &name); err != nil {
						t.Errorf("reader %d iter %d: scan failed: %v", readerID, i, err)
						failures.Add(1)
						break
					}
					if name == "" {
						t.Errorf("reader %d iter %d: got empty name for id %s", readerID, i, id)
						failures.Add(1)
						break
					}
					rowCount++
				}
				rows.Close()

				if rowCount == 0 {
					t.Errorf("reader %d iter %d: got 0 rows (expected at least seed row)", readerID, i)
					failures.Add(1)
				}

				reader.Close()
			}
		}(r)
	}

	readerWg.Wait()
	stop.Store(true)
	writerWg.Wait()

	if f := failures.Load(); f > 0 {
		t.Fatalf("%d failures across %d total reads — copy-on-read is not safe", f, numReaders*readsPerReader)
	}
	t.Logf("All %d concurrent reads succeeded with no corruption", numReaders*readsPerReader)
}
