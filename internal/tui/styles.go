package tui

import (
	"sync"

	"charm.land/lipgloss/v2"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tui/theme"
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

// buildStyles creates Styles from a Theme.
func buildStyles(t theme.Theme) Styles {
	return Styles{
		// Column border styles
		ActiveBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(t.AccentPrimary)),

		InactiveBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(t.BorderInactive)),

		// Status bar
		StatusBar: lipgloss.NewStyle().
			Background(lipgloss.Color(t.AccentPrimary)).
			Foreground(lipgloss.Color(t.TextPrimary)).
			Bold(true).
			Padding(0, 1),

		// Conversation block styles
		UserBlock: lipgloss.NewStyle().
			Background(lipgloss.Color(t.UserBlockBg)).
			Foreground(lipgloss.Color(t.UserBlockFg)).
			Padding(0, 1).
			MarginBottom(1),

		AssistantBlock: lipgloss.NewStyle().
			Background(lipgloss.Color(t.AssistantBlockBg)).
			Foreground(lipgloss.Color(t.AssistantBlockFg)).
			Padding(0, 1).
			MarginBottom(1),

		ThinkingBlock: lipgloss.NewStyle().
			Background(lipgloss.Color(t.ThinkingBlockBg)).
			Foreground(lipgloss.Color(t.ThinkingBlockFg)).
			Padding(0, 1).
			MarginBottom(1),

		ToolCallBlock: lipgloss.NewStyle().
			Background(lipgloss.Color(t.ToolCallBlockBg)).
			Foreground(lipgloss.Color(t.ToolCallBlockFg)).
			Padding(0, 1).
			MarginBottom(1),

		ToolResultBlock: lipgloss.NewStyle().
			Background(lipgloss.Color(t.ToolResultBlockBg)).
			Foreground(lipgloss.Color(t.ToolResultBlockFg)).
			Padding(0, 1).
			MarginBottom(1),

		// Block labels
		UserLabel:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.UserLabel)),
		AssistantLabel: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.AssistantLabel)),
		ThinkingLabel:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.ThinkingLabel)),
		ToolLabel:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.ToolLabel)),

		// Viewer styles
		ViewerTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(t.TextPrimary)),

		ViewerInfo: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextSecondary)),

		ViewerHelp: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextMuted)),

		ViewerBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(t.BorderInactive)),

		// Separators
		Separator: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.AccentPrimary)).
			Bold(true),

		MoreText: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextSecondary)).
			Italic(true),

		// Confirm dialog
		ConfirmPrompt: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(t.ConfirmPromptFg)),

		ConfirmSelected: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(t.ConfirmSelectedFg)).
			Background(lipgloss.Color(t.ConfirmSelectedBg)).
			Padding(0, 2),

		ConfirmUnselected: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.ConfirmUnselectedFg)).
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
