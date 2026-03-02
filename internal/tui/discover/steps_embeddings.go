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
	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	cmd := "thinkt config set embedding.enabled true"
	if !m.confirm {
		cmd = "thinkt config set embedding.enabled false"
	}

	return fmt.Sprintf("%s\n  %s\n\n  %s\n\n  %s\n\n%s\n\n%s\n",
		m.renderStepHeader(thinktI18n.T("tui.discover.embeddings.title", "Embeddings")),
		bodyStyle.Render(thinktI18n.T("tui.discover.embeddings.body",
			"Embeddings add semantic search so you can find sessions by intent, not only exact words. It uses a local model (nomic-embed-text-v1.5) and requires no API keys.")),
		bodyStyle.Render(thinktI18n.T("tui.discover.embeddings.resources",
			"Initial model download is about 200MB. GPU helps but is optional.")),
		bodyStyle.Render(thinktI18n.T("tui.discover.embeddings.prompt", "Enable semantic search?")),
		m.renderVerticalConfirm(),
		m.renderCLIHint(cmd),
	)
}
