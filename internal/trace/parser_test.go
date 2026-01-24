package trace

import (
	"strings"
	"testing"
)

func TestParseUserContent_String(t *testing.T) {
	input := `"hello world"`
	got := ParseUserContent([]byte(input))
	want := "hello world"
	if got != want {
		t.Errorf("ParseUserContent(%q) = %q, want %q", input, got, want)
	}
}

func TestParseUserContent_TextBlocks(t *testing.T) {
	input := `[{"type":"text","text":"first"},{"type":"text","text":"second"}]`
	got := ParseUserContent([]byte(input))
	want := "first\nsecond"
	if got != want {
		t.Errorf("ParseUserContent(%q) = %q, want %q", input, got, want)
	}
}

func TestParseUserContent_ToolResult(t *testing.T) {
	input := `[{"type":"tool_result","tool_use_id":"123","content":"result"}]`
	got := ParseUserContent([]byte(input))
	want := "" // tool_result blocks should not return text
	if got != want {
		t.Errorf("ParseUserContent(%q) = %q, want %q", input, got, want)
	}
}

func TestParser_ReadAllPrompts(t *testing.T) {
	jsonl := `{"type":"file-history-snapshot","messageId":"abc"}
{"type":"user","uuid":"1","timestamp":"2026-01-24T10:00:00Z","message":{"role":"user","content":"first prompt"}}
{"type":"assistant","uuid":"2","timestamp":"2026-01-24T10:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"response"}]}}
{"type":"user","uuid":"3","timestamp":"2026-01-24T10:00:02Z","message":{"role":"user","content":"second prompt"}}
{"type":"user","uuid":"4","timestamp":"2026-01-24T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"xyz","content":"result"}]}}
`

	parser := NewParser(strings.NewReader(jsonl))
	prompts, err := parser.ReadAllPrompts()
	if err != nil {
		t.Fatalf("ReadAllPrompts() error = %v", err)
	}

	if len(prompts) != 2 {
		t.Errorf("ReadAllPrompts() got %d prompts, want 2", len(prompts))
	}

	if prompts[0].Text != "first prompt" {
		t.Errorf("prompts[0].Text = %q, want %q", prompts[0].Text, "first prompt")
	}

	if prompts[1].Text != "second prompt" {
		t.Errorf("prompts[1].Text = %q, want %q", prompts[1].Text, "second prompt")
	}
}

func TestParser_HandlesMalformedLines(t *testing.T) {
	jsonl := `{"type":"user","uuid":"1","timestamp":"2026-01-24T10:00:00Z","message":{"role":"user","content":"valid"}}
not valid json
{"type":"user","uuid":"2","timestamp":"2026-01-24T10:00:01Z","message":{"role":"user","content":"also valid"}}
`

	parser := NewParser(strings.NewReader(jsonl))
	prompts, err := parser.ReadAllPrompts()
	if err != nil {
		t.Fatalf("ReadAllPrompts() error = %v", err)
	}

	if len(prompts) != 2 {
		t.Errorf("ReadAllPrompts() got %d prompts, want 2", len(prompts))
	}

	errors := parser.Errors()
	if len(errors) != 1 {
		t.Errorf("Errors() got %d errors, want 1", len(errors))
	}
}
