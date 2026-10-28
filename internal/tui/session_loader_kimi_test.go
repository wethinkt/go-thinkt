package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func copyKimiFixture(t *testing.T, name, dest string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "sources", "kimi", "testdata", name))
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

func TestOpenLazySessionKimiWirePath(t *testing.T) {
	base := t.TempDir()
	t.Setenv("THINKT_KIMI_HOME", base)
	t.Setenv("THINKT_HOME", t.TempDir())
	sessionDir := filepath.Join(base, "sessions", "wd_proj_abc", "session_p1")
	state := `{"id":"session_p1","version":2,"cwd":"/proj","createdAt":1790891444054,"updatedAt":1790894174521,"title":"t","agents":{"main":{"type":"main"},"agent-0":{"type":"sub","parentAgentId":"main","labels":{"profileName":"explore"}}}}`
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "state.json"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	mainWire := filepath.Join(sessionDir, "agents", "main", "wire.jsonl")
	childWire := filepath.Join(sessionDir, "agents", "agent-0", "wire.jsonl")
	copyKimiFixture(t, "wire-1.5.jsonl", mainWire)
	copyKimiFixture(t, "wire-1.4.jsonl", childWire)

	ls, err := OpenLazySession(mainWire)
	if err != nil {
		t.Fatalf("open main: %v", err)
	}
	defer ls.Close()
	if err := ls.LoadAll(); err != nil {
		t.Fatal(err)
	}
	entries := ls.Entries()
	if len(entries) != 17 || entries[0].Text != "fix the build" || ls.Metadata().ID != "session_p1" {
		t.Fatalf("main: %d entries, first %+v, meta %+v", len(entries), entries[0], ls.Metadata())
	}

	child, err := OpenLazySession(childWire)
	if err != nil {
		t.Fatalf("open child: %v", err)
	}
	defer child.Close()
	if err := child.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if len(child.Entries()) != 3 || child.Metadata().ID != "session_p1/agent-0" || child.Metadata().ParentSessionID != "session_p1" {
		t.Fatalf("child: %d entries, meta %+v", len(child.Entries()), child.Metadata())
	}
}
