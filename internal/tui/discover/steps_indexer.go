package discover

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
)

func (m Model) updateIndexer(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "y", "Y", "enter":
			m.result.Indexer = true
			m.step = stepEmbeddings
			return m, nil
		case "n", "N":
			m.result.Indexer = false
			m.step = stepEmbeddings
			return m, nil
		}
	}
	return m, nil
}

func (m Model) viewIndexer() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))

	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))

	codeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.accent))

	return fmt.Sprintf("\n  %s %s\n\n  %s\n\n  %s\n\n  %s\n  %s\n\n  %s\n",
		titleStyle.Render(thinktI18n.T("tui.discover.indexer.title", "Indexer")),
		m.stepIndicator(),
		bodyStyle.Render(thinktI18n.T("tui.discover.indexer.body",
			"The indexer watches your session files and builds a searchable\ndatabase using DuckDB. This enables fast full-text search,\nfiltering, and statistics across all your sessions.")),
		bodyStyle.Render(thinktI18n.T("tui.discover.indexer.resources",
			"Resources: ~50MB disk per 10k sessions, minimal CPU usage.")),
		mutedStyle.Render(thinktI18n.T("tui.discover.indexer.reversible", "Reversible:")),
		codeStyle.Render("  thinkt config set indexer.watch false"),
		mutedStyle.Render(thinktI18n.T("tui.discover.indexer.prompt", "Enable indexer? [Y/n]")),
	)
}
