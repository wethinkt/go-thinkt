package tui

import (
	"os"
	"path/filepath"
	"testing"
)

// copySourceFixture copies internal/sources/<src>/testdata/<name> to dest.
func copySourceFixture(t *testing.T, src, name, dest string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "sources", src, "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenLazySessionOpensDisabledSourceFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("THINKT_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(`{"sources":{"codex":{"enabled":false}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	t.Setenv("THINKT_CODEX_HOME", base)
	path := filepath.Join(base, "sessions", "2026", "10", "01", "rollout-2026-10-01T10-00-00-fixture.jsonl")
	copySourceFixture(t, "codex", "rollout-fixture.jsonl", path)

	ls, err := OpenLazySession(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer ls.Close()
	if err := ls.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if len(ls.Entries()) == 0 {
		t.Fatal("no entries")
	}
}

func TestOpenLazySessionClaudeTranscriptOpensDirectly(t *testing.T) {
	t.Setenv("THINKT_HOME", t.TempDir())
	base := t.TempDir()
	t.Setenv("THINKT_CLAUDE_HOME", base)
	path := filepath.Join(base, "projects", "-home-dev-project", "11111111-2222-4333-8444-555555555555.jsonl")
	copySourceFixture(t, "claude", "modern-session.jsonl", path)

	ls, err := OpenLazySession(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer ls.Close()
	if err := ls.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if len(ls.Entries()) == 0 {
		t.Fatal("no entries")
	}
}
