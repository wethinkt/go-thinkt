package discover

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
)

func (m Model) updateLanguage(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.langs)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.langs) > 0 {
				m.result.Language = m.langs[m.cursor].Tag
				thinktI18n.Init(m.result.Language)
			}
			m.step = stepHome
			return m, nil
		}
	}
	return m, nil
}

func (m Model) viewLanguage() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))

	activeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))

	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n  %s %s\n\n",
		titleStyle.Render(thinktI18n.T("tui.discover.language.title", "Language")),
		m.stepIndicator()))

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
		mutedStyle.Render(thinktI18n.T("tui.discover.language.help", "↑/↓: navigate · Enter: select · esc: exit"))))

	return b.String()
}
