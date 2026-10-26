package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMigrateToVersion3CreatesAttachments(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if SchemaVersion < 3 {
		t.Fatalf("SchemaVersion = %d, want >= 3", SchemaVersion)
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'entry_attachments'`).Scan(&name); err != nil {
		t.Fatalf("entry_attachments table missing: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO entry_attachments (session_id, entry_index, kind, label, timestamp) VALUES ('s', 3, 'hook_success', 'PreToolUse: ok', ?)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListAttachments("s", "")
	if err != nil || len(rows) != 1 || rows[0].EntryIndex != 3 || rows[0].Kind != "hook_success" || rows[0].Label != "PreToolUse: ok" {
		t.Fatalf("ListAttachments: %+v err %v", rows, err)
	}
	if rows, _ := db.ListAttachments("s", "task_status"); len(rows) != 0 {
		t.Fatalf("kind filter: %+v", rows)
	}
}
