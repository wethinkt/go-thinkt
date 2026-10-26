package index

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/index/db"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func openTestIngester(t *testing.T) (*Ingester, *db.DB) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewIngester(database, nil), database
}

func TestShouldSyncSessionOnParserVersionChange(t *testing.T) {
	ing, database := openTestIngester(t)
	meta := thinkt.SessionMeta{FullPath: "/tmp/session.jsonl", ModifiedAt: time.Now(), FileSize: 1024}
	if _, err := database.Exec(`INSERT INTO sync_state (file_path, last_mod_time, file_size, lines_read, parser_version) VALUES (?, ?, ?, ?, ?)`,
		meta.FullPath, meta.ModifiedAt.Unix(), meta.FileSize, 10, 1); err != nil {
		t.Fatal(err)
	}
	should, err := ing.ShouldSyncSession(meta)
	if err != nil || !should {
		t.Fatalf("older parser version must re-sync: should=%v err=%v", should, err)
	}
	if _, err := database.Exec(`UPDATE sync_state SET parser_version = ? WHERE file_path = ?`, thinkt.ParserVersion, meta.FullPath); err != nil {
		t.Fatal(err)
	}
	should, err = ing.ShouldSyncSession(meta)
	if err != nil || should {
		t.Fatalf("current parser version must not re-sync: should=%v err=%v", should, err)
	}
}

func TestWriteSessionStoresParserVersion(t *testing.T) {
	ing, database := openTestIngester(t)
	if _, err := database.Exec(`INSERT INTO projects (id, path, name, source, workspace_id) VALUES ('p', '/p', 'p', 'claude', 'ws')`); err != nil {
		t.Fatal(err)
	}
	meta := thinkt.SessionMeta{ID: "s1", FullPath: "/tmp/s1.jsonl", ModifiedAt: time.Now(), FileSize: 5}
	entries := []thinkt.Entry{{UUID: "u1", Role: thinkt.RoleUser, Text: "hi", Timestamp: time.Now()}}
	if err := ing.writeSession(context.Background(), parsedSession{meta: meta, projectID: "p", entries: entries}); err != nil {
		t.Fatal(err)
	}
	var version int
	if err := database.QueryRow(`SELECT parser_version FROM sync_state WHERE file_path = ?`, meta.FullPath).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != thinkt.ParserVersion {
		t.Fatalf("parser_version = %d, want %d", version, thinkt.ParserVersion)
	}
}
