package export

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestMarkdownDiffChild(t *testing.T) {
	entries := []thinkt.Entry{
		{UUID: "a1", Role: thinkt.RoleAssistant, ContentBlocks: []thinkt.ContentBlock{{Type: "tool_use", ToolUseID: "t1", ToolName: "Edit", ToolInput: map[string]any{"file_path": "/p/a.md"}}}},
		{UUID: "r1", Role: thinkt.RoleUser, ContentBlocks: []thinkt.ContentBlock{{
			Type: "tool_result", ToolUseID: "t1", ToolResult: "The file has been updated.",
			Children: []thinkt.ContentBlock{
				{Type: thinkt.BlockTypeDiff, Text: "--- a//p/a.md\n+++ b//p/a.md\n@@ -1,1 +1,1 @@\n-old\n+new\n"},
				{Type: "text", Text: "stderr noise"},
			}}}},
	}
	var out bytes.Buffer
	if err := ExportMarkdown(&out, entries, Options{}); err != nil {
		t.Fatal(err)
	}
	md := out.String()
	if !strings.Contains(md, "```diff\n--- a//p/a.md") {
		t.Fatalf("diff child must render as a diff fence:\n%s", md)
	}
	if !strings.Contains(md, "stderr noise") {
		t.Fatalf("text child must render:\n%s", md)
	}
}
