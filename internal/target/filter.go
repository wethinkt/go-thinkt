// Package target provides shared session resolution and content filtering
// for exports and shares.
package target

import (
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ContentFilter controls which content is included in exports and shares.
type ContentFilter struct {
	IncludeThinking    bool
	IncludeToolUse     bool
	IncludeToolResults bool
	IncludeMedia       bool
	IncludeSystem      bool
}

// DefaultFilter returns a filter with everything enabled except system entries.
func DefaultFilter() ContentFilter {
	return ContentFilter{
		IncludeThinking:    true,
		IncludeToolUse:     true,
		IncludeToolResults: true,
		IncludeMedia:       true,
		IncludeSystem:      false,
	}
}

// Redactor is a placeholder interface for future content redaction.
type Redactor interface {
	Redact(entries []thinkt.Entry) []thinkt.Entry
}

// FilterEntries applies the content filter to entries, returning a new slice.
// Returns nil for nil or empty input.
func FilterEntries(entries []thinkt.Entry, filter ContentFilter) []thinkt.Entry {
	if len(entries) == 0 {
		return nil
	}
	var out []thinkt.Entry
	for _, entry := range entries {
		if !shouldIncludeEntry(entry.Role, filter) {
			continue
		}
		if len(entry.ContentBlocks) > 0 {
			var blocks []thinkt.ContentBlock
			for _, block := range entry.ContentBlocks {
				if shouldIncludeBlock(block, filter) {
					blocks = append(blocks, block)
				}
			}
			filtered := entry
			filtered.ContentBlocks = blocks
			out = append(out, filtered)
		} else {
			out = append(out, entry)
		}
	}
	return out
}

func shouldIncludeEntry(role thinkt.Role, filter ContentFilter) bool {
	switch role {
	case thinkt.RoleUser:
		return true
	case thinkt.RoleAssistant:
		return true
	case thinkt.RoleTool:
		return filter.IncludeToolResults
	case thinkt.RoleSystem:
		return filter.IncludeSystem
	default:
		// Summary, Progress, Checkpoint, and any unknown roles are excluded.
		return false
	}
}

func shouldIncludeBlock(block thinkt.ContentBlock, filter ContentFilter) bool {
	switch block.Type {
	case "thinking":
		return filter.IncludeThinking
	case "tool_use":
		return filter.IncludeToolUse
	case "tool_result":
		return filter.IncludeToolResults
	case "image", "document":
		return filter.IncludeMedia
	default:
		return true
	}
}
