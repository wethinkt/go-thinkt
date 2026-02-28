package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// Shared glamour renderer (created lazily)
var sharedRenderer *glamour.TermRenderer
var sharedRendererWidth int

func getRenderer(width int) *glamour.TermRenderer {
	if sharedRenderer == nil || sharedRendererWidth != width {
		r, err := glamour.NewTermRenderer(
			glamour.WithStylePath("dark"),
			glamour.WithWordWrap(width),
		)
		if err == nil {
			sharedRenderer = r
			sharedRendererWidth = width
		}
	}
	return sharedRenderer
}

// RenderThinktSession converts a thinkt session's entries into a styled string for the viewport.
func RenderThinktSession(session *thinkt.Session, width int) string {
	return RenderThinktEntries(session.Entries, width)
}

// RenderThinktEntry renders a single entry into a styled string.
// If filters is non-nil, entries and blocks are filtered accordingly.
func RenderThinktEntry(entry *thinkt.Entry, width int, filters *RoleFilterSet) string {
	contentWidth := max(20, width-4)
	renderer := getRenderer(contentWidth)
	return renderThinktEntry(entry, contentWidth, renderer, renderer != nil, filters)
}

// RenderThinktEntries renders a slice of entries into a styled string.
func RenderThinktEntries(entries []thinkt.Entry, width int) string {
	tuilog.Log.Info("RenderThinktEntries: starting", "entryCount", len(entries), "width", width)
	if len(entries) == 0 {
		return ""
	}

	contentWidth := max(20, width-4)
	renderer := getRenderer(contentWidth)

	var b strings.Builder
	for i, entry := range entries {
		tuilog.Log.Debug("RenderThinktEntries: rendering entry", "index", i, "role", entry.Role)
		s := renderThinktEntry(&entry, contentWidth, renderer, renderer != nil, nil)
		if s != "" {
			b.WriteString(s)
			b.WriteString("\n")
		}
	}
	result := b.String()
	tuilog.Log.Info("RenderThinktEntries: complete", "outputLength", len(result))
	return result
}

func renderThinktEntry(entry *thinkt.Entry, width int, renderer *glamour.TermRenderer, useGlamour bool, filters *RoleFilterSet) string {
	if filters != nil && !filters.EntryVisible(entry) {
		return ""
	}

	switch entry.Role {
	case thinkt.RoleUser:
		return renderThinktUserEntry(entry, width, filters)
	case thinkt.RoleAssistant:
		return renderThinktAssistantEntry(entry, width, renderer, useGlamour, filters)
	case thinkt.RoleSystem, thinkt.RoleSummary, thinkt.RoleProgress, thinkt.RoleCheckpoint, thinkt.RoleTool:
		return renderThinktOtherEntry(entry, width)
	default:
		return ""
	}
}

func renderThinktUserEntry(entry *thinkt.Entry, width int, filters *RoleFilterSet) string {
	var parts []string

	text := entry.Text
	if text == "" {
		for _, cb := range entry.ContentBlocks {
			if cb.Type == "text" && cb.Text != "" {
				text = cb.Text
				break
			}
		}
	}
	if text != "" {
		label := userLabel.Render(thinktI18n.T("tui.label.user", "User"))
		content := userBlockStyle.Width(width).Render(text)
		parts = append(parts, label+"\n"+content)
	}

	// Render image/document blocks from user entries
	if filters == nil || filters.Media {
		for _, cb := range entry.ContentBlocks {
			if (cb.Type == "image" || cb.Type == "document") && cb.MediaData != "" {
				s := renderImageBlock(cb.MediaType, cb.MediaData, width)
				if s != "" {
					parts = append(parts, s)
				}
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

func renderThinktAssistantEntry(entry *thinkt.Entry, width int, renderer *glamour.TermRenderer, useGlamour bool, filters *RoleFilterSet) string {
	// Process content blocks
	var parts []string

	// First try content blocks
	if len(entry.ContentBlocks) > 0 {
		for _, block := range entry.ContentBlocks {
			if filters != nil && !filters.BlockVisible(block.Type) {
				continue
			}
			s := renderThinktContentBlock(&block, width, renderer, useGlamour)
			if s != "" {
				parts = append(parts, s)
			}
		}
	}

	// Fall back to text field (treated as output)
	if len(parts) == 0 && entry.Text != "" && (filters == nil || filters.Assistant) {
		label := assistantLabel.Render(thinktI18n.T("tui.label.assistant", "Assistant"))
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
		label := assistantLabel.Render(thinktI18n.T("tui.label.assistant", "Assistant"))
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
		label := thinkingLabel.Render(thinktI18n.T("tui.label.thinking", "Thinking"))
		// Truncate long thinking blocks
		text := block.Thinking
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		content := thinkingBlockStyle.Width(width).Render(text)
		return label + "\n" + content

	case "tool_use":
		label := toolLabel.Render(thinktI18n.Tf("tui.label.tool", "Tool: %s", block.ToolName))
		summary := thinktI18n.Tf("tui.label.toolID", "id: %s", block.ToolUseID)
		content := toolCallBlockStyle.Width(width).Render(summary)
		return label + "\n" + content

	case "tool_result":
		label := toolLabel.Render(thinktI18n.T("tui.label.toolResult", "Tool Result"))
		text := "(result)"
		if block.IsError {
			text = "(error)"
		}
		content := toolResultBlockStyle.Width(width).Render(text)
		return label + "\n" + content

	case "image", "document":
		return renderImageBlock(block.MediaType, block.MediaData, width)

	default:
		return ""
	}
}

func renderThinktOtherEntry(entry *thinkt.Entry, width int) string {
	text := entry.Text
	if text == "" {
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

	label := string(entry.Role)
	if label != "" {
		label = strings.ToUpper(label[:1]) + label[1:]
	}
	styledLabel := thinkingLabel.Render(label)
	content := thinkingBlockStyle.Width(width).Render(text)
	return styledLabel + "\n" + content
}
