package codex

import (
	"encoding/json"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// turnState carries the model, effort, and turn id the latest turn_context
// (or task_started) announced, so every entry of the turn can be stamped.
type turnState struct {
	TurnID, Model, Effort string
}

// apply updates the state from a turn_context or task_started line.
func (t *turnState) apply(h recordHead) {
	switch h.Key() {
	case "turn_context":
		var tc struct {
			TurnID string `json:"turn_id"`
			Model  string `json:"model"`
			Effort string `json:"effort"`
		}
		if json.Unmarshal(h.Payload, &tc) != nil {
			return
		}
		if tc.TurnID != "" {
			t.TurnID = tc.TurnID
		}
		if tc.Model != "" {
			t.Model = tc.Model
		}
		if tc.Effort != "" {
			t.Effort = tc.Effort
		}
	case "event_msg/task_started":
		var ts struct {
			TurnID string `json:"turn_id"`
		}
		if json.Unmarshal(h.Payload, &ts) == nil && ts.TurnID != "" {
			t.TurnID = ts.TurnID
		}
	}
}

// converter turns one classified rollout line into zero, one, or two entries.
// calls remembers wire call_id → tool name so an output line can inherit the
// visibility of its call.
type converter struct {
	sessionID string
	lineNo    int
	turn      *turnState
	calls     map[string]string
}

// newEntry builds the common skeleton: UUID (given, else composed from the
// line), role, timestamp, source, turn id, model, and effort.
func (c *converter) newEntry(role thinkt.Role, h recordHead, uuid string) thinkt.Entry {
	if uuid == "" {
		uuid = composeUUID(c.sessionID, c.lineNo, h.Key(), "")
	}
	e := thinkt.Entry{
		UUID:      uuid,
		Role:      role,
		Timestamp: parseTimestamp(h.Timestamp),
		Source:    thinkt.SourceCodex,
		Metadata:  map[string]any{},
	}
	if c.turn != nil {
		e.TurnID = c.turn.TurnID
		e.Model = c.turn.Model
		e.Effort = c.turn.Effort
	}
	if id := payloadTurnID(h.Payload); id != "" {
		e.TurnID = id
	}
	return e
}

func payloadTurnID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var p struct {
		TurnID      string `json:"turn_id"`
		Passthrough struct {
			TurnID string `json:"turn_id"`
		} `json:"internal_chat_message_metadata_passthrough"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return ""
	}
	if p.TurnID != "" {
		return p.TurnID
	}
	return p.Passthrough.TurnID
}

// convert returns the entries for one line: nil for non-entry classes, for
// token_usage_record (the parser merges it), and for lines whose content is
// empty. Dedupe candidates are converted like any turn line; the parser
// decides whether to keep them.
func (c *converter) convert(h recordHead, class RecordClass, line []byte) []thinkt.Entry {
	if !class.IsEntry() {
		return nil
	}
	switch h.Key() {
	case "token_usage_record":
		return nil
	case "response_item/message":
		return c.convertWireMessage(h, class)
	case "response_item/reasoning":
		return c.convertWireReasoning(h)
	case "response_item/agent_message":
		return c.convertWireAgentMessage(h)
	case "response_item/function_call", "response_item/custom_tool_call":
		return c.convertWireCall(h)
	case "response_item/function_call_output", "response_item/custom_tool_call_output":
		return c.convertWireOutput(h)
	case "item/AgentMessage":
		return c.convertAgentMessageItem(h)
	case "item/Reasoning":
		return c.convertReasoningItem(h)
	case "item/Plan":
		return c.convertPlanItem(h)
	case "item/SubAgentActivity":
		return c.convertSubAgentActivity(h)
	case "item/CollabAgentToolCall":
		return c.convertCollabToolCall(h)
	case "item/CommandExecution", "item/FileChange", "item/McpToolCall", "item/WebSearch", "item/ImageView", "item/Extension":
		if class == ClassUnknown {
			return c.convertUnknown(h, line)
		}
		return c.convertToolItem(h)
	case "event_msg/task_complete":
		return c.convertTaskComplete(h)
	case "event_msg/turn_aborted":
		return c.convertTurnAborted(h)
	case "compacted":
		return c.convertCompacted(h)
	case "event_msg/user_message", "event_msg/agent_message", "event_msg/agent_reasoning":
		return c.convertLegacyEvent(h)
	}
	if class == ClassUnknown {
		return c.convertUnknown(h, line)
	}
	return nil
}

// contentPart is one element of a wire message's content array.
type contentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ImageURL string `json:"image_url"`
}

type wireMessage struct {
	ID          string        `json:"id"`
	Role        string        `json:"role"`
	Phase       string        `json:"phase"`
	Author      string        `json:"author"`
	Recipient   string        `json:"recipient"`
	Content     []contentPart `json:"content"`
	Passthrough struct {
		ContentItemKinds []string `json:"content_item_kinds"`
	} `json:"internal_chat_message_metadata_passthrough"`
}

func joinParts(parts []contentPart, types ...string) string {
	var out []string
	for _, p := range parts {
		if len(types) > 0 && !contains(types, p.Type) {
			continue
		}
		if p.Text != "" {
			out = append(out, p.Text)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (c *converter) convertWireMessage(h recordHead, class RecordClass) []thinkt.Entry {
	var m wireMessage
	if json.Unmarshal(h.Payload, &m) != nil {
		return nil
	}
	switch h.Role {
	case "user":
		_, kind := userMessageClass(h)
		e := c.newEntry(thinkt.RoleUser, h, "")
		e.Text = joinParts(m.Content, "input_text", "text")
		switch kind {
		case thinkt.KindNone:
			e.ContentBlocks = imageBlocks(m.Content, e.Text)
		case thinkt.KindCommandOutput:
			e.Kind = kind
			e.Visibility = thinkt.VisibilitySecondary
			e.Subtype = "shell.user_command"
		default:
			e.Kind = thinkt.KindMeta
			e.Visibility = thinkt.VisibilitySecondary
			if len(h.Kinds) > 0 {
				e.Subtype = h.Kinds[0]
			} else {
				e.Subtype = harnessTag(h.LeadText)
			}
		}
		if e.Text == "" && len(e.ContentBlocks) == 0 {
			return nil
		}
		return []thinkt.Entry{e}
	case "developer":
		e := c.newEntry(thinkt.RoleSystem, h, "")
		e.Kind = thinkt.KindMeta
		e.Visibility = thinkt.VisibilitySecondary
		e.Subtype = "developer"
		if len(h.Kinds) > 0 {
			e.Subtype = h.Kinds[0]
		}
		e.Text = joinParts(m.Content)
		if e.Text == "" {
			return nil
		}
		return []thinkt.Entry{e}
	case "assistant":
		e := c.newEntry(thinkt.RoleAssistant, h, m.ID)
		e.Text = joinParts(m.Content)
		if m.Phase != "" {
			e.Metadata["phase"] = m.Phase
		}
		if e.Text == "" {
			return nil
		}
		return []thinkt.Entry{e}
	}
	return nil
}

// imageBlocks turns input_image parts into image blocks (data URLs) or text
// blocks (other URLs), with a leading text block when text is present.
func imageBlocks(parts []contentPart, text string) []thinkt.ContentBlock {
	var blocks []thinkt.ContentBlock
	for _, p := range parts {
		if p.Type != "input_image" || p.ImageURL == "" {
			continue
		}
		if mediaType, data, ok := parseDataURL(p.ImageURL); ok {
			blocks = append(blocks, thinkt.ContentBlock{Type: "image", MediaType: mediaType, MediaData: data})
		} else {
			blocks = append(blocks, thinkt.ContentBlock{Type: "text", Text: p.ImageURL})
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	if text != "" {
		blocks = append([]thinkt.ContentBlock{{Type: "text", Text: text}}, blocks...)
	}
	return blocks
}

func parseDataURL(url string) (mediaType, data string, ok bool) {
	if !strings.HasPrefix(url, "data:") {
		return "", "", false
	}
	rest := url[len("data:"):]
	semi := strings.IndexByte(rest, ';')
	if semi < 0 {
		return "", "", false
	}
	mediaType = rest[:semi]
	idx := strings.Index(rest, "base64,")
	if idx < 0 {
		return "", "", false
	}
	return mediaType, rest[idx+len("base64,"):], true
}

func (c *converter) convertWireReasoning(h recordHead) []thinkt.Entry {
	if !h.HasSummary {
		return nil
	}
	var payload map[string]any
	if json.Unmarshal(h.Payload, &payload) != nil {
		return nil
	}
	thinking := extractReasoningText(payload)
	if thinking == "" {
		return nil
	}
	e := c.newEntry(thinkt.RoleAssistant, h, readString(payload, "id"))
	e.ContentBlocks = []thinkt.ContentBlock{{Type: "thinking", Thinking: thinking}}
	return []thinkt.Entry{e}
}

func (c *converter) convertWireAgentMessage(h recordHead) []thinkt.Entry {
	var m wireMessage
	if json.Unmarshal(h.Payload, &m) != nil {
		return nil
	}
	e := c.newEntry(thinkt.RoleUser, h, m.ID)
	e.Kind = thinkt.KindAgentMessage
	e.Text = joinParts(m.Content)
	if m.Author != "" {
		e.Metadata["author"] = m.Author
	}
	if m.Recipient != "" {
		e.Metadata["recipient"] = m.Recipient
	}
	if e.Text == "" {
		return nil
	}
	return []thinkt.Entry{e}
}

// itemPayload is the envelope of an event_msg/item_completed payload.
type itemPayload struct {
	TurnID string          `json:"turn_id"`
	Item   json.RawMessage `json:"item"`
}

func decodeItem(h recordHead, v any) bool {
	var ip itemPayload
	if json.Unmarshal(h.Payload, &ip) != nil || len(ip.Item) == 0 {
		return false
	}
	return json.Unmarshal(ip.Item, v) == nil
}

func (c *converter) convertAgentMessageItem(h recordHead) []thinkt.Entry {
	var item struct {
		ID        string          `json:"id"`
		Phase     string          `json:"phase"`
		Content   []contentPart   `json:"content"`
		Questions json.RawMessage `json:"questions"`
	}
	if !decodeItem(h, &item) {
		return nil
	}
	e := c.newEntry(thinkt.RoleAssistant, h, item.ID)
	e.Text = joinParts(item.Content)
	if item.Phase != "" {
		e.Metadata["phase"] = item.Phase
	}
	if len(item.Questions) > 0 && string(item.Questions) != "null" {
		e.Metadata["questions"] = item.Questions
	}
	if e.Text == "" {
		return nil
	}
	return []thinkt.Entry{e}
}

func (c *converter) convertReasoningItem(h recordHead) []thinkt.Entry {
	var item struct {
		ID          string        `json:"id"`
		SummaryText []contentPart `json:"summary_text"`
	}
	if !decodeItem(h, &item) {
		return nil
	}
	thinking := joinParts(item.SummaryText)
	if thinking == "" {
		return nil
	}
	e := c.newEntry(thinkt.RoleAssistant, h, item.ID)
	e.ContentBlocks = []thinkt.ContentBlock{{Type: "thinking", Thinking: thinking}}
	return []thinkt.Entry{e}
}

func (c *converter) convertPlanItem(h recordHead) []thinkt.Entry {
	var item struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	if !decodeItem(h, &item) || strings.TrimSpace(item.Text) == "" {
		return nil
	}
	e := c.newEntry(thinkt.RoleAssistant, h, item.ID)
	e.Subtype = "plan"
	e.Text = item.Text
	return []thinkt.Entry{e}
}

func (c *converter) convertSubAgentActivity(h recordHead) []thinkt.Entry {
	var item struct {
		ID            string `json:"id"`
		Kind          string `json:"kind"`
		AgentThreadID string `json:"agent_thread_id"`
		AgentPath     string `json:"agent_path"`
	}
	if !decodeItem(h, &item) {
		return nil
	}
	uuid := item.ID
	if uuid != "" && item.Kind != "" {
		uuid += ":" + item.Kind
	}
	e := c.newEntry(thinkt.RoleSystem, h, uuid)
	e.Kind = thinkt.KindAgentMessage
	e.Subtype = item.Kind
	e.ChildSessionID = item.AgentThreadID
	e.Text = strings.TrimSpace(item.Kind + " " + item.AgentPath)
	if item.Kind == "interacted" {
		e.Visibility = thinkt.VisibilitySecondary
	}
	if e.Text == "" {
		return nil
	}
	return []thinkt.Entry{e}
}

func (c *converter) convertCollabToolCall(h recordHead) []thinkt.Entry {
	var item struct {
		ID        string   `json:"id"`
		Tool      string   `json:"tool"`
		Receivers []string `json:"receiver_thread_ids"`
	}
	if !decodeItem(h, &item) {
		return nil
	}
	uuid := item.ID
	if uuid != "" {
		uuid += ":collab"
	}
	e := c.newEntry(thinkt.RoleSystem, h, uuid)
	e.Kind = thinkt.KindAgentMessage
	e.Visibility = thinkt.VisibilitySecondary
	e.Subtype = item.Tool
	e.Text = strings.TrimSpace(item.Tool + " " + strings.Join(item.Receivers, " "))
	if e.Text == "" {
		return nil
	}
	return []thinkt.Entry{e}
}

func (c *converter) convertTaskComplete(h recordHead) []thinkt.Entry {
	var p struct {
		DurationMs   int64 `json:"duration_ms"`
		TTFTMs       int64 `json:"time_to_first_token_ms"`
		ErrorMessage struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(h.Payload, &p)
	e := c.newEntry(thinkt.RoleSystem, h, "")
	e.Kind = thinkt.KindTurnDuration
	e.Visibility = thinkt.VisibilitySecondary
	e.Text = "turn complete"
	e.Metadata["duration_ms"] = p.DurationMs
	e.Metadata["time_to_first_token_ms"] = p.TTFTMs
	e.Error = p.ErrorMessage.Message
	return []thinkt.Entry{e}
}

func (c *converter) convertTurnAborted(h recordHead) []thinkt.Entry {
	var p struct {
		Reason     string `json:"reason"`
		DurationMs int64  `json:"duration_ms"`
	}
	_ = json.Unmarshal(h.Payload, &p)
	e := c.newEntry(thinkt.RoleSystem, h, "")
	e.Kind = thinkt.KindTurnDuration
	e.Visibility = thinkt.VisibilitySecondary
	e.Text = "turn aborted"
	e.Metadata["duration_ms"] = p.DurationMs
	e.Error = p.Reason
	return []thinkt.Entry{e}
}

func (c *converter) convertCompacted(h recordHead) []thinkt.Entry {
	var p struct {
		Message string `json:"message"`
		Latest  *struct {
			ThreadTokenUsage struct {
				TotalTokens int `json:"total_tokens"`
			} `json:"thread_token_usage"`
		} `json:"latest_token_usage_record"`
	}
	_ = json.Unmarshal(h.Payload, &p)
	e := c.newEntry(thinkt.RoleSystem, h, "")
	e.Kind = thinkt.KindCompactBoundary
	e.Text = strings.TrimSpace(p.Message)
	if e.Text == "" {
		e.Text = "Context compacted"
	}
	e.Compaction = &thinkt.CompactionInfo{Trigger: "auto"}
	if p.Latest != nil {
		e.Compaction.PreTokens = p.Latest.ThreadTokenUsage.TotalTokens
	}
	return []thinkt.Entry{e}
}

// convertLegacyEvent handles the pre-item event_msg user_message,
// agent_message, and agent_reasoning records.
func (c *converter) convertLegacyEvent(h recordHead) []thinkt.Entry {
	var payload map[string]any
	if json.Unmarshal(h.Payload, &payload) != nil {
		return nil
	}
	switch h.PayloadType {
	case "user_message":
		text := strings.TrimSpace(readString(payload, "message"))
		if text == "" {
			return nil
		}
		e := c.newEntry(thinkt.RoleUser, h, "")
		e.Text = text
		return []thinkt.Entry{e}
	case "agent_message":
		text := strings.TrimSpace(readString(payload, "message"))
		if text == "" {
			return nil
		}
		e := c.newEntry(thinkt.RoleAssistant, h, "")
		e.Text = text
		return []thinkt.Entry{e}
	case "agent_reasoning":
		thinking := strings.TrimSpace(readString(payload, "text"))
		if thinking == "" {
			return nil
		}
		e := c.newEntry(thinkt.RoleAssistant, h, "")
		e.ContentBlocks = []thinkt.ContentBlock{{Type: "thinking", Thinking: thinking}}
		return []thinkt.Entry{e}
	}
	return nil
}

func (c *converter) convertUnknown(h recordHead, line []byte) []thinkt.Entry {
	e := c.newEntry(thinkt.RoleSystem, h, "")
	e.Kind = thinkt.KindUnknown
	e.Visibility = thinkt.VisibilitySecondary
	e.Text = string(line)
	return []thinkt.Entry{e}
}
