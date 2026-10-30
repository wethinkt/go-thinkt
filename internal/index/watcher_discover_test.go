package index

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/sources/antigravity"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Antigravity transcripts live under the source's home, not under the
// project folder: discovery must still find a conversation the watcher
// has not seen.
func TestDiscoverSessionOutsideProjectFolder(t *testing.T) {
	home := filepath.Join(t.TempDir(), "antigravity-cli")
	data, err := os.ReadFile(filepath.Join("..", "sources", "antigravity", "testdata", "main.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte("__REPO_B__"), []byte("/w/ds4-go"))
	path := filepath.Join(home, "brain", "c1", ".system_generated", "logs", "transcript_full.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	reg := thinkt.NewRegistry()
	reg.Register(antigravity.NewStoreWithCacheDir(home, t.TempDir()))
	w := &Watcher{registry: reg}

	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := w.discoverSession(real)
	if !ok || entry.source != thinkt.SourceAntigravity || entry.session.ID != "c1" {
		t.Fatalf("%+v %v", entry, ok)
	}
	// The sibling transcript is not a session file.
	if _, ok := w.discoverSession(filepath.Join(filepath.Dir(real), "transcript.jsonl")); ok {
		t.Fatal("sibling transcript discovered")
	}
}

// An unmatched path is warned about once, not on every write.
func TestUnmatchedPathWarnsOnce(t *testing.T) {
	w := &Watcher{}
	if !w.firstUnmatched("/a.jsonl") || w.firstUnmatched("/a.jsonl") || !w.firstUnmatched("/b.jsonl") {
		t.Fatal("firstUnmatched should be true once per path")
	}
}
