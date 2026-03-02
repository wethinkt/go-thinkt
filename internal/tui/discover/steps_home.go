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
		case "up", "down", "k", "j", "tab":
			m.confirm = !m.confirm
			return m, nil
		case "Y", "y":
			dir, err := config.Dir()
			if err == nil {
				m.result.HomeDir = dir
			}
			m.step = stepSourceConsent
			return m, nil
		case "N", "n":
			m.result.Completed = false
			m.step = stepDone
			return m, tea.Quit
		case "enter":
			if m.confirm {
				dir, err := config.Dir()
				if err == nil {
					m.result.HomeDir = dir
				}
				m.step = stepSourceConsent
				return m, nil
			}
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

	dir, _ := config.Dir()

	return fmt.Sprintf("\n  %s %s\n\n  %s\n\n  %s\n\n  %s\n\n%s\n\n\n\n%s\n",
		titleStyle.Render(thinktI18n.T("tui.discover.home.title", "Home Directory")),
		m.stepIndicator(),
		bodyStyle.Render(thinktI18n.T("tui.discover.home.body",
			"thinkt stores its configuration and index database in:")),
		pathStyle.Render("  "+dir),
		bodyStyle.Render(thinktI18n.T("tui.discover.home.prompt", "Create this directory?")),
		m.renderVerticalConfirm(thinktI18n.T("tui.discover.home.no", "No, exit")),
		m.renderCLIHint("thinkt discover"),
	)
}
