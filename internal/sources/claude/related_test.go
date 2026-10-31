package claude

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
	assistant := func(cwd, name, input string) string {
		return fmt.Sprintf(`{"type":"assistant","cwd":%q,"message":{"role":"assistant","model":"claude-opus-4","content":[{"type":"tool_use","id":"t","name":%q,"input":%s}]}}`, cwd, name, input)
	}
	content := `{"type":"user","cwd":"` + proj + `","message":{"role":"user","content":"go"}}` + "\n" +
		assistant(proj, "Read", fmt.Sprintf(`{"file_path":%q}`, filepath.Join(other, "a.go"))) + "\n" +
		assistant(proj, "Edit", fmt.Sprintf(`{"file_path":%q}`, filepath.Join(other, "b.go"))) + "\n" +
		assistant(other, "Bash", `{"command":"make"}`) + "\n" +
		assistant(other, "Read", `{"file_path":"rel.go"}`) + "\n" +
		assistant(proj, "Bash", `{"command":"ls"}`) + "\n" +
		assistant(proj, "Read", fmt.Sprintf(`{"file_path":%q}`, filepath.Join(proj, "own.go"))) + "\n"
	path := writeTempSession(t, root, "s.jsonl", content)
	// Claude's session metadata holds the storage folder, so the scan takes
	// the project from the session's first cwd.
	s := scanSessionFile(path, thinkt.NewRelatedTracker("", nil))
	want := []thinkt.RelatedPath{{Path: other, Reasons: []string{"command", "edited", "read"}, Count: 4}}
	if !reflect.DeepEqual(s.Related, want) {
		t.Fatalf("%+v", s.Related)
	}
	var meta thinkt.SessionMeta
	s.applyTo(&meta)
	if !reflect.DeepEqual(s.cached(meta).RelatedPaths, want) {
		t.Fatalf("cached %+v", s.cached(meta).RelatedPaths)
	}
}
