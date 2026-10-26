package claude

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func writeTempSession(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScanSessionFileModernFixture(t *testing.T) {
	s := scanSessionFile(modernFixture)
	if s.Title != "Final title" || s.LastPrompt != "hello" || s.EntryCount != modernFixtureEntries || s.FirstPrompt != "hello" {
		t.Fatalf("%+v", s)
	}
	if s.Model != "claude-opus-5-5" {
		t.Fatalf("model: %q", s.Model)
	}
	if s.Cost == nil || s.Cost.TotalUSD != 0.12 || s.Cost.LinesAdded != 3 || s.Cost.LinesRemoved != 1 || s.Cost.DurationMs != 5000 {
		t.Fatalf("cost %+v", s.Cost)
	}
}

// Legacy summary records were the session title before ai-title existed.
func TestScanSessionFileLegacySummaryIsTitleFallback(t *testing.T) {
	path := writeTempSession(t, t.TempDir(), "s.jsonl",
		`{"type":"summary","summary":"Legacy title","leafUuid":"l"}`+"\n"+
			`{"type":"user","uuid":"u","message":{"role":"user","content":"hi"}}`+"\n")
	var meta thinkt.SessionMeta
	scanSessionFile(path).applyTo(&meta)
	if meta.Title != "Legacy title" || meta.Summary != "Legacy title" {
		t.Fatalf("legacy summary must back the title: %+v", meta)
	}
	withTitle := writeTempSession(t, t.TempDir(), "t.jsonl",
		`{"type":"summary","summary":"Legacy title","leafUuid":"l"}`+"\n"+
			`{"type":"ai-title","aiTitle":"Real title","sessionId":"s"}`+"\n")
	var meta2 thinkt.SessionMeta
	scanSessionFile(withTitle).applyTo(&meta2)
	if meta2.Title != "Real title" {
		t.Fatalf("ai-title must win over legacy summary: %+v", meta2)
	}
}

func TestScanSessionFileEmptyTitleDoesNotBlank(t *testing.T) {
	path := writeTempSession(t, t.TempDir(), "s.jsonl",
		`{"type":"ai-title","aiTitle":"Keep","sessionId":"s"}`+"\n"+
			`{"type":"ai-title","aiTitle":"","sessionId":"s"}`+"\n")
	if s := scanSessionFile(path); s.Title != "Keep" {
		t.Fatalf("title %q", s.Title)
	}
}

func TestScanSessionFileStateRecords(t *testing.T) {
	path := writeTempSession(t, t.TempDir(), "s.jsonl",
		`{"type":"pr-link","sessionId":"s","prNumber":162,"prUrl":"https://example.com/pull/162","prRepository":"org/repo","timestamp":"2026-10-20T01:01:45.565Z"}`+"\n"+
			`{"type":"worktree-state","worktreeSession":{"originalCwd":"/p","worktreePath":"/p/.claude/worktrees/x","worktreeName":"x","worktreeBranch":"worktree-x","originalBranch":"main"},"sessionId":"s"}`+"\n"+
			`{"type":"relocated","sessionId":"s","relocatedCwd":"/p/.claude/worktrees/x"}`+"\n")
	s := scanSessionFile(path)
	if len(s.PRLinks) != 1 || s.PRLinks[0].Number != 162 || s.PRLinks[0].Repository != "org/repo" || s.PRLinks[0].Timestamp.IsZero() {
		t.Fatalf("pr links %+v", s.PRLinks)
	}
	if s.Worktree == nil || s.Worktree.Path != "/p/.claude/worktrees/x" || s.Worktree.Branch != "worktree-x" || s.Worktree.OriginalCwd != "/p" || s.Worktree.OriginalBranch != "main" {
		t.Fatalf("worktree %+v", s.Worktree)
	}
}

func TestScanSessionFileBookkeepingOnly(t *testing.T) {
	base := t.TempDir()
	projectDir := filepath.Join(base, "projects", "-p")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := writeTempSession(t, projectDir, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa.jsonl",
		`{"type":"ai-title","aiTitle":"Only title","sessionId":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}`+"\n"+
			`{"type":"mode","mode":"normal","sessionId":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}`+"\n"+
			`{"type":"permission-mode","permissionMode":"auto","sessionId":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}`+"\n")
	if s := scanSessionFile(path); s.EntryCount != 0 || s.Title != "Only title" {
		t.Fatalf("scan %+v", s)
	}

	store := NewStoreWithCacheDir(base, filepath.Join(base, "cache"))
	reader, err := store.OpenSession(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err := reader.ReadNext(); err != io.EOF {
		t.Fatalf("first read: want io.EOF, got %v", err)
	}
	meta, err := store.GetSessionMeta(context.Background(), path)
	if err != nil || meta == nil {
		t.Fatalf("meta %v err %v", meta, err)
	}
	if meta.Title != "Only title" || meta.Summary != "Only title" || meta.EntryCount != 0 {
		t.Fatalf("meta %+v", meta)
	}
}

func TestCountSubagents(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb.jsonl")
	sub := filepath.Join(dir, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "subagents")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"agent-a.jsonl", "agent-b.jsonl", "agent-a.meta.json"} {
		writeTempSession(t, sub, name, "{}\n")
	}
	if got := countSubagents(sessionPath); got != 2 {
		t.Fatalf("countSubagents = %d", got)
	}
	if got := countSubagents(filepath.Join(dir, "missing.jsonl")); got != 0 {
		t.Fatalf("missing dir = %d", got)
	}
}

func TestLazySessionMetadataCarriesTitle(t *testing.T) {
	ls, err := OpenLazySession(modernFixture)
	if err != nil {
		t.Fatal(err)
	}
	defer ls.Close()
	if err := ls.LoadAll(); err != nil {
		t.Fatal(err)
	}
	meta := ls.Metadata()
	if meta.Title != "Final title" || meta.LastPrompt != "hello" || meta.Summary != "Final title" {
		t.Fatalf("lazy metadata %+v", meta)
	}
}

func TestListSessionsMergesTitleAndCost(t *testing.T) {
	base := t.TempDir()
	projectDir := filepath.Join(base, "projects", "-p")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(modernFixture)
	if err != nil {
		t.Fatal(err)
	}
	writeTempSession(t, projectDir, "11111111-1111-4111-8111-111111111111.jsonl", string(data))
	store := NewStoreWithCacheDir(base, filepath.Join(base, "cache"))
	done := make(chan []thinkt.SessionMeta, 4)
	sessions, err := store.ListSessions(context.Background(), projectDir, thinkt.WithEnrich(func(_ string, s []thinkt.SessionMeta) { done <- s }))
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions %v err %v", sessions, err)
	}
	enriched := <-done
	got := enriched[0]
	if got.Title != "Final title" || got.Summary != "Final title" || got.LastPrompt != "hello" || got.EntryCount != modernFixtureEntries {
		t.Fatalf("enriched %+v", got)
	}
	if got.Cost == nil || got.Cost.TotalUSD != 0.12 {
		t.Fatalf("cost %+v", got.Cost)
	}
	// A second listing must come from the cache with the same fields.
	again, err := store.ListSessions(context.Background(), projectDir)
	if err != nil || again[0].Title != "Final title" || again[0].EntryCount != modernFixtureEntries {
		t.Fatalf("cached listing %+v err %v", again, err)
	}
}
