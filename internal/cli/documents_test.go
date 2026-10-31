package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func sampleDocs() []thinkt.AgentDocument {
	return []thinkt.AgentDocument{{
		Name: "plan.md", Kind: thinkt.DocPlan, Summary: "the plan", Final: "final text",
		Versions: []thinkt.DocumentVersion{
			{Number: 1, Tool: "write", Content: "one"},
			{Number: 2, Tool: "edit", Content: "two", Partial: true},
		},
	}, {Name: "task.md", Kind: thinkt.DocTask, Final: "tasks"}}
}

func TestFormatDocuments(t *testing.T) {
	var b bytes.Buffer
	FormatDocuments(&b, sampleDocs())
	out := b.String()
	for _, want := range []string{"plan.md", "plan", "2 versions", "the plan", "task.md", "0 versions"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	b.Reset()
	FormatDocuments(&b, nil)
	if !strings.Contains(b.String(), "No documents") {
		t.Fatalf("%q", b.String())
	}
}

func TestDocumentText(t *testing.T) {
	docs := sampleDocs()
	for _, tt := range []struct {
		name    string
		version int
		want    string
		fails   bool
	}{
		{"plan.md", 0, "final text", false},
		{"plan", 0, "final text", false},
		{"plan.md", 2, "two", false},
		{"plan.md", 3, "", true},
		{"nope", 0, "", true},
	} {
		got, err := DocumentText(docs, tt.name, tt.version)
		if (err != nil) != tt.fails || got != tt.want {
			t.Errorf("%+v: %q %v", tt, got, err)
		}
	}
}
