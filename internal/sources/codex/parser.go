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
// It holds the entries of one model response until the token_usage_record
// that closes it, so every entry of the response gets the record's
// response_id as TurnID and the usage attaches to the response's assistant
// entry. A wire twin of a harness item (assistant message, reasoning) is
// dropped by comparing text with the item that preceded it, as each line is
// read. Legacy
// pre-item rollouts pair event_msg user/agent/reasoning events with the wire
// record that follows them; the wire record wins.
type Parser struct {
	scanner   *bufio.Scanner
	sessionID string
	lineNo    int
	conv      converter
	turn      turnState

	out          []thinkt.Entry
	hold         []heldEntry       // entries of the open response
	usageTarget  int               // index in hold of the newest line's last assistant entry, or -1
	closed       bool              // a usage record closed the hold; the next producing line releases it
	callResponse map[string]string // call id (no wire: prefix) → response id of the response that made it

	lastAgentText     string
	lastReasoningText string

	legacy    *thinkt.Entry
	legacyKey string

	unknown thinkt.UnknownTally[string]
	done    bool
}

// NewParser creates a new Codex parser.
func NewParser(r io.Reader, sessionID string) *Parser {
	p := &Parser{
		unknown:      thinkt.UnknownTally[string]{Source: thinkt.SourceCodex},
		scanner:      thinkt.NewScannerWithMaxCapacityCustom(r, 64*1024, thinkt.MaxScannerCapacity),
		sessionID:    sessionID,
		usageTarget:  -1,
		callResponse: map[string]string{},
	}
	p.conv = converter{sessionID: sessionID, turn: &p.turn, calls: map[string]string{}}
	return p
}

// UnknownTypes returns the record keys the parser did not recognise and how
// often each appeared.
func (p *Parser) UnknownTypes() map[string]int {
	return p.unknown.Snapshot()
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
			p.release()
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

// heldEntry is an entry waiting for the usage record of its response.
// mapped marks an entry whose TurnID came from the call-id map; a usage
// record does not overwrite it.
type heldEntry struct {
	entry  thinkt.Entry
	mapped bool
}

// release emits the held entries as they are: those a usage record stamped
// keep its response id, the rest have none.
func (p *Parser) release() {
	for _, h := range p.hold {
		p.out = append(p.out, h.entry)
	}
	p.hold = nil
	p.closed = false
	p.usageTarget = -1
}

// step processes one non-empty line.
func (p *Parser) step(line []byte) {
	head, class := ClassifyLine(line)
	p.conv.lineNo = p.lineNo
	p.turn.apply(head)
	if class == thinkt.ClassUnknown {
		p.unknown.Note(head.Key(), "line", p.lineNo)
	}
	key := head.Key()
	switch key {
	case "token_usage_record":
		p.closeHold(head)
		return
	case "turn_context", "event_msg/task_started":
		p.release() // a new turn: no usage record will close this response
	}

	entries := p.conv.convert(head, class, line)
	if len(entries) == 0 {
		return
	}
	// The dedupe memory covers only the item's adjacent twin: whatever
	// producing line comes next, twin or not, clears it.
	twin := isDedupeCandidate(head) && p.isItemTwin(head, entries[0])
	p.lastAgentText, p.lastReasoningText = "", ""
	if twin {
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
	// Legacy rollouts predate usage records; their events never join a hold.
	if isLegacyEvent(head) {
		p.release()
		p.legacy = &entries[0]
		p.legacyKey = head.PayloadType
		return
	}

	mapped := p.stampFromCalls(entries)
	switch {
	case isUserMessage(head):
		p.release()
		p.out = append(p.out, entries...)
	case isResultLine(key):
		if p.closed {
			p.release()
		}
		if len(p.hold) == 0 {
			p.out = append(p.out, entries...)
		} else {
			p.join(entries, mapped)
		}
	default:
		if p.closed {
			p.release()
		}
		p.join(entries, mapped)
	}

	switch key {
	case "item/AgentMessage":
		p.lastAgentText = strings.TrimSpace(entries[0].Text)
	case "item/Reasoning":
		p.lastReasoningText = thinkingText(&entries[0])
	}
}

// join adds one line's entries to the hold. The usage target becomes the
// line's last assistant entry, or none when the line has no assistant entry.
func (p *Parser) join(entries []thinkt.Entry, mapped []bool) {
	p.usageTarget = -1
	for i, e := range entries {
		if e.Role == thinkt.RoleAssistant {
			p.usageTarget = len(p.hold)
		}
		p.hold = append(p.hold, heldEntry{entry: e, mapped: mapped[i]})
	}
}

// closeHold applies a token_usage_record to the open response: usage on the
// target, the response id on every held entry the call-id map did not
// stamp, and the response's calls into the map. A later record in the same
// run applies to the same hold.
func (p *Parser) closeHold(head recordHead) {
	if len(p.hold) == 0 {
		return
	}
	if p.usageTarget >= 0 {
		if u := decodeUsage(head.Payload); u != nil {
			p.hold[p.usageTarget].entry.Usage = u
		}
	}
	if id := responseID(head.Payload); id != "" {
		for i := range p.hold {
			if p.hold[i].mapped {
				continue
			}
			p.hold[i].entry.TurnID = id
			for _, b := range p.hold[i].entry.ContentBlocks {
				if b.Type == "tool_use" && b.ToolUseID != "" {
					p.callResponse[strings.TrimPrefix(b.ToolUseID, "wire:")] = id
				}
			}
		}
	}
	p.closed = true
}

// stampFromCalls gives each entry that reports on a call of a closed
// response that response's id, and reports which entries it stamped.
func (p *Parser) stampFromCalls(entries []thinkt.Entry) []bool {
	mapped := make([]bool, len(entries))
	for i := range entries {
		for _, b := range entries[i].ContentBlocks {
			if b.ToolUseID == "" {
				continue
			}
			if id, ok := p.callResponse[strings.TrimPrefix(b.ToolUseID, "wire:")]; ok {
				entries[i].TurnID = id
				mapped[i] = true
				break
			}
		}
	}
	return mapped
}

// isUserMessage reports whether the line is a user prompt on the wire: it
// ends the open response and is emitted on its own.
func isUserMessage(h recordHead) bool {
	return h.Type == "response_item" && h.PayloadType == "message" && h.Role == "user"
}

// isResultLine reports whether the line reports a tool's outcome: a wire
// output, or a harness tool item (both halves of the pair).
func isResultLine(key string) bool {
	switch key {
	case "response_item/function_call_output", "response_item/custom_tool_call_output",
		"item/CommandExecution", "item/FileChange", "item/McpToolCall", "item/WebSearch", "item/ImageView", "item/Extension":
		return true
	}
	return false
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
