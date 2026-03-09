package setup

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hybridgroup/yzma/pkg/llama"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
)

func (m Model) enableEmbeddings() (tea.Model, tea.Cmd) {
	m.result.Embeddings = true
	// Build sorted model ID list for picker
	m.embModelIDs = make([]string, 0, len(embedding.KnownModels))
	for id := range embedding.KnownModels {
		m.embModelIDs = append(m.embModelIDs, id)
	}
	sort.Strings(m.embModelIDs)
	// Pre-select default model
	m.embCursor = 0
	for i, id := range m.embModelIDs {
		if id == embedding.DefaultModelID {
			m.embCursor = i
			break
		}
	}
	m.step = stepEmbeddingModel
	return m, nil
}

func (m Model) updateEmbeddings(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "up", "down", "k", "j", "tab":
			m.confirm = !m.confirm
			return m, nil
		case "Y", "y":
			return m.enableEmbeddings()
		case "N", "n":
			m.result.Embeddings = false
			m.step = stepSuggestions
			return m, nil
		case "enter":
			if m.confirm {
				return m.enableEmbeddings()
			}
			m.result.Embeddings = false
			m.step = stepSuggestions
			return m, nil
		}
	}
	return m, nil
}

// --- stepEmbeddingModel: pick embedding model ---

func (m Model) updateEmbeddingModel(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case "up", "k":
			if m.embCursor > 0 {
				m.embCursor--
			}
			return m, nil
		case "down", "j":
			if m.embCursor < len(m.embModelIDs)-1 {
				m.embCursor++
			}
			return m, nil
		case "tab":
			if len(m.embModelIDs) > 0 {
				m.embCursor = (m.embCursor + 1) % len(m.embModelIDs)
			}
			return m, nil
		case "enter":
			if m.embCursor < len(m.embModelIDs) {
				m.result.EmbeddingModel = m.embModelIDs[m.embCursor]
			}
			m.step = stepSuggestions
			return m, nil
		}
	}
	return m, nil
}

func (m Model) viewEmbeddingModel() string {
	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))
	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))
	accentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.accent))

	var b strings.Builder
	b.WriteString(m.renderStepHeader(thinktI18n.T("tui.setup.embeddingModel.title", "Embedding Model")))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n\n",
		bodyStyle.Render(thinktI18n.T("tui.setup.embeddingModel.body", "Select the embedding model for semantic search:"))))

	// Compute column width for alignment
	maxIDLen := 0
	for _, id := range m.embModelIDs {
		if len(id) > maxIDLen {
			maxIDLen = len(id)
		}
	}

	for i, id := range m.embModelIDs {
		spec := embedding.KnownModels[id]
		pointer := "  "
		nameStyle := bodyStyle
		if i == m.embCursor {
			pointer = "▸ "
			nameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.primary))
		}

		// Format detail: "768-dim, mean pooling, ~140MB"
		pooling := "mean"
		if spec.PoolingType != llama.PoolingTypeMean {
			pooling = "last-token"
		}
		modelSizes := map[string]string{
			"nomic-embed-text-v1.5": "~140MB",
			"qwen3-embedding-0.6b":  "~800MB",
		}
		size := modelSizes[id]
		detail := mutedStyle.Render(fmt.Sprintf("%d-dim, %s pooling, %s", spec.Dim, pooling, size))

		isDefault := ""
		if id == embedding.DefaultModelID {
			isDefault = " " + accentStyle.Render(thinktI18n.T("tui.setup.embeddingModel.default", "(default)"))
		}

		b.WriteString(fmt.Sprintf("  %s%-*s  %s%s\n",
			pointer,
			maxIDLen, nameStyle.Render(id),
			detail,
			isDefault,
		))
	}

	b.WriteString(fmt.Sprintf("\n  %s\n",
		mutedStyle.Render(m.withEscQ(thinktI18n.T("tui.setup.embeddingModel.help", "↑/↓: navigate · Enter: select · esc: exit")))))

	b.WriteString(fmt.Sprintf("\n\n  %s\n", m.renderCLIHint("thinkt embeddings model")))

	return b.String()
}

func (m Model) viewEmbeddings() string {
	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	cmd := "thinkt embeddings enable"
	if !m.confirm {
		cmd = "thinkt embeddings disable"
	}

	return fmt.Sprintf("%s\n  %s\n\n  %s\n\n  %s\n\n%s\n\n%s\n",
		m.renderStepHeader(thinktI18n.T("tui.setup.embeddings.title", "Embeddings")),
		bodyStyle.Render(thinktI18n.T("tui.setup.embeddings.body",
			"Embeddings enable semantic search, so you can find sessions by intent, not only exact words. It uses a local model (nomic-embed-text-v1.5) and requires no API keys.")),
		bodyStyle.Render(thinktI18n.T("tui.setup.embeddings.resources",
			"Initial model download is about 150MB. GPU helps but is optional.\n\n  The intial background scan may be resource intensive.\n\n  Turn it off with 'thinkt embeddings disable'.")),
		bodyStyle.Render(thinktI18n.T("tui.setup.embeddings.prompt", "Enable semantic search?")),
		m.renderVerticalConfirm(),
		m.renderCLIHint(cmd),
	)
}
