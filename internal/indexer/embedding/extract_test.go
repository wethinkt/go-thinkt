package embedding_test

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestExtractText_UserEntry(t *testing.T) {
	entry := thinkt.Entry{
		Role: thinkt.RoleUser,
		Text: "How do I fix auth timeouts?",
	}
	text := embedding.ExtractText(entry)
	if text != "How do I fix auth timeouts?" {
		t.Fatalf("unexpected: %q", text)
	}
}

func TestExtractText_AssistantWithBlocks(t *testing.T) {
	entry := thinkt.Entry{
		Role: thinkt.RoleAssistant,
		ContentBlocks: []thinkt.ContentBlock{
			{Type: "thinking", Thinking: "Let me analyze..."},
			{Type: "text", Text: "Here is the fix."},
			{Type: "tool_use", ToolName: "Read", ToolInput: "file.go"},
		},
	}
	text := embedding.ExtractText(entry)
	if text != "Let me analyze...\nHere is the fix." {
		t.Fatalf("unexpected: %q", text)
	}
}

func TestExtractText_ToolResult(t *testing.T) {
	entry := thinkt.Entry{
		Role: thinkt.RoleTool,
		ContentBlocks: []thinkt.ContentBlock{
			{Type: "tool_result", ToolResult: "func main() { fmt.Println(\"hello\") }"},
		},
	}
	text := embedding.ExtractText(entry)
	if text != "func main() { fmt.Println(\"hello\") }" {
		t.Fatalf("unexpected: %q", text)
	}
}

func TestExtractText_SkipsShort(t *testing.T) {
	entry := thinkt.Entry{Role: thinkt.RoleUser, Text: "ok"}
	text := embedding.ExtractText(entry)
	if text != "" {
		t.Fatalf("expected empty for short text, got: %q", text)
	}
}

func TestExtractText_SkipsCheckpoints(t *testing.T) {
	entry := thinkt.Entry{Role: thinkt.RoleCheckpoint, Text: "checkpoint data..."}
	text := embedding.ExtractText(entry)
	if text != "" {
		t.Fatalf("expected empty for checkpoint, got: %q", text)
	}
}
