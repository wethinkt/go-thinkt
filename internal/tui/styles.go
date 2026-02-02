package tui

import (
	"sync"

	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// Styles holds all the computed lipgloss styles for the TUI.
type Styles struct {
	// Column border styles
	ActiveBorder   lipgloss.Style
	InactiveBorder lipgloss.Style

	// Status bar
	StatusBar lipgloss.Style

	// Conversation block styles
	UserBlock       lipgloss.Style
	AssistantBlock  lipgloss.Style
	ThinkingBlock   lipgloss.Style
	ToolCallBlock   lipgloss.Style
	ToolResultBlock lipgloss.Style

	// Block labels
	UserLabel      lipgloss.Style
	AssistantLabel lipgloss.Style
	ThinkingLabel  lipgloss.Style
	ToolLabel      lipgloss.Style

	// Viewer styles
	ViewerTitle  lipgloss.Style
	ViewerInfo   lipgloss.Style
	ViewerHelp   lipgloss.Style
	ViewerBorder lipgloss.Style

	// Separators
	Separator lipgloss.Style
	MoreText  lipgloss.Style

	// Confirm dialog
	ConfirmPrompt     lipgloss.Style
	ConfirmSelected   lipgloss.Style
	ConfirmUnselected lipgloss.Style
}

var (
	stylesOnce sync.Once
	styles     Styles
)

// GetStyles returns the current styles, initializing from theme if needed.
func GetStyles() *Styles {
	stylesOnce.Do(func() {
		styles = buildStyles(theme.Current())
	})
	return &styles
}

// ReloadStyles rebuilds styles from the current theme.
func ReloadStyles() *Styles {
	theme.Reload()
	styles = buildStyles(theme.Current())
	return &styles
}

// applyStyle applies a theme.Style to a lipgloss.Style builder.
func applyStyle(s lipgloss.Style, ts theme.Style) lipgloss.Style {
	if ts.Fg != "" {
		s = s.Foreground(lipgloss.Color(ts.Fg))
	}
	if ts.Bg != "" {
		s = s.Background(lipgloss.Color(ts.Bg))
	}
	if ts.Bold {
		s = s.Bold(true)
	}
	if ts.Italic {
		s = s.Italic(true)
	}
	if ts.Underline {
		s = s.Underline(true)
	}
	return s
}

// buildStyles creates Styles from a Theme.
func buildStyles(t theme.Theme) Styles {
	return Styles{
		// Column border styles
		ActiveBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(t.GetBorderActive())),

		InactiveBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(t.GetBorderInactive())),

		// Status bar
		StatusBar: lipgloss.NewStyle().
			Background(lipgloss.Color(t.GetAccent())).
			Foreground(lipgloss.Color(t.TextPrimary.Fg)).
			Bold(true).
			Padding(0, 1),

		// Conversation block styles
		UserBlock: applyStyle(lipgloss.NewStyle(), t.UserBlock).
			Padding(0, 1).
			MarginBottom(1),

		AssistantBlock: applyStyle(lipgloss.NewStyle(), t.AssistantBlock).
			Padding(0, 1).
			MarginBottom(1),

		ThinkingBlock: applyStyle(lipgloss.NewStyle(), t.ThinkingBlock).
			Padding(0, 1).
			MarginBottom(1),

		ToolCallBlock: applyStyle(lipgloss.NewStyle(), t.ToolCallBlock).
			Padding(0, 1).
			MarginBottom(1),

		ToolResultBlock: applyStyle(lipgloss.NewStyle(), t.ToolResultBlock).
			Padding(0, 1).
			MarginBottom(1),

		// Block labels
		UserLabel:      applyStyle(lipgloss.NewStyle(), t.UserLabel),
		AssistantLabel: applyStyle(lipgloss.NewStyle(), t.AssistantLabel),
		ThinkingLabel:  applyStyle(lipgloss.NewStyle(), t.ThinkingLabel),
		ToolLabel:      applyStyle(lipgloss.NewStyle(), t.ToolLabel),

		// Viewer styles
		ViewerTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(t.TextPrimary.Fg)),

		ViewerInfo: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextSecondary.Fg)),

		ViewerHelp: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextMuted.Fg)),

		ViewerBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(t.GetBorderInactive())),

		// Separators
		Separator: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.GetAccent())).
			Bold(true),

		MoreText: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextSecondary.Fg)).
			Italic(true),

		// Confirm dialog
		ConfirmPrompt: applyStyle(lipgloss.NewStyle(), t.ConfirmPrompt).
			Bold(true),

		ConfirmSelected: applyStyle(lipgloss.NewStyle(), t.ConfirmSelected).
			Bold(true).
			Padding(0, 2),

		ConfirmUnselected: applyStyle(lipgloss.NewStyle(), t.ConfirmUnselected).
			Padding(0, 2),
	}
}

// Package-level style accessors for backward compatibility.
// These are initialized on first access via GetStyles().
var (
	activeBorderStyle   = lipgloss.Style{}
	inactiveBorderStyle = lipgloss.Style{}
	statusBarStyle      = lipgloss.Style{}

	userBlockStyle       = lipgloss.Style{}
	assistantBlockStyle  = lipgloss.Style{}
	thinkingBlockStyle   = lipgloss.Style{}
	toolCallBlockStyle   = lipgloss.Style{}
	toolResultBlockStyle = lipgloss.Style{}

	userLabel      = lipgloss.Style{}
	assistantLabel = lipgloss.Style{}
	thinkingLabel  = lipgloss.Style{}
	toolLabel      = lipgloss.Style{}

	viewerTitleStyle  = lipgloss.Style{}
	viewerInfoStyle   = lipgloss.Style{}
	viewerHelpStyle   = lipgloss.Style{}
	viewerBorderStyle = lipgloss.Style{}
)

func init() {
	s := GetStyles()

	activeBorderStyle = s.ActiveBorder
	inactiveBorderStyle = s.InactiveBorder
	statusBarStyle = s.StatusBar

	userBlockStyle = s.UserBlock
	assistantBlockStyle = s.AssistantBlock
	thinkingBlockStyle = s.ThinkingBlock
	toolCallBlockStyle = s.ToolCallBlock
	toolResultBlockStyle = s.ToolResultBlock

	userLabel = s.UserLabel
	assistantLabel = s.AssistantLabel
	thinkingLabel = s.ThinkingLabel
	toolLabel = s.ToolLabel

	viewerTitleStyle = s.ViewerTitle
	viewerInfoStyle = s.ViewerInfo
	viewerHelpStyle = s.ViewerHelp
	viewerBorderStyle = s.ViewerBorder
}
