package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"

	"github.com/wethinkt/go-thinkt/internal/sources/claude"
)

// RenderSession converts a session's entries into a styled string for the viewport.
func RenderSession(session *claude.Session, width int) string {
	if session == nil || len(session.Entries) == 0 {
		return "No content"
	}

	contentWidth := max(20, width-4)

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(contentWidth),
	)

	var b strings.Builder
	for _, entry := range session.Entries {
		s := renderEntry(&entry, contentWidth, renderer, err == nil)
		if s != "" {
			b.WriteString(s)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderEntry(entry *claude.Entry, width int, renderer *glamour.TermRenderer, useGlamour bool) string {
	switch entry.Type {
	case claude.EntryTypeUser:
		return renderUserEntry(entry, width)
	case claude.EntryTypeAssistant:
		return renderAssistantEntry(entry, width, renderer, useGlamour)
	default:
		return ""
	}
}

func renderUserEntry(entry *claude.Entry, width int) string {
	text := entry.GetPromptText()
	if text == "" {
		return ""
	}

	label := userLabel.Render("User")
	content := userBlockStyle.Width(width).Render(text)
	return label + "\n" + content
}

func renderAssistantEntry(entry *claude.Entry, width int, renderer *glamour.TermRenderer, useGlamour bool) string {
	msg := entry.GetAssistantMessage()
	if msg == nil {
		return ""
	}

	var parts []string
	for _, block := range msg.Content {
		s := renderContentBlock(&block, width, renderer, useGlamour)
		if s != "" {
			parts = append(parts, s)
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

func renderContentBlock(block *claude.ContentBlock, width int, renderer *glamour.TermRenderer, useGlamour bool) string {
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
		label := toolLabel.Render(fmt.Sprintf("Tool: %s", block.Name))
		summary := fmt.Sprintf("id: %s", block.ID)
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

// max returns the larger of a and b.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
