package claude

import (
	"encoding/json"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// EntryType identifies the type of trace entry.
type EntryType string

const (
	EntryTypeUser                EntryType = "user"
	EntryTypeAssistant           EntryType = "assistant"
	EntryTypeSystem              EntryType = "system"
	EntryTypeProgress            EntryType = "progress"
	EntryTypeFileHistorySnapshot EntryType = "file-history-snapshot"
	EntryTypeSummary             EntryType = "summary"
	EntryTypeQueueOperation      EntryType = "queue-operation"
)

// Entry represents a single line in a Claude Code JSONL trace file.
// Fields are a superset across all entry types; unused fields are zero-valued.
type Entry struct {
	// Common fields (present on most entry types)
	Type        EntryType       `json:"type"`
	UUID        string          `json:"uuid,omitempty"`
	ParentUUID  *string         `json:"parentUuid,omitempty"`
	Timestamp   string          `json:"timestamp,omitempty"`
	SessionID   string          `json:"sessionId,omitempty"`
	Version     string          `json:"version,omitempty"`
	GitBranch   string          `json:"gitBranch,omitempty"`
	CWD         string          `json:"cwd,omitempty"`
	Slug        string          `json:"slug,omitempty"`
	UserType    string          `json:"userType,omitempty"`
	IsSidechain bool            `json:"isSidechain,omitempty"`
	AgentID     string          `json:"agentId,omitempty"`
	Message     json.RawMessage `json:"message,omitempty"`

	// User entry fields
	ThinkingMetadata          *ThinkingMetadata `json:"thinkingMetadata,omitempty"`
	Todos                     []any             `json:"todos,omitempty"`
	PermissionMode            string            `json:"permissionMode,omitempty"`
	ToolUseResult             json.RawMessage   `json:"toolUseResult,omitempty"` // string | []any | object
	SourceToolAssistantUUID   string            `json:"sourceToolAssistantUUID,omitempty"`
	ImagePasteIDs             []string          `json:"imagePasteIds,omitempty"`
	IsMeta                    bool              `json:"isMeta,omitempty"`
	IsVisibleInTranscriptOnly bool              `json:"isVisibleInTranscriptOnly,omitempty"`
	IsCompactSummary          bool              `json:"isCompactSummary,omitempty"`

	// Assistant entry fields
	RequestID         string `json:"requestId,omitempty"`
	Error             string `json:"error,omitempty"`
	IsApiErrorMessage bool   `json:"isApiErrorMessage,omitempty"`

	// System entry fields
	Subtype           string          `json:"subtype,omitempty"`
	DurationMs        int             `json:"durationMs,omitempty"`
	Level             string          `json:"level,omitempty"`
	LogicalParentUUID string          `json:"logicalParentUuid,omitempty"`
	CompactMetadata   json.RawMessage `json:"compactMetadata,omitempty"`
	Content           string          `json:"content,omitempty"` // system & queue-operation

	// Progress entry fields
	Data            json.RawMessage `json:"data,omitempty"`
	ToolUseID       string          `json:"toolUseID,omitempty"`
	ParentToolUseID string          `json:"parentToolUseID,omitempty"`

	// File history snapshot fields
	MessageID        string          `json:"messageId,omitempty"`
	Snapshot         json.RawMessage `json:"snapshot,omitempty"`
	IsSnapshotUpdate bool            `json:"isSnapshotUpdate,omitempty"`

	// Summary entry fields
	Summary  string `json:"summary,omitempty"`
	LeafUUID string `json:"leafUuid,omitempty"`

	// Queue operation fields
	Operation string `json:"operation,omitempty"`

	// Parsed messages (lazily populated on first access via GetUserMessage/GetAssistantMessage)
	userMessage      *UserMessage
	assistantMessage *AssistantMessage
	messageParsed    bool
}

// GetUserMessage returns the parsed user message, parsing lazily on first access.
func (e *Entry) GetUserMessage() *UserMessage {
	e.ensureMessageParsed()
	return e.userMessage
}

// GetAssistantMessage returns the parsed assistant message, parsing lazily on first access.
func (e *Entry) GetAssistantMessage() *AssistantMessage {
	e.ensureMessageParsed()
	return e.assistantMessage
}

// SetUserMessage sets the user message (for callers that pre-parse).
func (e *Entry) SetUserMessage(msg *UserMessage) {
	e.userMessage = msg
	e.messageParsed = true
}

// SetAssistantMessage sets the assistant message (for callers that pre-parse).
func (e *Entry) SetAssistantMessage(msg *AssistantMessage) {
	e.assistantMessage = msg
	e.messageParsed = true
}

// ensureMessageParsed parses the Message field if not already done.
func (e *Entry) ensureMessageParsed() {
	if e.messageParsed || len(e.Message) == 0 {
		return
	}
	e.messageParsed = true

	switch e.Type {
	case EntryTypeUser:
		var msg UserMessage
		if err := json.Unmarshal(e.Message, &msg); err == nil {
			e.userMessage = &msg
		}
	case EntryTypeAssistant:
		var msg AssistantMessage
		if err := json.Unmarshal(e.Message, &msg); err == nil {
			e.assistantMessage = &msg
		}
	}
}

// ThinkingMetadata contains metadata about thinking mode.
type ThinkingMetadata struct {
	Level    string   `json:"level,omitempty"`
	Disabled bool     `json:"disabled,omitempty"`
	Triggers []string `json:"triggers,omitempty"`
}

// UserMessage represents the message field for user entries.
type UserMessage struct {
	Role    string      `json:"role"`
	Content UserContent `json:"content"`
}

// UserContent handles the polymorphic content field in user messages.
// It can be either a plain string or an array of ContentBlock.
type UserContent struct {
	Text   string         // Set when content is a string
	Blocks []ContentBlock // Set when content is an array
}

func (c *UserContent) UnmarshalJSON(data []byte) error {
	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		c.Text = s
		return nil
	}

	// Try array of content blocks
	var blocks []ContentBlock
	if err := json.Unmarshal(data, &blocks); err == nil {
		c.Blocks = blocks
		return nil
	}

	// Ignore unrecognized content types
	return nil
}

func (c UserContent) MarshalJSON() ([]byte, error) {
	if c.Text != "" {
		return json.Marshal(c.Text)
	}
	return json.Marshal(c.Blocks)
}

// GetText extracts all text content from the user content.
func (c *UserContent) GetText() string {
	if c.Text != "" {
		return c.Text
	}

	hasOnlyToolResults := len(c.Blocks) > 0
	var text string
	for _, b := range c.Blocks {
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
		return ""
	}
	return text
}

// AssistantMessage represents the message field for assistant entries.
type AssistantMessage struct {
	Role              string         `json:"role"`
	Model             string         `json:"model,omitempty"`
	ID                string         `json:"id,omitempty"`
	Type              string         `json:"type,omitempty"`
	Content           []ContentBlock `json:"content,omitempty"`
	StopReason        *string        `json:"stop_reason,omitempty"`
	StopSequence      *string        `json:"stop_sequence,omitempty"`
	Usage             *Usage         `json:"usage,omitempty"`
	Container         any            `json:"container,omitempty"`
	ContextManagement any            `json:"context_management,omitempty"`
}

// Usage represents token usage in an assistant message.
type Usage struct {
	InputTokens              int            `json:"input_tokens,omitempty"`
	OutputTokens             int            `json:"output_tokens,omitempty"`
	CacheCreationInputTokens int            `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int            `json:"cache_read_input_tokens,omitempty"`
	CacheCreation            *CacheCreation `json:"cache_creation,omitempty"`
	ServerToolUse            any            `json:"server_tool_use,omitempty"`
	ServiceTier              string         `json:"service_tier,omitempty"`
}

// CacheCreation contains cache creation token details.
type CacheCreation struct {
	Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens,omitempty"`
	Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens,omitempty"`
}

// ContentBlock represents a content block within a message.
// Different block types populate different fields.
type ContentBlock struct {
	Type string `json:"type"`

	// text block
	Text string `json:"text,omitempty"`

	// thinking block
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`

	// tool_use block
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`

	// tool_result block
	ToolUseID   string          `json:"tool_use_id,omitempty"`
	ToolContent json.RawMessage `json:"content,omitempty"` // string or []ContentBlock
	IsError     bool            `json:"is_error,omitempty"`

	// image / document block
	Source *MediaSource `json:"source,omitempty"`
}

// MediaSource represents the source of an image or document content block.
type MediaSource struct {
	Type      string `json:"type,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
}

// Prompt represents an extracted user prompt.
type Prompt struct {
	Text      string
	Timestamp string
	UUID      string
}

// GetPromptText extracts user prompt text from an entry.
// Returns empty string for non-user entries or tool results.
func (e *Entry) GetPromptText() string {
	if e.Type != EntryTypeUser {
		return ""
	}
	msg := e.GetUserMessage()
	if msg == nil {
		return ""
	}
	return msg.Content.GetText()
}

// GetThinkingBlocks returns all thinking blocks from an assistant entry.
func (e *Entry) GetThinkingBlocks() []string {
	if e.Type != EntryTypeAssistant {
		return nil
	}
	msg := e.GetAssistantMessage()
	if msg == nil {
		return nil
	}
	var thinking []string
	for _, b := range msg.Content {
		if b.Type == "thinking" && b.Thinking != "" {
			thinking = append(thinking, b.Thinking)
		}
	}
	return thinking
}

// GetToolCalls returns all tool use blocks from an assistant entry.
func (e *Entry) GetToolCalls() []ContentBlock {
	if e.Type != EntryTypeAssistant {
		return nil
	}
	msg := e.GetAssistantMessage()
	if msg == nil {
		return nil
	}
	var tools []ContentBlock
	for _, b := range msg.Content {
		if b.Type == "tool_use" {
			tools = append(tools, b)
		}
	}
	return tools
}

// ToThinktEntry converts a Claude Entry to a thinkt.Entry.
func (e *Entry) ToThinktEntry() thinkt.Entry {
	entry := thinkt.Entry{
		UUID:      e.UUID,
		Role:      convertEntryTypeToRole(e.Type),
		Text:      e.GetPromptText(),
		GitBranch: e.GitBranch,
		CWD:       e.CWD,
	}

	// Parse timestamp
	if e.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339, e.Timestamp); err == nil {
			entry.Timestamp = t
		}
	}

	// Convert content blocks from assistant messages
	if e.Type == EntryTypeAssistant {
		msg := e.GetAssistantMessage()
		if msg != nil {
			entry.Model = msg.Model
			for _, cb := range msg.Content {
				entry.ContentBlocks = append(entry.ContentBlocks, convertContentBlock(cb))
			}
			// If no text set from prompt, try to extract from content blocks
			if entry.Text == "" {
				for _, cb := range entry.ContentBlocks {
					if cb.Type == "text" && cb.Text != "" {
						if entry.Text != "" {
							entry.Text += "\n"
						}
						entry.Text += cb.Text
					}
				}
			}
		}
	}

	return entry
}

// convertEntryTypeToRole converts a Claude EntryType to a thinkt.Role.
func convertEntryTypeToRole(t EntryType) thinkt.Role {
	switch t {
	case EntryTypeUser:
		return thinkt.RoleUser
	case EntryTypeAssistant:
		return thinkt.RoleAssistant
	case EntryTypeSystem:
		return thinkt.RoleSystem
	default:
		return thinkt.RoleSystem
	}
}

// convertContentBlock converts a Claude ContentBlock to a thinkt.ContentBlock.
func convertContentBlock(cb ContentBlock) thinkt.ContentBlock {
	return thinkt.ContentBlock{
		Type:       cb.Type,
		Text:       cb.Text,
		Thinking:   cb.Thinking,
		Signature:  cb.Signature,
		ToolUseID:  cb.ToolUseID,
		ToolName:   cb.Name,
		ToolInput:  cb.Input,
		ToolResult: string(cb.ToolContent),
		IsError:    cb.IsError,
	}
}
