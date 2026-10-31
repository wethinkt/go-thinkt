package codex

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// wireVisibility returns the kind and visibility of a wire tool entry: the
// orchestration tools have a structured harness item that carries the same
// call, so their wire pair is a secondary KindToolWire.
func wireVisibility(name string) (thinkt.EntryKind, thinkt.Visibility) {
	if orchestrationTools[name] {
		return thinkt.KindToolWire, thinkt.VisibilitySecondary
	}
	return thinkt.KindNone, ""
}

// convertWireCall turns a response_item function_call or custom_tool_call
// into a single assistant tool_use entry with a wire-namespaced id.
func (c *converter) convertWireCall(h recordHead) []thinkt.Entry {
	var payload map[string]any
	if json.Unmarshal(h.Payload, &payload) != nil {
		return nil
	}
	callID := readString(payload, "call_id")
	name := readString(payload, "name")
	if callID == "" && name == "" {
		return nil
	}
	if callID != "" {
		c.calls[callID] = name
	}
	id := c.wireID(h, callID)
	e := c.newEntry(thinkt.RoleAssistant, h, id)
	e.Kind, e.Visibility = wireVisibility(name)
	e.ContentBlocks = []thinkt.ContentBlock{{
		Type:      "tool_use",
		ToolUseID: id,
		ToolName:  name,
		ToolInput: parseToolInput(payload),
	}}
	return []thinkt.Entry{e}
}

// convertWireOutput turns a function_call_output or custom_tool_call_output
// into a single user tool_result entry paired to its wire call.
func (c *converter) convertWireOutput(h recordHead) []thinkt.Entry {
	var payload map[string]any
	if json.Unmarshal(h.Payload, &payload) != nil {
		return nil
	}
	callID := readString(payload, "call_id")
	output, media := splitToolOutput(payload["output"])
	if callID == "" && output == "" && len(media) == 0 {
		return nil
	}
	id := c.wireID(h, callID)
	e := c.newEntry(thinkt.RoleUser, h, id+":result")
	e.Kind, e.Visibility = wireVisibility(c.calls[callID])
	e.ContentBlocks = []thinkt.ContentBlock{{
		Type:       "tool_result",
		ToolUseID:  id,
		ToolResult: output,
		Children:   media,
	}}
	if code, ok := parseExitCode(output); ok {
		e.Metadata["exit_code"] = code
	}
	return []thinkt.Entry{e}
}

// splitToolOutput returns a wire output's text and its images. An array
// output holding images is split; anything else is normalizeToolOutput's.
func splitToolOutput(v any) (string, []thinkt.ContentBlock) {
	list, ok := v.([]any)
	if !ok {
		return normalizeToolOutput(v), nil
	}
	var texts []string
	var media []thinkt.ContentBlock
	for _, raw := range list {
		part, _ := raw.(map[string]any)
		switch part["type"] {
		case "input_image":
			if u, _ := part["image_url"].(string); u != "" {
				if b, ok := thinkt.MediaBlockURL(u); ok {
					media = append(media, b)
				}
			}
		case "input_text", "text":
			if s, _ := part["text"].(string); s != "" {
				texts = append(texts, s)
			}
		default:
			if data, err := json.Marshal(raw); err == nil {
				media = append(media, thinkt.ContentBlock{Type: "text", Text: string(data)})
			}
		}
	}
	if !slices.ContainsFunc(media, func(b thinkt.ContentBlock) bool { return b.Media != nil }) {
		return normalizeToolOutput(v), nil
	}
	return strings.Join(texts, "\n"), media
}

// wireID is the UUID stem and ToolUseID of a wire tool pair: "wire:<call_id>",
// or a line-based id when the record carries no call_id so two such lines
// never share a UUID.
func (c *converter) wireID(h recordHead, callID string) string {
	if callID == "" {
		return "wire:" + composeUUID(c.sessionID, c.lineNo, h.Key(), "")
	}
	return "wire:" + callID
}

// toolPair builds the assistant tool_use entry and the user tool_result
// entry for one harness item. The halves share ToolUseID = uuid; the result
// takes uuid+":result" so the index never upserts one over the other.
func (c *converter) toolPair(h recordHead, uuid string, call thinkt.ContentBlock, result thinkt.ContentBlock, visibility thinkt.Visibility, kind thinkt.EntryKind) []thinkt.Entry {
	if uuid == "" {
		uuid = composeUUID(c.sessionID, c.lineNo, h.Key(), "")
	}
	call.Type = "tool_use"
	call.ToolUseID = uuid
	result.Type = "tool_result"
	result.ToolUseID = uuid

	use := c.newEntry(thinkt.RoleAssistant, h, uuid)
	use.Kind, use.Visibility = kind, visibility
	use.ContentBlocks = []thinkt.ContentBlock{call}

	res := c.newEntry(thinkt.RoleUser, h, uuid+":result")
	res.Kind, res.Visibility = kind, visibility
	res.ContentBlocks = []thinkt.ContentBlock{result}
	return []thinkt.Entry{use, res}
}

// itemDuration is the {secs, nanos} object harness items carry.
type itemDuration struct {
	Secs  int64 `json:"secs"`
	Nanos int64 `json:"nanos"`
}

func durationMs(d itemDuration) int64 {
	return d.Secs*1000 + d.Nanos/1e6
}

func stripFileURI(s string) string {
	return strings.TrimPrefix(s, "file://")
}

var exitCodeRE = regexp.MustCompile(`Process exited with code (\d+)`)

// parseExitCode reads the exit code a wire exec_command output reports.
func parseExitCode(output string) (int, bool) {
	m := exitCodeRE.FindStringSubmatch(output)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// wrapDiff gives a hunk-only unified diff the file header the diff renderer
// expects.
func wrapDiff(path, body string) string {
	return "--- a/" + path + "\n+++ b/" + path + "\n" + body
}

// diffLineCounts counts added and removed lines. Only the two-line file
// header is skipped, so body lines that happen to start with "--" or "++"
// (SQL comments, YAML documents) still count.
func diffLineCounts(diff string) (added, removed int) {
	for i, line := range strings.Split(diff, "\n") {
		switch {
		case i < 2 && (strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---")):
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}
	return added, removed
}

// convertToolItem turns a harness tool item into a tool_use/tool_result pair.
func (c *converter) convertToolItem(h recordHead) []thinkt.Entry {
	switch h.Key() {
	case "item/CommandExecution":
		return c.commandExecution(h)
	case "item/FileChange":
		return c.fileChange(h)
	case "item/McpToolCall":
		return c.mcpToolCall(h)
	case "item/WebSearch":
		return c.webSearch(h)
	case "item/ImageView":
		return c.imageView(h)
	case "item/Extension":
		switch h.ItemKind {
		case "web.search":
			return c.webSearch(h)
		case "clock.sleep":
			return c.clockSleep(h)
		}
	}
	return nil
}

func (c *converter) commandExecution(h recordHead) []thinkt.Entry {
	var item struct {
		ID               string       `json:"id"`
		Command          []string     `json:"command"`
		CWD              string       `json:"cwd"`
		Source           string       `json:"source"`
		Status           string       `json:"status"`
		Stdout           string       `json:"stdout"`
		Stderr           string       `json:"stderr"`
		AggregatedOutput string       `json:"aggregated_output"`
		ExitCode         *int         `json:"exit_code"`
		Duration         itemDuration `json:"duration"`
	}
	if !decodeItem(h, &item) {
		return nil
	}
	call := thinkt.ContentBlock{
		ToolName:  item.Source,
		ToolInput: map[string]any{"command": strings.Join(item.Command, " "), "cwd": stripFileURI(item.CWD)},
	}
	output := item.AggregatedOutput
	if output == "" {
		output = item.Stdout
	}
	result := thinkt.ContentBlock{ToolResult: output}
	if item.Stderr != "" && item.Stderr != item.AggregatedOutput {
		result.Children = []thinkt.ContentBlock{{Type: "text", Text: item.Stderr}}
	}
	exitCode := 0
	if item.ExitCode != nil {
		exitCode = *item.ExitCode
	}
	result.IsError = exitCode != 0 || item.Status == "failed"
	entries := c.toolPair(h, item.ID, call, result, "", thinkt.KindNone)
	res := &entries[1]
	res.Metadata["exit_code"] = exitCode
	res.Metadata["duration_ms"] = durationMs(item.Duration)
	if item.Status != "" {
		res.Metadata["status"] = item.Status
	}
	return entries
}

// fileChangeEntry is one change of a FileChange item. Updates carry a
// unified_diff; adds and deletes carry the whole file in content instead.
type fileChangeEntry struct {
	Type        string `json:"type"`
	UnifiedDiff string `json:"unified_diff"`
	Content     string `json:"content"`
	MovePath    string `json:"move_path"`
}

// changeDiff returns the unified diff for one change with a file header:
// the item's own diff when present, else a diff synthesized from content
// (all lines added for an add, all removed for a delete), else "".
func changeDiff(path string, ch fileChangeEntry) string {
	if ch.UnifiedDiff != "" {
		return wrapDiff(path, ch.UnifiedDiff)
	}
	if ch.Content == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(ch.Content, "\n"), "\n")
	var b strings.Builder
	switch ch.Type {
	case "add":
		fmt.Fprintf(&b, "--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", path, len(lines))
		for _, l := range lines {
			b.WriteString("+" + l + "\n")
		}
	case "delete":
		fmt.Fprintf(&b, "--- a/%s\n+++ /dev/null\n@@ -1,%d +0,0 @@\n", path, len(lines))
		for _, l := range lines {
			b.WriteString("-" + l + "\n")
		}
	default:
		return ""
	}
	return b.String()
}

func (c *converter) fileChange(h recordHead) []thinkt.Entry {
	var item struct {
		ID      string                     `json:"id"`
		Status  string                     `json:"status"`
		Changes map[string]fileChangeEntry `json:"changes"`
	}
	if !decodeItem(h, &item) {
		return nil
	}
	paths := make([]string, 0, len(item.Changes))
	for p := range item.Changes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	input := make(map[string]any, len(paths))
	lines := make([]string, 0, len(paths))
	var children []thinkt.ContentBlock
	for _, p := range paths {
		ch := item.Changes[p]
		kind := ch.Type
		if ch.MovePath != "" {
			input[p] = kind + " -> " + ch.MovePath
		} else {
			input[p] = kind
		}
		lines = append(lines, strings.TrimSpace(kind+" "+p))
		if diff := changeDiff(p, ch); diff != "" {
			children = append(children, thinkt.ContentBlock{Type: thinkt.BlockTypeDiff, Text: diff})
		}
	}
	call := thinkt.ContentBlock{ToolName: "apply_patch", ToolInput: input}
	result := thinkt.ContentBlock{
		ToolResult: strings.Join(lines, "\n"),
		Children:   children,
		IsError:    item.Status == "failed",
	}
	entries := c.toolPair(h, item.ID, call, result, "", thinkt.KindNone)
	if item.Status != "" {
		entries[1].Metadata["status"] = item.Status
	}
	return entries
}

func (c *converter) mcpToolCall(h recordHead) []thinkt.Entry {
	var item struct {
		ID        string       `json:"id"`
		Server    string       `json:"server"`
		Tool      string       `json:"tool"`
		Arguments any          `json:"arguments"`
		Status    string       `json:"status"`
		Duration  itemDuration `json:"duration"`
		Result    *struct {
			Content []json.RawMessage `json:"content"`
			IsError bool              `json:"isError"`
		} `json:"result"`
	}
	if !decodeItem(h, &item) {
		return nil
	}
	call := thinkt.ContentBlock{ToolName: "mcp:" + item.Server + "/" + item.Tool, ToolInput: item.Arguments}
	result := thinkt.ContentBlock{IsError: item.Status == "failed"}
	if item.Result != nil {
		result.IsError = result.IsError || item.Result.IsError
		var texts []string
		for _, raw := range item.Result.Content {
			var part struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Data     string `json:"data"`
				MimeType string `json:"mimeType"`
			}
			if json.Unmarshal(raw, &part) == nil && part.Type == "text" {
				texts = append(texts, part.Text)
				continue
			}
			if part.Type == "image" && part.Data != "" {
				result.Children = append(result.Children, thinkt.MediaBlock(part.MimeType, part.Data))
				continue
			}
			childType := part.Type
			if childType == "" {
				childType = "text" // untyped or undecodable content: keep its raw JSON readable
			}
			result.Children = append(result.Children, thinkt.ContentBlock{Type: childType, Text: string(raw)})
		}
		result.ToolResult = strings.Join(texts, "\n")
	}
	entries := c.toolPair(h, item.ID, call, result, "", thinkt.KindNone)
	entries[1].Metadata["duration_ms"] = durationMs(item.Duration)
	if item.Status != "" {
		entries[1].Metadata["status"] = item.Status
	}
	return entries
}

func (c *converter) webSearch(h recordHead) []thinkt.Entry {
	var item struct {
		ID     string `json:"id"`
		Query  string `json:"query"`
		Action struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"action"`
		Results []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"results"`
	}
	if !decodeItem(h, &item) {
		return nil
	}
	input := map[string]any{"query": item.Query}
	if item.Action.URL != "" {
		input["url"] = item.Action.URL
	}
	lines := make([]string, 0, len(item.Results))
	for _, r := range item.Results {
		lines = append(lines, strings.TrimSpace(r.Title+" "+r.URL))
	}
	output := strings.Join(lines, "\n")
	if output == "" {
		output = item.Action.URL
	}
	if output == "" {
		output = item.Query
	}
	call := thinkt.ContentBlock{ToolName: "web_search", ToolInput: input}
	return c.toolPair(h, item.ID, call, thinkt.ContentBlock{ToolResult: output}, "", thinkt.KindNone)
}

func (c *converter) clockSleep(h recordHead) []thinkt.Entry {
	var item struct {
		ID         string `json:"id"`
		DurationMs int64  `json:"durationMs"`
	}
	if !decodeItem(h, &item) {
		return nil
	}
	call := thinkt.ContentBlock{ToolName: "sleep", ToolInput: map[string]any{"durationMs": item.DurationMs}}
	result := thinkt.ContentBlock{ToolResult: fmt.Sprintf("slept %d ms", item.DurationMs)}
	return c.toolPair(h, item.ID, call, result, "", thinkt.KindNone)
}

func (c *converter) imageView(h recordHead) []thinkt.Entry {
	var item struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	}
	if !decodeItem(h, &item) {
		return nil
	}
	path := stripFileURI(item.Path)
	call := thinkt.ContentBlock{ToolName: "view_image", ToolInput: map[string]any{"path": path}}
	image := thinkt.MediaBlockURI("", path)
	image.Type = "image"
	result := thinkt.ContentBlock{ToolResult: path, Children: []thinkt.ContentBlock{image}}
	return c.toolPair(h, item.ID, call, result, "", thinkt.KindNone)
}
