package thinkt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var docTools = map[string]DocTool{
	"write": {Path: "path", Content: "content"},
	"edit":  {Path: "path", Old: "old", New: "new", All: "all"},
	"multi": {Path: "path", Chunks: "chunks", Old: "old", New: "new", All: "all"},
}

var quotedTools = map[string]DocTool{
	"write": {Path: "path", Content: "content", Quoted: true},
	"edit":  {Path: "path", Old: "old", New: "new", Quoted: true},
}

func allDocs(string) bool { return true }

func call(uuid, tool string, args map[string]any) *Entry {
	return &Entry{UUID: uuid, Timestamp: time.Unix(100, 0), ContentBlocks: []ContentBlock{{Type: "tool_use", ToolName: tool, ToolInput: args}}}
}

func TestDocumentTrackerRebuildsVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.md")
	d := NewDocumentTracker(docTools, allDocs)
	d.Entry(call("e1", "write", map[string]any{"path": path, "content": "one two one"}))
	d.Entry(call("e2", "edit", map[string]any{"path": path, "old": "one", "new": "1"}))
	d.Entry(call("e3", "edit", map[string]any{"path": path, "old": "1", "new": "x", "all": true}))
	d.Entry(call("e4", "multi", map[string]any{"path": path, "chunks": []any{
		map[string]any{"old": "x", "new": "y"}, map[string]any{"old": "two", "new": "2"},
	}}))
	docs := d.Documents()
	if len(docs) != 1 || docs[0].Name != "plan.md" || docs[0].Path != path {
		t.Fatalf("%+v", docs)
	}
	want := []string{"one two one", "1 two one", "x two one", "y 2 one"}
	vs := docs[0].Versions
	if len(vs) != len(want) {
		t.Fatalf("%+v", vs)
	}
	for i, w := range want {
		if vs[i].Content != w || vs[i].Number != i+1 || vs[i].Partial {
			t.Fatalf("version %d: %+v", i, vs[i])
		}
	}
	if vs[1].EntryUUID != "e2" || vs[1].Tool != "edit" || vs[1].Timestamp.IsZero() {
		t.Fatalf("%+v", vs[1])
	}
}

func TestDocumentTrackerMarksMissingHistoryPartial(t *testing.T) {
	d := NewDocumentTracker(docTools, allDocs)
	d.Entry(call("e1", "edit", map[string]any{"path": "/d/a.md", "old": "a", "new": "b"})) // no base
	d.Entry(call("e2", "write", map[string]any{"path": "/d/b.md", "content": "text"}))
	d.Entry(call("e3", "edit", map[string]any{"path": "/d/b.md", "old": "absent", "new": "b"}))
	d.Entry(call("e4", "edit", map[string]any{"path": "/d/b.md", "old": "text", "new": "done"}))
	docs := d.Documents()
	if len(docs) != 2 {
		t.Fatalf("%+v", docs)
	}
	if v := docs[0].Versions[0]; !v.Partial || v.Content != "" {
		t.Fatalf("%+v", v)
	}
	vs := docs[1].Versions
	if !vs[1].Partial || vs[1].Content != "text" || vs[2].Partial || vs[2].Content != "done" {
		t.Fatalf("%+v", vs)
	}
}

func TestDocumentTrackerSkipsOtherPathsAndTools(t *testing.T) {
	d := NewDocumentTracker(docTools, func(p string) bool { return strings.HasSuffix(p, ".md") })
	d.Entry(call("e1", "write", map[string]any{"path": "/d/a.go", "content": "x"}))
	d.Entry(call("e2", "other", map[string]any{"path": "/d/a.md", "content": "x"}))
	d.Entry(&Entry{ContentBlocks: []ContentBlock{{Type: "text", Text: "hi"}}})
	if got := d.Documents(); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestDocumentTrackerUnquotesPaths(t *testing.T) {
	d := NewDocumentTracker(docTools, allDocs)
	d.Entry(call("e1", "write", map[string]any{"path": `"/d/a.md"`, "content": "x"}))
	if got := d.Documents(); len(got) != 1 || got[0].Path != "/d/a.md" {
		t.Fatalf("%+v", got)
	}
}

// Some sources store text arguments JSON-quoted, as they do paths.
func TestDocumentTrackerUnquotesText(t *testing.T) {
	d := NewDocumentTracker(quotedTools, allDocs)
	d.Entry(call("e1", "write", map[string]any{"path": "/d/a.md", "content": `"line one\nline two"`}))
	d.Entry(call("e2", "edit", map[string]any{"path": "/d/a.md", "old": `"line two"`, "new": `"line 2"`}))
	vs := d.Documents()[0].Versions
	if vs[0].Content != "line one\nline two" || vs[1].Content != "line one\nline 2" || vs[1].Partial {
		t.Fatalf("%+v", vs)
	}
}

func TestDocumentTrackerAcceptsChunksAsJSON(t *testing.T) {
	d := NewDocumentTracker(docTools, allDocs)
	d.Entry(call("e1", "write", map[string]any{"path": "/d/a.md", "content": "a"}))
	d.Entry(call("e2", "multi", map[string]any{"path": "/d/a.md", "chunks": `[{"old":"a","new":"b"}]`}))
	if v := d.Documents()[0].Versions[1]; v.Content != "b" || v.Partial {
		t.Fatalf("%+v", v)
	}
}

func TestDocumentTrackerNote(t *testing.T) {
	d := NewDocumentTracker(nil, nil)
	ts := time.Unix(200, 0)
	d.Note("plan", DocPlan, &Entry{UUID: "e1", Timestamp: ts}, "ExitPlanMode", "first")
	d.Note("plan", DocPlan, &Entry{UUID: "e2", Timestamp: ts}, "ExitPlanMode", "second")
	docs := d.Documents()
	if len(docs) != 1 || docs[0].Kind != DocPlan || docs[0].Final != "second" || !docs[0].UpdatedAt.Equal(ts) {
		t.Fatalf("%+v", docs)
	}
	if len(docs[0].Versions) != 2 || docs[0].Versions[1].Number != 2 {
		t.Fatalf("%+v", docs[0].Versions)
	}
}

func TestDocumentTrackerFinalIsTheFileOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.md")
	if err := os.WriteFile(path, []byte("on disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := NewDocumentTracker(docTools, allDocs)
	d.Entry(call("e1", "write", map[string]any{"path": path, "content": "older"}))
	docs := d.Documents()
	if docs[0].Final != "on disk" || docs[0].Versions[0].Content != "older" {
		t.Fatalf("%+v", docs[0])
	}
}

func TestDocumentTrackerFinalFallsBackToLastVersion(t *testing.T) {
	d := NewDocumentTracker(docTools, allDocs)
	d.Entry(call("e1", "write", map[string]any{"path": "/no/such/a.md", "content": "last"}))
	if docs := d.Documents(); docs[0].Final != "last" {
		t.Fatalf("%+v", docs[0])
	}
}

func TestDocumentKind(t *testing.T) {
	for in, want := range map[string]string{
		"implementation_plan.md": DocPlan, "roadmap.md": DocPlan, "task.md": DocTask,
		"walkthrough.md": DocWalkthrough, "notes.md": DocOther,
	} {
		if got := DocumentKind(in); got != want {
			t.Errorf("%s: %s", in, got)
		}
	}
}

func TestDocumentTrackerKeepsQuotesUnlessToldOtherwise(t *testing.T) {
	d := NewDocumentTracker(docTools, allDocs)
	d.Entry(call("e1", "write", map[string]any{"path": "/d/a.md", "content": `"quoted"`}))
	if v := d.Documents()[0].Versions[0]; v.Content != `"quoted"` {
		t.Fatalf("%+v", v)
	}
}

func TestDocumentTrackerAllAcceptsStringsAndChunks(t *testing.T) {
	d := NewDocumentTracker(docTools, allDocs)
	d.Entry(call("e1", "write", map[string]any{"path": "/d/a.md", "content": "x x"}))
	d.Entry(call("e2", "edit", map[string]any{"path": "/d/a.md", "old": "x", "new": "y", "all": "true"}))
	d.Entry(call("e3", "multi", map[string]any{"path": "/d/a.md", "chunks": []any{
		map[string]any{"old": "y", "new": "z", "all": true},
	}}))
	vs := d.Documents()[0].Versions
	if vs[1].Content != "y y" || vs[2].Content != "z z" {
		t.Fatalf("%+v", vs)
	}
}

func TestDocumentTrackerReadsOnlyRegularFiles(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "a.md")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	d := NewDocumentTracker(docTools, allDocs)
	d.Entry(call("e1", "write", map[string]any{"path": link, "content": "known"}))
	d.Entry(call("e2", "write", map[string]any{"path": dir, "content": "dir"}))
	for _, doc := range d.Documents() {
		if doc.Final == "secret" || doc.Final == "" {
			t.Fatalf("%+v", doc)
		}
	}
}

type nilReaderStore struct{ Store }

func (nilReaderStore) OpenSession(context.Context, string) (SessionReader, error) { return nil, nil }

func TestVisitEntriesUnknownSession(t *testing.T) {
	err := VisitEntries(context.Background(), nilReaderStore{}, "nope", func(*Entry) {})
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("%v", err)
	}
}
