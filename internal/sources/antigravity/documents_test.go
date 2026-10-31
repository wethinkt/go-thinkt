package antigravity

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestSessionDocumentsReadsBrainFiles(t *testing.T) {
	home, _, _ := relatedHome(t)
	brain := filepath.Join(home, "brain", "11111111-aaaa")
	for name, body := range map[string]string{
		"task.md":               "- [x] done",
		"task.md.metadata.json": `{"artifactType":"ARTIFACT_TYPE_TASK","summary":"the tasks","updatedAt":"2026-05-23T12:45:29Z"}`,
		"notes.txt":             "not a document",
	} {
		if err := os.WriteFile(filepath.Join(brain, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := testStore(t, home, t.TempDir())
	docs, err := s.SessionDocuments(context.Background(), "11111111-aaaa")
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("%+v", docs)
	}
	d := docs[0]
	if d.Name != "task.md" || d.Kind != thinkt.DocTask || d.Summary != "the tasks" || d.Final != "- [x] done" || d.UpdatedAt.IsZero() {
		t.Fatalf("%+v", d)
	}
}

func TestDocumentToolsRebuildEdits(t *testing.T) {
	path := "/b/plan.md"
	d := thinkt.NewDocumentTracker(docTools, func(string) bool { return true })
	feed := func(tool string, args map[string]any) {
		d.Entry(&thinkt.Entry{UUID: tool, ContentBlocks: []thinkt.ContentBlock{{Type: "tool_use", ToolName: tool, ToolInput: args}}})
	}
	feed("write_to_file", map[string]any{"TargetFile": path, "CodeContent": "alpha beta gamma"})
	feed("replace_file_content", map[string]any{"TargetFile": path, "TargetContent": "alpha", "ReplacementContent": "one"})
	feed("multi_replace_file_content", map[string]any{"TargetFile": path, "ReplacementChunks": []any{
		map[string]any{"TargetContent": "beta", "ReplacementContent": "two"},
	}})
	feed("edit_file", map[string]any{"Path": path, "Target": "gamma", "Replacement": "three"})
	vs := d.Documents()[0].Versions
	if got := vs[len(vs)-1]; got.Content != "one two three" || got.Partial {
		t.Fatalf("%+v", vs)
	}
}

func TestDocumentToolsUnquoteText(t *testing.T) {
	d := thinkt.NewDocumentTracker(docTools, func(string) bool { return true })
	d.Entry(&thinkt.Entry{ContentBlocks: []thinkt.ContentBlock{{Type: "tool_use", ToolName: "write_to_file",
		ToolInput: map[string]any{"TargetFile": "/b/a.md", "CodeContent": `"one\ntwo"`}}}})
	if v := d.Documents()[0].Versions[0]; v.Content != "one\ntwo" {
		t.Fatalf("%+v", v)
	}
}
