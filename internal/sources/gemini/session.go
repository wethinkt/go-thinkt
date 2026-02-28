package gemini

import (
	"encoding/json"
	"strings"
	"time"
)

// Session represents a Gemini CLI conversation session.
type Session struct {
	SessionID   string    `json:"sessionId"`
	ProjectHash string    `json:"projectHash"`
	StartTime   time.Time `json:"startTime"`
	LastUpdated time.Time `json:"lastUpdated"`
	Messages    []Message `json:"messages"`
}

// Message represents a single message in the conversation.
type Message struct {
	ID        string     `json:"id"`
	Timestamp time.Time  `json:"timestamp"`
	Type      string     `json:"type"` // "user" or "gemini"
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"toolCalls,omitempty"`
	Thoughts  []Thought  `json:"thoughts,omitempty"`
	Tokens    *Tokens    `json:"tokens,omitempty"`
	Model     string     `json:"model,omitempty"`
}

// contentPart represents a single part in a multi-part content array.
type contentPart struct {
	Text string `json:"text"`
}

// UnmarshalJSON handles content being either a string or an array of {text} objects.
func (m *Message) UnmarshalJSON(data []byte) error {
	type messageAlias Message
	aux := &struct {
		Content json.RawMessage `json:"content"`
		*messageAlias
	}{
		messageAlias: (*messageAlias)(m),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if len(aux.Content) == 0 {
		return nil
	}

	// Try string first.
	var s string
	if err := json.Unmarshal(aux.Content, &s); err == nil {
		m.Content = s
		return nil
	}

	// Try array of parts.
	var parts []contentPart
	if err := json.Unmarshal(aux.Content, &parts); err == nil {
		texts := make([]string, 0, len(parts))
		for _, p := range parts {
			if p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
		m.Content = strings.Join(texts, "\n")
		return nil
	}

	return nil
}

// ToolCall represents a tool execution.
type ToolCall struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Args   any          `json:"args"`
	Result []ToolResult `json:"result,omitempty"`
}

// ToolResult represents the output of a tool call.
type ToolResult struct {
	FunctionResponse FunctionResponse `json:"functionResponse"`
}

// FunctionResponse wraps the actual response.
type FunctionResponse struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

// Thought represents the agent's internal thinking process.
type Thought struct {
	Subject     string    `json:"subject"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
}

// Tokens represents token usage statistics.
type Tokens struct {
	Input    int `json:"input"`
	Output   int `json:"output"`
	Cached   int `json:"cached"`
	Thoughts int `json:"thoughts"`
	Tool     int `json:"tool"`
	Total    int `json:"total"`
}
