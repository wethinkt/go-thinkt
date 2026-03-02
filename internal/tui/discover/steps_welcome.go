package discover

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
)

func (m Model) updateWelcome(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.applyLanguagePreview()
			}
		case "down", "j":
			if m.cursor < len(m.langs)-1 {
				m.cursor++
				m.applyLanguagePreview()
			}
		case "enter":
			if len(m.langs) > 0 {
				m.result.Language = m.langs[m.cursor].Tag
			}
			m.step = stepHome
			return m, nil
		}
	}
	return m, nil
}

// applyLanguagePreview switches the active i18n language to match the cursor.
func (m *Model) applyLanguagePreview() {
	if m.cursor >= 0 && m.cursor < len(m.langs) {
		thinktI18n.Init(m.langs[m.cursor].Tag)
	}
}

func (m Model) viewWelcome() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))

	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	activeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))

	var b strings.Builder

	// Title + tagline
	b.WriteString(fmt.Sprintf("\n  %s\n\n  %s\n\n\n",
		titleStyle.Render(thinktI18n.T("tui.discover.welcome.title", "thinkt")),
		bodyStyle.Render(thinktI18n.T("tui.discover.welcome.tagline",
			"Your AI coding sessions, indexed and searchable."))))

	// Language picker
	b.WriteString(fmt.Sprintf("  %s\n\n",
		mutedStyle.Render(thinktI18n.T("tui.discover.welcome.selectLanguage",
			"Select your language:"))))
	for i, lang := range m.langs {
		prefix := "  "
		if i == m.cursor {
			prefix = "▸ "
		}

		name := lang.Name
		if lang.EnglishName != lang.Name {
			name += " (" + lang.EnglishName + ")"
		}

		if i == m.cursor {
			b.WriteString(fmt.Sprintf("  %s%s\n", prefix, activeStyle.Render(name)))
		} else {
			b.WriteString(fmt.Sprintf("  %s%s\n", prefix, normalStyle.Render(name)))
		}
	}

	b.WriteString(fmt.Sprintf("\n  %s\n",
		mutedStyle.Render(thinktI18n.T("tui.discover.welcome.prompt",
			"↑/↓: language · Enter: begin · esc: exit"))))

	// CLI hint
	if m.cursor >= 0 && m.cursor < len(m.langs) {
		b.WriteString(fmt.Sprintf("\n\n  %s\n", m.renderCLIHint("thinkt language set "+m.langs[m.cursor].Tag)))
	}

	return b.String()
}
