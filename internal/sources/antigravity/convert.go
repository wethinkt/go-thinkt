package antigravity

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

var (
	userRequestRe    = regexp.MustCompile(`(?s)<USER_REQUEST>\n?(.*?)\n?</USER_REQUEST>`)
	additionalMetaRe = regexp.MustCompile(`(?s)<ADDITIONAL_METADATA>\n?(.*?)\n?</ADDITIONAL_METADATA>`)
	settingsChangeRe = regexp.MustCompile(`(?s)<USER_SETTINGS_CHANGE>\n?(.*?)\n?</USER_SETTINGS_CHANGE>`)
	modelSelectionRe = regexp.MustCompile("changed setting `Model Selection` from .*? to (.*?)\\.(?:\\s|$)")
	messageSenderRe  = regexp.MustCompile(`\[Message\] [^\n]*?sender=(\S+)`)
	conversationIDRe = regexp.MustCompile(`"conversationId":\s*"([^"]+)"`)
)

// subagentMarker opens the text of an invoke_subagent result.
const subagentMarker = "Created the following subagents:"

// splitUserInput returns the text inside <USER_REQUEST> (the whole content
// when there is no wrapper) and the harness sections that ride along with it.
func splitUserInput(content string) (request string, meta map[string]string) {
	meta = map[string]string{}
	if m := additionalMetaRe.FindStringSubmatch(content); m != nil {
		meta["additional_metadata"] = strings.TrimSpace(m[1])
	}
	if m := settingsChangeRe.FindStringSubmatch(content); m != nil {
		meta["settings_change"] = strings.TrimSpace(m[1])
	}
	if m := userRequestRe.FindStringSubmatch(content); m != nil {
		return strings.TrimSpace(m[1]), meta
	}
	return strings.TrimSpace(content), meta
}

// modelFromSettingsChange returns the model a `Model Selection` change
// switched to, or "" when the change is about another setting.
func modelFromSettingsChange(s string) string {
	if m := modelSelectionRe.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// stripResultHeader removes the leading "Created At:" and "Completed At:"
// lines a result step starts with and returns their values.
func stripResultHeader(content string) (body string, created, completed string) {
	body = content
	for {
		line, rest, _ := strings.Cut(body, "\n")
		switch {
		case strings.HasPrefix(line, "Created At: "):
			created = strings.TrimPrefix(line, "Created At: ")
		case strings.HasPrefix(line, "Completed At: "):
			completed = strings.TrimPrefix(line, "Completed At: ")
		default:
			return body, created, completed
		}
		body = rest
	}
}

// messageSender returns the sender of a SYSTEM_MESSAGE: "<conversation>/task-N"
// for a background task, a conversation id for a subagent or parent, or
// "system".
func messageSender(content string) string {
	if m := messageSenderRe.FindStringSubmatch(content); m != nil {
		return m[1]
	}
	return ""
}

// messageBody returns the content= part of a SYSTEM_MESSAGE, without the
// preamble and the closing tag; the whole content when it has no [Message].
func messageBody(content string) string {
	i := strings.Index(content, "[Message] ")
	if i < 0 {
		return strings.TrimSpace(content)
	}
	_, body, ok := strings.Cut(content[i:], " content=")
	if !ok {
		return strings.TrimSpace(content)
	}
	body, _, _ = strings.Cut(body, "\n</SYSTEM_MESSAGE>")
	return strings.TrimSpace(body)
}

// isConversationSender reports whether a sender names a conversation (a
// subagent or the parent) rather than a background task or "system".
func isConversationSender(sender string) bool {
	return strings.Contains(sender, "-") && !strings.Contains(sender, "/")
}

// subagentIDs returns the conversation ids an invoke_subagent result lists.
func subagentIDs(content string) []string {
	_, after, ok := strings.Cut(content, subagentMarker)
	if !ok {
		return nil
	}
	var ids []string
	for _, m := range conversationIDRe.FindAllStringSubmatch(after, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

// withMedia appends a text reference for each attachment; the files live
// under the brain dir's .tempmediaStorage and are not read.
func withMedia(text string, media []mediaRef) string {
	for _, m := range media {
		ref := "[" + m.MimeType + ": " + m.URI + "]"
		if text == "" {
			text = ref
		} else {
			text += "\n" + ref
		}
	}
	return text
}

// toolCall is one call of a model turn, with its synthesized id.
type toolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// converter turns steps of one conversation into entries. It carries the
// turn id and the model across steps.
type converter struct {
	conversationID, parentID string
	turnID                   string
	model                    string
	firstParentMessageSeen   bool
}

func (c *converter) newEntry(role thinkt.Role, h stepHead) thinkt.Entry {
	if c.turnID == "" {
		c.turnID = strconv.Itoa(h.Index)
	}
	return thinkt.Entry{
		UUID:      c.conversationID + ":" + strconv.Itoa(h.Index),
		Role:      role,
		Timestamp: h.CreatedAt,
		Source:    thinkt.SourceAntigravity,
		TurnID:    c.turnID,
		Metadata:  map[string]any{"step_type": h.Type, "status": h.Status},
	}
}

// convert turns every class except model turns and tool results into at
// most one entry; ok is false when the step emits nothing.
func (c *converter) convert(h stepHead, class RecordClass) (thinkt.Entry, bool) {
	switch class {
	case ClassUserInput:
		c.turnID = strconv.Itoa(h.Index)
		req, meta := splitUserInput(h.Content)
		if m := modelFromSettingsChange(meta["settings_change"]); m != "" {
			c.model = m
		}
		e := c.newEntry(thinkt.RoleUser, h)
		e.Text = withMedia(req, h.Media)
		for k, v := range meta {
			e.Metadata[k] = v
		}
		return e, e.Text != ""
	case ClassSystemMessage:
		return c.systemMessage(h)
	case ClassHarness:
		text := strings.TrimSpace(h.Content)
		if text == "" {
			return thinkt.Entry{}, false
		}
		e := c.newEntry(thinkt.RoleSystem, h)
		e.Kind, e.Visibility, e.Text = thinkt.KindMeta, thinkt.VisibilitySecondary, text
		return e, true
	case ClassCheckpoint:
		text := strings.TrimSpace(h.Content)
		if text == "" {
			return thinkt.Entry{}, false
		}
		e := c.newEntry(thinkt.RoleSystem, h)
		e.Kind, e.Visibility, e.Text = thinkt.KindCompactSummary, thinkt.VisibilityNormal, text
		return e, true
	case ClassAPIError:
		e := c.newEntry(thinkt.RoleSystem, h)
		e.Kind, e.Visibility, e.Error = thinkt.KindAPIError, thinkt.VisibilitySecondary, h.Error
		e.Text = h.Error
		if e.Text == "" {
			e.Text = strings.TrimSpace(h.Content)
		}
		return e, e.Text != ""
	case ClassUnknown:
		e := c.newEntry(thinkt.RoleSystem, h)
		e.Kind, e.Visibility, e.Subtype = thinkt.KindUnknown, thinkt.VisibilitySecondary, h.Type
		e.Text = strings.TrimSpace(h.Content)
		if e.Text == "" {
			e.Text = h.Type
		}
		return e, true
	default:
		return thinkt.Entry{}, false
	}
}

// systemMessage sorts a SYSTEM_MESSAGE by its sender: the parent's first
// message in a subagent conversation is the task prompt; a background task
// is a notification; another conversation is an agent message; anything
// else is harness meta.
func (c *converter) systemMessage(h stepHead) (thinkt.Entry, bool) {
	sender := messageSender(h.Content)
	body := withMedia(messageBody(h.Content), h.Media)
	if body == "" {
		return thinkt.Entry{}, false
	}
	if c.parentID != "" && sender == c.parentID && !c.firstParentMessageSeen {
		c.firstParentMessageSeen = true
		e := c.newEntry(thinkt.RoleUser, h)
		e.Text = body
		e.Metadata["sender"] = sender
		return e, true
	}
	e := c.newEntry(thinkt.RoleSystem, h)
	e.Text, e.Visibility = body, thinkt.VisibilitySecondary
	e.Metadata["sender"] = sender
	switch {
	case strings.Contains(sender, "/task-"):
		e.Kind = thinkt.KindTaskNotification
	case isConversationSender(sender):
		e.Kind = thinkt.KindAgentMessage
	default:
		e.Kind = thinkt.KindMeta
	}
	return e, true
}

// modelTurnRecord is the part of a PLANNER_RESPONSE the head does not carry.
type modelTurnRecord struct {
	Thinking  string `json:"thinking"`
	ToolCalls []struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args"`
	} `json:"tool_calls"`
	InputTokens     *int `json:"input_tokens"`
	CacheReadTokens *int `json:"cache_read_tokens"`
	OutputTokens    *int `json:"output_tokens"`
}

// modelTurn builds the assistant entry of a PLANNER_RESPONSE: a thinking
// block, a text block, and one tool_use block per call, with ids
// "<step_index>.<n>". ok is false when the response has none of these.
func (c *converter) modelTurn(h stepHead) (thinkt.Entry, []toolCall, bool) {
	var r modelTurnRecord
	_ = json.Unmarshal(h.Raw, &r)
	var blocks []thinkt.ContentBlock
	if r.Thinking != "" {
		blocks = append(blocks, thinkt.ContentBlock{Type: "thinking", Thinking: r.Thinking})
	}
	text := strings.TrimSpace(h.Content)
	if text != "" {
		blocks = append(blocks, thinkt.ContentBlock{Type: "text", Text: text})
	}
	var calls []toolCall
	for n, tc := range r.ToolCalls {
		call := toolCall{ID: strconv.Itoa(h.Index) + "." + strconv.Itoa(n), Name: tc.Name, Args: tc.Args}
		calls = append(calls, call)
		blocks = append(blocks, thinkt.ContentBlock{Type: "tool_use", ToolUseID: call.ID, ToolName: tc.Name, ToolInput: tc.Args})
	}
	if len(blocks) == 0 {
		return thinkt.Entry{}, nil, false
	}
	e := c.newEntry(thinkt.RoleAssistant, h)
	e.ContentBlocks, e.Text, e.Model = blocks, text, c.model
	if r.InputTokens != nil || r.CacheReadTokens != nil || r.OutputTokens != nil {
		e.Usage = &thinkt.TokenUsage{InputTokens: deref(r.InputTokens), CacheReadInputTokens: deref(r.CacheReadTokens), OutputTokens: deref(r.OutputTokens)}
	}
	return e, calls, true
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// resultEntry builds the user entry carrying one tool result. An empty
// toolUseID means no call was waiting: the result becomes a secondary
// unknown entry carrying its text.
func (c *converter) resultEntry(h stepHead, toolUseID string) thinkt.Entry {
	body, created, completed := stripResultHeader(h.Content)
	body = withMedia(strings.TrimSpace(body), h.Media)
	if body == "" {
		body = h.Error
	}
	e := c.newEntry(thinkt.RoleUser, h)
	if created != "" {
		e.Metadata["created_at"] = created
	}
	if completed != "" {
		e.Metadata["completed_at"] = completed
	}
	if ids := subagentIDs(body); ids != nil {
		e.Metadata["child_session_ids"] = ids
	}
	if toolUseID == "" {
		if body == "" {
			body = h.Type
		}
		e.Kind, e.Visibility, e.Text = thinkt.KindUnknown, thinkt.VisibilitySecondary, body
		return e
	}
	e.ContentBlocks = []thinkt.ContentBlock{{
		Type:       "tool_result",
		ToolUseID:  toolUseID,
		ToolResult: body,
		IsError:    h.Status == "ERROR" || h.Error != "",
	}}
	return e
}
