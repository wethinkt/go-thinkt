package codex

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// The classifier sorts codex lines into thinkt.RecordClass values:
//   thinkt.ClassTurn         user, assistant, tool pairs
//   thinkt.ClassEvent        system events: task_complete, turn_aborted, compacted, subagent activity
//   thinkt.ClassContext      harness-injected context: developer messages, environment, AGENTS.md
//   thinkt.ClassCheckpoint   never returned
//   thinkt.ClassSessionState session_meta, turn_context
//   thinkt.ClassBookkeeping  dropped: harness mirrors, encrypted reasoning, counters
//   thinkt.ClassUnknown      key never seen; emitted as a secondary system entry so drift is visible

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
	LeadText    string // first non-empty text part of a wire message or reasoning record, trimmed
	HasContent  bool   // a wire message carries text or an input_image
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
		Content     json.RawMessage `json:"content"`
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

// leadText joins the non-empty text parts of a content array, trimmed, the
// way the converter joins them, so the scan and the parser compare the same
// text when deciding whether a wire record twins a harness item.
func leadText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var parts []textPart
	if err := json.Unmarshal(raw, &parts); err != nil {
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

// ClassifyLine decodes the head of one rollout line and classifies it. A line
// that is not a JSON object is bookkeeping with an empty head.
func ClassifyLine(line []byte) (recordHead, thinkt.RecordClass) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return recordHead{}, thinkt.ClassBookkeeping
	}
	var lh lineHead
	if err := json.Unmarshal(line, &lh); err != nil {
		return recordHead{}, thinkt.ClassBookkeeping
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
			switch ph.Item.Type {
			case "Reasoning":
				h.HasSummary = anyText(ph.Item.SummaryText)
				h.LeadText = leadText(ph.Item.SummaryText)
			case "AgentMessage":
				h.LeadText = leadText(ph.Item.Content)
			}
		}
	case "response_item":
		switch ph.Type {
		case "message", "agent_message":
			h.LeadText = leadText(ph.Content)
			h.HasContent = h.LeadText != "" || hasImage(ph.Content)
		case "reasoning":
			h.HasSummary = anyText(ph.Summary) || strings.TrimSpace(ph.Text) != ""
			h.LeadText = leadText(ph.Summary)
			if h.LeadText == "" {
				h.LeadText = strings.TrimSpace(ph.Text)
			}
		}
	}
	return h, classify(h)
}

func hasImage(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var parts []struct {
		Type     string `json:"type"`
		ImageURL string `json:"image_url"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return false
	}
	for _, p := range parts {
		if p.Type == "input_image" && p.ImageURL != "" {
			return true
		}
	}
	return false
}

// isLegacyTwin reports whether wire line h repeats the legacy event
// (event_msg user_message / agent_message / agent_reasoning) that preceded
// it, so the pair yields one entry.
func isLegacyTwin(legacyKey, legacyText string, h recordHead) bool {
	if h.Type != "response_item" || legacyText == "" || h.LeadText != legacyText {
		return false
	}
	switch legacyKey {
	case "event_msg/user_message":
		return h.PayloadType == "message" && h.Role == "user"
	case "event_msg/agent_message":
		return h.PayloadType == "message" && h.Role == "assistant"
	case "event_msg/agent_reasoning":
		return h.PayloadType == "reasoning"
	}
	return false
}

func classify(h recordHead) thinkt.RecordClass {
	switch h.Type {
	case "session_meta", "turn_context":
		return thinkt.ClassSessionState
	case "token_usage_record", "compacted":
		return thinkt.ClassEvent
	case "world_state", "inter_agent_communication_metadata":
		return thinkt.ClassBookkeeping
	case "event_msg":
		return classifyEvent(h)
	case "response_item":
		return classifyWire(h)
	default:
		return thinkt.ClassUnknown
	}
}

func classifyEvent(h recordHead) thinkt.RecordClass {
	switch h.PayloadType {
	case "item_completed":
		switch h.ItemType {
		case "UserMessage", "ContextCompaction":
			return thinkt.ClassBookkeeping
		case "AgentMessage", "Plan", "CommandExecution", "FileChange", "McpToolCall", "WebSearch", "ImageView":
			return thinkt.ClassTurn
		case "Reasoning":
			if h.HasSummary {
				return thinkt.ClassTurn
			}
			return thinkt.ClassBookkeeping
		case "Extension":
			if h.ItemKind == "web.search" || h.ItemKind == "clock.sleep" {
				return thinkt.ClassTurn
			}
			return thinkt.ClassUnknown
		case "SubAgentActivity", "CollabAgentToolCall":
			return thinkt.ClassEvent
		default:
			return thinkt.ClassUnknown
		}
	case "task_complete", "turn_aborted":
		return thinkt.ClassEvent
	case "task_started", "token_count", "thread_settings_applied":
		return thinkt.ClassBookkeeping
	case "user_message", "agent_message", "agent_reasoning":
		return thinkt.ClassTurn // legacy pre-item events
	default:
		return thinkt.ClassUnknown
	}
}

func classifyWire(h recordHead) thinkt.RecordClass {
	switch h.PayloadType {
	case "message":
		switch h.Role {
		case "user":
			class, _ := userMessageClass(h)
			return class
		case "developer":
			return thinkt.ClassContext
		case "assistant":
			return thinkt.ClassTurn
		default:
			return thinkt.ClassUnknown
		}
	case "reasoning":
		if h.HasSummary {
			return thinkt.ClassTurn
		}
		return thinkt.ClassBookkeeping
	case "function_call", "custom_tool_call":
		if orchestrationTools[h.Name] {
			return thinkt.ClassEvent
		}
		return thinkt.ClassTurn
	case "function_call_output", "custom_tool_call_output":
		// Outputs carry no tool name; the parser reclassifies one whose call
		// it remembers as orchestration.
		return thinkt.ClassTurn
	case "agent_message":
		return thinkt.ClassTurn
	case "web_search_call", "additional_tools":
		return thinkt.ClassBookkeeping
	default:
		return thinkt.ClassUnknown
	}
}

// userMessageClass decides a user-role wire message from its content item
// kinds, falling back to a harness tag sniff when the kinds are absent.
func userMessageClass(h recordHead) (thinkt.RecordClass, thinkt.EntryKind) {
	if len(h.Kinds) == 0 {
		if hasHarnessTag(h.LeadText) {
			return thinkt.ClassContext, thinkt.KindMeta
		}
		return thinkt.ClassTurn, thinkt.KindNone
	}
	for _, k := range h.Kinds {
		if strings.HasPrefix(k, "user.") {
			return thinkt.ClassTurn, thinkt.KindNone
		}
	}
	for _, k := range h.Kinds {
		if k == "shell.user_command" {
			return thinkt.ClassTurn, thinkt.KindCommandOutput
		}
	}
	return thinkt.ClassContext, thinkt.KindMeta
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
func entryWeight(h recordHead, class thinkt.RecordClass) int {
	if !class.IsEntry() || isDedupeCandidate(h) || h.Type == "token_usage_record" {
		return 0
	}
	switch h.Key() {
	case "response_item/message", "response_item/agent_message":
		if !h.HasContent {
			return 0
		}
		return 1
	case "item/CommandExecution", "item/FileChange", "item/McpToolCall", "item/WebSearch", "item/ImageView":
		return 2
	case "item/Extension":
		if h.ItemKind == "web.search" || h.ItemKind == "clock.sleep" {
			return 2
		}
	}
	return 1
}

// IsSearchable reports whether a hit inside this line is conversation
// content: false for non-entry classes and for wire twins of harness items
// (assistant messages, reasoning); otherwise true when the line yields at
// least one entry that is not secondary.
func IsSearchable(line []byte) bool {
	h, class := ClassifyLine(line)
	if !class.IsEntry() || isDedupeCandidate(h) {
		return false
	}
	c := &converter{turn: &turnState{}, calls: map[string]string{}}
	for _, e := range c.convert(h, class, line) {
		if !e.IsSecondary() {
			return true
		}
	}
	return false
}
