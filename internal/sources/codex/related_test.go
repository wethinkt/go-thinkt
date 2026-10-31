package codex

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

func relatedHead(cwd string) []string {
	return []string{
		fmt.Sprintf(`{"timestamp":"2026-10-01T00:00:00Z","type":"session_meta","payload":{"id":"s","cwd":%q}}`, cwd),
		fmt.Sprintf(`{"timestamp":"2026-10-01T00:00:01Z","type":"turn_context","payload":{"cwd":%q,"model":"gpt-5"}}`, cwd),
	}
}

func execItem(cwd string) string {
	return fmt.Sprintf(`{"timestamp":"2026-10-01T00:00:02Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"c","command":["make"],"cwd":%q,"status":"completed"}}}`, cwd)
}

func changeItem(path string) string {
	return fmt.Sprintf(`{"timestamp":"2026-10-01T00:00:03Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"FileChange","id":"f","status":"completed","changes":{%q:{"type":"update"}}}}}`, path)
}

func wireExec(workdir string) string {
	return fmt.Sprintf(`{"timestamp":"2026-10-01T00:00:04Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"w","arguments":%q}}`, fmt.Sprintf(`{"cmd":"make","workdir":%q}`, workdir))
}

func wirePatch(path string) string {
	return fmt.Sprintf(`{"timestamp":"2026-10-01T00:00:05Z","type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","call_id":"p","input":%q}}`, "*** Begin Patch\n*** Update File: "+path+"\n@@\n-a\n+b\n*** End Patch\n")
}

func TestScanRecordsRelatedFromItems(t *testing.T) {
	root := t.TempDir()
	proj, other := mkRepo(t, root, "p"), mkRepo(t, root, "o")
	path := filepath.Join(root, "r.jsonl")
	lines := append(relatedHead(proj),
		execItem(other), execItem(proj), execItem(""),
		changeItem(filepath.Join(other, "a.go")), changeItem("own.go"),
		wireExec(other), wirePatch(filepath.Join(other, "c.go"))) // twins of items: not counted
	writeLines(t, path, lines...)
	s := scanSessionFile(path, thinkt.NewRelatedTracker(proj, nil))
	want := []thinkt.RelatedPath{{Path: other, Reasons: []string{"command", "edited"}, Count: 2}}
	if !reflect.DeepEqual(s.Related, want) {
		t.Fatalf("%+v", s.Related)
	}
	var meta thinkt.SessionMeta
	s.applyTo(&meta)
	if !reflect.DeepEqual(s.cached(meta).RelatedPaths, want) {
		t.Fatalf("cached %+v", s.cached(meta).RelatedPaths)
	}
}

// A rollout without harness items falls back to its wire calls.
func TestScanRecordsRelatedFromWireCalls(t *testing.T) {
	root := t.TempDir()
	proj, other := mkRepo(t, root, "p"), mkRepo(t, root, "o")
	path := filepath.Join(root, "r.jsonl")
	writeLines(t, path, append(relatedHead(proj), wireExec(other), wirePatch(filepath.Join(other, "a.go")))...)
	s := scanSessionFile(path, thinkt.NewRelatedTracker(proj, nil))
	want := []thinkt.RelatedPath{{Path: other, Reasons: []string{"command", "edited"}, Count: 2}}
	if !reflect.DeepEqual(s.Related, want) {
		t.Fatalf("%+v", s.Related)
	}
}

// A turn that moved folders resolves relative edits there.
func TestScanFollowsTurnCwd(t *testing.T) {
	root := t.TempDir()
	proj, other := mkRepo(t, root, "p"), mkRepo(t, root, "o")
	path := filepath.Join(root, "r.jsonl")
	writeLines(t, path, append(relatedHead(proj),
		fmt.Sprintf(`{"timestamp":"2026-10-01T00:00:01Z","type":"turn_context","payload":{"cwd":%q}}`, other),
		changeItem("rel.go"))...)
	s := scanSessionFile(path, thinkt.NewRelatedTracker(proj, nil))
	if len(s.Related) != 1 || s.Related[0].Path != other {
		t.Fatalf("%+v", s.Related)
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
