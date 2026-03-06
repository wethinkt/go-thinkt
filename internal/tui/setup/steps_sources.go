package setup

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/config"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/tui"
)

// --- sourceConsent step ---

func (m Model) selectConsentChoice(choice int) (tea.Model, tea.Cmd) {
	switch choice {
	case 0:
		m.sourceMode = sourceModeOneByOne
		m.step = stepSourceApproval
		return m, m.startProgressiveScan()
	case 1:
		m.sourceMode = sourceModeAll
		m.step = stepSourceApproval
		return m, m.startProgressiveScan()
	case 2:
		m.sourceMode = sourceModeDisableAll
		m.apps = config.DefaultApps()
		m.appCursor = 0
		m.step = stepApps
		return m, nil
	case 3:
		m.result.Completed = false
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) updateSourceConsent(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "up", "k":
			if m.consentCursor > 0 {
				m.consentCursor--
			}
			return m, nil
		case "down", "j":
			if m.consentCursor < 3 {
				m.consentCursor++
			}
			return m, nil
		case "tab":
			m.consentCursor = (m.consentCursor + 1) % 4
			return m, nil
		case "enter":
			return m.selectConsentChoice(m.consentCursor)
		case "1":
			return m.selectConsentChoice(0)
		case "2":
			return m.selectConsentChoice(1)
		case "3":
			return m.selectConsentChoice(2)
		case "4":
			return m.selectConsentChoice(3)
		}
	}
	return m, nil
}

func (m Model) viewSourceConsent() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))

	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))

	var b strings.Builder
	b.WriteString(m.renderStepHeader(thinktI18n.T("tui.setup.sources.title", "Source Discovery")))
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("  %s\n\n",
		bodyStyle.Render(thinktI18n.T("tui.setup.sources.consent",
			"thinkt scans specific paths in your home directory (for example ~/.claude/ and ~/.codex).\n\n  It only reads session logs and never modifies or deletes source files.  All data remains local, even with 'thinkt web'.\n\n  Choose how to enable session sources:"))))

	choices := []struct {
		key   string
		label string
	}{
		{"1", thinktI18n.T("tui.setup.sources.oneByOne", "Review each source before enabling")},
		{"2", thinktI18n.T("tui.setup.sources.all", "Enable all discovered sources")},
		{"3", thinktI18n.T("tui.setup.sources.disableAll", "Disable all sources")},
		{"4", thinktI18n.T("tui.setup.sources.exitNoSave", "Exit setup without saving")},
	}
	for i, c := range choices {
		pointer := "  "
		keyStyle := mutedStyle
		labelStyle := bodyStyle
		if i == m.consentCursor {
			pointer = "▸ "
			keyStyle = titleStyle
			labelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.primary))
		}
		b.WriteString(fmt.Sprintf("  %s%s  %s\n",
			pointer,
			keyStyle.Render(c.key),
			labelStyle.Render(c.label)))
	}

	b.WriteString(fmt.Sprintf("\n  %s\n",
		mutedStyle.Render(m.withEscQ(thinktI18n.T("tui.setup.sources.consentHelp", "↑/↓ or tab: select · 1-4: choose · Enter: confirm · esc: exit")))))

	// CLI hint based on current cursor
	consentCmds := []string{
		"thinkt sources enable",
		"thinkt sources enable --all",
		"thinkt sources disable --all",
		"",
	}
	if m.consentCursor < len(consentCmds) && consentCmds[m.consentCursor] != "" {
		b.WriteString(fmt.Sprintf("\n\n  %s\n", m.renderCLIHint(consentCmds[m.consentCursor])))
	}

	return b.String()
}

// --- sourceApproval step (progressive scan + approval + summary) ---

func (m Model) updateSourceApproval(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		// While waiting for user approval on a specific source (one-by-one mode)
		if m.pendingApproval {
			advanceApproval := func() (tea.Model, tea.Cmd) {
				m.pendingApproval = false
				m.confirm = true
				if m.scanDone {
					return m, nil
				}
				return m, m.waitForScan()
			}
			switch msg.String() {
			case "up", "down", "k", "j", "tab":
				m.confirm = !m.confirm
				return m, nil
			case "Y", "y":
				m.sources[m.approvalIdx].Approved = true
				return advanceApproval()
			case "N", "n":
				m.sources[m.approvalIdx].Approved = false
				return advanceApproval()
			case "enter":
				m.sources[m.approvalIdx].Approved = m.confirm
				return advanceApproval()
			}
			return m, nil
		}

		// Summary view (scan complete, no pending approval) — Enter continues
		if m.scanDone && !m.pendingApproval {
			if msg.String() == "enter" {
				for _, src := range m.sources {
					m.result.Sources[string(src.Info.Source)] = src.Approved
				}
				m.apps = config.DefaultApps()
				m.appCursor = 0
				m.step = stepApps
				return m, nil
			}
		}
	}
	return m, nil
}

func (m Model) viewSourceApproval() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))

	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))

	var b strings.Builder
	b.WriteString(m.renderStepHeader(thinktI18n.T("tui.setup.sources.title", "Source Discovery")))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n\n",
		mutedStyle.Render(thinktI18n.T("tui.setup.sources.discovered", "Discovered sources:"))))

	// Show running list of discovered sources
	for i, src := range m.sources {
		color := tui.SourceColorHex(src.Info.Source)
		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)

		status := ""
		if m.pendingApproval && i == m.approvalIdx {
			status = "  " + mutedStyle.Render("awaiting decision")
		} else if src.Approved {
			status = fmt.Sprintf("  enabled · %d sessions · %s",
				src.Info.SessionCount, formatBytes(src.Info.TotalSize))
		} else if !m.pendingApproval || i < m.approvalIdx {
			status = "  " + mutedStyle.Render("disabled")
		} else {
			status = fmt.Sprintf("  found · %d sessions · %s",
				src.Info.SessionCount, formatBytes(src.Info.TotalSize))
		}

		mark := "  "
		if !m.pendingApproval || i < m.approvalIdx {
			if src.Approved {
				mark = "✓ "
			} else {
				mark = "✗ "
			}
		} else if m.pendingApproval && i == m.approvalIdx {
			mark = "▸ "
		}

		b.WriteString(fmt.Sprintf("  %s%s%s\n",
			mark,
			padRight(nameStyle.Render(src.Info.Name), 18),
			status))
	}

	// Show details for the source being approved
	if m.pendingApproval && m.approvalIdx < len(m.sources) {
		info := m.sources[m.approvalIdx].Info
		const detailCol = 12
		b.WriteString(fmt.Sprintf("\n  %s\n",
			mutedStyle.Render(thinktI18n.T("tui.setup.approval.selected", "Selected source:"))))
		b.WriteString(fmt.Sprintf("\n    %s %s\n",
			padRight(mutedStyle.Render(thinktI18n.T("tui.setup.approval.path", "Path:")), detailCol),
			bodyStyle.Render(info.BasePath)))
		b.WriteString(fmt.Sprintf("    %s %d\n",
			padRight(mutedStyle.Render(thinktI18n.T("tui.setup.approval.sessions", "Sessions:")), detailCol),
			info.SessionCount))
		b.WriteString(fmt.Sprintf("    %s %s\n",
			padRight(mutedStyle.Render(thinktI18n.T("tui.setup.approval.size", "Size:")), detailCol),
			bodyStyle.Render(formatBytes(info.TotalSize))))

		b.WriteString(fmt.Sprintf("\n    %s\n\n%s\n",
			bodyStyle.Render(thinktI18n.T("tui.setup.approval.prompt", "Enable this source now?")),
			m.renderVerticalConfirm()))
	}

	// Bottom status line
	if m.scanning {
		b.WriteString(fmt.Sprintf("\n  %s\n",
			mutedStyle.Render(thinktI18n.Tf(
				"tui.setup.sources.scanningCount",
				"Scanning sources... %d discovered so far.",
				len(m.sources),
			))))
	} else if m.scanDone && !m.pendingApproval {
		// Summary footer
		if len(m.sources) == 0 {
			b.WriteString(fmt.Sprintf("\n  %s\n",
				bodyStyle.Render(thinktI18n.T("tui.setup.sources.none",
					"No AI coding sessions found on this machine."))))
		} else {
			var totalSessions int
			var totalSize int64
			for _, src := range m.sources {
				if src.Approved {
					totalSessions += src.Info.SessionCount
					totalSize += src.Info.TotalSize
				}
			}
			if totalSessions > 0 {
				b.WriteString(fmt.Sprintf("\n  %s %d sessions, %s\n",
					titleStyle.Render(thinktI18n.T("tui.setup.summary.total", "Total:")),
					totalSessions,
					formatBytes(totalSize)))
			}
		}
		b.WriteString(fmt.Sprintf("\n  %s\n",
			mutedStyle.Render(m.withEscQ(thinktI18n.T("tui.setup.summary.continue", "Enter: continue to indexer setup · esc: exit")))))
	}

	return b.String()
}
