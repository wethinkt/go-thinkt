package discover

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
)

func (m Model) updateEmbeddings(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "up", "down", "k", "j", "tab":
			m.confirm = !m.confirm
			return m, nil
		case "Y", "y":
			m.result.Embeddings = true
			m.step = stepSuggestions
			return m, nil
		case "N", "n":
			m.result.Embeddings = false
			m.step = stepSuggestions
			return m, nil
		case "enter":
			m.result.Embeddings = m.confirm
			m.step = stepSuggestions
			return m, nil
		}
	}
	return m, nil
}

func (m Model) viewEmbeddings() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))

	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	cmd := "thinkt config set embedding.enabled true"
	if !m.confirm {
		cmd = "thinkt config set embedding.enabled false"
	}

	return fmt.Sprintf("\n  %s %s\n\n  %s\n\n  %s\n\n  %s\n\n%s\n\n\n\n%s\n",
		titleStyle.Render(thinktI18n.T("tui.discover.embeddings.title", "Embeddings")),
		m.stepIndicator(),
		bodyStyle.Render(thinktI18n.T("tui.discover.embeddings.body",
			"Embeddings enable semantic search — find sessions by meaning,\nnot just keywords. This uses a local model (nomic-embed-text-v1.5)\nand requires no API keys or network access.")),
		bodyStyle.Render(thinktI18n.T("tui.discover.embeddings.resources",
			"Resources: ~200MB model download, GPU recommended.")),
		bodyStyle.Render(thinktI18n.T("tui.discover.embeddings.prompt", "Enable embeddings?")),
		m.renderVerticalConfirm(),
		m.renderCLIHint(cmd),
	)
}
