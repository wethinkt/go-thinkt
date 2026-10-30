package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestOpenLazySessionAntigravity(t *testing.T) {
	base := filepath.Join(t.TempDir(), ".gemini")
	home := filepath.Join(base, "antigravity-cli")
	t.Setenv("THINKT_ANTIGRAVITY_HOME", home)
	t.Setenv("THINKT_GEMINI_HOME", base)
	t.Setenv("THINKT_HOME", t.TempDir())
	data, err := os.ReadFile(filepath.Join("..", "sources", "antigravity", "testdata", "main.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "brain", "11111111-aaaa", ".system_generated", "logs", "transcript_full.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	ls, err := OpenLazySession(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer ls.Close()
	if err := ls.LoadAll(); err != nil {
		t.Fatal(err)
	}
	entries := ls.Entries()
	if len(entries) != 15 || entries[0].Source != thinkt.SourceAntigravity || entries[0].UUID != "11111111-aaaa:0" ||
		entries[0].Text != "fix the build" || ls.Metadata().ID != "11111111-aaaa" {
		t.Fatalf("%d entries, first %+v, meta %+v", len(entries), entries[0], ls.Metadata())
	}
}
