package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestStore_ListProjectsAndSessions(t *testing.T) {
	base := t.TempDir()
	projectPath := filepath.Join(base, "repo")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	sessionPath := filepath.Join(base, "sessions", "2026", "02", "10", "rollout-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}

	content := strings.Join([]string{
		`{"timestamp":"2026-02-10T00:00:00Z","type":"session_meta","payload":{"id":"sess-1","cwd":"` + projectPath + `","git":{"branch":"main"}}}`,
		`{"timestamp":"2026-02-10T00:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"hello codex"}}`,
		`{"timestamp":"2026-02-10T00:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"hi"}}`,
	}, "\n")
	if err := os.WriteFile(sessionPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	store := NewStore(base)
	ctx := context.Background()

	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Source != thinkt.SourceCodex {
		t.Fatalf("unexpected source: %s", projects[0].Source)
	}

	sessions, err := store.ListSessions(ctx, projects[0].ID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != "sess-1" {
		t.Fatalf("unexpected session id: %s", sessions[0].ID)
	}
	if sessions[0].FirstPrompt != "hello codex" {
		t.Fatalf("unexpected first prompt: %q", sessions[0].FirstPrompt)
	}
}

func TestStore_LoadSession(t *testing.T) {
	base := t.TempDir()
	projectPath := filepath.Join(base, "repo")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	sessionPath := filepath.Join(base, "sessions", "2026", "02", "10", "rollout-2.jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}

	content := strings.Join([]string{
		`{"timestamp":"2026-02-10T00:00:00Z","type":"session_meta","payload":{"id":"sess-2","cwd":"` + projectPath + `"}}`,
		`{"timestamp":"2026-02-10T00:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"hello"}}`,
		`{"timestamp":"2026-02-10T00:00:02Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"call_1","arguments":"{\"cmd\":\"pwd\"}"}}`,
		`{"timestamp":"2026-02-10T00:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_1","output":"ok"}}`,
	}, "\n")
	if err := os.WriteFile(sessionPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	store := NewStore(base)
	ctx := context.Background()

	session, err := store.LoadSession(ctx, sessionPath)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session == nil {
		t.Fatalf("expected session, got nil")
	}
	if session.Meta.Source != thinkt.SourceCodex {
		t.Fatalf("unexpected source: %s", session.Meta.Source)
	}
	if len(session.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(session.Entries))
	}
}
