package codex

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// RecordClass says what a rollout line is for, decided from its record key
// (type, payload.type, item.type) and a few cheap payload fields. The values
// mirror claude.RecordClass so a later lift into thinkt is mechanical.
type RecordClass int

const (
	ClassTurn         RecordClass = iota // user, assistant, tool pairs
	ClassEvent                           // system events: task_complete, turn_aborted, compacted, subagent activity
	ClassContext                         // harness-injected context: developer messages, environment, AGENTS.md
	ClassCheckpoint                      // declared for parity with claude.RecordClass; never returned
	ClassSessionState                    // session_meta, turn_context
	ClassBookkeeping                     // dropped: harness mirrors, encrypted reasoning, counters
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

// recordHead is the cheap view of one rollout line that classification and
// conversion share. Payload is the raw payload object so the converter does
// not split the line again.
type recordHead struct {
	Timestamp   string
	Type        string
	PayloadType string
	ItemType    string
	ItemKind    string
	Role        string
	Name        string
	Kinds       []string
	LeadText    string
	HasSummary  bool
	Payload     json.RawMessage
}

// Key is the record key the class table is written in: "<type>" for
// top-level records, "event_msg/<payload.type>", "response_item/<payload.type>",
// and "item/<item.type>" for event_msg/item_completed.
func (h recordHead) Key() string {
	switch h.Type {
	case "event_msg":
		if h.PayloadType == "item_completed" {
			return "item/" + h.ItemType
		}
		return "event_msg/" + h.PayloadType
	case "response_item":
		return "response_item/" + h.PayloadType
	default:
		return h.Type
	}
}

// orchestrationTools are the wire tool names whose structured harness item
// carries the same call; their wire pair is kept as a secondary KindToolWire.
var orchestrationTools = map[string]bool{
	"exec": true, "wait": true, "js": true, "apply_patch": true, "sleep": true,
	"spawn_agent": true, "send_message": true, "wait_agent": true, "interrupt_agent": true,
	"followup_task": true, "request_user_input_async": true,
}

// harnessTags open user-role wire messages the harness wrote, not the person.
var harnessTags = []string{
	"<environment_context>",
	"<turn_aborted>",
	"# AGENTS.md instructions",
	"<permissions instructions>",
	"<collaboration_mode>",
	"<user_shell_command>",
}

// harnessTag returns the tag the trimmed text starts with, or "".
func harnessTag(text string) string {
	text = strings.TrimSpace(text)
	for _, tag := range harnessTags {
		if strings.HasPrefix(text, tag) {
			return tag
		}
	}
	return ""
}

func hasHarnessTag(text string) bool { return harnessTag(text) != "" }

type lineHead struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type payloadHead struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Name    string          `json:"name"`
	Text    string          `json:"text"`
	Content json.RawMessage `json:"content"`
	Summary json.RawMessage `json:"summary"`
	Item    struct {
		Type        string          `json:"type"`
		Kind        string          `json:"kind"`
		SummaryText json.RawMessage `json:"summary_text"`
	} `json:"item"`
	Passthrough struct {
		ContentItemKinds []string `json:"content_item_kinds"`
	} `json:"internal_chat_message_metadata_passthrough"`
}

type textPart struct {
	Text string `json:"text"`
}

func anyText(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var parts []textPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return false
	}
	for _, p := range parts {
		if strings.TrimSpace(p.Text) != "" {
			return true
		}
	}
	return false
}

func leadText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var parts []textPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	for _, p := range parts {
		if t := strings.TrimSpace(p.Text); t != "" {
			return t
		}
	}
	return ""
}

// ClassifyLine decodes the head of one rollout line and classifies it. A line
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
	h := recordHead{Timestamp: lh.Timestamp, Type: lh.Type, Payload: lh.Payload}
	var ph payloadHead
	if len(lh.Payload) > 0 {
		_ = json.Unmarshal(lh.Payload, &ph)
	}
	h.PayloadType = ph.Type
	h.Role = ph.Role
	h.Name = ph.Name
	h.Kinds = ph.Passthrough.ContentItemKinds
	switch lh.Type {
	case "event_msg":
		if ph.Type == "item_completed" {
			h.ItemType = ph.Item.Type
			h.ItemKind = ph.Item.Kind
			if ph.Item.Type == "Reasoning" {
				h.HasSummary = anyText(ph.Item.SummaryText)
			}
		}
	case "response_item":
		switch ph.Type {
		case "message":
			h.LeadText = leadText(ph.Content)
		case "reasoning":
			h.HasSummary = anyText(ph.Summary) || strings.TrimSpace(ph.Text) != ""
		}
	}
	return h, classify(h)
}

func classify(h recordHead) RecordClass {
	switch h.Type {
	case "session_meta", "turn_context":
		return ClassSessionState
	case "token_usage_record", "compacted":
		return ClassEvent
	case "world_state", "inter_agent_communication_metadata":
		return ClassBookkeeping
	case "event_msg":
		return classifyEvent(h)
	case "response_item":
		return classifyWire(h)
	default:
		return ClassUnknown
	}
}

func classifyEvent(h recordHead) RecordClass {
	switch h.PayloadType {
	case "item_completed":
		switch h.ItemType {
		case "UserMessage", "ContextCompaction":
			return ClassBookkeeping
		case "AgentMessage", "Plan", "CommandExecution", "FileChange", "McpToolCall", "WebSearch", "ImageView":
			return ClassTurn
		case "Reasoning":
			if h.HasSummary {
				return ClassTurn
			}
			return ClassBookkeeping
		case "Extension":
			if h.ItemKind == "web.search" || h.ItemKind == "clock.sleep" {
				return ClassTurn
			}
			return ClassUnknown
		case "SubAgentActivity", "CollabAgentToolCall":
			return ClassEvent
		default:
			return ClassUnknown
		}
	case "task_complete", "turn_aborted":
		return ClassEvent
	case "task_started", "token_count", "thread_settings_applied":
		return ClassBookkeeping
	case "user_message", "agent_message", "agent_reasoning":
		return ClassTurn // legacy pre-item events
	default:
		return ClassUnknown
	}
}

func classifyWire(h recordHead) RecordClass {
	switch h.PayloadType {
	case "message":
		switch h.Role {
		case "user":
			class, _ := userMessageClass(h)
			return class
		case "developer":
			return ClassContext
		case "assistant":
			return ClassTurn
		default:
			return ClassUnknown
		}
	case "reasoning":
		if h.HasSummary {
			return ClassTurn
		}
		return ClassBookkeeping
	case "function_call", "custom_tool_call":
		if orchestrationTools[h.Name] {
			return ClassEvent
		}
		return ClassTurn
	case "function_call_output", "custom_tool_call_output":
		// Outputs carry no tool name; the parser reclassifies one whose call
		// it remembers as orchestration.
		return ClassTurn
	case "agent_message":
		return ClassTurn
	case "web_search_call", "additional_tools":
		return ClassBookkeeping
	default:
		return ClassUnknown
	}
}

// userMessageClass decides a user-role wire message from its content item
// kinds, falling back to a harness tag sniff when the kinds are absent.
func userMessageClass(h recordHead) (RecordClass, thinkt.EntryKind) {
	if len(h.Kinds) == 0 {
		if hasHarnessTag(h.LeadText) {
			return ClassContext, thinkt.KindMeta
		}
		return ClassTurn, thinkt.KindNone
	}
	for _, k := range h.Kinds {
		if strings.HasPrefix(k, "user.") {
			return ClassTurn, thinkt.KindNone
		}
	}
	for _, k := range h.Kinds {
		if k == "shell.user_command" {
			return ClassTurn, thinkt.KindCommandOutput
		}
	}
	return ClassContext, thinkt.KindMeta
}

// isDedupeCandidate reports whether the parser may drop this wire line
// because a harness item carries the same content.
func isDedupeCandidate(h recordHead) bool {
	if h.Type != "response_item" {
		return false
	}
	switch h.PayloadType {
	case "message":
		return h.Role == "assistant"
	case "reasoning":
		return h.HasSummary
	}
	return false
}

// entryWeight is the number of entries one line yields when every dedupe
// candidate is dropped: the session scan sums it for EntryCount.
func entryWeight(h recordHead, class RecordClass) int {
	if !class.IsEntry() || isDedupeCandidate(h) || h.Type == "token_usage_record" {
		return 0
	}
	switch h.Key() {
	case "item/CommandExecution", "item/FileChange", "item/McpToolCall", "item/WebSearch", "item/ImageView":
		return 2
	case "item/Extension":
		if h.ItemKind == "web.search" || h.ItemKind == "clock.sleep" {
			return 2
		}
	}
	return 1
}
