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
			m.step = stepDone
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) viewSuggestions() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))

	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))

	codeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.accent))

	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n  %s %s\n\n",
		titleStyle.Render(thinktI18n.T("tui.discover.suggestions.title", "Setup Complete")),
		m.stepIndicator()))

	// Config summary
	b.WriteString(fmt.Sprintf("  %s\n\n",
		bodyStyle.Render(thinktI18n.T("tui.discover.suggestions.summary", "Configuration summary:"))))

	if m.result.Language != "" {
		b.WriteString(fmt.Sprintf("    %s  %s\n",
			mutedStyle.Render("Language:"),
			bodyStyle.Render(m.result.Language)))
	}

	enabledSources := 0
	for _, enabled := range m.result.Sources {
		if enabled {
			enabledSources++
		}
	}
	b.WriteString(fmt.Sprintf("    %s  %s\n",
		mutedStyle.Render("Sources:"),
		bodyStyle.Render(fmt.Sprintf("%d", enabledSources))))

	indexerStatus := thinktI18n.T("tui.discover.suggestions.disabled", "disabled")
	if m.result.Indexer {
		indexerStatus = thinktI18n.T("tui.discover.suggestions.enabled", "enabled")
	}
	b.WriteString(fmt.Sprintf("    %s  %s\n",
		mutedStyle.Render("Indexer:"),
		bodyStyle.Render(indexerStatus)))

	embeddingStatus := thinktI18n.T("tui.discover.suggestions.disabled", "disabled")
	if m.result.Embeddings {
		embeddingStatus = thinktI18n.T("tui.discover.suggestions.enabled", "enabled")
	}
	b.WriteString(fmt.Sprintf("    %s  %s\n",
		mutedStyle.Render("Embeddings:"),
		bodyStyle.Render(embeddingStatus)))

	// Suggested commands
	b.WriteString(fmt.Sprintf("\n  %s\n\n",
		bodyStyle.Render(thinktI18n.T("tui.discover.suggestions.next", "Try these commands next:"))))

	if m.result.Indexer {
		b.WriteString(fmt.Sprintf("    %s  %s\n",
			codeStyle.Render("thinkt indexer watch"),
			mutedStyle.Render(thinktI18n.T("tui.discover.suggestions.cmdWatch", "Start indexing sessions"))))
	}

	b.WriteString(fmt.Sprintf("    %s  %s\n",
		codeStyle.Render("thinkt search"),
		mutedStyle.Render(thinktI18n.T("tui.discover.suggestions.cmdSearch", "Search your sessions"))))

	b.WriteString(fmt.Sprintf("    %s  %s\n",
		codeStyle.Render("thinkt tui"),
		mutedStyle.Render(thinktI18n.T("tui.discover.suggestions.cmdTui", "Open the interactive browser"))))

	if !m.result.Embeddings {
		b.WriteString(fmt.Sprintf("    %s  %s\n",
			codeStyle.Render("thinkt config set embedding.enabled true"),
			mutedStyle.Render(thinktI18n.T("tui.discover.suggestions.cmdEmbed", "Enable semantic search later"))))
	}

	b.WriteString(fmt.Sprintf("\n  %s\n",
		mutedStyle.Render(thinktI18n.T("tui.discover.suggestions.done", "Enter: finish · esc: exit"))))

	return b.String()
}
