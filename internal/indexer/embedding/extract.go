package embedding

import (
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// rolePrefix returns a bracketed role prefix like "[user] ".
func rolePrefix(role thinkt.Role) string {
	return "[" + string(role) + "] "
}

const MinTextLength = 10

// Tier identifies the embedding tier for a chunk.
type Tier string

const (
	TierConversation Tier = "conversation"
	TierReasoning    Tier = "reasoning"
)

// TierText holds extracted text for a single tier.
type TierText struct {
	Tier Tier
	Text string
}

// ExtractTiered extracts tier-tagged text from an entry.
// Conversation tier contains user/assistant text blocks;
// reasoning tier contains thinking and tool_result blocks.
// Each tier's text is prefixed with the block's role for semantic clarity.
func ExtractTiered(entry thinkt.Entry) []TierText {
	switch entry.Role {
	case thinkt.RoleCheckpoint, thinkt.RoleProgress, thinkt.RoleSystem:
		return nil
	}

	if len(entry.ContentBlocks) > 0 {
		var convParts, reasonParts []string
		for _, b := range entry.ContentBlocks {
			switch b.Type {
			case "text":
				if b.Text != "" {
					convParts = append(convParts, rolePrefix(entry.Role)+b.Text)
				}
			case "thinking":
				if b.Thinking != "" {
					reasonParts = append(reasonParts, "[thinking] "+b.Thinking)
				}
			case "tool_result":
				if b.ToolResult != "" {
					reasonParts = append(reasonParts, "[tool_result] "+b.ToolResult)
				}
			}
		}

		var results []TierText
		if convText := strings.Join(convParts, "\n"); len(convText) >= MinTextLength {
			results = append(results, TierText{Tier: TierConversation, Text: convText})
		}
		if reasonText := strings.Join(reasonParts, "\n"); len(reasonText) >= MinTextLength {
			results = append(results, TierText{Tier: TierReasoning, Text: reasonText})
		}
		return results
	}

	// Plain text entry
	text := rolePrefix(entry.Role) + entry.Text
	if len(text) < MinTextLength {
		return nil
	}
	return []TierText{{Tier: TierConversation, Text: text}}
}
