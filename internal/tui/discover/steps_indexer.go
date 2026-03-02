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
		case "up", "down", "k", "j", "tab":
			m.confirm = !m.confirm
			return m, nil
		case "Y", "y":
			m.result.Indexer = true
			m.step = stepEmbeddings
			m.confirm = false // embeddings defaults to No
			return m, nil
		case "N", "n":
			m.result.Indexer = false
			m.step = stepEmbeddings
			m.confirm = false // embeddings defaults to No
			return m, nil
		case "enter":
			m.result.Indexer = m.confirm
			m.step = stepEmbeddings
			m.confirm = false // embeddings defaults to No
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

	cmd := "thinkt config set indexer.watch true"
	if !m.confirm {
		cmd = "thinkt config set indexer.watch false"
	}

	return fmt.Sprintf("\n  %s %s\n\n  %s\n\n  %s\n\n  %s\n\n%s\n\n\n\n%s\n",
		titleStyle.Render(thinktI18n.T("tui.discover.indexer.title", "Indexer")),
		m.stepIndicator(),
		bodyStyle.Render(thinktI18n.T("tui.discover.indexer.body",
			"The indexer watches your session files and builds a searchable\ndatabase using DuckDB. This enables fast full-text search,\nfiltering, and statistics across all your sessions.")),
		bodyStyle.Render(thinktI18n.T("tui.discover.indexer.resources",
			"Resources: ~50MB disk per 10k sessions, minimal CPU usage.")),
		bodyStyle.Render(thinktI18n.T("tui.discover.indexer.prompt", "Enable indexer?")),
		m.renderVerticalConfirm(),
		m.renderCLIHint(cmd),
	)
}
