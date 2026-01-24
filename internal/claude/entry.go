package claude

import "encoding/json"

// EntryType identifies the type of trace entry.
type EntryType string

const (
	EntryTypeUser              EntryType = "user"
	EntryTypeAssistant         EntryType = "assistant"
	EntryTypeFileHistorySnapshot EntryType = "file-history-snapshot"
	EntryTypeSummary           EntryType = "summary"
)

// Entry represents a single line in a Claude Code trace file.
type Entry struct {
	Type       EntryType       `json:"type"`
	UUID       string          `json:"uuid"`
	ParentUUID *string         `json:"parentUuid"`
	Timestamp  string          `json:"timestamp"`
	SessionID  string          `json:"sessionId"`
	Version    string          `json:"version"`
	GitBranch  string          `json:"gitBranch"`
	CWD        string          `json:"cwd"`
	IsSidechain bool           `json:"isSidechain"`
	Message    json.RawMessage `json:"message"`

	// Parsed message (populated after parsing)
	UserMessage      *UserMessage      `json:"-"`
	AssistantMessage *AssistantMessage `json:"-"`
}

// UserMessage represents the message field for user entries.
type UserMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // Can be string or []ContentBlock
}

// AssistantMessage represents the message field for assistant entries.
type AssistantMessage struct {
	Role       string         `json:"role"`
	Model      string         `json:"model"`
	ID         string         `json:"id"`
	Content    []ContentBlock `json:"content"`
	StopReason *string        `json:"stop_reason"`
	Usage      *Usage         `json:"usage"`
}

// Usage represents token usage in an assistant message.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
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
	Content   string `json:"content,omitempty"`     // tool_result (as string)
}

// GetPromptText extracts user prompt text from an entry.
// Returns empty string for non-user entries or tool results.
func (e *Entry) GetPromptText() string {
	if e.Type != EntryTypeUser {
		return ""
	}
	if e.UserMessage == nil {
		return ""
	}
	return ParseUserContent(e.UserMessage.Content)
}

// GetThinkingBlocks returns all thinking blocks from an assistant entry.
func (e *Entry) GetThinkingBlocks() []string {
	if e.Type != EntryTypeAssistant || e.AssistantMessage == nil {
		return nil
	}
	var thinking []string
	for _, b := range e.AssistantMessage.Content {
		if b.Type == "thinking" && b.Thinking != "" {
			thinking = append(thinking, b.Thinking)
		}
	}
	return thinking
}

// GetToolCalls returns all tool use blocks from an assistant entry.
func (e *Entry) GetToolCalls() []ContentBlock {
	if e.Type != EntryTypeAssistant || e.AssistantMessage == nil {
		return nil
	}
	var tools []ContentBlock
	for _, b := range e.AssistantMessage.Content {
		if b.Type == "tool_use" {
			tools = append(tools, b)
		}
	}
	return tools
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
		// Check if this is a tool result (skip those)
		hasOnlyToolResults := true
		var text string
		for _, b := range blocks {
			if b.Type == "tool_result" {
				continue
			}
			hasOnlyToolResults = false
			if b.Type == "text" && b.Text != "" {
				if text != "" {
					text += "\n"
				}
				text += b.Text
			}
		}
		if hasOnlyToolResults {
			return "" // Skip pure tool result entries
		}
		return text
	}

	return ""
}
