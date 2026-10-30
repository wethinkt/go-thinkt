package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/index/db"
	"github.com/wethinkt/go-thinkt/internal/sources/claude"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// With a symlinked Claude home the watcher sees resolved paths; the index
// must still key the session by the path the store lists.
func TestDiscoverSessionKeepsHomeSpellingThroughSymlink(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(root, "real")
	link := filepath.Join(root, "link")
	data, err := os.ReadFile(filepath.Join("..", "sources", "claude", "testdata", "modern-session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	name := "11111111-2222-4333-8444-555555555555.jsonl"
	resolved := writeLines(t, filepath.Join(real, "projects", "-home-dev-project", name), strings.TrimRight(string(data), "\n"))
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THINKT_CLAUDE_HOME", link)
	reg := thinkt.NewRegistry()
	reg.Register(claude.NewStoreWithCacheDir(link, t.TempDir()))
	w := &Watcher{registry: reg}
	entry, ok := w.discoverSession(resolved)
	want := filepath.Join(link, "projects", "-home-dev-project", name)
	if !ok || entry.session.FullPath != want {
		t.Fatalf("FullPath = %q ok=%v, want %q", entry.session.FullPath, ok, want)
	}
	if entry.projectID != db.ScopedProjectID(thinkt.SourceClaude, filepath.Join(link, "projects", "-home-dev-project")) {
		t.Fatalf("projectID = %q", entry.projectID)
	}
}
