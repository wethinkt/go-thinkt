package kimi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// turnState carries the turn id, model, and effort the latest records
// announced, so every entry of the turn can be stamped.
type turnState struct {
	TurnID, Model, Effort string
}

// apply updates the state from agent.turn.started, turn.* records, loop
// step.begin, and llm.request.
func (t *turnState) apply(h recordHead) {
	switch h.Type {
	case "llm.request":
		var r struct {
			Model          string `json:"model"`
			ModelAlias     string `json:"modelAlias"`
			ThinkingEffort string `json:"thinkingEffort"`
		}
		if json.Unmarshal(h.Payload, &r) != nil {
			return
		}
		if r.ModelAlias != "" {
			t.Model = r.ModelAlias
		} else if r.Model != "" {
			t.Model = r.Model
		}
		if r.ThinkingEffort != "" {
			t.Effort = r.ThinkingEffort
		}
	default:
		if h.TurnID != "" && (h.Type == "agent.turn.started" || strings.HasPrefix(h.Type, "turn.") || h.Key == "loop/step.begin") {
			t.TurnID = h.TurnID
		}
	}
}

// converter turns classified non-loop lines into entries. The parser owns
// the loop-step assembly; the converter owns everything that is one line.
type converter struct {
	sessionID string
	lineNo    int
	turn      *turnState
}

func composeUUID(sessionID string, lineNo int, key string) string {
	return fmt.Sprintf("%s:%06d:%s", sessionID, lineNo, key)
}

// childSessionID names a subagent session: the root session id plus the
// agent id, so children of children still hang off the root.
func childSessionID(sessionID, agentID string) string {
	root := sessionID
	if i := strings.IndexByte(root, '/'); i >= 0 {
		root = root[:i]
	}
	return root + "/" + agentID
}

func msTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

// newEntry builds the common skeleton: UUID (given, else composed from the
// line), role, timestamp, source, turn id, model, and effort.
func (c *converter) newEntry(role thinkt.Role, h recordHead, uuid string) thinkt.Entry {
	if uuid == "" {
		uuid = composeUUID(c.sessionID, c.lineNo, h.Key)
	}
	e := thinkt.Entry{
		UUID:      uuid,
		Role:      role,
		Timestamp: msTime(h.Time),
		Source:    thinkt.SourceKimi,
		Metadata:  map[string]any{},
	}
	if c.turn != nil {
		e.PromptID = c.turn.TurnID
		e.Model = c.turn.Model
		e.Effort = c.turn.Effort
	}
	if h.TurnID != "" {
		e.PromptID = h.TurnID
	}
	return e
}

// convertUser converts an appended input record, a context user message, a
// turn.steer, or an appended notify record.
func (c *converter) convertUser(h recordHead) []thinkt.Entry {
	text := strings.TrimSpace(h.LeadText)
	media := c.userMedia(h)
	if text == "" && len(media) == 0 {
		return nil
	}
	class, kind, subtype := userClass(h)
	e := c.newEntry(thinkt.RoleUser, h, h.ID)
	e.Text = text
	e.Kind = kind
	e.Subtype = subtype
	// A subagent task prompt shows the task; the git context it opens with
	// is kept in metadata.
	if subtype == "system_trigger" && class == thinkt.ClassTurn {
		if block, rest := splitGitContext(text); block != "" {
			e.Text = rest
			e.Metadata["git_context"] = block
		}
	}
	if class == thinkt.ClassContext || kind == thinkt.KindCommandOutput {
		e.Visibility = thinkt.VisibilitySecondary
	}
	if len(media) > 0 {
		if e.Text != "" {
			e.ContentBlocks = []thinkt.ContentBlock{{Type: "text", Text: e.Text}}
		}
		e.ContentBlocks = append(e.ContentBlocks, media...)
	}
	return []thinkt.Entry{e}
}

// userMedia returns the images of a user message.
func (c *converter) userMedia(h recordHead) []thinkt.ContentBlock {
	m, ok := c.legacyMessage(h)
	if !ok {
		return nil
	}
	var media []thinkt.ContentBlock
	for _, b := range contentBlocks(m.Content) {
		if b.Media != nil {
			media = append(media, b)
		}
	}
	return media
}

// convertEvent converts the system events and unknown lines.
func (c *converter) convertEvent(h recordHead, class thinkt.RecordClass, line []byte) []thinkt.Entry {
	switch h.Type {
	case "turn.ended":
		return c.turnEnded(h)
	case "subagent.spawned", "subagent.completed", "subagent.failed", "subagent.cancelled":
		return c.subagentEvent(h)
	case "task.started", "task.terminated":
		return c.taskEvent(h)
	case "plan.revision":
		return c.planRevision(h)
	}
	if class == thinkt.ClassUnknown {
		e := c.newEntry(thinkt.RoleSystem, h, "")
		e.Kind = thinkt.KindUnknown
		e.Visibility = thinkt.VisibilitySecondary
		e.Text = string(line)
		return []thinkt.Entry{e}
	}
	return nil
}

func (c *converter) turnEnded(h recordHead) []thinkt.Entry {
	var p struct {
		Reason     string `json:"reason"`
		DurationMs int64  `json:"durationMs"`
		Error      struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(h.Payload, &p)
	e := c.newEntry(thinkt.RoleSystem, h, "")
	e.Kind = thinkt.KindTurnDuration
	e.Visibility = thinkt.VisibilitySecondary
	e.Text = "turn ended: " + p.Reason
	e.Metadata["duration_ms"] = p.DurationMs
	if p.Reason == "failed" {
		e.Error = p.Error.Message
		if e.Error == "" {
			e.Error = "failed"
		}
	}
	return []thinkt.Entry{e}
}

func (c *converter) subagentEvent(h recordHead) []thinkt.Entry {
	var p struct {
		SubagentID    string `json:"subagentId"`
		SubagentName  string `json:"subagentName"`
		Description   string `json:"description"`
		ResultSummary string `json:"resultSummary"`
		Error         string `json:"error"`
	}
	_ = json.Unmarshal(h.Payload, &p)
	verb := strings.TrimPrefix(h.Type, "subagent.")
	e := c.newEntry(thinkt.RoleSystem, h, composeUUID(c.sessionID, c.lineNo, h.Key)+":"+p.SubagentID)
	e.Kind = thinkt.KindAgentMessage
	e.Subtype = verb
	if p.SubagentID != "" {
		e.ChildSessionID = childSessionID(c.sessionID, p.SubagentID)
	}
	switch verb {
	case "spawned":
		e.Text = strings.TrimSpace("spawned " + p.SubagentName + ": " + p.Description)
		e.Text = strings.TrimSuffix(e.Text, ":")
	case "failed":
		e.Visibility = thinkt.VisibilitySecondary
		e.Text = p.Error
		e.Error = p.Error
		if e.Text == "" {
			e.Text = verb
		}
	default:
		e.Visibility = thinkt.VisibilitySecondary
		e.Text = strings.TrimSpace(p.ResultSummary)
		if e.Text == "" {
			e.Text = verb
		}
	}
	return []thinkt.Entry{e}
}

func (c *converter) taskEvent(h recordHead) []thinkt.Entry {
	var p struct {
		Info struct {
			TaskID      string `json:"taskId"`
			Description string `json:"description"`
			Status      string `json:"status"`
		} `json:"info"`
		OutputTail string `json:"outputTail"`
	}
	_ = json.Unmarshal(h.Payload, &p)
	verb := strings.TrimPrefix(h.Type, "task.")
	e := c.newEntry(thinkt.RoleSystem, h, "")
	e.Kind = thinkt.KindCommandOutput
	e.Visibility = thinkt.VisibilitySecondary
	e.Subtype = verb
	e.Text = p.Info.Description
	if e.Text == "" {
		e.Text = p.Info.TaskID
	}
	if p.Info.Status != "" {
		e.Metadata["status"] = p.Info.Status
	}
	if p.OutputTail != "" {
		e.Text = strings.TrimSpace(e.Text + "\n" + p.OutputTail)
	}
	if e.Text == "" {
		e.Text = verb
	}
	return []thinkt.Entry{e}
}

func (c *converter) planRevision(h recordHead) []thinkt.Entry {
	var p struct {
		Key     string `json:"key"`
		Version int64  `json:"version"`
	}
	_ = json.Unmarshal(h.Payload, &p)
	e := c.newEntry(thinkt.RoleSystem, h, "")
	e.Kind = thinkt.KindAttachment
	e.Visibility = thinkt.VisibilitySecondary
	e.Subtype = "plan"
	e.Text = p.Key
	e.Metadata["version"] = p.Version
	if e.Text == "" {
		e.Text = "plan"
	}
	return []thinkt.Entry{e}
}

// legacyMessage is the message object of a context.append_message record or
// of an appended envelope.
type legacyMessage struct {
	Role       string            `json:"role"`
	ID         string            `json:"id"`
	ToolCallID string            `json:"toolCallId"`
	Content    []json.RawMessage `json:"content"`
	ToolCalls  []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Args     string `json:"arguments"`
		Function struct {
			Name string `json:"name"`
			Args string `json:"arguments"`
		} `json:"function"`
	} `json:"toolCalls"`
}

func (c *converter) legacyMessage(h recordHead) (legacyMessage, bool) {
	var m legacyMessage
	if h.Type == "agent.message.appended" {
		var env struct {
			Message json.RawMessage `json:"message"`
		}
		var outer struct {
			Message json.RawMessage `json:"message"`
		}
		if json.Unmarshal(h.Payload, &outer) != nil || json.Unmarshal(outer.Message, &env) != nil {
			return m, false
		}
		return m, json.Unmarshal(env.Message, &m) == nil
	}
	var outer struct {
		Message json.RawMessage `json:"message"`
	}
	if json.Unmarshal(h.Payload, &outer) != nil {
		return m, false
	}
	return m, json.Unmarshal(outer.Message, &m) == nil
}

// contentBlocks turns a message content array into thinking and text blocks.
func contentBlocks(parts []json.RawMessage) []thinkt.ContentBlock {
	var blocks []thinkt.ContentBlock
	for _, raw := range parts {
		var p struct {
			Type     string   `json:"type"`
			Text     string   `json:"text"`
			Think    string   `json:"think"`
			ImageURL imageURL `json:"imageUrl"`
			Snake    imageURL `json:"image_url"`
		}
		if json.Unmarshal(raw, &p) != nil {
			continue
		}
		switch p.Type {
		case "think":
			if p.Think != "" {
				blocks = append(blocks, thinkt.ContentBlock{Type: "thinking", Thinking: p.Think})
			}
		case "text":
			if p.Text != "" {
				blocks = append(blocks, thinkt.ContentBlock{Type: "text", Text: p.Text})
			}
		case "image_url":
			if b, ok := imageBlock(p.ImageURL.URL + p.Snake.URL); ok {
				blocks = append(blocks, b)
			}
		}
	}
	return blocks
}

type imageURL struct {
	URL string `json:"url"`
}

// imageBlock turns an image URL into a media block. A blobref names its bytes
// by hash; the session reader finds the file.
func imageBlock(url string) (thinkt.ContentBlock, bool) {
	if rest, ok := strings.CutPrefix(url, "blobref:"); ok {
		mediaType, hash, _ := strings.Cut(rest, ";")
		b := thinkt.MediaBlockURI(mediaType, "")
		b.Media.SHA256 = hash
		return b, hash != ""
	}
	return thinkt.MediaBlockURL(url)
}

// resolveBlobs points blobref media at their files, in the agent's blobs
// folder or else the main agent's.
func resolveBlobs(blocks []thinkt.ContentBlock, agentDir string) {
	for i := range blocks {
		resolveBlobs(blocks[i].Children, agentDir)
		m := blocks[i].Media
		if m == nil || m.URI != "" || m.SHA256 == "" || filepath.Base(m.SHA256) != m.SHA256 {
			continue
		}
		for _, dir := range []string{agentDir, filepath.Join(filepath.Dir(agentDir), "main")} {
			path := filepath.Join(dir, "blobs", m.SHA256)
			if info, err := os.Stat(path); err == nil {
				m.URI, m.Size = path, info.Size()
				break
			}
		}
	}
}

func joinTextBlocks(blocks []thinkt.ContentBlock) string {
	var out []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			out = append(out, b.Text)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func parseArguments(args string) any {
	if args == "" {
		return nil
	}
	var out any
	if err := json.Unmarshal([]byte(args), &out); err == nil {
		return out
	}
	return args
}

// convertLegacyAssistant converts a protocol 1.0 assistant message or an
// appended assistant record the loop never covered.
func (c *converter) convertLegacyAssistant(h recordHead) []thinkt.Entry {
	m, ok := c.legacyMessage(h)
	if !ok {
		return nil
	}
	blocks := contentBlocks(m.Content)
	for _, tc := range m.ToolCalls {
		name, args := tc.Function.Name, tc.Function.Args
		if name == "" {
			name, args = tc.Name, tc.Args
		}
		if tc.ID == "" && name == "" {
			continue
		}
		blocks = append(blocks, thinkt.ContentBlock{Type: "tool_use", ToolUseID: tc.ID, ToolName: name, ToolInput: parseArguments(args)})
	}
	if len(blocks) == 0 {
		return nil
	}
	e := c.newEntry(thinkt.RoleAssistant, h, h.ID)
	e.ContentBlocks = blocks
	e.Text = joinTextBlocks(blocks)
	return []thinkt.Entry{e}
}

// convertLegacyTool converts a protocol 1.0 tool message or an appended tool
// record the loop never covered.
func (c *converter) convertLegacyTool(h recordHead) []thinkt.Entry {
	m, ok := c.legacyMessage(h)
	if !ok {
		return nil
	}
	parts := make([]any, 0, len(m.Content))
	for _, raw := range m.Content {
		var v any
		if json.Unmarshal(raw, &v) == nil {
			parts = append(parts, v)
		}
	}
	callID := m.ToolCallID
	if callID == "" {
		callID = h.ToolCallID
	}
	block := toolResultBlock(callID, parts, "", false)
	block.IsError = strings.HasPrefix(block.ToolResult, "<system>ERROR")
	if block.ToolResult == "" && len(block.Children) == 0 && callID == "" {
		return nil
	}
	uuid := callID + ":result"
	if callID == "" {
		uuid = ""
	}
	e := c.newEntry(thinkt.RoleUser, h, uuid)
	e.ContentBlocks = []thinkt.ContentBlock{block}
	return []thinkt.Entry{e}
}

// toolResultBlock builds a tool_result block from a loop or message output:
// a string as is; an array of parts joined by text, with data-URL images as
// image children and other URLs (blobref:) as text children; note as a
// trailing text child.
func toolResultBlock(toolCallID string, output any, note string, isError bool) thinkt.ContentBlock {
	b := thinkt.ContentBlock{Type: "tool_result", ToolUseID: toolCallID, IsError: isError}
	switch v := output.(type) {
	case string:
		b.ToolResult = v
	case []any:
		var texts []string
		for _, part := range v {
			m, ok := part.(map[string]any)
			if !ok {
				continue
			}
			switch m["type"] {
			case "text":
				if s, _ := m["text"].(string); s != "" {
					texts = append(texts, s)
				}
			case "image_url":
				url := ""
				if iu, ok := m["imageUrl"].(map[string]any); ok {
					url, _ = iu["url"].(string)
				} else if iu, ok := m["image_url"].(map[string]any); ok {
					url, _ = iu["url"].(string)
				}
				if child, ok := imageBlock(url); ok {
					b.Children = append(b.Children, child)
				}
			default:
				if raw, err := json.Marshal(m); err == nil {
					b.Children = append(b.Children, thinkt.ContentBlock{Type: "text", Text: string(raw)})
				}
			}
		}
		b.ToolResult = strings.Join(texts, "\n")
	case nil:
	default:
		if raw, err := json.Marshal(v); err == nil {
			b.ToolResult = string(raw)
		}
	}
	if note != "" {
		b.Children = append(b.Children, thinkt.ContentBlock{Type: "text", Text: note})
	}
	return b
}

// decodeUsage maps a loop usage object onto TokenUsage; nil for empty input.
func decodeUsage(raw json.RawMessage) *thinkt.TokenUsage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var u struct {
		InputOther         int `json:"inputOther"`
		Output             int `json:"output"`
		InputCacheRead     int `json:"inputCacheRead"`
		InputCacheCreation int `json:"inputCacheCreation"`
	}
	if json.Unmarshal(raw, &u) != nil {
		return nil
	}
	return &thinkt.TokenUsage{
		InputTokens:              u.InputOther,
		OutputTokens:             u.Output,
		CacheReadInputTokens:     u.InputCacheRead,
		CacheCreationInputTokens: u.InputCacheCreation,
	}
}
