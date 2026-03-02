package discover

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
)

func (m Model) updateWelcome(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		if msg.String() == "enter" {
			m.step = stepLanguage
			return m, nil
		}
	}
	return m, nil
}

func (m Model) viewWelcome() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))

	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))

	title := titleStyle.Render(thinktI18n.T("tui.discover.welcome.title", "thinkt"))
	tagline := bodyStyle.Render(thinktI18n.T("tui.discover.welcome.tagline", "Your AI coding sessions, indexed and searchable."))
	body := bodyStyle.Render(thinktI18n.T("tui.discover.welcome.body",
		"This wizard will help you set up thinkt by discovering\nyour AI coding assistant sessions and configuring indexing."))
	prompt := mutedStyle.Render(thinktI18n.T("tui.discover.welcome.prompt", "Enter: begin · esc: exit"))

	return fmt.Sprintf("\n\n  %s\n\n  %s\n\n  %s\n\n\n  %s\n", title, tagline, body, prompt)
}
