package index

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fsnotify/fsnotify"

	"github.com/wethinkt/go-thinkt/internal/sources/gemini"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// A subagent's folder is watched, so its writes reach discoverSession, but
// the subagent is not put in the index: the index holds no subagent
// sessions.
func TestWatchProjectWatchesSubagentDir(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join("..", "sources", "gemini", "testdata", "chats")
	parent := "17503250-82e4-479a-ab9d-69002dcb6d31"
	copyFile := func(from, to string) {
		t.Helper()
		b, err := os.ReadFile(from)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(to, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chats := filepath.Join(base, "tmp", "proj", "chats")
	copyFile(filepath.Join(src, "session-2026-05-17T15-05-17503250.jsonl"), filepath.Join(chats, "session-2026-05-17T15-05-17503250.jsonl"))
	copyFile(filepath.Join(src, parent, "92szal.jsonl"), filepath.Join(chats, parent, "92szal.jsonl"))
	if err := os.WriteFile(filepath.Join(base, "tmp", "proj", ".project_root"), []byte("/Users/dev/proj\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := gemini.NewStoreWithCacheDir(base, t.TempDir())
	reg := thinkt.NewRegistry()
	reg.Register(store)
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer fw.Close()
	w := &Watcher{registry: reg, watcher: fw, sessionIndex: map[string]sessionIndexEntry{}}

	projects, err := store.ListProjects(context.Background())
	if err != nil || len(projects) != 1 {
		t.Fatalf("projects %+v %v", projects, err)
	}
	if err := w.watchProject(projects[0]); err != nil {
		t.Fatal(err)
	}
	watched := fw.WatchList()
	if !slices.Contains(watched, filepath.Join(chats, parent)) {
		t.Fatalf("subagent folder not watched: %v", watched)
	}
	if _, ok := w.sessionIndex[filepath.Join(chats, parent, "92szal.jsonl")]; ok {
		t.Fatal("subagent put in the session index")
	}
	if _, ok := w.sessionIndex[filepath.Join(chats, "session-2026-05-17T15-05-17503250.jsonl")]; !ok {
		t.Fatalf("main session not indexed: %v", w.sessionIndex)
	}
}
