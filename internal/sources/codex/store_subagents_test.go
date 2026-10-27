package codex

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// subagentFixture writes a parent rollout, a child subagent rollout, and a
// session_index.jsonl under base, and returns the project path.
func subagentFixture(t *testing.T, base string, withParent bool) string {
	t.Helper()
	proj := filepath.Join(base, "repo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	day := filepath.Join(base, "sessions", "2026", "10", "01")
	if withParent {
		writeLines(t, filepath.Join(day, "rollout-p.jsonl"),
			`{"timestamp":"2026-10-01T10:00:00Z","type":"session_meta","payload":{"id":"p","session_id":"p","cwd":"`+proj+`","git":{"branch":"main"}}}`,
			`{"timestamp":"2026-10-01T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"parent prompt"}],"internal_chat_message_metadata_passthrough":{"turn_id":"t1","content_item_kinds":["user.text"]}}}`,
		)
	}
	writeLines(t, filepath.Join(day, "rollout-c.jsonl"),
		`{"timestamp":"2026-10-01T10:00:05Z","type":"session_meta","payload":{"id":"c","session_id":"p","cwd":"`+proj+`","source":{"subagent":{"thread_spawn":{"parent_thread_id":"p","depth":1,"agent_path":"/root/x","agent_nickname":"Kierkegaard"}}},"agent_nickname":"Kierkegaard","agent_path":"/root/x"}}`,
		`{"timestamp":"2026-10-01T10:00:06Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"child prompt"}],"internal_chat_message_metadata_passthrough":{"turn_id":"t2","content_item_kinds":["user.text"]}}}`,
	)
	writeLines(t, filepath.Join(base, "session_index.jsonl"), `{"id":"p","thread_name":"Fix build","updated_at":"2026-10-01T10:00:00Z"}`)
	return proj
}

func TestListSessionsHidesSubagentsByDefault(t *testing.T) {
	base := t.TempDir()
	proj := subagentFixture(t, base, true)
	store := NewStoreWithCacheDir(base, filepath.Join(base, "cache"))
	ctx := context.Background()

	projects, err := store.ListProjects(ctx)
	if err != nil || len(projects) != 1 || projects[0].SessionCount != 1 || projects[0].SubagentCount != 1 {
		t.Fatalf("projects = %+v err = %v", projects, err)
	}
	sessions, err := store.ListSessions(ctx, proj)
	if err != nil || len(sessions) != 1 || sessions[0].ID != "p" || sessions[0].Title != "Fix build" || sessions[0].SubagentCount != 1 {
		t.Fatalf("sessions = %+v err = %v", sessions, err)
	}
	if sessions[0].FirstPrompt != "parent prompt" {
		t.Fatalf("first prompt = %q", sessions[0].FirstPrompt)
	}
	all, err := store.ListSessions(ctx, proj, thinkt.WithSubagents())
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %+v err = %v", all, err)
	}
	var child *thinkt.SessionMeta
	for i := range all {
		if all[i].ID == "c" {
			child = &all[i]
		}
	}
	if child == nil || child.ParentSessionID != "p" || child.Title != "Kierkegaard /root/x" {
		t.Fatalf("child = %+v", child)
	}
	meta, err := store.GetSessionMeta(ctx, "c")
	if err != nil || meta == nil || meta.ParentSessionID != "p" || meta.ID != "c" {
		t.Fatalf("meta = %+v err = %v", meta, err)
	}
}

func TestListSessionsParentIDSurvivesCacheHit(t *testing.T) {
	base := t.TempDir()
	proj := subagentFixture(t, base, true)
	cacheDir := filepath.Join(base, "cache")
	ctx := context.Background()

	first := NewStoreWithCacheDir(base, cacheDir)
	if _, err := first.ListProjects(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := first.ListSessions(ctx, proj, thinkt.WithSubagents()); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(cacheDir, "*"))
	found := false
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err == nil && regexp.MustCompile(`"parent_session_id":\s*"p"`).Match(b) {
			found = true
		}
	}
	if !found {
		t.Fatalf("cache files %v do not record the parent id", files)
	}

	second := NewStoreWithCacheDir(base, cacheDir)
	if _, err := second.ListProjects(ctx); err != nil {
		t.Fatal(err)
	}
	all, err := second.ListSessions(ctx, proj, thinkt.WithSubagents())
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %+v err = %v", all, err)
	}
	for _, s := range all {
		if s.ID == "c" && s.ParentSessionID != "p" {
			t.Fatalf("child lost parent on cache hit: %+v", s)
		}
		if s.ID == "p" && s.SubagentCount != 1 {
			t.Fatalf("parent lost subagent count on cache hit: %+v", s)
		}
	}
	hidden, err := second.ListSessions(ctx, proj)
	if err != nil || len(hidden) != 1 || hidden[0].ID != "p" {
		t.Fatalf("hidden = %+v err = %v", hidden, err)
	}
}

func TestListSessionsOrphanSubagent(t *testing.T) {
	base := t.TempDir()
	proj := subagentFixture(t, base, false)
	store := NewStoreWithCacheDir(base, filepath.Join(base, "cache"))
	ctx := context.Background()
	projects, err := store.ListProjects(ctx)
	if err != nil || len(projects) != 1 || projects[0].SessionCount != 0 || projects[0].SubagentCount != 1 {
		t.Fatalf("projects = %+v err = %v", projects, err)
	}
	sessions, err := store.ListSessions(ctx, proj)
	if err != nil || len(sessions) != 0 {
		t.Fatalf("sessions = %+v err = %v", sessions, err)
	}
	all, err := store.ListSessions(ctx, proj, thinkt.WithSubagents())
	if err != nil || len(all) != 1 || all[0].ParentSessionID != "p" {
		t.Fatalf("all = %+v err = %v", all, err)
	}
}

func TestListSessionsEnrichSyncFillsEntryCountAndCost(t *testing.T) {
	base := t.TempDir()
	fixture, err := os.ReadFile("testdata/rollout-fixture.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	// The fixture's cwd is /work/proj; rewrite it to a directory under base
	// so the project exists.
	proj := filepath.Join(base, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.ReplaceAll(string(fixture), `"cwd":"/work/proj"`, `"cwd":"`+proj+`"`)
	path := filepath.Join(base, "sessions", "2026", "10", "01", "rollout-thr-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStoreWithCacheDir(base, filepath.Join(base, "cache"))
	ctx := context.Background()
	if _, err := store.ListProjects(ctx); err != nil {
		t.Fatal(err)
	}
	sessions, err := store.ListSessions(ctx, proj, thinkt.WithEnrichSync())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %+v err = %v", sessions, err)
	}
	s := sessions[0]
	if s.ID != "thr-1" || s.EntryCount != 39 || s.Cost == nil || s.Cost.DurationMs != 14000 || s.LastPrompt != "fix the build" || s.Model != "gpt-5.5" || s.GitBranch != "main" {
		t.Fatalf("session = %+v cost = %+v", s, s.Cost)
	}
}

func TestListSessionsEnrichCallbackRunsInBackground(t *testing.T) {
	base := t.TempDir()
	proj := subagentFixture(t, base, true)
	store := NewStoreWithCacheDir(base, filepath.Join(base, "cache"))
	ctx := context.Background()
	if _, err := store.ListProjects(ctx); err != nil {
		t.Fatal(err)
	}
	done := make(chan []thinkt.SessionMeta, 1)
	_, err := store.ListSessions(ctx, proj, thinkt.WithEnrich(func(id string, sessions []thinkt.SessionMeta) {
		done <- sessions
	}))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case enriched := <-done:
		if len(enriched) != 1 || enriched[0].ID != "p" || enriched[0].EntryCount != 1 {
			t.Fatalf("enriched = %+v", enriched)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("enrichment callback never fired")
	}
}

func TestGetSessionMetaByPathUsesSessionMetaID(t *testing.T) {
	base := t.TempDir()
	proj := filepath.Join(base, "repo")
	path := filepath.Join(base, "sessions", "2026", "10", "01", "rollout-x.jsonl")
	writeLines(t, path,
		`{"timestamp":"2026-10-01T10:00:00Z","type":"session_meta","payload":{"id":"thr-9","session_id":"thr-9","cwd":"`+proj+`"}}`,
		`{"timestamp":"2026-10-01T10:00:01Z","type":"turn_context","payload":{"turn_id":"t1","cwd":"`+proj+`","model":"gpt-5.5","effort":"medium"}}`,
		`{"timestamp":"2026-10-01T10:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}],"internal_chat_message_metadata_passthrough":{"turn_id":"t1","content_item_kinds":["user.text"]}}}`,
	)
	store := NewStoreWithCacheDir(base, filepath.Join(base, "cache"))
	meta, err := store.GetSessionMeta(context.Background(), path)
	if err != nil || meta == nil || meta.ID != "thr-9" || meta.Model != "gpt-5.5" || meta.FirstPrompt != "hello" || meta.EntryCount != 1 {
		t.Fatalf("meta = %+v err = %v", meta, err)
	}
}
