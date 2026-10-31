package gemini

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

func relatedCall(id, name, args string) string {
	return fmt.Sprintf(`{"id":%q,"timestamp":"2026-05-17T11:00:01Z","type":"gemini","content":"","toolCalls":[{"id":%q,"name":%q,"args":%s,"status":"success"}]}`, id, id+"c", name, args)
}

func TestScanRecordsRelatedPaths(t *testing.T) {
	root := t.TempDir()
	proj, other := mkRepo(t, root, "p"), mkRepo(t, root, "o")
	path := writeJSONL(t, hdr,
		relatedCall("g1", "read_file", fmt.Sprintf(`{"file_path":%q}`, filepath.Join(other, "a.go"))),
		relatedCall("g2", "replace", fmt.Sprintf(`{"file_path":%q}`, filepath.Join(other, "b.go"))),
		relatedCall("g3", "run_shell_command", fmt.Sprintf(`{"command":"make","dir_path":%q}`, other)),
		relatedCall("g4", "read_file", `{"file_path":"own.go"}`))
	s, err := scanJSONL(path, "session-x", "", thinkt.NewRelatedTracker(proj, nil))
	if err != nil || s == nil {
		t.Fatal(err)
	}
	want := []thinkt.RelatedPath{{Path: other, Reasons: []string{"command", "edited", "read"}, Count: 3}}
	if !reflect.DeepEqual(s.Meta.RelatedPaths, want) {
		t.Fatalf("%+v", s.Meta.RelatedPaths)
	}
}

// A call the session rewound away does not count.
func TestScanSkipsRewoundCalls(t *testing.T) {
	root := t.TempDir()
	proj, other := mkRepo(t, root, "p"), mkRepo(t, root, "o")
	path := writeJSONL(t, hdr,
		`{"id":"u1","timestamp":"2026-05-17T11:00:00Z","type":"user","content":"go"}`,
		relatedCall("g1", "read_file", fmt.Sprintf(`{"file_path":%q}`, filepath.Join(other, "a.go"))),
		`{"$rewindTo":"u1"}`)
	s, err := scanJSONL(path, "session-x", "", thinkt.NewRelatedTracker(proj, nil))
	if err != nil || s == nil {
		t.Fatal(err)
	}
	if s.Meta.RelatedPaths != nil {
		t.Fatalf("%+v", s.Meta.RelatedPaths)
	}
}

func TestRelatedPathsSurviveCache(t *testing.T) {
	root := t.TempDir()
	proj, other := mkRepo(t, root, "p"), mkRepo(t, root, "o")
	path := writeJSONL(t, hdr, relatedCall("g1", "read_file", fmt.Sprintf(`{"file_path":%q}`, filepath.Join(other, "a.go"))))
	mc, err := thinkt.LoadMetadataCache(thinkt.SourceGemini, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{}
	first, ok := s.jsonlFile(mc, path, "session-x", "", proj)
	if !ok || len(first.meta.RelatedPaths) != 1 {
		t.Fatalf("%+v", first.meta)
	}
	second, ok := s.jsonlFile(mc, path, "session-x", "", proj)
	if !ok || !reflect.DeepEqual(second.meta.RelatedPaths, first.meta.RelatedPaths) {
		t.Fatalf("%+v", second.meta)
	}
}

func TestOlderJSONSessionRecordsRelatedPaths(t *testing.T) {
	root := t.TempDir()
	proj, other := mkRepo(t, root, "p"), mkRepo(t, root, "o")
	path := filepath.Join(root, "session-old.json")
	body := fmt.Sprintf(`{"sessionId":"x","messages":[{"id":"g","timestamp":"2026-01-01T00:00:00Z","type":"gemini","content":"","toolCalls":[{"id":"c","name":"write_file","args":{"file_path":%q},"status":"success"}]}]}`, filepath.Join(other, "a.go"))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	s := &Store{}
	meta, _, err := s.readSessionMeta(path, info.Size(), info.ModTime(), "w", proj)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.RelatedPaths) != 1 || meta.RelatedPaths[0].Path != other {
		t.Fatalf("%+v", meta.RelatedPaths)
	}
}
