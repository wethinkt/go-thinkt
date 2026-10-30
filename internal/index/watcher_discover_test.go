package index

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/index/db"
	"github.com/wethinkt/go-thinkt/internal/sources/antigravity"
	"github.com/wethinkt/go-thinkt/internal/sources/claude"
	"github.com/wethinkt/go-thinkt/internal/sources/codex"
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
	t.Setenv("THINKT_ANTIGRAVITY_HOME", home)
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

// writeLines writes lines to path, creating parent directories.
func writeLines(t *testing.T, path string, lines ...string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// codexRollouts writes a parent rollout and a subagent rollout under base
// and returns the project directory and both paths.
func codexRollouts(t *testing.T, base string) (proj, parent, child string) {
	t.Helper()
	proj = filepath.Join(base, "repo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	day := filepath.Join(base, "sessions", "2026", "10", "01")
	parent = writeLines(t, filepath.Join(day, "rollout-p.jsonl"),
		`{"timestamp":"2026-10-01T10:00:00Z","type":"session_meta","payload":{"id":"p","session_id":"p","cwd":"`+proj+`"}}`,
		`{"timestamp":"2026-10-01T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"parent prompt"}]}}`,
	)
	child = writeLines(t, filepath.Join(day, "rollout-c.jsonl"),
		`{"timestamp":"2026-10-01T10:00:05Z","type":"session_meta","payload":{"id":"c","session_id":"p","cwd":"`+proj+`","source":{"subagent":{"thread_spawn":{"parent_thread_id":"p","depth":1}}}}}`,
		`{"timestamp":"2026-10-01T10:00:06Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"child prompt"}]}}`,
	)
	return proj, parent, child
}

// countingStore is a Store whose ListProjects calls are counted; every
// other method is unused by these tests.
type countingStore struct {
	thinkt.Store
	source thinkt.Source
	calls  int
}

func (s *countingStore) Source() thinkt.Source { return s.source }

func (s *countingStore) ListProjects(context.Context) ([]thinkt.Project, error) {
	s.calls++
	return nil, nil
}

func TestDiscoverSessionSkipsDisabledSource(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("THINKT_CODEX_HOME", base)
	_, parent, _ := codexRollouts(t, base)
	other := &countingStore{source: thinkt.SourceClaude}
	reg := thinkt.NewRegistry()
	reg.Register(other)
	w := &Watcher{registry: reg}
	if entry, ok := w.discoverSession(parent); ok {
		t.Fatalf("discovered %+v through a source that is not registered", entry)
	}
	if other.calls != 0 {
		t.Fatalf("scanned another store's projects %d times", other.calls)
	}
}

func TestDiscoverSessionSkipsSubagent(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("THINKT_CODEX_HOME", base)
	proj, parent, child := codexRollouts(t, base)
	reg := thinkt.NewRegistry()
	reg.Register(codex.NewStoreWithCacheDir(base, t.TempDir()))
	w := &Watcher{registry: reg}
	if entry, ok := w.discoverSession(child); ok {
		t.Fatalf("subagent discovered: %+v", entry)
	}
	entry, ok := w.discoverSession(parent)
	if !ok || entry.session.ID != "p" || entry.projectID != db.ScopedProjectID(thinkt.SourceCodex, proj) {
		t.Fatalf("parent: %+v %v", entry, ok)
	}
}

func TestDiscoverSessionUsesProjectIDFromPath(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("THINKT_CLAUDE_HOME", base)
	data, err := os.ReadFile(filepath.Join("..", "sources", "claude", "testdata", "modern-session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "projects", "-home-dev-project")
	path := writeLines(t, filepath.Join(dir, "11111111-2222-4333-8444-555555555555.jsonl"), strings.TrimRight(string(data), "\n"))
	reg := thinkt.NewRegistry()
	reg.Register(claude.NewStoreWithCacheDir(base, t.TempDir()))
	w := &Watcher{registry: reg}
	entry, ok := w.discoverSession(path)
	if !ok || entry.source != thinkt.SourceClaude || entry.projectID != db.ScopedProjectID(thinkt.SourceClaude, dir) {
		t.Fatalf("%+v %v", entry, ok)
	}
}
