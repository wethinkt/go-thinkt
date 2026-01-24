// Package trace provides types and parsing for Claude Code JSONL trace files.
package trace

import "encoding/json"

// Entry represents a single line in a Claude Code trace file.
// The top-level "type" field determines the entry kind (user, assistant, etc.)
type Entry struct {
	Type      string          `json:"type"`
	UUID      string          `json:"uuid"`
	ParentUUID *string        `json:"parentUuid"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"sessionId"`
	Message   json.RawMessage `json:"message"` // Varies by type
}

// UserMessage represents the message field for user entries.
type UserMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // Can be string or []ContentBlock
}

// AssistantMessage represents the message field for assistant entries.
type AssistantMessage struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
	Model   string         `json:"model"`
}

// ContentBlock represents a content block within a message.
type ContentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
	Name      string `json:"name,omitempty"`       // tool_use
	ID        string `json:"id,omitempty"`         // tool_use
	Input     any    `json:"input,omitempty"`      // tool_use
	ToolUseID string `json:"tool_use_id,omitempty"` // tool_result
	Content   string `json:"content,omitempty"`     // tool_result
}

// Prompt represents an extracted user prompt.
type Prompt struct {
	Text      string
	Timestamp string
	UUID      string
}

// ParseUserContent extracts the text content from a user message.
// User content can be either a plain string or an array of content blocks.
func ParseUserContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try parsing as a string first
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str
	}

	// Try parsing as an array of content blocks
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var text string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				if text != "" {
					text += "\n"
				}
				text += b.Text
			}
		}
		return text
	}

	return ""
}
