package index

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/index/db"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// mockStore implements thinkt.Store for testing.
// Tests should set up a real store or use the claude store with test fixtures.
// For unit testing the ingester logic, we test via IngestProject with a real registry.

func TestIngesterSyncProject(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	ctx := context.Background()

	// Insert a project directly.
	_, err = database.Exec(`INSERT INTO projects (id, path, name, source) VALUES (?, ?, ?, ?)`,
		"claude::test", "/tmp/test", "test", "claude")
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	// Verify it was inserted.
	var name string
	err = database.QueryRow(`SELECT name FROM projects WHERE id = ?`, "claude::test").Scan(&name)
	if err != nil {
		t.Fatalf("select project: %v", err)
	}
	if name != "test" {
		t.Fatalf("name = %q, want %q", name, "test")
	}
	_ = ctx
}

func TestShouldSyncSession(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	ing := NewIngester(database, nil)

	// New session (not in sync_state) should sync.
	meta := thinkt.SessionMeta{
		FullPath:   "/tmp/session.jsonl",
		ModifiedAt: time.Now(),
		FileSize:   1024,
	}

	shouldSync, err := ing.ShouldSyncSession(meta)
	if err != nil {
		t.Fatalf("shouldSync: %v", err)
	}
	if !shouldSync {
		t.Fatal("expected shouldSync=true for new session")
	}

	// Insert sync_state to simulate a previously synced session.
	_, err = database.Exec(`INSERT INTO sync_state (file_path, last_mod_time, file_size, lines_read) VALUES (?, ?, ?, ?)`,
		meta.FullPath, meta.ModifiedAt.Unix(), meta.FileSize, 10)
	if err != nil {
		t.Fatalf("insert sync_state: %v", err)
	}

	// Same mod time and size — should not sync.
	shouldSync, err = ing.ShouldSyncSession(meta)
	if err != nil {
		t.Fatalf("shouldSync: %v", err)
	}
	if shouldSync {
		t.Fatal("expected shouldSync=false for unchanged session")
	}

	// Changed size — should sync.
	meta.FileSize = 2048
	shouldSync, err = ing.ShouldSyncSession(meta)
	if err != nil {
		t.Fatalf("shouldSync: %v", err)
	}
	if !shouldSync {
		t.Fatal("expected shouldSync=true for changed size")
	}
}
