package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCreatesDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Verify file was created
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("db file not created: %v", err)
	}

	// Verify WAL mode is enabled
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want %q", mode, "wal")
	}

	// Verify tables exist by inserting and reading
	_, err = db.Exec(`INSERT INTO projects (id, path, name, source) VALUES ('p1', '/tmp/test', 'test', 'claude')`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	var name string
	if err := db.QueryRow("SELECT name FROM projects WHERE id = 'p1'").Scan(&name); err != nil {
		t.Fatalf("select project: %v", err)
	}
	if name != "test" {
		t.Fatalf("name = %q, want %q", name, "test")
	}
}

func TestOpenCreatesDirIfNeeded(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "test.db")

	db, err := Open(nested)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db.Close()
}

func TestOpenReadOnlyNonExistent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.db")

	_, err := OpenReadOnly(path)
	if err == nil {
		t.Fatal("expected error for non-existent DB")
	}
}

func TestMigrateRecordsVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow("SELECT max(version) FROM migrations").Scan(&version); err != nil {
		t.Fatalf("query migration version: %v", err)
	}
	if version != 1 {
		t.Fatalf("version = %d, want 1", version)
	}
}

func TestOpenIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	db1.Close()

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	db2.Close()
}

func TestOpenReadOnlyExistingDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, err = db.Exec(`INSERT INTO projects (id, path, name, source) VALUES ('p1', '/tmp', 'test', 'claude')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	db.Close()

	roDB, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer roDB.Close()

	var name string
	if err := roDB.QueryRow("SELECT name FROM projects WHERE id = 'p1'").Scan(&name); err != nil {
		t.Fatalf("select: %v", err)
	}
	if name != "test" {
		t.Fatalf("name = %q, want %q", name, "test")
	}
}

func TestDefaultPath(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("dbs", "index.db")) {
		t.Fatalf("unexpected path: %s", path)
	}
}
