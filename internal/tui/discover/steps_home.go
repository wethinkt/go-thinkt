package discover

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/config"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
)

func (m Model) updateHome(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "y", "Y", "enter":
			dir, err := config.Dir()
			if err == nil {
				m.result.HomeDir = dir
			}
			m.step = stepSourceConsent
			return m, m.startScan()
		case "n", "N":
			m.result.Completed = false
			m.step = stepDone
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) viewHome() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))

	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	pathStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))

	dir, _ := config.Dir()

	return fmt.Sprintf("\n  %s %s\n\n  %s\n\n  %s\n\n  %s\n",
		titleStyle.Render(thinktI18n.T("tui.discover.home.title", "Home Directory")),
		m.stepIndicator(),
		bodyStyle.Render(thinktI18n.T("tui.discover.home.body",
			"thinkt stores its configuration and index database in:")),
		pathStyle.Render("  "+dir),
		mutedStyle.Render(thinktI18n.T("tui.discover.home.prompt", "Create this directory? [Y/n]")),
	)
}
