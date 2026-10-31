package gemini

import (
	"encoding/json"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// part is one element of a Gemini part list (PartListUnion).
type part struct {
	Text             string          `json:"text"`
	Thought          bool            `json:"thought"`
	FunctionCall     *functionCall   `json:"functionCall"`
	FunctionResponse *functionResult `json:"functionResponse"`
}

type functionCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type functionResult struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

// parts decodes content: a part list, one part, or a plain string.
func parts(raw json.RawMessage) []part {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []part{{Text: s}}
	}
	var list []part
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var one part
	if json.Unmarshal(raw, &one) == nil {
		return []part{one}
	}
	return nil
}

// joinText joins the non-thought text parts.
func joinText(ps []part) string {
	var texts []string
	for _, p := range ps {
		if p.Text != "" && !p.Thought && p.FunctionCall == nil && p.FunctionResponse == nil {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// isContinuation reports whether a user text is one the CLI injects to keep
// a turn going rather than one the person typed.
func isContinuation(text string) bool {
	t := strings.TrimSpace(text)
	return t == "Please continue." ||
		strings.HasPrefix(t, "System: Potential loop detected") ||
		strings.HasPrefix(t, "[System:")
}

// isIgnoredUserContent mirrors the CLI's rule for user text that cannot be
// a session's first resumable prompt.
func isIgnoredUserContent(text string) bool {
	t := strings.TrimSpace(text)
	return t == "" || strings.HasPrefix(t, "/") || strings.HasPrefix(t, "?") ||
		strings.HasPrefix(t, "<session_context>") || strings.HasPrefix(t, "<hook_context>")
}

// resultText returns a tool result body: response.output, or response.error
// for a failed call. Non-string values are JSON-encoded.
func resultText(resp map[string]any, failed bool) string {
	key := "output"
	if failed {
		key = "error"
	}
	v, ok := resp[key]
	if !ok {
		if v, ok = resp["output"]; !ok {
			v = resp["error"]
		}
	}
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// statusLineTools are tools the CLI calls to update its own status line;
// their calls are secondary.
var statusLineTools = map[string]bool{"update_topic": true}

// converter turns a replayed session into entries.
type converter struct {
	sessionID string
	promptID  string
	emitted   map[string]bool // tool call ids already emitted inline
}

func (c *converter) entry(m *lineMessage, role thinkt.Role) thinkt.Entry {
	return thinkt.Entry{
		UUID:      m.ID,
		Role:      role,
		Timestamp: m.Timestamp,
		Source:    thinkt.SourceGemini,
		PromptID:  c.promptID,
		Metadata:  map[string]any{},
	}
}

// convert turns a replayed session into entries. sessionID is the thinkt
// session ID, used to name subagent sessions.
func convert(r *replayed, sessionID string) []thinkt.Entry {
	if r == nil {
		return nil
	}
	c := &converter{sessionID: sessionID, emitted: map[string]bool{}}
	var out []thinkt.Entry
	for i := range r.Messages {
		out = append(out, c.message(&r.Messages[i])...)
	}
	return out
}

func (c *converter) message(m *lineMessage) []thinkt.Entry {
	switch m.Type {
	case "user":
		return c.user(m)
	case "gemini":
		return c.model(m)
	case "info", "warning", "error":
		e := c.entry(m, thinkt.RoleSystem)
		e.Text = joinText(parts(m.Content))
		e.Subtype = m.Type
		e.Visibility = thinkt.VisibilitySecondary
		if m.Type == "error" {
			e.Error = e.Text
		}
		return []thinkt.Entry{e}
	}
	e := c.entry(m, thinkt.RoleSystem)
	e.Kind, e.Visibility, e.Subtype = thinkt.KindUnknown, thinkt.VisibilitySecondary, m.Type
	e.Text = joinText(parts(m.Content))
	return []thinkt.Entry{e}
}

func (c *converter) user(m *lineMessage) []thinkt.Entry {
	ps := parts(m.Content)
	var responses []*functionResult
	for _, p := range ps {
		if p.FunctionResponse != nil {
			responses = append(responses, p.FunctionResponse)
		}
	}
	if len(responses) > 0 && len(responses) == len(ps) {
		// A later copy of tool results: dropped when every one was
		// already emitted inline, else each new one is a result entry.
		var out []thinkt.Entry
		for _, fr := range responses {
			if c.emitted[fr.ID] {
				continue
			}
			c.emitted[fr.ID] = true
			e := c.entry(m, thinkt.RoleUser)
			e.UUID = fr.ID + ":result"
			_, failed := fr.Response["error"]
			e.ContentBlocks = []thinkt.ContentBlock{{Type: "tool_result", ToolUseID: fr.ID, ToolResult: resultText(fr.Response, failed), IsError: failed}}
			out = append(out, e)
		}
		return out
	}

	text := joinText(ps)
	if isContinuation(text) {
		e := c.entry(m, thinkt.RoleUser)
		e.Text = text
		e.Kind, e.Visibility = thinkt.KindMeta, thinkt.VisibilitySecondary
		return []thinkt.Entry{e}
	}
	c.promptID = m.ID
	e := c.entry(m, thinkt.RoleUser)
	e.Text = text
	return []thinkt.Entry{e}
}

func (c *converter) model(m *lineMessage) []thinkt.Entry {
	a := c.entry(m, thinkt.RoleAssistant)
	a.TurnID = m.ID
	a.Model = m.Model
	a.Usage = usageFromTokens(m.Tokens)

	for _, t := range m.Thoughts {
		text := t.Description
		if t.Subject != "" {
			text = t.Subject + ": " + t.Description
		}
		a.ContentBlocks = append(a.ContentBlocks, thinkt.ContentBlock{Type: "thinking", Thinking: text})
	}
	ps := parts(m.Content)
	for _, p := range ps {
		if p.Thought && p.Text != "" {
			a.ContentBlocks = append(a.ContentBlocks, thinkt.ContentBlock{Type: "thinking", Thinking: p.Text})
		}
	}
	if text := joinText(ps); strings.TrimSpace(text) != "" {
		a.Text = text
		a.ContentBlocks = append(a.ContentBlocks, thinkt.ContentBlock{Type: "text", Text: text})
	}

	recorded := map[string]bool{}
	for _, tc := range m.ToolCalls {
		recorded[tc.ID] = true
	}
	onlyStatus := len(m.ToolCalls) > 0
	for _, tc := range m.ToolCalls {
		b := thinkt.ContentBlock{Type: "tool_use", ToolUseID: tc.ID, ToolName: tc.Name, ToolInput: tc.Args}
		if tc.Name == "invoke_agent" && tc.AgentID != "" {
			b.ChildSessionID = c.sessionID + "/" + tc.AgentID
		}
		if !statusLineTools[tc.Name] {
			onlyStatus = false
		}
		a.ContentBlocks = append(a.ContentBlocks, b)
	}
	for _, p := range ps {
		if p.FunctionCall != nil && !recorded[p.FunctionCall.ID] {
			a.ContentBlocks = append(a.ContentBlocks, thinkt.ContentBlock{Type: "tool_use", ToolUseID: p.FunctionCall.ID, ToolName: p.FunctionCall.Name, ToolInput: p.FunctionCall.Args})
			onlyStatus = false
		}
	}
	if onlyStatus && a.Text == "" && len(m.Thoughts) == 0 {
		a.Visibility = thinkt.VisibilitySecondary
	}

	out := []thinkt.Entry{a}
	for _, tc := range m.ToolCalls {
		failed := tc.Status == "error"
		// The CLI's masking sync can store a whole user turn as each
		// call's result; take the response with this call's id.
		var body string
		var first *functionResult
		for _, p := range parts(tc.Result) {
			if p.FunctionResponse == nil {
				continue
			}
			if first == nil {
				first = p.FunctionResponse
			}
			if p.FunctionResponse.ID == tc.ID {
				first = p.FunctionResponse
				break
			}
		}
		if first != nil {
			body = resultText(first.Response, failed)
		}
		e := c.entry(m, thinkt.RoleUser)
		e.UUID = tc.ID + ":result"
		e.TurnID = m.ID
		if !tc.Timestamp.IsZero() {
			e.Timestamp = tc.Timestamp
		}
		e.ContentBlocks = []thinkt.ContentBlock{{Type: "tool_result", ToolUseID: tc.ID, ToolResult: body, IsError: failed}}
		if statusLineTools[tc.Name] {
			e.Visibility = thinkt.VisibilitySecondary
		}
		c.emitted[tc.ID] = true
		out = append(out, e)
	}
	return out
}
