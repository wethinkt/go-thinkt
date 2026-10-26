package claude

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Non-interactive listings (sessions list --json) cannot wait for the
// background enrichment callback; WithEnrichSync scans stale sessions before
// returning so titles and counts are present on a cold cache.
func TestListSessionsEnrichSyncFillsTitlesOnColdCache(t *testing.T) {
	base := t.TempDir()
	projectDir := filepath.Join(base, "projects", "-home-dev-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(modernFixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "11111111-1111-4111-8111-111111111111.jsonl"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStoreWithCacheDir(base, filepath.Join(base, "cache"))

	plain, err := store.ListSessions(context.Background(), projectDir)
	if err != nil || len(plain) != 1 {
		t.Fatalf("plain list: %v %v", plain, err)
	}
	if plain[0].Title != "" {
		t.Fatalf("precondition: cold cache must not have a title yet, got %q", plain[0].Title)
	}

	synced, err := store.ListSessions(context.Background(), projectDir, thinkt.WithEnrichSync())
	if err != nil || len(synced) != 1 {
		t.Fatalf("sync list: %v %v", synced, err)
	}
	if synced[0].Title != "Final title" || synced[0].EntryCount != modernFixtureEntries || synced[0].LastPrompt != "hello" {
		t.Fatalf("sync enrichment missing: %+v", synced[0])
	}
}

func TestStoreOpenSessionModernFixture(t *testing.T) {
	base := t.TempDir()
	projectDir := filepath.Join(base, "projects", "-home-dev-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(modernFixture)
	if err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(projectDir, "11111111-1111-4111-8111-111111111111.jsonl")
	if err := os.WriteFile(sessionPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStoreWithCacheDir(base, filepath.Join(base, "cache"))

	reader, err := store.OpenSession(context.Background(), sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var entries []thinkt.Entry
	for {
		entry, err := reader.ReadNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, *entry)
	}
	if len(entries) != modernFixtureEntries {
		t.Fatalf("entries = %d, want %d", len(entries), modernFixtureEntries)
	}
	want := []struct {
		role thinkt.Role
		kind thinkt.EntryKind
		vis  thinkt.Visibility
	}{
		{thinkt.RoleUser, thinkt.KindNone, thinkt.VisibilityNormal},
		{thinkt.RoleUser, thinkt.KindAttachment, thinkt.VisibilitySecondary}, // instructions rendered into the user turn
		{thinkt.RoleAssistant, thinkt.KindNone, thinkt.VisibilityNormal},
		{thinkt.RoleAssistant, thinkt.KindNone, thinkt.VisibilityNormal},
		{thinkt.RoleUser, thinkt.KindNone, thinkt.VisibilityNormal},
		{thinkt.RoleSystem, thinkt.KindTurnDuration, thinkt.VisibilitySecondary},
		{thinkt.RoleCheckpoint, thinkt.KindNone, thinkt.VisibilityNormal},
	}
	for i, w := range want {
		e := entries[i]
		if e.Role != w.role || e.Kind != w.kind || e.Visibility != w.vis {
			t.Errorf("entry %d: got role=%s kind=%q vis=%q want role=%s kind=%q vis=%q", i, e.Role, e.Kind, e.Visibility, w.role, w.kind, w.vis)
		}
		if e.Role == thinkt.RoleSystem && e.Text == "" {
			t.Errorf("entry %d: empty system text", i)
		}
	}
	if entries[2].TurnID != "msg_1" || entries[3].TurnID != "msg_1" {
		t.Fatalf("assistant records must share TurnID: %q %q", entries[2].TurnID, entries[3].TurnID)
	}
	if entries[1].Subtype != "instructions" || entries[1].Text != "/home/dev/project/AGENTS.md" {
		t.Fatalf("instructions attachment: %+v", entries[1])
	}
	result := entries[4].ContentBlocks[0]
	if result.Type != "tool_result" || result.ToolResult != "ok" || result.Children != nil {
		t.Fatalf("tool result: %+v", result)
	}
	if entries[4].Permission == nil || entries[4].Permission.Decision != "accept" {
		t.Fatalf("permission: %+v", entries[4].Permission)
	}

	meta, err := store.GetSessionMeta(context.Background(), sessionPath)
	if err != nil || meta == nil {
		t.Fatalf("meta %v err %v", meta, err)
	}
	if meta.Title != "Final title" || meta.Summary != "Final title" || meta.EntryCount != modernFixtureEntries || meta.LastPrompt != "hello" {
		t.Fatalf("meta %+v", meta)
	}
	if meta.Cost == nil || meta.Cost.TotalUSD != 0.12 {
		t.Fatalf("cost %+v", meta.Cost)
	}
	if meta.FirstPrompt != "hello" || meta.Model != "claude-opus-5-5" {
		t.Fatalf("hints %+v", meta)
	}
}
