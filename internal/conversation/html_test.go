package conversation

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

var update = flag.Bool("update", false, "update conversation HTML fixture")

func TestHTMLGolden(t *testing.T) {
	doc, err := Build(sampleEntries(t), BuildOptions{SessionID: "demo-session", Title: "A conversation, in terminal and web"})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := WritePage(&out, doc, View{}); err != nil {
		t.Fatal(err)
	}
	const path = "testdata/conversation.html"
	if *update {
		if err := os.WriteFile(path, out.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatal("HTML contract changed; review with go test ./internal/conversation -run TestHTMLGolden -update")
	}
}

func TestHTMLDoesNotActivateContent(t *testing.T) {
	text := "<script>alert(1)</script>\n\n[bad](javascript:alert(1))\n\n![image](https://example.invalid/private.png)"
	doc, err := Build([]thinkt.Entry{{Role: thinkt.RoleAssistant, Text: text},
		{Role: thinkt.RoleTool, ContentBlocks: []thinkt.ContentBlock{{Type: "tool_result", ToolResult: "<img src=x onerror=alert(1)>"}}}},
		BuildOptions{Title: "<script>title</script>", SessionID: "\" data-evil=\"yes"})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := WritePage(&out, doc, View{}); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"<script", "<img", "javascript:", "https://example.invalid/private.png", "fixture-image-payload", "data-evil=\"yes"} {
		if strings.Contains(out.String(), forbidden) {
			t.Errorf("HTML contains active or remote content %q", forbidden)
		}
	}
	if !strings.Contains(out.String(), "[Image: image]") || !strings.Contains(out.String(), "&lt;img") {
		t.Fatal("image placeholder or literal tool output lost")
	}
}

func TestViewDoesNotMutateDocument(t *testing.T) {
	doc, err := Build(sampleEntries(t), BuildOptions{SessionID: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	var before, filtered, after bytes.Buffer
	if err := WriteHTML(&before, doc, View{}); err != nil {
		t.Fatal(err)
	}
	view := View{HiddenRoles: map[thinkt.Role]bool{thinkt.RoleUser: true}, HiddenKinds: map[string]bool{"image": true}, Collapsed: map[string]bool{doc.Messages[1].Blocks[0].ID: true}}
	if err := WriteHTML(&filtered, doc, view); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(filtered.String(), doc.Messages[0].ID) || strings.Contains(filtered.String(), "data-media-id") || !strings.Contains(filtered.String(), "Thinking (collapsed)") {
		t.Fatal("view policy not applied")
	}
	if !strings.Contains(filtered.String(), doc.Messages[1].ID) {
		t.Fatal("filter changed surviving message identity")
	}
	if err := WriteHTML(&after, doc, View{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before.Bytes(), after.Bytes()) {
		t.Fatal("view mutated the document")
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestHTMLPropagatesWriteError(t *testing.T) {
	want := errors.New("output failed")
	if err := WritePage(failingWriter{want}, &Document{}, View{}); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
