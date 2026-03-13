package export

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func testEntries() []thinkt.Entry {
	ts := time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC)
	return []thinkt.Entry{
		{
			UUID:      "u1",
			Role:      thinkt.RoleUser,
			Timestamp: ts,
			Text:      "Fix the bug in main.go",
		},
		{
			UUID:      "a1",
			Role:      thinkt.RoleAssistant,
			Timestamp: ts.Add(time.Second),
			ContentBlocks: []thinkt.ContentBlock{
				{Type: "thinking", Thinking: "Let me analyze the code..."},
				{Type: "text", Text: "I found the issue. Here's the fix:"},
				{Type: "tool_use", ToolUseID: "t1", ToolName: "Read", ToolInput: map[string]any{"file_path": "main.go"}},
				{Type: "tool_result", ToolUseID: "t1", ToolResult: "package main\n\nfunc main() {}"},
				{Type: "text", Text: "The file looks good."},
			},
		},
	}
}

func TestExportMarkdown(t *testing.T) {
	var buf bytes.Buffer
	opts := Options{
		Title:              "Test Session",
		IncludeThinking:    true,
		IncludeToolUse:     true,
		IncludeToolResults: true,
		IncludeMedia:       true,
	}
	if err := ExportMarkdown(&buf, testEntries(), opts); err != nil {
		t.Fatal(err)
	}
	md := buf.String()

	checks := []struct{ name, substr string }{
		{"title", "# Test Session"},
		{"user heading", "## User"},
		{"user text", "Fix the bug in main.go"},
		{"thinking block", "<details>"},
		{"thinking label", "<summary>Thinking</summary>"},
		{"thinking content", "Let me analyze the code"},
		{"assistant text", "I found the issue"},
		{"tool use", "Tool: Read"},
		{"tool summary", "main.go"},
		{"tool input json", "file_path"},
		{"tool result", "package main"},
		{"second text block", "The file looks good"},
		{"separator", "---"},
		{"footer", "Exported from thinkt"},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(md, c.substr) {
				t.Errorf("markdown missing %q:\n%s", c.substr, md)
			}
		})
	}
}

func TestExportHTML(t *testing.T) {
	var buf bytes.Buffer
	opts := Options{
		Title:              "Test Session",
		IncludeThinking:    true,
		IncludeToolUse:     true,
		IncludeToolResults: true,
		IncludeMedia:       true,
	}
	if err := ExportHTML(&buf, testEntries(), opts); err != nil {
		t.Fatal(err)
	}
	html := buf.String()

	checks := []struct{ name, substr string }{
		{"doctype", "<!DOCTYPE html>"},
		{"title tag", "<title>Test Session</title>"},
		{"style tag", "<style>"},
		{"css content", ".entry"},
		{"role class", "role-user"},
		{"user text", "Fix the bug in main.go"},
		{"thinking block", "thinking-header"},
		{"tool name", "Read"},
		{"footer", "Exported from thinkt"},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(html, c.substr) {
				t.Errorf("HTML missing %q", c.substr)
			}
		})
	}
}

func TestExportMarkdownEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := ExportMarkdown(&buf, nil, Options{Title: "Empty"}); err != nil {
		t.Fatal(err)
	}
	md := buf.String()
	if !strings.Contains(md, "# Empty") {
		t.Error("expected title in empty export")
	}
}
