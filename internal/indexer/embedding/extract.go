package embedding

import (
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

const MinTextLength = 10

// ExtractText extracts embeddable text from an entry.
// Returns empty string for entries that should be skipped
// (checkpoints, progress, too short).
func ExtractText(entry thinkt.Entry) string {
	// Skip non-content roles
	switch entry.Role {
	case thinkt.RoleCheckpoint, thinkt.RoleProgress, thinkt.RoleSystem:
		return ""
	}

	// If entry has content blocks, extract text from them
	if len(entry.ContentBlocks) > 0 {
		var parts []string
		for _, b := range entry.ContentBlocks {
			switch b.Type {
			case "text":
				if b.Text != "" {
					parts = append(parts, b.Text)
				}
			case "thinking":
				if b.Thinking != "" {
					parts = append(parts, b.Thinking)
				}
			case "tool_result":
				if b.ToolResult != "" {
					parts = append(parts, b.ToolResult)
				}
			// Skip tool_use (just function names/args, not meaningful text)
			// Skip media blocks
			}
		}
		text := strings.Join(parts, "\n")
		if len(text) < MinTextLength {
			return ""
		}
		return text
	}

	// Fall back to plain text
	if len(entry.Text) < MinTextLength {
		return ""
	}
	return entry.Text
}
