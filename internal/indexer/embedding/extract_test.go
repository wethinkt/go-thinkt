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

// --- ExtractTiered tests ---

func TestExtractTiered_UserEntry(t *testing.T) {
	entry := thinkt.Entry{
		Role: thinkt.RoleUser,
		Text: "How do I fix auth timeouts?",
	}
	results := embedding.ExtractTiered(entry)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Tier != embedding.TierConversation {
		t.Fatalf("expected conversation tier, got %q", results[0].Tier)
	}
	if results[0].Text != "[user] How do I fix auth timeouts?" {
		t.Fatalf("unexpected text: %q", results[0].Text)
	}
}

func TestExtractTiered_AssistantWithThinking(t *testing.T) {
	entry := thinkt.Entry{
		Role: thinkt.RoleAssistant,
		ContentBlocks: []thinkt.ContentBlock{
			{Type: "thinking", Thinking: "Let me analyze the auth code..."},
			{Type: "text", Text: "Here is the fix."},
			{Type: "tool_use", ToolName: "Read", ToolInput: "file.go"},
		},
	}
	results := embedding.ExtractTiered(entry)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(results), results)
	}

	// Conversation tier first (text blocks)
	if results[0].Tier != embedding.TierConversation {
		t.Fatalf("expected conversation tier first, got %q", results[0].Tier)
	}
	if results[0].Text != "[assistant] Here is the fix." {
		t.Fatalf("unexpected conversation text: %q", results[0].Text)
	}

	// Reasoning tier second (thinking blocks)
	if results[1].Tier != embedding.TierReasoning {
		t.Fatalf("expected reasoning tier second, got %q", results[1].Tier)
	}
	if results[1].Text != "[thinking] Let me analyze the auth code..." {
		t.Fatalf("unexpected reasoning text: %q", results[1].Text)
	}
}

func TestExtractTiered_ToolResult(t *testing.T) {
	entry := thinkt.Entry{
		Role: thinkt.RoleTool,
		ContentBlocks: []thinkt.ContentBlock{
			{Type: "tool_result", ToolResult: "func main() { fmt.Println(\"hello\") }"},
		},
	}
	results := embedding.ExtractTiered(entry)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Tier != embedding.TierReasoning {
		t.Fatalf("expected reasoning tier, got %q", results[0].Tier)
	}
	if results[0].Text != "[tool_result] func main() { fmt.Println(\"hello\") }" {
		t.Fatalf("unexpected text: %q", results[0].Text)
	}
}

func TestExtractTiered_SkipsShort(t *testing.T) {
	entry := thinkt.Entry{Role: thinkt.RoleUser, Text: "ok"}
	results := embedding.ExtractTiered(entry)
	if len(results) != 0 {
		t.Fatalf("expected empty for short text, got %d results", len(results))
	}
}

func TestExtractTiered_SkipsCheckpoints(t *testing.T) {
	entry := thinkt.Entry{Role: thinkt.RoleCheckpoint, Text: "checkpoint data that is long enough"}
	results := embedding.ExtractTiered(entry)
	if len(results) != 0 {
		t.Fatalf("expected empty for checkpoint, got %d results", len(results))
	}
}
