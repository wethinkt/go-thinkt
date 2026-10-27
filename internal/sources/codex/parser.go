package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Parser reads Codex rollout JSONL and yields thinkt entries in file order.
//
// It holds the entries of the most recent producing line in pending so a
// trailing token_usage_record can attach to the assistant entry it belongs
// to, and so a wire twin of a harness item (assistant message, reasoning)
// can be dropped by comparing text with the item that preceded it. Legacy
// pre-item rollouts pair event_msg user/agent/reasoning events with the wire
// record that follows them; the wire record wins.
type Parser struct {
	scanner   *bufio.Scanner
	sessionID string
	lineNo    int
	conv      converter
	turn      turnState

	out         []thinkt.Entry
	pending     []thinkt.Entry
	usageTarget int

	lastAgentText     string
	lastReasoningText string

	legacy    *thinkt.Entry
	legacyKey string

	unknown unknownTally
	errors  []error
	done    bool
}

// NewParser creates a new Codex parser.
func NewParser(r io.Reader, sessionID string) *Parser {
	p := &Parser{
		scanner:     thinkt.NewScannerWithMaxCapacityCustom(r, 64*1024, thinkt.MaxScannerCapacity),
		sessionID:   sessionID,
		usageTarget: -1,
	}
	p.conv = converter{sessionID: sessionID, turn: &p.turn, calls: map[string]string{}}
	return p
}

// UnknownTypes returns the record keys the parser did not recognise and how
// often each appeared.
func (p *Parser) UnknownTypes() map[string]int {
	return p.unknown.snapshot()
}

// NextEntry returns the next entry, or io.EOF at the end of the stream.
func (p *Parser) NextEntry() (*thinkt.Entry, error) {
	for {
		if len(p.out) > 0 {
			e := p.out[0]
			p.out = p.out[1:]
			return &e, nil
		}
		if p.done {
			return nil, io.EOF
		}
		if !p.scanner.Scan() {
			p.done = true
			if err := p.scanner.Err(); err != nil {
				return nil, err
			}
			p.flushPending()
			if p.legacy != nil {
				p.out = append(p.out, *p.legacy)
				p.legacy = nil
			}
			continue
		}
		p.lineNo++
		line := bytes.TrimSpace(p.scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		p.step(line)
	}
}

// flushPending moves the held entries to the output queue.
func (p *Parser) flushPending() {
	p.out = append(p.out, p.pending...)
	p.pending = nil
	p.usageTarget = -1
}

// step processes one non-empty line.
func (p *Parser) step(line []byte) {
	head, class := ClassifyLine(line)
	p.conv.lineNo = p.lineNo
	p.turn.apply(head)
	if class == ClassUnknown {
		p.unknown.note(head.Key(), "line", p.lineNo)
	}
	key := head.Key()
	if key == "token_usage_record" {
		p.attachUsage(head)
		return
	}

	entries := p.conv.convert(head, class, line)
	if len(entries) == 0 {
		return
	}
	if isDedupeCandidate(head) && p.isItemTwin(head, entries[0]) {
		return
	}

	if p.legacy != nil {
		if isWireTwin(p.legacyKey, p.legacy, head, &entries[0]) {
			p.legacy = nil
		} else {
			p.out = append(p.out, *p.legacy)
			p.legacy = nil
		}
	}
	if isLegacyEvent(head) {
		p.flushPending()
		p.legacy = &entries[0]
		p.legacyKey = head.PayloadType
		return
	}

	p.flushPending()
	p.pending = entries
	for i := len(p.pending) - 1; i >= 0; i-- {
		if p.pending[i].Role == thinkt.RoleAssistant {
			p.usageTarget = i
			break
		}
	}
	switch key {
	case "item/AgentMessage":
		p.lastAgentText = strings.TrimSpace(entries[0].Text)
	case "item/Reasoning":
		p.lastReasoningText = thinkingText(&entries[0])
	}
}

// attachUsage stores a token_usage_record on the assistant entry held in
// pending. A second record for the same hold overwrites the first; a record
// with no assistant entry pending is dropped (the session scan still sums it).
func (p *Parser) attachUsage(head recordHead) {
	if p.usageTarget < 0 || p.usageTarget >= len(p.pending) {
		return
	}
	if u := decodeUsage(head.Payload); u != nil {
		p.pending[p.usageTarget].Usage = u
	}
}

// isItemTwin reports whether a wire assistant message or reasoning record
// repeats the harness item that preceded it.
func (p *Parser) isItemTwin(head recordHead, e thinkt.Entry) bool {
	switch head.PayloadType {
	case "message":
		return p.lastAgentText != "" && strings.TrimSpace(e.Text) == p.lastAgentText
	case "reasoning":
		return p.lastReasoningText != "" && thinkingText(&e) == p.lastReasoningText
	}
	return false
}

func isLegacyEvent(h recordHead) bool {
	if h.Type != "event_msg" {
		return false
	}
	switch h.PayloadType {
	case "user_message", "agent_message", "agent_reasoning":
		return true
	}
	return false
}

// isWireTwin reports whether the wire record that follows a legacy event
// carries the same content (same role, same trimmed text, matching kind).
func isWireTwin(legacyKey string, legacy *thinkt.Entry, h recordHead, cur *thinkt.Entry) bool {
	if h.Type != "response_item" || legacy.Role != cur.Role {
		return false
	}
	a, b := comparableEntryText(legacy), comparableEntryText(cur)
	if a == "" || a != b {
		return false
	}
	switch legacyKey {
	case "user_message", "agent_message":
		return h.PayloadType == "message"
	case "agent_reasoning":
		return h.PayloadType == "reasoning"
	}
	return false
}

func thinkingText(e *thinkt.Entry) string {
	for _, b := range e.ContentBlocks {
		if b.Type == "thinking" {
			return strings.TrimSpace(b.Thinking)
		}
	}
	return ""
}

func comparableEntryText(entry *thinkt.Entry) string {
	if entry == nil {
		return ""
	}
	if text := strings.TrimSpace(entry.Text); text != "" {
		return text
	}
	return thinkingText(entry)
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

// logLine is the envelope of one rollout line.
type logLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}
