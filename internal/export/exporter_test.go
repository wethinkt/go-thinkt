package export

import (
	"testing"
)

func TestParseRawEntry_AssistantWithThinking(t *testing.T) {
	// Real Claude JSONL structure: assistant entry with thinking block + text block.
	// Thinking and text never appear alongside tool_use in the same response.
	line := []byte(`{
		"uuid": "abc-123",
		"type": "assistant",
		"timestamp": "2025-06-01T12:00:00Z",
		"message": {
			"model": "claude-sonnet-4-20250514",
			"role": "assistant",
			"content": [
				{"type": "thinking", "thinking": "Let me analyze this problem carefully.", "signature": "sig..."},
				{"type": "text", "text": "Here is my answer."}
			],
			"stop_reason": "end_turn",
			"usage": {
				"input_tokens": 1500,
				"output_tokens": 42
			}
		}
	}`)

	entry, ok := parseRawEntry(line)
	if !ok {
		t.Fatal("parseRawEntry returned false")
	}

	if entry.Role != "assistant" {
		t.Errorf("Role = %q, want %q", entry.Role, "assistant")
	}
	if entry.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want %q", entry.Model, "claude-sonnet-4-20250514")
	}
	if entry.Text != "Here is my answer." {
		t.Errorf("Text = %q, want %q", entry.Text, "Here is my answer.")
	}
	if entry.InputTokens != 1500 {
		t.Errorf("InputTokens = %d, want %d", entry.InputTokens, 1500)
	}
	if entry.OutputTokens != 42 {
		t.Errorf("OutputTokens = %d, want %d", entry.OutputTokens, 42)
	}
	if !entry.HasThinking {
		t.Error("HasThinking = false, want true")
	}
	// "Let me analyze this problem carefully." = 38 bytes
	if entry.ThinkingLen != 38 {
		t.Errorf("ThinkingLen = %d, want %d", entry.ThinkingLen, 38)
	}
	if entry.HasToolUse {
		t.Error("HasToolUse = true, want false")
	}
	if entry.IsError {
		t.Error("IsError = true, want false")
	}
}

func TestParseRawEntry_AssistantWithToolUse(t *testing.T) {
	// Claude sends tool_use as content blocks inside an assistant message.
	// The top-level "type" is still "assistant" in the JSONL.
	line := []byte(`{
		"uuid": "def-456",
		"type": "assistant",
		"timestamp": "2025-06-01T12:01:00Z",
		"message": {
			"model": "claude-sonnet-4-20250514",
			"role": "assistant",
			"content": [
				{"type": "tool_use", "id": "toolu_abc", "name": "Read", "input": {"file_path": "/tmp/test.go"}}
			],
			"stop_reason": "tool_use",
			"usage": {
				"input_tokens": 800,
				"output_tokens": 25
			}
		}
	}`)

	entry, ok := parseRawEntry(line)
	if !ok {
		t.Fatal("parseRawEntry returned false")
	}

	if !entry.HasToolUse {
		t.Error("HasToolUse = false, want true")
	}
	if entry.ToolName != "Read" {
		t.Errorf("ToolName = %q, want %q", entry.ToolName, "Read")
	}
	if entry.HasThinking {
		t.Error("HasThinking = true, want false")
	}
	if entry.ThinkingLen != 0 {
		t.Errorf("ThinkingLen = %d, want 0", entry.ThinkingLen)
	}
}

func TestParseRawEntry_MultipleToolUseBlocks(t *testing.T) {
	// Claude can emit multiple tool_use blocks in one response (parallel tool calls).
	line := []byte(`{
		"uuid": "multi-tool-1",
		"type": "assistant",
		"timestamp": "2025-06-01T12:02:00Z",
		"message": {
			"model": "claude-sonnet-4-20250514",
			"role": "assistant",
			"content": [
				{"type": "tool_use", "id": "toolu_1", "name": "Read", "input": {"file_path": "/a.go"}},
				{"type": "tool_use", "id": "toolu_2", "name": "Glob", "input": {"pattern": "*.go"}}
			],
			"stop_reason": "tool_use",
			"usage": {"input_tokens": 100, "output_tokens": 50}
		}
	}`)

	entry, ok := parseRawEntry(line)
	if !ok {
		t.Fatal("parseRawEntry returned false")
	}

	if !entry.HasToolUse {
		t.Error("HasToolUse = false, want true")
	}
	// First tool_use block's name wins
	if entry.ToolName != "Read" {
		t.Errorf("ToolName = %q, want %q", entry.ToolName, "Read")
	}
}

func TestParseRawEntry_UserEntry(t *testing.T) {
	// Claude Code stores user messages with type "user".
	// Older formats may use "human" which should map to "user".
	tests := []struct {
		name string
		line []byte
		role string
	}{
		{
			name: "type=user",
			line: []byte(`{"uuid": "u1", "type": "user", "timestamp": "2025-06-01T12:00:00Z", "text": "hello"}`),
			role: "user",
		},
		{
			name: "type=human maps to user",
			line: []byte(`{"uuid": "u2", "type": "human", "timestamp": "2025-06-01T12:00:00Z", "text": "hello"}`),
			role: "user",
		},
		{
			name: "role field preferred over type",
			line: []byte(`{"uuid": "u3", "type": "ignored", "role": "user", "timestamp": "2025-06-01T12:00:00Z", "text": "hello"}`),
			role: "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := parseRawEntry(tt.line)
			if !ok {
				t.Fatal("parseRawEntry returned false")
			}
			if entry.Role != tt.role {
				t.Errorf("Role = %q, want %q", entry.Role, tt.role)
			}
		})
	}
}

func TestParseRawEntry_IsError(t *testing.T) {
	// Claude's JSONL uses camelCase "isError" on tool_result entries.
	line := []byte(`{
		"uuid": "err-1",
		"type": "tool_result",
		"timestamp": "2025-06-01T12:00:00Z",
		"isError": true,
		"text": "Error: file not found"
	}`)

	entry, ok := parseRawEntry(line)
	if !ok {
		t.Fatal("parseRawEntry returned false")
	}

	if !entry.IsError {
		t.Error("IsError = false, want true")
	}
	if entry.Text != "Error: file not found" {
		t.Errorf("Text = %q, want %q", entry.Text, "Error: file not found")
	}
}

func TestParseRawEntry_InvalidEntries(t *testing.T) {
	tests := []struct {
		name string
		line []byte
	}{
		{"empty json", []byte(`{}`)},
		{"no uuid", []byte(`{"type": "assistant"}`)},
		{"invalid role", []byte(`{"uuid": "x", "type": "progress"}`)},
		{"malformed json", []byte(`{not json}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := parseRawEntry(tt.line)
			if ok {
				t.Error("parseRawEntry returned true, want false")
			}
		})
	}
}

func TestParseRawEntry_SystemEntry(t *testing.T) {
	// System entries have type "system" and typically just text.
	line := []byte(`{
		"uuid": "sys-1",
		"type": "system",
		"timestamp": "2025-06-01T12:00:00Z",
		"text": "System prompt text"
	}`)

	entry, ok := parseRawEntry(line)
	if !ok {
		t.Fatal("parseRawEntry returned false")
	}

	if entry.Role != "system" {
		t.Errorf("Role = %q, want %q", entry.Role, "system")
	}
	if entry.Text != "System prompt text" {
		t.Errorf("Text = %q, want %q", entry.Text, "System prompt text")
	}
	if entry.HasThinking || entry.HasToolUse || entry.IsError {
		t.Error("classification flags should all be false for system entry")
	}
}

func TestExtractFromMessage_StringContent(t *testing.T) {
	// Some messages have content as a plain string, not an array of blocks.
	msg := []byte(`{"model": "claude-sonnet-4-20250514", "content": "plain text response"}`)
	info := extractFromMessage(msg, "")

	if info.Text != "plain text response" {
		t.Errorf("Text = %q, want %q", info.Text, "plain text response")
	}
	if info.HasThinking || info.HasToolUse {
		t.Error("flags should be false for string content")
	}
}

func TestExtractFromMessage_MultipleThinkingBlocks(t *testing.T) {
	// Thinking length should accumulate across multiple thinking blocks.
	msg := []byte(`{
		"model": "test-model",
		"content": [
			{"type": "thinking", "thinking": "first thought"},
			{"type": "thinking", "thinking": "second thought"},
			{"type": "text", "text": "final answer"}
		]
	}`)

	info := extractFromMessage(msg, "")

	if !info.HasThinking {
		t.Error("HasThinking = false, want true")
	}
	// "first thought" (13) + "second thought" (14) = 27
	if info.ThinkingLen != 27 {
		t.Errorf("ThinkingLen = %d, want %d", info.ThinkingLen, 27)
	}
}
