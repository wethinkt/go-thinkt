package copilot

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Parser reads Copilot events from an io.Reader.
type Parser struct {
	scanner *bufio.Scanner
}

// NewParser creates a new parser.
func NewParser(r io.Reader) *Parser {
	return &Parser{
		scanner: bufio.NewScanner(r),
	}
}

// NextEntry reads the next entry from the stream.
func (p *Parser) NextEntry() (*thinkt.Entry, error) {
	for p.scanner.Scan() {
		line := p.scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Skip malformed lines
			continue
		}

		entry := p.convertEvent(event)
		if entry != nil {
			return entry, nil
		}
	}

	if err := p.scanner.Err(); err != nil {
		return nil, err
	}

	return nil, io.EOF
}

func (p *Parser) convertEvent(e Event) *thinkt.Entry {
	// Base entry
	entry := &thinkt.Entry{
		UUID:      e.ID,
		Timestamp: e.Timestamp,
		Source:    thinkt.SourceCopilot,
		Metadata:  make(map[string]any),
	}

	if e.ParentID != nil {
		entry.ParentUUID = e.ParentID
	}

	switch e.Type {
	case EventTypeUserMessage:
		entry.Role = thinkt.RoleUser
		if content, ok := e.Data["content"].(string); ok {
			entry.Text = content
		}
		// Check for transformedContent (often contains the actual prompt sent)
		if transformed, ok := e.Data["transformedContent"].(string); ok && transformed != "" {
			entry.Metadata["original_content"] = entry.Text
			entry.Text = transformed
		}

	case EventTypeAssistantMsg:
		entry.Role = thinkt.RoleAssistant
		
		// Parse structured data from generic map
		dataBytes, _ := json.Marshal(e.Data)
		var msgData AssistantMessageData
		json.Unmarshal(dataBytes, &msgData)

		entry.Metadata["message_id"] = msgData.MessageID
		
		// Add text content
		if msgData.Content != "" {
			entry.ContentBlocks = append(entry.ContentBlocks, thinkt.ContentBlock{
				Type: "text",
				Text: msgData.Content,
			})
		}

		// Add reasoning
		if msgData.ReasoningText != "" {
			entry.ContentBlocks = append(entry.ContentBlocks, thinkt.ContentBlock{
				Type:     "thinking",
				Thinking: msgData.ReasoningText,
			})
		} else if opaque, ok := e.Data["reasoningOpaque"].(string); ok && opaque != "" {
			// Handle opaque reasoning (encrypted/hidden)
			entry.ContentBlocks = append(entry.ContentBlocks, thinkt.ContentBlock{
				Type:      "thinking",
				Thinking:  "(Encrypted Thinking Block)",
				Signature: opaque,
			})
		}

		// Add tool calls
		for _, tool := range msgData.ToolRequests {
			entry.ContentBlocks = append(entry.ContentBlocks, thinkt.ContentBlock{
				Type:      "tool_use",
				ToolUseID: tool.ToolCallID,
				ToolName:  tool.Name,
				ToolInput: tool.Arguments,
			})
		}

	case "tool.execution_success", "tool.execution_complete": // Copilot CLI uses both?
		// Copilot CLI seems to use tool.execution_complete for results
		// Let's check the data for success
		entry.Role = thinkt.RoleTool
		
		// Manual extraction since structure varies
		toolCallID, _ := e.Data["toolCallId"].(string)
		
		var resultStr string
		if result, ok := e.Data["result"]; ok {
			// Result can be complex object
			if resultMap, ok := result.(map[string]any); ok {
				// Try to get content string if available
				if content, ok := resultMap["content"].(string); ok {
					resultStr = content
				} else {
					// Fallback to JSON
					bytes, _ := json.Marshal(result)
					resultStr = string(bytes)
				}
			} else if s, ok := result.(string); ok {
				resultStr = s
			}
		}

		entry.ContentBlocks = append(entry.ContentBlocks, thinkt.ContentBlock{
			Type:       "tool_result",
			ToolUseID:  toolCallID,
			ToolResult: resultStr,
		})

	case EventTypeToolExecError:
		entry.Role = thinkt.RoleTool
		toolCallID, _ := e.Data["toolCallId"].(string)
		errorMsg, _ := e.Data["error"].(string)
		
		entry.ContentBlocks = append(entry.ContentBlocks, thinkt.ContentBlock{
			Type:       "tool_result",
			ToolUseID:  toolCallID,
			ToolResult: errorMsg,
			IsError:    true,
		})

	case EventTypeSessionStart:
		// These are metadata, typically handled at Session level, 
		// but we return them as System events for the stream
		entry.Role = thinkt.RoleSystem
		entry.Text = "Session Started"
		entry.Metadata["event_type"] = e.Type
		if context, ok := e.Data["context"].(map[string]any); ok {
			if cwd, ok := context["cwd"].(string); ok {
				entry.CWD = cwd
			}
			if repo, ok := context["repository"].(string); ok {
				entry.Metadata["repository"] = repo
			}
		}

	default:
		return nil
	}

	return entry
}

// Helper to parse timestamp
func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
