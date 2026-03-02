package discover

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
)

func (m Model) updateSuggestions(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		if msg.String() == "enter" {
			m.result.Completed = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) viewSuggestions() string {
	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))

	codeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.accent))

	var b strings.Builder
	b.WriteString(m.renderStepHeader(thinktI18n.T("tui.discover.suggestions.title", "Setup Complete")))
	b.WriteString("\n")

	// Suggested commands
	b.WriteString(fmt.Sprintf("  %s\n\n",
		bodyStyle.Render(thinktI18n.T("tui.discover.suggestions.next", "Suggested next steps:"))))

	const cmdCol = 42
	if m.result.Indexer {
		b.WriteString(fmt.Sprintf("    %s %s\n",
			padRight(codeStyle.Render("thinkt indexer watch"), cmdCol),
			mutedStyle.Render(thinktI18n.T("tui.discover.suggestions.cmdWatch", "Start the indexer watcher"))))
	}

	b.WriteString(fmt.Sprintf("    %s %s\n",
		padRight(codeStyle.Render("thinkt search"), cmdCol),
		mutedStyle.Render(thinktI18n.T("tui.discover.suggestions.cmdSearch", "Run keyword search"))))

	b.WriteString(fmt.Sprintf("    %s %s\n",
		padRight(codeStyle.Render("thinkt tui"), cmdCol),
		mutedStyle.Render(thinktI18n.T("tui.discover.suggestions.cmdTui", "Open the interactive browser"))))

	if !m.result.Embeddings {
		b.WriteString(fmt.Sprintf("    %s %s\n",
			padRight(codeStyle.Render("thinkt config set embedding.enabled true"), cmdCol),
			mutedStyle.Render(thinktI18n.T("tui.discover.suggestions.cmdEmbed", "Enable semantic search later"))))
	}

	b.WriteString(fmt.Sprintf("\n  %s\n",
		mutedStyle.Render(thinktI18n.T("tui.discover.suggestions.rerun", "You can rerun this setup anytime with: thinkt discover"))))

	b.WriteString(fmt.Sprintf("\n  %s\n",
		mutedStyle.Render(thinktI18n.T("tui.discover.suggestions.done", "Enter: finish setup · esc: exit"))))

	return b.String()
}
