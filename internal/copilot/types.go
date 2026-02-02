package copilot

import (
	"time"
)

// Event types
const (
	EventTypeSessionStart    = "session.start"
	EventTypeSessionInfo     = "session.info"
	EventTypeUserMessage     = "user.message"
	EventTypeAssistantMsg    = "assistant.message"
	EventTypeToolExecStart   = "tool.execution_start"
	EventTypeToolExecSuccess = "tool.execution_success"
	EventTypeToolExecComplete = "tool.execution_complete"
	EventTypeToolExecError   = "tool.execution_error"
)

// Event represents a single line in events.jsonl
type Event struct {
	Type      string          `json:"type"`
	Data      map[string]any  `json:"data"`
	ID        string          `json:"id"`
	Timestamp time.Time       `json:"timestamp"`
	ParentID  *string         `json:"parentId"`
}

type ToolRequest struct {
	ToolCallID string         `json:"toolCallId"`
	Name       string         `json:"name"`
	Arguments  map[string]any `json:"arguments"`
	Type       string         `json:"type"`
}

type AssistantMessageData struct {
	MessageID     string        `json:"messageId"`
	Content       string        `json:"content"`
	ToolRequests  []ToolRequest `json:"toolRequests"`
	ReasoningText string        `json:"reasoningText"` // Thinking content
}

type ToolExecutionSuccessData struct {
	ToolCallID string `json:"toolCallId"`
	Result     any    `json:"result"` // Usually string, but can be object
}

type ToolExecutionErrorData struct {
	ToolCallID string `json:"toolCallId"`
	Error      string `json:"error"`
}
