package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Parser reads Codex session JSONL entries from an io.Reader.
type Parser struct {
	scanner      *bufio.Scanner
	sessionID    string
	lineNo       int
	pendingEvent *parsedEntry
	queued       *parsedEntry
}

type logLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type parsedEntry struct {
	entry     *thinkt.Entry
	kind      string
	fromEvent bool
}

// NewParser creates a new Codex parser.
func NewParser(r io.Reader, sessionID string) *Parser {
	scanner := thinkt.NewScannerWithMaxCapacityCustom(r, 64*1024, thinkt.MaxScannerCapacity)
	return &Parser{
		scanner:   scanner,
		sessionID: sessionID,
	}
}

// NextEntry reads the next convertible entry from the JSONL stream.
func (p *Parser) NextEntry() (*thinkt.Entry, error) {
	if p.queued != nil {
		out := p.queued.entry
		p.queued = nil
		return out, nil
	}

	for p.scanner.Scan() {
		p.lineNo++
		line := strings.TrimSpace(p.scanner.Text())
		if line == "" {
			continue
		}

		parsed := p.convertLine([]byte(line))
		if parsed == nil || parsed.entry == nil {
			continue
		}

		if p.pendingEvent == nil {
			if isEventMessageCandidate(parsed) {
				p.pendingEvent = parsed
				continue
			}
			return parsed.entry, nil
		}

		if isDuplicateEventResponsePair(p.pendingEvent, parsed) {
			p.pendingEvent = nil
			return parsed.entry, nil
		}

		out := p.pendingEvent.entry
		p.pendingEvent = nil
		if isEventMessageCandidate(parsed) {
			p.pendingEvent = parsed
		} else {
			p.queued = parsed
		}
		return out, nil
	}

	if p.pendingEvent != nil {
		out := p.pendingEvent.entry
		p.pendingEvent = nil
		return out, nil
	}

	if err := p.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

func (p *Parser) convertLine(line []byte) *parsedEntry {
	var l logLine
	if err := json.Unmarshal(line, &l); err != nil {
		return nil
	}

	timestamp := parseTimestamp(l.Timestamp)
	switch l.Type {
	case "event_msg":
		return p.convertEventMsg(l.Payload, timestamp)
	case "response_item":
		return p.convertResponseItem(l.Payload, timestamp)
	default:
		return nil
	}
}

func (p *Parser) convertEventMsg(raw json.RawMessage, timestamp time.Time) *parsedEntry {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	eventType := readString(payload, "type")
	switch eventType {
	case "user_message":
		text := readString(payload, "message")
		if text == "" {
			return nil
		}
		return &parsedEntry{
			entry:     p.newEntry(thinkt.RoleUser, timestamp, eventType, text),
			kind:      eventType,
			fromEvent: true,
		}
	case "agent_message":
		text := readString(payload, "message")
		if text == "" {
			return nil
		}
		return &parsedEntry{
			entry:     p.newEntry(thinkt.RoleAssistant, timestamp, eventType, text),
			kind:      eventType,
			fromEvent: true,
		}
	case "agent_reasoning":
		thinking := readString(payload, "text")
		if thinking == "" {
			return nil
		}
		e := p.newEntry(thinkt.RoleAssistant, timestamp, eventType, "")
		e.ContentBlocks = []thinkt.ContentBlock{{Type: "thinking", Thinking: thinking}}
		return &parsedEntry{entry: e, kind: eventType, fromEvent: true}
	default:
		return nil
	}
}

func (p *Parser) convertResponseItem(raw json.RawMessage, timestamp time.Time) *parsedEntry {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	itemType := readString(payload, "type")
	switch itemType {
	case "message":
		role := mapMessageRole(readString(payload, "role"))
		text := extractMessageText(payload["content"])
		if text == "" {
			return nil
		}
		return &parsedEntry{
			entry:     p.newEntry(role, timestamp, itemType, text),
			kind:      itemType,
			fromEvent: false,
		}

	case "reasoning":
		thinking := extractReasoningText(payload)
		if thinking == "" {
			return nil
		}
		e := p.newEntry(thinkt.RoleAssistant, timestamp, itemType, "")
		e.ContentBlocks = []thinkt.ContentBlock{{Type: "thinking", Thinking: thinking}}
		return &parsedEntry{entry: e, kind: itemType, fromEvent: false}

	case "function_call", "custom_tool_call":
		callID := readString(payload, "call_id")
		toolName := readString(payload, "name")
		if callID == "" && toolName == "" {
			return nil
		}
		e := p.newEntry(thinkt.RoleAssistant, timestamp, itemType, "")
		e.UUID = composeUUID(p.sessionID, p.lineNo, itemType, callID)
		e.ContentBlocks = []thinkt.ContentBlock{{
			Type:      "tool_use",
			ToolUseID: callID,
			ToolName:  toolName,
			ToolInput: parseToolInput(payload),
		}}
		return &parsedEntry{entry: e, kind: itemType, fromEvent: false}

	case "function_call_output", "custom_tool_call_output":
		callID := readString(payload, "call_id")
		output := normalizeToolOutput(payload["output"])
		if callID == "" && output == "" {
			return nil
		}
		e := p.newEntry(thinkt.RoleTool, timestamp, itemType, "")
		e.UUID = composeUUID(p.sessionID, p.lineNo, itemType, callID)
		e.ContentBlocks = []thinkt.ContentBlock{{
			Type:       "tool_result",
			ToolUseID:  callID,
			ToolResult: output,
		}}
		return &parsedEntry{entry: e, kind: itemType, fromEvent: false}

	default:
		return nil
	}
}

func isEventMessageCandidate(p *parsedEntry) bool {
	if p == nil || !p.fromEvent || p.entry == nil {
		return false
	}
	switch p.kind {
	case "user_message", "agent_message", "agent_reasoning":
		return comparableEntryText(p.entry) != ""
	default:
		return false
	}
}

func isDuplicateEventResponsePair(event, current *parsedEntry) bool {
	if !isEventMessageCandidate(event) || current == nil || current.entry == nil {
		return false
	}
	if current.fromEvent {
		return false
	}
	if event.entry.Role != current.entry.Role {
		return false
	}
	eventText := comparableEntryText(event.entry)
	currentText := comparableEntryText(current.entry)
	if eventText == "" || currentText == "" || eventText != currentText {
		return false
	}

	switch event.kind {
	case "user_message", "agent_message":
		return current.kind == "message"
	case "agent_reasoning":
		return current.kind == "reasoning"
	default:
		return false
	}
}

func comparableEntryText(entry *thinkt.Entry) string {
	if entry == nil {
		return ""
	}
	if text := strings.TrimSpace(entry.Text); text != "" {
		return text
	}
	for _, block := range entry.ContentBlocks {
		if block.Type == "thinking" {
			if text := strings.TrimSpace(block.Thinking); text != "" {
				return text
			}
		}
	}
	return ""
}

func (p *Parser) newEntry(role thinkt.Role, timestamp time.Time, kind, text string) *thinkt.Entry {
	return &thinkt.Entry{
		UUID:      composeUUID(p.sessionID, p.lineNo, kind, ""),
		Role:      role,
		Timestamp: timestamp,
		Source:    thinkt.SourceCodex,
		Text:      text,
	}
}

func parseTimestamp(v string) time.Time {
	if v == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t
	}
	return time.Time{}
}

func composeUUID(sessionID string, lineNo int, kind, suffix string) string {
	base := fmt.Sprintf("%s:%06d:%s", sessionID, lineNo, kind)
	if suffix != "" {
		return base + ":" + suffix
	}
	return base
}

func mapMessageRole(role string) thinkt.Role {
	switch role {
	case "user":
		return thinkt.RoleUser
	case "assistant":
		return thinkt.RoleAssistant
	case "system", "developer":
		return thinkt.RoleSystem
	default:
		return thinkt.RoleSystem
	}
}

func readString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func extractMessageText(v any) string {
	items, ok := v.([]any)
	if !ok {
		return ""
	}

	parts := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		// Most message blocks use "text" while some input variants use
		// explicit input/output text fields.
		text := readString(m, "text")
		if text == "" {
			text = readString(m, "input_text")
		}
		if text == "" {
			text = readString(m, "output_text")
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func extractReasoningText(payload map[string]any) string {
	summary, ok := payload["summary"].([]any)
	if ok {
		parts := make([]string, 0, len(summary))
		for _, item := range summary {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if readString(m, "type") != "summary_text" {
				continue
			}
			text := readString(m, "text")
			if text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.TrimSpace(strings.Join(parts, "\n"))
		}
	}
	return strings.TrimSpace(readString(payload, "text"))
}

func parseToolInput(payload map[string]any) any {
	// function_call usually stores JSON as a string in "arguments".
	if args := readString(payload, "arguments"); args != "" {
		var out any
		if err := json.Unmarshal([]byte(args), &out); err == nil {
			return out
		}
		return args
	}

	// custom_tool_call usually stores raw text in "input".
	if input := readString(payload, "input"); input != "" {
		return input
	}
	return nil
}

func normalizeToolOutput(v any) string {
	switch out := v.(type) {
	case nil:
		return ""
	case string:
		trimmed := strings.TrimSpace(out)
		if trimmed == "" {
			return ""
		}

		// custom_tool_call_output often wraps command output in a JSON string.
		var wrapped struct {
			Output string `json:"output"`
		}
		if err := json.Unmarshal([]byte(trimmed), &wrapped); err == nil && wrapped.Output != "" {
			return wrapped.Output
		}
		return out
	default:
		b, err := json.Marshal(out)
		if err != nil {
			return ""
		}
		return string(b)
	}
}
