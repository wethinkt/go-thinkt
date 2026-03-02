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
	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	cmd := "thinkt config set indexer.watch true"
	if !m.confirm {
		cmd = "thinkt config set indexer.watch false"
	}

	return fmt.Sprintf("%s\n  %s\n\n  %s\n\n  %s\n\n%s\n\n%s\n",
		m.renderStepHeader(thinktI18n.T("tui.discover.indexer.title", "Indexer")),
		bodyStyle.Render(thinktI18n.T("tui.discover.indexer.body",
			"The indexer keeps a local DuckDB database in sync with your session files. It enables fast search, filtering, and usage statistics across all sources.")),
		bodyStyle.Render(thinktI18n.T("tui.discover.indexer.resources",
			"Typical usage: ~50MB disk per 10k sessions, low background CPU.")),
		bodyStyle.Render(thinktI18n.T("tui.discover.indexer.prompt", "Enable background indexing?")),
		m.renderVerticalConfirm(),
		m.renderCLIHint(cmd),
	)
}
