package claude

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

func TestParser_ReadSession(t *testing.T) {
	jsonl := `{"type":"user","uuid":"1","sessionId":"sess-123","version":"2.1.16","gitBranch":"main","cwd":"/home/user/project","timestamp":"2026-01-24T10:00:00Z","message":{"role":"user","content":"hello"}}
{"type":"assistant","uuid":"2","sessionId":"sess-123","timestamp":"2026-01-24T10:00:05Z","message":{"role":"assistant","model":"claude-opus-4-5-20251101","content":[{"type":"text","text":"hi there"}]}}
{"type":"user","uuid":"3","sessionId":"sess-123","timestamp":"2026-01-24T10:00:10Z","message":{"role":"user","content":"goodbye"}}
`

	parser := NewParser(strings.NewReader(jsonl))
	session, err := parser.ReadSession("/fake/path.jsonl")
	if err != nil {
		t.Fatalf("ReadSession() error = %v", err)
	}

	if session.ID != "sess-123" {
		t.Errorf("session.ID = %q, want %q", session.ID, "sess-123")
	}
	if session.Branch != "main" {
		t.Errorf("session.Branch = %q, want %q", session.Branch, "main")
	}
	if session.Version != "2.1.16" {
		t.Errorf("session.Version = %q, want %q", session.Version, "2.1.16")
	}
	if session.Model != "claude-opus-4-5-20251101" {
		t.Errorf("session.Model = %q, want %q", session.Model, "claude-opus-4-5-20251101")
	}
	if len(session.Entries) != 3 {
		t.Errorf("len(session.Entries) = %d, want 3", len(session.Entries))
	}

	prompts := session.UserPrompts()
	if len(prompts) != 2 {
		t.Errorf("len(session.UserPrompts()) = %d, want 2", len(prompts))
	}
}

func TestEntry_GetThinkingBlocks(t *testing.T) {
	jsonl := `{"type":"assistant","uuid":"1","timestamp":"2026-01-24T10:00:00Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"let me think..."},{"type":"text","text":"response"}]}}`

	parser := NewParser(strings.NewReader(jsonl))
	entry, err := parser.NextEntry()
	if err != nil {
		t.Fatalf("NextEntry() error = %v", err)
	}

	thinking := entry.GetThinkingBlocks()
	if len(thinking) != 1 {
		t.Fatalf("GetThinkingBlocks() got %d blocks, want 1", len(thinking))
	}
	if thinking[0] != "let me think..." {
		t.Errorf("thinking[0] = %q, want %q", thinking[0], "let me think...")
	}
}

func TestEntry_GetToolCalls(t *testing.T) {
	jsonl := `{"type":"assistant","uuid":"1","timestamp":"2026-01-24T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tool-1","name":"Read","input":{"file":"/tmp/test"}}]}}`

	parser := NewParser(strings.NewReader(jsonl))
	entry, err := parser.NextEntry()
	if err != nil {
		t.Fatalf("NextEntry() error = %v", err)
	}

	tools := entry.GetToolCalls()
	if len(tools) != 1 {
		t.Fatalf("GetToolCalls() got %d tools, want 1", len(tools))
	}
	if tools[0].Name != "Read" {
		t.Errorf("tools[0].Name = %q, want %q", tools[0].Name, "Read")
	}
}
