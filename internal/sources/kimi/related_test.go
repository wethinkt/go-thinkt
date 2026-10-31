package kimi

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// mkRepo creates root/name with a .git directory and returns its path.
func mkRepo(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestScanRecordsRelatedPaths(t *testing.T) {
	root := t.TempDir()
	proj, other := mkRepo(t, root, "p"), mkRepo(t, root, "o")
	call := func(id, name, args string) string {
		return fmt.Sprintf(`{"message":{"message":{"role":"assistant","content":[{"type":"text","text":"x"}],"toolCalls":[{"type":"function","id":%q,"name":%q,"arguments":%q}]},"meta":{"source":"llm","messageId":%q}},"type":"agent.message.appended","time":1,"kind":"event"}`, id, name, args, id)
	}
	path := filepath.Join(root, "wire.jsonl")
	writeFile(t, path, call("t1", "Read", fmt.Sprintf(`{"path":%q}`, filepath.Join(other, "a.go")))+"\n"+
		call("t2", "StrReplaceFile", fmt.Sprintf(`{"path":%q}`, filepath.Join(other, "b.go")))+"\n"+
		call("t3", "Bash", fmt.Sprintf(`{"command":"make","cwd":%q}`, other))+"\n"+
		call("t4", "Bash", `{"command":"ls"}`)+"\n"+
		call("t5", "Edit", `{"path":"own.go"}`)+"\n")
	s := scanSessionFile(path, "sid", thinkt.NewRelatedTracker(proj, nil))
	want := []thinkt.RelatedPath{{Path: other, Reasons: []string{"command", "edited", "read"}, Count: 3}}
	if !reflect.DeepEqual(s.Related, want) {
		t.Fatalf("%+v", s.Related)
	}
	var meta thinkt.SessionMeta
	s.applyTo(&meta)
	if !reflect.DeepEqual(s.cached(meta).RelatedPaths, want) {
		t.Fatalf("cached %+v", s.cached(meta).RelatedPaths)
	}
}

// An unknown project is a placeholder, not a folder to measure from.
func TestTrackerNeedsARealProject(t *testing.T) {
	s := &Store{}
	if s.tracker("unknown") != nil || s.tracker("") != nil {
		t.Fatal("a tracker with no project")
	}
	if s.tracker("/p") == nil {
		t.Fatal("no tracker for a project")
	}
}
