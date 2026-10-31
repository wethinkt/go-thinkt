package thinkt

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var testTools = ToolPaths{
	"read":  {Arg: "path", Reason: RelatedRead},
	"edit":  {Arg: "path", Reason: RelatedEdited},
	"shell": {Arg: "cwd", Reason: RelatedCommand},
}

// mkRepo creates root/name with a .git directory and returns its path.
func mkRepo(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestUnquotePath(t *testing.T) {
	for in, want := range map[string]string{`"/w/a"`: "/w/a", "/w/a": "/w/a", `""`: "", ` "/w/b" `: "/w/b"} {
		if UnquotePath(in) != want {
			t.Errorf("%q → %q", in, UnquotePath(in))
		}
	}
}

func TestRepoRoot(t *testing.T) {
	root := t.TempDir()
	repo := mkRepo(t, root, "r")
	if got := repoRoot(filepath.Join(repo, "gone", "deleted.go")); got != repo {
		t.Fatalf("deleted file in repo: %q", got)
	}
	if got := repoRoot(repo); got != repo {
		t.Fatalf("repo itself: %q", got)
	}
	if got := repoRoot(filepath.Join(root, "plain", "x.go")); got != "" {
		t.Fatalf("no repo: %q", got)
	}
}

func TestRelatedTrackerFoldsToRepoRoots(t *testing.T) {
	root := t.TempDir()
	proj, other := mkRepo(t, root, "ds4dg"), mkRepo(t, root, "ds4-go")
	r := NewRelatedTracker(proj, []string{filepath.Join(root, "home")})
	r.Workspace(other)
	r.Call(testTools, "edit", map[string]any{"path": filepath.Join(other, "x", "a.go")}, "")
	r.Call(testTools, "shell", map[string]any{"cwd": `"` + other + `"`}, "")
	r.Call(testTools, "read", map[string]any{"path": filepath.Join(proj, "b.go")}, "") // own project: ignored
	want := []RelatedPath{{Path: other, Reasons: []string{"command", "edited", "workspace"}, Count: 2}}
	if got := r.Paths(); !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
}

func TestRelatedTrackerSortsByCountThenPath(t *testing.T) {
	root := t.TempDir()
	proj, a, b := mkRepo(t, root, "p"), mkRepo(t, root, "a"), mkRepo(t, root, "b")
	r := NewRelatedTracker(proj, nil)
	r.Call(testTools, "read", map[string]any{"path": filepath.Join(a, "1.go")}, "")
	r.Call(testTools, "read", map[string]any{"path": b}, "")
	r.Call(testTools, "edit", map[string]any{"path": filepath.Join(b, "x")}, "")
	r.Call(testTools, "other", map[string]any{"path": filepath.Join(a, "2.go")}, "")
	got := r.Paths()
	if len(got) != 2 || got[0].Path != b || got[0].Count != 2 || got[1].Path != a || got[1].Count != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestRelatedTrackerSkipsMissingAndExcluded(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	tmpRepo := mkRepo(t, root, "scratch") // a real repo, so only the exclude prefix can drop it
	r := NewRelatedTracker(mkRepo(t, root, "p"), []string{home, os.TempDir()})
	r.Touch("/no/such/repo/a.go", "", RelatedRead)
	r.Touch(filepath.Join(tmpRepo, "x.go"), "", RelatedRead)
	r.Touch(filepath.Join(home, "brain", "plan.md"), "", RelatedEdited)
	if got := r.Paths(); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestRelatedTrackerResolvesRelativePaths(t *testing.T) {
	root := t.TempDir()
	proj, other := mkRepo(t, root, "p"), mkRepo(t, root, "o")
	r := NewRelatedTracker(proj, nil)
	r.Touch("../o/a.go", proj, RelatedRead)
	r.Touch("a.go", other, RelatedEdited)
	r.Touch("b.go", proj, RelatedRead) // relative inside the project
	r.Touch("c.go", "", RelatedRead)   // joins the project
	want := []RelatedPath{{Path: other, Reasons: []string{"edited", "read"}, Count: 2}}
	if got := r.Paths(); !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
}

func TestRelatedTrackerDropsRelativeWithNoProject(t *testing.T) {
	r := NewRelatedTracker("", nil)
	r.Touch("c.go", "", RelatedRead)
	if got := r.Paths(); got != nil {
		t.Fatalf("%+v", got)
	}
}

func TestRelatedTrackerParentRepoOfProject(t *testing.T) {
	root := t.TempDir()
	outer := mkRepo(t, root, "outer")
	proj := mkRepo(t, outer, "inner")
	r := NewRelatedTracker(proj, nil)
	r.Touch(filepath.Join(outer, "a.go"), "", RelatedRead)
	if got := r.Paths(); got != nil {
		t.Fatalf("a repo containing the project: %+v", got)
	}
}

func TestRelatedTrackerSafeWhenNil(t *testing.T) {
	var r *RelatedTracker
	r.Touch("/a", "", RelatedRead)
	r.Workspace("/a")
	r.Call(testTools, "read", map[string]any{"path": "/a"}, "")
	if r.Paths() != nil {
		t.Fatal("nil tracker has paths")
	}
}

func TestTrackRelatedNeedsAProject(t *testing.T) {
	if TrackRelated("", nil) != nil {
		t.Fatal("no project, but a tracker")
	}
	r := TrackRelated("/p", nil)
	if r == nil {
		t.Fatal("no tracker")
	}
}

func TestRelatedTrackerBlocks(t *testing.T) {
	root := t.TempDir()
	other := mkRepo(t, root, "o")
	r := NewRelatedTracker(mkRepo(t, root, "p"), nil)
	r.Blocks(testTools, []ContentBlock{
		{Type: "text", Text: "hi"},
		{Type: "tool_use", ToolName: "read", ToolInput: map[string]any{"path": filepath.Join(other, "a")}},
		{Type: "tool_use", ToolName: "read", ToolInput: "not a map"},
	}, "")
	if got := r.Paths(); len(got) != 1 || got[0].Count != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestRelatedExclude(t *testing.T) {
	got := RelatedExclude("/h")
	if got[0] != "/h" || len(got) != 4 {
		t.Fatalf("%v", got)
	}
}

// The repo lookup asks the disk once per directory.
func TestRelatedTrackerMemoisesRepoLookups(t *testing.T) {
	root := t.TempDir()
	other := mkRepo(t, root, "o")
	r := NewRelatedTracker(mkRepo(t, root, "p"), nil)
	r.Touch(filepath.Join(other, "a.go"), "", RelatedRead)
	if err := os.RemoveAll(filepath.Join(other, ".git")); err != nil {
		t.Fatal(err)
	}
	r.Touch(filepath.Join(other, "b.go"), "", RelatedRead)
	if got := r.Paths(); len(got) != 1 || got[0].Count != 2 {
		t.Fatalf("%+v", got)
	}
}
