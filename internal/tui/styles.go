package tui

import "charm.land/lipgloss/v2"

// Column border styles.
var (
	activeBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#7D56F4"))

	inactiveBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#444444"))

	statusBarStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#7D56F4")).
				Foreground(lipgloss.Color("#ffffff")).
				Bold(true).
				Padding(0, 1)
)

// Conversation block styles.
var (
	userBlockStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#1a3a5c")).
				Foreground(lipgloss.Color("#e0e0e0")).
				Padding(0, 1).
				MarginBottom(1)

	assistantBlockStyle = lipgloss.NewStyle().
					Background(lipgloss.Color("#1a3c1a")).
					Foreground(lipgloss.Color("#e0e0e0")).
					Padding(0, 1).
					MarginBottom(1)

	thinkingBlockStyle = lipgloss.NewStyle().
					Background(lipgloss.Color("#3a1a3c")).
					Foreground(lipgloss.Color("#c0a0c0")).
					Padding(0, 1).
					MarginBottom(1)

	toolCallBlockStyle = lipgloss.NewStyle().
					Background(lipgloss.Color("#3c2a1a")).
					Foreground(lipgloss.Color("#e0c080")).
					Padding(0, 1).
					MarginBottom(1)

	toolResultBlockStyle = lipgloss.NewStyle().
					Background(lipgloss.Color("#1a2a3c")).
					Foreground(lipgloss.Color("#a0c0e0")).
					Padding(0, 1).
					MarginBottom(1)
)

// Block label styles.
var (
	userLabel      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5dade2"))
	assistantLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#58d68d"))
	thinkingLabel  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#af7ac5"))
	toolLabel      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f0b27a"))
)
