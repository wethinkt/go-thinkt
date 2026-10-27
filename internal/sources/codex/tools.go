package codex

import (
	"encoding/json"

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
	id := "wire:" + callID
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
	output := normalizeToolOutput(payload["output"])
	if callID == "" && output == "" {
		return nil
	}
	id := "wire:" + callID
	e := c.newEntry(thinkt.RoleUser, h, id+":result")
	e.Kind, e.Visibility = wireVisibility(c.calls[callID])
	e.ContentBlocks = []thinkt.ContentBlock{{
		Type:       "tool_result",
		ToolUseID:  id,
		ToolResult: output,
	}}
	return []thinkt.Entry{e}
}

// convertToolItem turns a harness tool item into a tool_use/tool_result
// pair. Item kinds are added in Task 4.
func (c *converter) convertToolItem(h recordHead) []thinkt.Entry {
	return nil
}
