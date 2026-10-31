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
