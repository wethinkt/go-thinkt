package antigravity

import (
	"bytes"
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

func TestUnquotePath(t *testing.T) {
	for in, want := range map[string]string{`"/w/a"`: "/w/a", "/w/a": "/w/a", `""`: "", ` "/w/b" `: "/w/b"} {
		if unquotePath(in) != want {
			t.Errorf("%q → %q", in, unquotePath(in))
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
	proj, other := mkRepo(t, root, "ds4dg"), mkRepo(t, root, "ds4-go") // dirs with .git
	r := newRelatedTracker(proj, []string{filepath.Join(root, "home")})
	r.workspace(other)
	r.call("replace_file_content", map[string]any{"TargetFile": filepath.Join(other, "x", "a.go")})
	r.call("run_command", map[string]any{"Cwd": `"` + other + `"`})
	r.call("view_file", map[string]any{"AbsolutePath": filepath.Join(proj, "b.go")}) // own project: ignored
	got := r.paths()
	want := []thinkt.RelatedPath{{Path: other, Reasons: []string{"command", "edited", "workspace"}, Count: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
}

func TestRelatedTrackerSortsByCountThenPath(t *testing.T) {
	root := t.TempDir()
	proj, a, b := mkRepo(t, root, "p"), mkRepo(t, root, "a"), mkRepo(t, root, "b")
	r := newRelatedTracker(proj, nil)
	r.call("view_file", map[string]any{"AbsolutePath": filepath.Join(a, "1.go")})
	r.call("list_dir", map[string]any{"DirectoryPath": b})
	r.call("grep_search", map[string]any{"SearchPath": b})
	r.call("read_file", map[string]any{"Path": filepath.Join(b, "x")})
	r.call("unknown_tool", map[string]any{"Path": filepath.Join(a, "2.go")}) // not a path-bearing tool
	got := r.paths()
	if len(got) != 2 || got[0].Path != b || got[0].Count != 3 || got[1].Path != a || got[1].Count != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestRelatedFolderSkipsMissingTempAndHome(t *testing.T) {
	root := t.TempDir() // under os.TempDir()
	home := filepath.Join(root, "home")
	tmpRepo := mkRepo(t, root, "scratch") // a real repo, so only the exclude prefix can drop it
	r := newRelatedTracker(mkRepo(t, root, "p"), []string{home, os.TempDir()})
	r.call("view_file", map[string]any{"AbsolutePath": "/no/such/repo/a.go"})
	r.call("view_file", map[string]any{"AbsolutePath": filepath.Join(tmpRepo, "x.go")})
	r.call("write_to_file", map[string]any{"TargetFile": filepath.Join(home, "brain", "c1", "plan.md")})
	if got := r.paths(); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestParserTracksRelated(t *testing.T) {
	root := t.TempDir()
	proj, repoB := mkRepo(t, root, "ds4dg"), mkRepo(t, root, "ds4-go")
	data := bytes.ReplaceAll(fixtureBytes(t, "main.jsonl"), []byte("__REPO_B__"), []byte(repoB))
	p := NewParser(bytes.NewReader(data), "11111111-aaaa", "")
	p.TrackRelated(proj, nil)
	for _, err := p.NextEntry(); err == nil; _, err = p.NextEntry() {
	}
	// step 2: view_file + run_command, step 6: replace_file_content + grep_search
	want := []thinkt.RelatedPath{{Path: repoB, Reasons: []string{"command", "edited", "read"}, Count: 4}}
	if got := p.RelatedPaths(); !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
	if NewParser(bytes.NewReader(data), "c", "").RelatedPaths() != nil {
		t.Fatal("untracked parser has related paths")
	}
}
