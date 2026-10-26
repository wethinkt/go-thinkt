package index

import (
	"context"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestWriteSessionRecordsAttachments(t *testing.T) {
	ing, database := openTestIngester(t)
	if _, err := database.Exec(`INSERT INTO projects (id, path, name, source, workspace_id) VALUES ('p', '/p', 'p', 'claude', 'ws')`); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	meta := thinkt.SessionMeta{ID: "s", FullPath: "/tmp/s.jsonl", ModifiedAt: now, FileSize: 5}
	entries := []thinkt.Entry{
		{UUID: "u1", Role: thinkt.RoleUser, Text: "hi", Timestamp: now},
		{UUID: "at1", Role: thinkt.RoleSystem, Kind: thinkt.KindAttachment, Subtype: "hook_success", Visibility: thinkt.VisibilitySecondary, Text: "PreToolUse: ok\nmore", Timestamp: now},
		{UUID: "at2", Role: thinkt.RoleSystem, Kind: thinkt.KindAttachment, Subtype: "task_status", Visibility: thinkt.VisibilitySecondary, Text: "Implement Task 1 — completed", Timestamp: now},
		{UUID: "s1", Role: thinkt.RoleSystem, Kind: thinkt.KindTurnDuration, Visibility: thinkt.VisibilitySecondary, Text: "turn_duration", Timestamp: now},
	}
	if err := ing.writeSession(context.Background(), parsedSession{meta: meta, projectID: "p", entries: entries}); err != nil {
		t.Fatal(err)
	}
	rows, err := database.ListAttachments("s", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].EntryIndex != 1 || rows[0].Kind != "hook_success" || rows[0].Label != "PreToolUse: ok" {
		t.Fatalf("attachments: %+v", rows)
	}
	if rows, _ := database.ListAttachments("s", "task_status"); len(rows) != 1 || rows[0].EntryIndex != 2 {
		t.Fatalf("task_status filter: %+v", rows)
	}
	// Re-ingesting replaces rows rather than duplicating them.
	if err := ing.writeSession(context.Background(), parsedSession{meta: meta, projectID: "p", entries: entries[:2]}); err != nil {
		t.Fatal(err)
	}
	if rows, _ := database.ListAttachments("s", ""); len(rows) != 1 {
		t.Fatalf("re-ingest must replace rows: %+v", rows)
	}
}
