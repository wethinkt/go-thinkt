// internal/target/filter_test.go
package target

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestFilterEntries_DefaultIncludesUserAndAssistant(t *testing.T) {
	entries := []thinkt.Entry{
		{Role: thinkt.RoleUser, Text: "hello"},
		{Role: thinkt.RoleAssistant, Text: "hi"},
		{Role: thinkt.RoleSystem, Text: "sys"},
	}
	result := FilterEntries(entries, DefaultFilter())
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if result[0].Role != thinkt.RoleUser || result[1].Role != thinkt.RoleAssistant {
		t.Fatalf("unexpected roles: %v, %v", result[0].Role, result[1].Role)
	}
}

func TestFilterEntries_SystemIncludedWhenEnabled(t *testing.T) {
	entries := []thinkt.Entry{
		{Role: thinkt.RoleUser, Text: "hello"},
		{Role: thinkt.RoleSystem, Text: "sys"},
	}
	f := DefaultFilter()
	f.IncludeSystem = true
	result := FilterEntries(entries, f)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
}

func TestFilterEntries_ToolRoleFollowsToolResults(t *testing.T) {
	entries := []thinkt.Entry{
		{Role: thinkt.RoleUser, Text: "hello"},
		{Role: thinkt.RoleTool, Text: "tool output"},
	}
	result := FilterEntries(entries, DefaultFilter())
	if len(result) != 2 {
		t.Fatalf("expected 2 entries (tool included by default), got %d", len(result))
	}

	f := DefaultFilter()
	f.IncludeToolResults = false
	result = FilterEntries(entries, f)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry (tool excluded), got %d", len(result))
	}
	if result[0].Role != thinkt.RoleUser {
		t.Fatalf("expected user, got %s", result[0].Role)
	}
}

func TestFilterEntries_SummaryProgressCheckpointExcluded(t *testing.T) {
	entries := []thinkt.Entry{
		{Role: thinkt.RoleUser, Text: "hello"},
		{Role: thinkt.RoleSummary, Text: "summary"},
		{Role: thinkt.RoleProgress, Text: "progress"},
		{Role: thinkt.RoleCheckpoint, Text: "checkpoint"},
	}
	result := FilterEntries(entries, DefaultFilter())
	if len(result) != 1 {
		t.Fatalf("expected 1 entry (only user), got %d", len(result))
	}
}

func TestFilterEntries_ThinkingBlocksFiltered(t *testing.T) {
	entries := []thinkt.Entry{
		{Role: thinkt.RoleAssistant, ContentBlocks: []thinkt.ContentBlock{
			{Type: "text", Text: "answer"},
			{Type: "thinking", Thinking: "hmm"},
		}},
	}
	f := DefaultFilter()
	f.IncludeThinking = false
	result := FilterEntries(entries, f)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if len(result[0].ContentBlocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result[0].ContentBlocks))
	}
	if result[0].ContentBlocks[0].Type != "text" {
		t.Fatalf("expected text block, got %s", result[0].ContentBlocks[0].Type)
	}
}

func TestFilterEntries_ToolUseAndResultsFiltered(t *testing.T) {
	entries := []thinkt.Entry{
		{Role: thinkt.RoleAssistant, ContentBlocks: []thinkt.ContentBlock{
			{Type: "text", Text: "answer"},
			{Type: "tool_use", ToolName: "Read"},
			{Type: "tool_result", ToolResult: "contents"},
		}},
	}
	f := DefaultFilter()
	f.IncludeToolUse = false
	f.IncludeToolResults = false
	result := FilterEntries(entries, f)
	if len(result[0].ContentBlocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result[0].ContentBlocks))
	}
}

func TestFilterEntries_MediaFiltered(t *testing.T) {
	entries := []thinkt.Entry{
		{Role: thinkt.RoleAssistant, ContentBlocks: []thinkt.ContentBlock{
			{Type: "text", Text: "see this"},
			{Type: "image", MediaType: "image/png"},
			{Type: "document", MediaType: "application/pdf"},
		}},
	}
	f := DefaultFilter()
	f.IncludeMedia = false
	result := FilterEntries(entries, f)
	if len(result[0].ContentBlocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result[0].ContentBlocks))
	}
}

func TestFilterEntries_NilAndEmpty(t *testing.T) {
	result := FilterEntries(nil, DefaultFilter())
	if result != nil {
		t.Fatalf("expected nil for nil input, got %v", result)
	}

	result = FilterEntries([]thinkt.Entry{}, DefaultFilter())
	if result != nil {
		t.Fatalf("expected nil for empty input, got %v", result)
	}
}
