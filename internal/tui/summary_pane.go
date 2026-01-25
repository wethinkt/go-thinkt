package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

// summaryModel manages the summary info pane.
type summaryModel struct {
	visible     bool
	width       int
	project     *claude.Project
	session     *claude.Session
	sessionMeta *claude.SessionMeta // Quick info before full session loads
	sessions    []claude.SessionMeta
}

func newSummaryModel() summaryModel {
	return summaryModel{visible: true}
}

func (m *summaryModel) setSize(w int) {
	m.width = w
}

func (m *summaryModel) setProject(project *claude.Project) {
	m.project = project
}

func (m *summaryModel) setSessions(sessions []claude.SessionMeta) {
	m.sessions = sessions
}

func (m *summaryModel) setSession(session *claude.Session) {
	m.session = session
}

func (m *summaryModel) setSessionMeta(meta *claude.SessionMeta) {
	m.sessionMeta = meta
	// Clear full session when switching to new session meta
	if meta != nil {
		m.session = nil
	}
}

func (m *summaryModel) toggle() {
	m.visible = !m.visible
}

func (m *summaryModel) isVisible() bool {
	return m.visible
}

func (m summaryModel) height() int {
	if !m.visible {
		return 0
	}
	return 6 // Fixed height for summary pane
}

func (m summaryModel) view() string {
	if !m.visible {
		return ""
	}

	var sections []string

	// Project section
	if m.project != nil {
		sections = append(sections, m.renderProjectSummary())
	}

	// Session section - prefer full session, fall back to meta
	if m.session != nil {
		sections = append(sections, m.renderSessionSummary())
	} else if m.sessionMeta != nil {
		sections = append(sections, m.renderSessionMetaSummary())
	}

	if len(sections) == 0 {
		return summaryPaneStyle.Width(m.width).Render("Select a project and session to view info")
	}

	content := strings.Join(sections, "  |  ")
	return summaryPaneStyle.Width(m.width).Render(content)
}

func (m summaryModel) renderProjectSummary() string {
	if m.project == nil {
		return ""
	}

	var parts []string
	parts = append(parts, summaryLabelStyle.Render("Project: ")+m.project.DisplayName)
	parts = append(parts, fmt.Sprintf("%d sessions", m.project.SessionCount))

	if !m.project.LastModified.IsZero() {
		parts = append(parts, "last: "+m.project.LastModified.Local().Format("Jan 02 15:04"))
	}

	// Calculate total messages across all sessions
	totalMsgs := 0
	for _, s := range m.sessions {
		totalMsgs += s.MessageCount
	}
	if totalMsgs > 0 {
		parts = append(parts, fmt.Sprintf("%d total msgs", totalMsgs))
	}

	return strings.Join(parts, " | ")
}

func (m summaryModel) renderSessionSummary() string {
	if m.session == nil {
		return ""
	}

	var parts []string

	// Session ID (truncated)
	if m.session.ID != "" {
		id := m.session.ID
		if len(id) > 8 {
			id = id[:8]
		}
		parts = append(parts, summaryLabelStyle.Render("Session: ")+id)
	}

	// Duration
	if !m.session.StartTime.IsZero() && !m.session.EndTime.IsZero() {
		duration := m.session.Duration()
		parts = append(parts, fmt.Sprintf("duration: %s", formatDuration(duration)))
	}

	// Turn count
	turnCount := m.session.TurnCount()
	if turnCount > 0 {
		parts = append(parts, fmt.Sprintf("%d turns", turnCount))
	}

	// Entry count
	parts = append(parts, fmt.Sprintf("%d entries", len(m.session.Entries)))

	// Model
	if m.session.Model != "" {
		model := m.session.Model
		// Shorten common model names
		if strings.Contains(model, "opus") {
			model = "opus"
		} else if strings.Contains(model, "sonnet") {
			model = "sonnet"
		} else if strings.Contains(model, "haiku") {
			model = "haiku"
		}
		parts = append(parts, summaryLabelStyle.Render("model: ")+model)
	}

	// Git branch
	if m.session.Branch != "" {
		branch := m.session.Branch
		if len(branch) > 20 {
			branch = branch[:20] + "..."
		}
		parts = append(parts, summaryLabelStyle.Render("branch: ")+branch)
	}

	// Count thinking blocks
	thinkingCount := 0
	toolCallCount := 0
	for _, e := range m.session.Entries {
		thinkingCount += len(e.GetThinkingBlocks())
		toolCallCount += len(e.GetToolCalls())
	}
	if thinkingCount > 0 {
		parts = append(parts, fmt.Sprintf("%d thinking", thinkingCount))
	}
	if toolCallCount > 0 {
		parts = append(parts, fmt.Sprintf("%d tools", toolCallCount))
	}

	return strings.Join(parts, " | ")
}

func (m summaryModel) renderSessionMetaSummary() string {
	if m.sessionMeta == nil {
		return ""
	}

	var parts []string

	// Session ID (truncated)
	if m.sessionMeta.SessionID != "" {
		id := m.sessionMeta.SessionID
		if len(id) > 8 {
			id = id[:8]
		}
		parts = append(parts, summaryLabelStyle.Render("Session: ")+id)
	}

	// First prompt (truncated)
	if m.sessionMeta.FirstPrompt != "" {
		prompt := m.sessionMeta.FirstPrompt
		if len(prompt) > 40 {
			prompt = prompt[:40] + "..."
		}
		parts = append(parts, fmt.Sprintf("\"%s\"", prompt))
	}

	// Message count
	if m.sessionMeta.MessageCount > 0 {
		parts = append(parts, fmt.Sprintf("%d msgs", m.sessionMeta.MessageCount))
	}

	// Created time
	if !m.sessionMeta.Created.IsZero() {
		parts = append(parts, m.sessionMeta.Created.Local().Format("Jan 02 15:04"))
	}

	// Git branch
	if m.sessionMeta.GitBranch != "" {
		branch := m.sessionMeta.GitBranch
		if len(branch) > 15 {
			branch = branch[:15] + "..."
		}
		parts = append(parts, summaryLabelStyle.Render("branch: ")+branch)
	}

	// Hint to load full session
	parts = append(parts, "(Enter for details)")

	return strings.Join(parts, " | ")
}

func formatDuration(d interface{ Seconds() float64 }) string {
	secs := d.Seconds()
	if secs < 60 {
		return fmt.Sprintf("%.0fs", secs)
	}
	mins := int(secs) / 60
	if mins < 60 {
		return fmt.Sprintf("%dm %ds", mins, int(secs)%60)
	}
	hours := mins / 60
	return fmt.Sprintf("%dh %dm", hours, mins%60)
}

// Summary pane styles
var (
	summaryPaneStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#1a1a2e")).
				Foreground(lipgloss.Color("#e0e0e0")).
				Padding(0, 1).
				MarginBottom(0)

	summaryLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#7D56F4"))
)
