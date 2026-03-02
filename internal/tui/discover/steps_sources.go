package discover

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/tui"
)

// --- sourceConsent step ---

func (m Model) updateSourceConsent(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Wait for scan to finish
	if m.scanning {
		return m, nil
	}

	if msg, ok := msg.(tea.KeyMsg); ok {
		// No sources found — Enter skips to indexer
		if m.scanDone && len(m.scanResults) == 0 {
			if msg.String() == "enter" {
				m.confirm = true // indexer defaults to Yes
				m.step = stepIndexer
				return m, nil
			}
			return m, nil
		}

		switch msg.String() {
		case "1":
			m.sourceMode = sourceModeOneByOne
			m.sources = make([]sourceResult, len(m.scanResults))
			for i, r := range m.scanResults {
				m.sources[i] = sourceResult{Info: r, Approved: false}
			}
			m.approvalIdx = 0
			m.confirm = true // default Yes for source approval
			m.step = stepSourceApproval
			return m, nil
		case "2":
			m.sourceMode = sourceModeAll
			m.sources = make([]sourceResult, len(m.scanResults))
			for i, r := range m.scanResults {
				m.sources[i] = sourceResult{Info: r, Approved: true}
			}
			m.step = stepSourceSummary
			return m, nil
		case "3":
			m.sourceMode = sourceModeSkip
			m.confirm = true // indexer defaults to Yes
			m.step = stepIndexer
			return m, nil
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

	if m.scanning {
		return fmt.Sprintf("\n  %s %s\n\n  %s\n",
			titleStyle.Render(thinktI18n.T("tui.discover.sources.title", "Source Discovery")),
			m.stepIndicator(),
			bodyStyle.Render(thinktI18n.T("tui.discover.sources.scanning", "Scanning for AI coding sessions...")),
		)
	}

	if m.scanDone && len(m.scanResults) == 0 {
		return fmt.Sprintf("\n  %s %s\n\n  %s\n\n  %s\n",
			titleStyle.Render(thinktI18n.T("tui.discover.sources.title", "Source Discovery")),
			m.stepIndicator(),
			bodyStyle.Render(thinktI18n.T("tui.discover.sources.none",
				"No AI coding sessions found on this machine.")),
			mutedStyle.Render(thinktI18n.T("tui.discover.sources.noneHelp", "Enter: continue · esc: exit")),
		)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n  %s %s\n\n",
		titleStyle.Render(thinktI18n.T("tui.discover.sources.title", "Source Discovery")),
		m.stepIndicator()))

	b.WriteString(fmt.Sprintf("  %s\n\n",
		bodyStyle.Render(thinktI18n.Tf("tui.discover.sources.found", "Found %d source(s):", len(m.scanResults)))))

	for _, r := range m.scanResults {
		color := tui.SourceColorHex(r.Source)
		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)
		b.WriteString(fmt.Sprintf("    %s  %d sessions, %s\n",
			nameStyle.Render(r.Name),
			r.SessionCount,
			formatBytes(r.TotalSize),
		))
	}

	b.WriteString(fmt.Sprintf("\n  %s\n",
		bodyStyle.Render(thinktI18n.T("tui.discover.sources.consent",
			"How would you like to proceed?"))))
	b.WriteString(fmt.Sprintf("\n  %s  %s\n",
		titleStyle.Render("1"),
		bodyStyle.Render(thinktI18n.T("tui.discover.sources.oneByOne", "Review each source individually"))))
	b.WriteString(fmt.Sprintf("  %s  %s\n",
		titleStyle.Render("2"),
		bodyStyle.Render(thinktI18n.T("tui.discover.sources.all", "Enable all sources"))))
	b.WriteString(fmt.Sprintf("  %s  %s\n",
		titleStyle.Render("3"),
		bodyStyle.Render(thinktI18n.T("tui.discover.sources.skip", "Skip source setup"))))

	b.WriteString(fmt.Sprintf("\n  %s\n",
		mutedStyle.Render(thinktI18n.T("tui.discover.sources.consentHelp", "1/2/3: select · esc: exit"))))

	return b.String()
}

// --- sourceApproval step ---

func (m Model) updateSourceApproval(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "left", "right", "tab", "h", "l":
			m.confirm = !m.confirm
			return m, nil
		case "enter":
			m.sources[m.approvalIdx].Approved = m.confirm
			m.approvalIdx++
			if m.approvalIdx >= len(m.sources) {
				m.step = stepSourceSummary
			}
			m.confirm = true // reset default to Yes for next source
			return m, nil
		}
	}
	return m, nil
}

func (m Model) viewSourceApproval() string {
	if m.approvalIdx >= len(m.sources) {
		return ""
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))

	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))

	src := m.sources[m.approvalIdx]
	info := src.Info
	color := tui.SourceColorHex(info.Source)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n  %s %s\n\n",
		titleStyle.Render(thinktI18n.T("tui.discover.approval.title", "Source Approval")),
		m.stepIndicator()))

	b.WriteString(fmt.Sprintf("  %s (%d/%d)\n\n",
		nameStyle.Render(info.Name),
		m.approvalIdx+1,
		len(m.sources),
	))

	b.WriteString(fmt.Sprintf("  %s  %s\n",
		mutedStyle.Render(thinktI18n.T("tui.discover.approval.path", "Path:")),
		bodyStyle.Render(info.BasePath)))

	b.WriteString(fmt.Sprintf("  %s  %d\n",
		mutedStyle.Render(thinktI18n.T("tui.discover.approval.sessions", "Sessions:")),
		info.SessionCount))

	b.WriteString(fmt.Sprintf("  %s  %s\n",
		mutedStyle.Render(thinktI18n.T("tui.discover.approval.size", "Size:")),
		bodyStyle.Render(formatBytes(info.TotalSize))))

	if !info.FirstSession.IsZero() && !info.LastSession.IsZero() {
		b.WriteString(fmt.Sprintf("  %s  %s to %s\n",
			mutedStyle.Render(thinktI18n.T("tui.discover.approval.range", "Range:")),
			bodyStyle.Render(info.FirstSession.Format("Jan 2006")),
			bodyStyle.Render(info.LastSession.Format("Jan 2006")),
		))
	}

	b.WriteString(fmt.Sprintf("\n  %s\n\n  %s\n",
		bodyStyle.Render(thinktI18n.T("tui.discover.approval.prompt", "Enable this source?")),
		m.renderConfirm()))

	return b.String()
}

// --- sourceSummary step ---

func (m Model) updateSourceSummary(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		if msg.String() == "enter" {
			// Persist approved sources
			for _, src := range m.sources {
				m.result.Sources[string(src.Info.Source)] = src.Approved
			}
			m.confirm = true // indexer defaults to Yes
			m.step = stepIndexer
			return m, nil
		}
	}
	return m, nil
}

func (m Model) viewSourceSummary() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))

	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))

	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n  %s %s\n\n",
		titleStyle.Render(thinktI18n.T("tui.discover.summary.title", "Source Summary")),
		m.stepIndicator()))

	var totalSessions int
	var totalSize int64
	approvedCount := 0

	for _, src := range m.sources {
		color := tui.SourceColorHex(src.Info.Source)
		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)

		status := mutedStyle.Render("  skipped")
		if src.Approved {
			status = bodyStyle.Render(fmt.Sprintf("  %d sessions, %s",
				src.Info.SessionCount, formatBytes(src.Info.TotalSize)))
			totalSessions += src.Info.SessionCount
			totalSize += src.Info.TotalSize
			approvedCount++
		}

		b.WriteString(fmt.Sprintf("  %s%s\n",
			nameStyle.Render(fmt.Sprintf("%-18s", src.Info.Name)),
			status))
	}

	if approvedCount > 0 {
		b.WriteString(fmt.Sprintf("\n  %s %d sessions, %s\n",
			titleStyle.Render(thinktI18n.T("tui.discover.summary.total", "Total:")),
			totalSessions,
			formatBytes(totalSize)))
	} else {
		b.WriteString(fmt.Sprintf("\n  %s\n",
			mutedStyle.Render(thinktI18n.T("tui.discover.summary.noneApproved", "No sources enabled."))))
	}

	b.WriteString(fmt.Sprintf("\n  %s\n",
		mutedStyle.Render(thinktI18n.T("tui.discover.summary.continue", "Enter: continue · esc: exit"))))

	return b.String()
}
