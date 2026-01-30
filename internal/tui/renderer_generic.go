package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tuilog"
)

// RenderThinktSession converts a thinkt session's entries into a styled string for the viewport.
func RenderThinktSession(session *thinkt.Session, width int) string {
	tuilog.Log.Info("RenderThinktSession: starting", "entryCount", len(session.Entries), "width", width)
	if session == nil || len(session.Entries) == 0 {
		tuilog.Log.Info("RenderThinktSession: no content")
		return "No content"
	}

	contentWidth := max(20, width-4)

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(contentWidth),
	)

	var b strings.Builder
	for i, entry := range session.Entries {
		tuilog.Log.Debug("RenderThinktSession: rendering entry", "index", i, "role", entry.Role)
		s := renderThinktEntry(&entry, contentWidth, renderer, err == nil)
		if s != "" {
			b.WriteString(s)
			b.WriteString("\n")
		}
	}
	result := b.String()
	tuilog.Log.Info("RenderThinktSession: complete", "outputLength", len(result))
	return result
}

func renderThinktEntry(entry *thinkt.Entry, width int, renderer *glamour.TermRenderer, useGlamour bool) string {
	switch entry.Role {
	case thinkt.RoleUser:
		return renderThinktUserEntry(entry, width)
	case thinkt.RoleAssistant:
		return renderThinktAssistantEntry(entry, width, renderer, useGlamour)
	default:
		return ""
	}
}

func renderThinktUserEntry(entry *thinkt.Entry, width int) string {
	text := entry.Text
	if text == "" {
		// Try to extract from content blocks
		for _, cb := range entry.ContentBlocks {
			if cb.Type == "text" && cb.Text != "" {
				text = cb.Text
				break
			}
		}
	}
	if text == "" {
		return ""
	}

	label := userLabel.Render("User")
	content := userBlockStyle.Width(width).Render(text)
	return label + "\n" + content
}

func renderThinktAssistantEntry(entry *thinkt.Entry, width int, renderer *glamour.TermRenderer, useGlamour bool) string {
	// Process content blocks
	var parts []string

	// First try content blocks
	if len(entry.ContentBlocks) > 0 {
		for _, block := range entry.ContentBlocks {
			s := renderThinktContentBlock(&block, width, renderer, useGlamour)
			if s != "" {
				parts = append(parts, s)
			}
		}
	}

	// Fall back to text field
	if len(parts) == 0 && entry.Text != "" {
		label := assistantLabel.Render("Assistant")
		text := entry.Text
		if useGlamour && renderer != nil {
			if rendered, err := renderer.Render(text); err == nil {
				text = strings.TrimSpace(rendered)
			}
		}
		content := assistantBlockStyle.Width(width).Render(text)
		parts = append(parts, label+"\n"+content)
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

func renderThinktContentBlock(block *thinkt.ContentBlock, width int, renderer *glamour.TermRenderer, useGlamour bool) string {
	switch block.Type {
	case "text":
		if block.Text == "" {
			return ""
		}
		label := assistantLabel.Render("Assistant")
		text := block.Text
		if useGlamour && renderer != nil {
			if rendered, err := renderer.Render(text); err == nil {
				text = strings.TrimSpace(rendered)
			}
		}
		content := assistantBlockStyle.Width(width).Render(text)
		return label + "\n" + content

	case "thinking":
		if block.Thinking == "" {
			return ""
		}
		label := thinkingLabel.Render("Thinking")
		// Truncate long thinking blocks
		text := block.Thinking
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		content := thinkingBlockStyle.Width(width).Render(text)
		return label + "\n" + content

	case "tool_use":
		label := toolLabel.Render(fmt.Sprintf("Tool: %s", block.ToolName))
		summary := fmt.Sprintf("id: %s", block.ToolUseID)
		content := toolCallBlockStyle.Width(width).Render(summary)
		return label + "\n" + content

	case "tool_result":
		label := toolLabel.Render("Tool Result")
		text := "(result)"
		if block.IsError {
			text = "(error)"
		}
		content := toolResultBlockStyle.Width(width).Render(text)
		return label + "\n" + content

	default:
		return ""
	}
}
