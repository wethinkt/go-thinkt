package kimi

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// RecordClass says what a wire line is for, decided from its record key and
// a few cheap fields. The values mirror claude.RecordClass and
// codex.RecordClass so a later lift into thinkt is mechanical.
type RecordClass int

const (
	ClassTurn         RecordClass = iota // user prompts, loop parts and calls, tool results
	ClassEvent                           // system events and the loop's step markers
	ClassContext                         // harness-injected context: reminders, preambles, notifications
	ClassCheckpoint                      // declared for parity; never returned
	ClassSessionState                    // metadata
	ClassBookkeeping                     // dropped: prompts repeated by turn.prompt, counters, configuration
	ClassUnknown                         // key never seen; emitted as a secondary system entry so drift is visible
)

func (c RecordClass) String() string {
	switch c {
	case ClassTurn:
		return "turn"
	case ClassEvent:
		return "event"
	case ClassContext:
		return "context"
	case ClassCheckpoint:
		return "checkpoint"
	case ClassSessionState:
		return "session-state"
	case ClassBookkeeping:
		return "bookkeeping"
	case ClassUnknown:
		return "unknown"
	default:
		return "invalid"
	}
}

// IsEntry reports whether lines of this class become thinkt entries.
func (c RecordClass) IsEntry() bool {
	switch c {
	case ClassTurn, ClassEvent, ClassContext, ClassCheckpoint, ClassUnknown:
		return true
	default:
		return false
	}
}

// recordHead is the cheap view of one wire line that classification and
// conversion share.
type recordHead struct {
	Type       string
	Key        string // "<type>", "loop/<event.type>", "ctx/<role>", "appended/<role>"
	Role       string // message role; "user" for turn.steer
	Origin     string // origin kind of a user message or steer
	Source     string // appended meta.source
	ID         string // message id: ctx message.id, appended userMessageId or messageId
	ToolCallID string
	TurnID     string // stringified turnId of loop, turn.*, and agent.turn.* records
	StepUUID   string // loop event uuid (step.begin/end) or stepUuid
	LeadText   string // joined trimmed text parts of a user message or steer input
	Time       int64  // epoch ms, zero when absent
	Payload    json.RawMessage
}

// harnessLeads open user messages the harness wrote, not the person.
var harnessLeads = []string{"<system-reminder>", "<system>"}

const compactionLead = "<system>Previous context has been compacted"

type textPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func joinTextParts(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var parts []textPart
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var out []string
	for _, p := range parts {
		if p.Text != "" {
			out = append(out, p.Text)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// stringID renders a turnId that may be a JSON number or string.
func stringID(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
		return ""
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return strconv.Quote(string(raw))
}

type lineHead struct {
	Type       string          `json:"type"`
	Time       int64           `json:"time"`
	TurnID     json.RawMessage `json:"turnId"`
	ToolCallID string          `json:"toolCallId"`
	SubagentID string          `json:"subagentId"`
	Input      json.RawMessage `json:"input"`
	Origin     struct {
		Kind string `json:"kind"`
	} `json:"origin"`
	Event struct {
		Type       string          `json:"type"`
		UUID       string          `json:"uuid"`
		StepUUID   string          `json:"stepUuid"`
		TurnID     json.RawMessage `json:"turnId"`
		ToolCallID string          `json:"toolCallId"`
	} `json:"event"`
	Message json.RawMessage `json:"message"`
}

type ctxMessage struct {
	Role       string          `json:"role"`
	ID         string          `json:"id"`
	ToolCallID string          `json:"toolCallId"`
	Content    json.RawMessage `json:"content"`
	Origin     struct {
		Kind string `json:"kind"`
	} `json:"origin"`
}

type appendedEnvelope struct {
	Message struct {
		Role       string          `json:"role"`
		ToolCallID string          `json:"toolCallId"`
		Content    json.RawMessage `json:"content"`
	} `json:"message"`
	Meta struct {
		Source string `json:"source"`
		Origin struct {
			Kind string `json:"kind"`
		} `json:"origin"`
		UserMessageID string `json:"userMessageId"`
		MessageID     string `json:"messageId"`
	} `json:"meta"`
}

// ClassifyLine decodes the head of one wire line and classifies it. A line
// that is not a JSON object is bookkeeping with an empty head.
func ClassifyLine(line []byte) (recordHead, RecordClass) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return recordHead{}, ClassBookkeeping
	}
	var lh lineHead
	if err := json.Unmarshal(line, &lh); err != nil {
		return recordHead{}, ClassBookkeeping
	}
	h := recordHead{Type: lh.Type, Key: lh.Type, Time: lh.Time, Payload: json.RawMessage(line), ToolCallID: lh.ToolCallID, TurnID: stringID(lh.TurnID)}
	switch lh.Type {
	case "context.append_loop_event":
		h.Key = "loop/" + lh.Event.Type
		h.ToolCallID = lh.Event.ToolCallID
		h.TurnID = stringID(lh.Event.TurnID)
		h.StepUUID = lh.Event.StepUUID
		if h.StepUUID == "" {
			h.StepUUID = lh.Event.UUID
		}
	case "context.append_message":
		var m ctxMessage
		_ = json.Unmarshal(lh.Message, &m)
		h.Key = "ctx/" + m.Role
		h.Role = m.Role
		h.ID = m.ID
		h.Origin = m.Origin.Kind
		h.ToolCallID = m.ToolCallID
		if m.Role == "user" {
			h.LeadText = joinTextParts(m.Content)
		}
	case "agent.message.appended":
		var a appendedEnvelope
		_ = json.Unmarshal(lh.Message, &a)
		h.Key = "appended/" + a.Message.Role
		h.Role = a.Message.Role
		h.Source = a.Meta.Source
		h.Origin = a.Meta.Origin.Kind
		h.ToolCallID = a.Message.ToolCallID
		switch a.Message.Role {
		case "user":
			h.ID = a.Meta.UserMessageID
			h.LeadText = joinTextParts(a.Message.Content)
		case "assistant":
			h.ID = a.Meta.MessageID
		}
	case "turn.steer":
		h.Role = "user"
		h.Origin = lh.Origin.Kind
		h.LeadText = joinTextParts(lh.Input)
	}
	return h, classify(h)
}

func classify(h recordHead) RecordClass {
	switch {
	case h.Type == "metadata":
		return ClassSessionState
	case strings.HasPrefix(h.Key, "loop/"):
		switch h.Key {
		case "loop/step.begin", "loop/step.end":
			return ClassEvent
		case "loop/content.part", "loop/tool.call", "loop/tool.result":
			return ClassTurn
		default:
			return ClassUnknown
		}
	case strings.HasPrefix(h.Key, "ctx/"):
		switch h.Role {
		case "user":
			class, _, _ := userClass(h)
			return class
		case "assistant", "tool":
			return ClassTurn
		default:
			return ClassUnknown
		}
	case strings.HasPrefix(h.Key, "appended/"):
		switch h.Role {
		case "user":
			class, _, _ := userClass(h)
			return class
		case "assistant", "tool":
			return ClassTurn
		default:
			return ClassUnknown
		}
	case h.Type == "turn.steer":
		return ClassContext
	case h.Type == "turn.step.interrupted", h.Type == "turn.ended",
		h.Type == "subagent.spawned", h.Type == "subagent.completed", h.Type == "subagent.failed", h.Type == "subagent.cancelled",
		h.Type == "task.started", h.Type == "task.terminated",
		h.Type == "plan.revision",
		h.Type == "permission.record_approval_result":
		return ClassEvent
	}
	if isBookkeepingType(h.Type) {
		return ClassBookkeeping
	}
	return ClassUnknown
}

func isBookkeepingType(t string) bool {
	switch t {
	case "turn.prompt", "subagent.started", "task.waitDelivered", "turn.step.retrying", "turn.cancel",
		"profile.bind", "runtime.set_binding", "permission.set_mode", "config.update",
		"plugin.session_start", "changeNotifier.snapshot", "staleGuard.recorded":
		return true
	}
	for _, prefix := range []string{"prompt.", "agent.turn.", "interaction.", "usage.", "token_counting.", "llm.", "tools.", "file_history.", "swarm_mode.", "plan_mode."} {
		if strings.HasPrefix(t, prefix) {
			return true
		}
	}
	return false
}

// splitGitContext separates a leading <git-context …>…</git-context> block
// from the text that follows it. Text without such a block is returned as
// the remainder unchanged; an unterminated block leaves no remainder.
func splitGitContext(text string) (block, rest string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "<git-context") {
		return "", text
	}
	const closing = "</git-context>"
	i := strings.Index(text, closing)
	if i < 0 {
		return text, ""
	}
	return text[:i+len(closing)], strings.TrimSpace(text[i+len(closing):])
}

// userClass decides a user message (appended input, context message, steer,
// or appended notify) from its origin and lead text: class, kind, subtype.
func userClass(h recordHead) (RecordClass, thinkt.EntryKind, string) {
	text := strings.TrimSpace(h.LeadText)
	switch h.Origin {
	case "injection":
		return ClassContext, thinkt.KindMeta, "injection"
	case "system_trigger":
		// A subagent's task usually follows a <git-context> block in the
		// same message; only a bare preamble or a nudge is harness text.
		_, task := splitGitContext(text)
		if task == "" || task == "continue" || strings.HasPrefix(task, "Working directory:") {
			return ClassContext, thinkt.KindMeta, "system_trigger"
		}
		return ClassTurn, thinkt.KindNone, "system_trigger"
	case "task", "background_task":
		return ClassContext, thinkt.KindTaskNotification, h.Origin
	case "skill_activation":
		return ClassContext, thinkt.KindMeta, "skill_activation"
	case "shell_command":
		return ClassTurn, thinkt.KindCommandOutput, "shell_command"
	case "user":
		return ClassTurn, thinkt.KindNone, ""
	}
	if h.Source == "notify" {
		return ClassContext, thinkt.KindTaskNotification, "notify"
	}
	if strings.HasPrefix(text, compactionLead) {
		return ClassContext, thinkt.KindCompactSummary, "compaction"
	}
	for _, lead := range harnessLeads {
		if strings.HasPrefix(text, lead) {
			return ClassContext, thinkt.KindMeta, "injection"
		}
	}
	return ClassTurn, thinkt.KindNone, ""
}

// NewSearchFilter returns a per-file predicate for the search scanner. It
// must see every line of one wire file in order, and reports whether a hit
// in that line is conversation content. It applies the parser's user-message
// rule: whichever of an appended input record and its context twin comes
// first is the prompt, the later one is a twin. Lines that would become
// secondary entries are not searchable.
func NewSearchFilter() func(line []byte) bool {
	seenUserIDs := map[string]bool{}
	return func(line []byte) bool {
		h, class := ClassifyLine(line)
		if !class.IsEntry() {
			return false
		}
		switch h.Key {
		case "appended/user", "ctx/user":
			if h.Key == "appended/user" && h.Source != "input" {
				return false
			}
			if h.ID != "" {
				if seenUserIDs[h.ID] {
					return false
				}
				seenUserIDs[h.ID] = true
			}
			c, kind, _ := userClass(h)
			return c == ClassTurn && kind == thinkt.KindNone
		case "loop/content.part", "loop/tool.call", "loop/tool.result", "ctx/assistant", "ctx/tool":
			return true
		}
		return false
	}
}

// IsSearchable reports whether a hit inside this line, read on its own, is
// conversation content. The search scanner uses NewSearchFilter instead,
// which also knows which user messages earlier lines already carried.
func IsSearchable(line []byte) bool {
	return NewSearchFilter()(line)
}
