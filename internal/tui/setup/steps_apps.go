package setup

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/config"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
)

// --- stepApps: multi-select checklist of discovered apps ---

func (m Model) updateApps(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case "up", "k":
			if m.appCursor > 0 {
				m.appCursor--
			}
			return m, nil
		case "down", "j":
			if m.appCursor < len(m.apps)-1 {
				m.appCursor++
			}
			return m, nil
		case "tab":
			if len(m.apps) > 0 {
				m.appCursor = (m.appCursor + 1) % len(m.apps)
			}
			return m, nil
		case " ", "space":
			if m.appCursor < len(m.apps) {
				m.apps[m.appCursor].Enabled = !m.apps[m.appCursor].Enabled
			}
			return m, nil
		case "enter":
			// Save app preferences to result
			m.result.Apps = make(map[string]bool, len(m.apps))
			for _, app := range m.apps {
				m.result.Apps[app.ID] = app.Enabled
			}
			// Build list of enabled terminal apps
			m.termApps = nil
			for _, app := range m.apps {
				if len(app.ExecRun) > 0 && app.Enabled {
					m.termApps = append(m.termApps, app)
				}
			}
			if len(m.termApps) == 0 {
				// No terminal apps enabled — skip terminal step
				m.confirm = true
				m.step = stepIndexer
				return m, nil
			}
			m.detectedTerm = config.DetectTerminal(m.apps)
			if m.detectedTerm != "" {
				// Verify detected terminal is in enabled terminal apps
				found := false
				for _, app := range m.termApps {
					if app.ID == m.detectedTerm {
						found = true
						break
					}
				}
				if !found {
					m.detectedTerm = ""
				}
			}
			if m.detectedTerm != "" {
				m.confirm = true
				m.termPicking = false
				m.step = stepTerminal
			} else {
				m.termPicking = true
				m.termCursor = 0
				m.step = stepTerminal
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) viewApps() string {
	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))
	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))
	accentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.accent))

	var b strings.Builder
	b.WriteString(m.renderStepHeader(thinktI18n.T("tui.setup.apps.title", "Apps")))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n\n",
		bodyStyle.Render(thinktI18n.T("tui.setup.apps.body", "Select which apps thinkt can open files and run commands in."))))

	for i, app := range m.apps {
		pointer := "  "
		if i == m.appCursor {
			pointer = "▸ "
		}

		check := "[ ]"
		if app.Enabled {
			check = "[x]"
		}

		nameStyle := bodyStyle
		checkStyle := mutedStyle
		if i == m.appCursor {
			nameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.primary))
			checkStyle = accentStyle
		}

		tag := ""
		if len(app.ExecRun) > 0 {
			tag = mutedStyle.Render(" " + thinktI18n.T("tui.setup.apps.terminal", "(terminal)"))
		}

		b.WriteString(fmt.Sprintf("  %s%s %s%s\n",
			pointer,
			checkStyle.Render(check),
			nameStyle.Render(app.Name),
			tag,
		))
	}

	b.WriteString(fmt.Sprintf("\n  %s\n",
		mutedStyle.Render(m.withEscQ(thinktI18n.T("tui.setup.apps.help", "↑/↓: navigate · Space: toggle · Enter: continue · esc: exit")))))

	b.WriteString(fmt.Sprintf("\n\n  %s\n", m.renderCLIHint("thinkt apps enable/disable")))

	return b.String()
}

// --- stepTerminal: detect and confirm default terminal ---

func (m Model) updateTerminal(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		if m.termPicking {
			// Terminal picker mode
			switch msg.String() {
			case "up", "k":
				if m.termCursor > 0 {
					m.termCursor--
				}
				return m, nil
			case "down", "j":
				if m.termCursor < len(m.termApps)-1 {
					m.termCursor++
				}
				return m, nil
			case "tab":
				if len(m.termApps) > 0 {
					m.termCursor = (m.termCursor + 1) % len(m.termApps)
				}
				return m, nil
			case "enter":
				if m.termCursor < len(m.termApps) {
					m.result.Terminal = m.termApps[m.termCursor].ID
				}
				m.confirm = true
				m.step = stepIndexer
				return m, nil
			}
			return m, nil
		}

		// Confirmation mode (detected terminal)
		switch msg.String() {
		case "up", "down", "k", "j", "tab":
			m.confirm = !m.confirm
			return m, nil
		case "Y", "y":
			m.result.Terminal = m.detectedTerm
			m.confirm = true
			m.step = stepIndexer
			return m, nil
		case "N", "n":
			m.termPicking = true
			m.termCursor = 0
			return m, nil
		case "enter":
			if m.confirm {
				m.result.Terminal = m.detectedTerm
				m.confirm = true
				m.step = stepIndexer
			} else {
				m.termPicking = true
				m.termCursor = 0
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) viewTerminal() string {
	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))
	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))
	accentStyle := lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color(m.accent))

	var b strings.Builder
	b.WriteString(m.renderStepHeader(thinktI18n.T("tui.setup.terminal.title", "Default Terminal")))
	b.WriteString("\n")

	if m.termPicking {
		b.WriteString(fmt.Sprintf("  %s\n\n",
			bodyStyle.Render(thinktI18n.T("tui.setup.terminal.pick", "Select your default terminal app:"))))

		for i, app := range m.termApps {
			pointer := "  "
			nameStyle := bodyStyle
			if i == m.termCursor {
				pointer = "▸ "
				nameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.primary))
			}
			b.WriteString(fmt.Sprintf("  %s%s\n", pointer, nameStyle.Render(app.Name)))
		}

		b.WriteString(fmt.Sprintf("\n  %s\n",
			mutedStyle.Render(m.withEscQ(thinktI18n.T("tui.setup.terminal.pickHelp", "↑/↓: navigate · Enter: select · esc: exit")))))
	} else {
		// Find detected terminal name
		termName := m.detectedTerm
		for _, app := range m.termApps {
			if app.ID == m.detectedTerm {
				termName = app.Name
				break
			}
		}

		b.WriteString(fmt.Sprintf("  %s %s%s\n\n",
			bodyStyle.Render(thinktI18n.T("tui.setup.terminal.detected", "Detected")),
			accentStyle.Render(termName),
			bodyStyle.Render(thinktI18n.T("tui.setup.terminal.useAsDefault", " as your terminal. Use as default?"))))

		b.WriteString(m.renderVerticalConfirm())
	}

	b.WriteString(fmt.Sprintf("\n\n  %s\n", m.renderCLIHint("thinkt apps set-terminal")))

	return b.String()
}
