package tui

import "github.com/wethinkt/go-thinkt/internal/thinkt"

// RoleFilterSet controls which entry types are visible in the conversation viewer.
type RoleFilterSet struct {
	Input    bool // User entries
	Output   bool // Assistant text content blocks
	Tools    bool // tool_use + tool_result content blocks
	Thinking bool // thinking content blocks
	Other    bool // system, summary, progress, checkpoint, tool-role entries
}

// NewRoleFilterSet returns a RoleFilterSet with all toggles enabled.
func NewRoleFilterSet() RoleFilterSet {
	return RoleFilterSet{
		Input:    true,
		Output:   true,
		Tools:    true,
		Thinking: true,
		Other:    true,
	}
}

// EntryVisible returns whether an entry should be rendered at all based on its role.
// For assistant entries, individual blocks are filtered separately via BlockVisible.
func (f *RoleFilterSet) EntryVisible(entry *thinkt.Entry) bool {
	switch entry.Role {
	case thinkt.RoleUser:
		return f.Input
	case thinkt.RoleAssistant:
		// Assistant entries may contain a mix of block types.
		// Return true here; block-level filtering happens in BlockVisible.
		return f.Output || f.Tools || f.Thinking
	case thinkt.RoleTool:
		return f.Other
	case thinkt.RoleSystem, thinkt.RoleSummary, thinkt.RoleProgress, thinkt.RoleCheckpoint:
		return f.Other
	default:
		return f.Other
	}
}

// BlockVisible returns whether a content block type should be rendered.
func (f *RoleFilterSet) BlockVisible(blockType string) bool {
	switch blockType {
	case "text":
		return f.Output
	case "thinking":
		return f.Thinking
	case "tool_use", "tool_result":
		return f.Tools
	default:
		return f.Other
	}
}
