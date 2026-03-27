package summarize

import (
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

const minThinkingLength = 50

// ExtractThinkingText returns the concatenated thinking text from an entry.
func ExtractThinkingText(entry thinkt.Entry) string {
	if entry.Role != thinkt.RoleAssistant {
		return ""
	}

	var parts []string
	for _, b := range entry.ContentBlocks {
		if b.Type == "thinking" && len(b.Thinking) >= minThinkingLength {
			parts = append(parts, b.Thinking)
		}
	}
	return strings.Join(parts, "\n\n")
}

// ExtractSessionContext builds a condensed session context string.
func ExtractSessionContext(entries []thinkt.Entry) string {
	var parts []string
	tokenBudget := 6000
	used := 0

	for _, e := range entries {
		if used >= tokenBudget {
			break
		}
		switch e.Role {
		case thinkt.RoleUser:
			text := entryText(e)
			if text != "" {
				snippet := truncate(text, 500)
				parts = append(parts, "[user] "+snippet)
				used += len(snippet)
			}
		case thinkt.RoleAssistant:
			thinking := ExtractThinkingText(e)
			if thinking != "" {
				snippet := truncate(thinking, 800)
				parts = append(parts, "[thinking] "+snippet)
				used += len(snippet)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func entryText(e thinkt.Entry) string {
	if e.Text != "" {
		return e.Text
	}
	for _, b := range e.ContentBlocks {
		if b.Type == "text" && b.Text != "" {
			return b.Text
		}
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
