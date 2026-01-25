package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

// headerModel manages the fixed two-line header.
type headerModel struct {
	width       int
	project     *claude.Project
	sessionMeta *claude.SessionMeta
	session     *claude.Session
	sessions    []claude.SessionMeta
}

func newHeaderModel() headerModel {
	return headerModel{}
}

func (m *headerModel) setWidth(w int) {
	m.width = w
}

func (m *headerModel) setProject(project *claude.Project) {
	m.project = project
}

func (m *headerModel) setSessions(sessions []claude.SessionMeta) {
	m.sessions = sessions
}

func (m *headerModel) setSessionMeta(meta *claude.SessionMeta) {
	m.sessionMeta = meta
}

func (m *headerModel) setSession(session *claude.Session) {
	m.session = session
}

func (m headerModel) height() int {
	return 2 // Fixed two-line header
}

func (m headerModel) view() string {
	if m.width < 20 {
		return ""
	}

	line1 := m.renderProjectLine()
	line2 := m.renderSessionLine()

	// Combine into a single borderless box at full terminal width
	content := line1 + "\n" + line2
	return headerBoxStyle.Width(m.width).Render(content)
}

func (m headerModel) renderProjectLine() string {
	brand := headerBrandStyle.Render("🧠 thinkt")
	brandWidth := lipgloss.Width(brand)

	// Calculate available width for project info (account for brand on right)
	availWidth := max(10, m.width-brandWidth-2)

	var projectInfo string
	if m.project != nil {
		// Build project info parts
		parts := []string{m.project.DisplayName}

		if m.project.SessionCount > 0 {
			parts = append(parts, fmt.Sprintf("%d sessions", m.project.SessionCount))
		}

		// Total messages across sessions
		totalMsgs := 0
		for _, s := range m.sessions {
			totalMsgs += s.MessageCount
		}
		if totalMsgs > 0 {
			parts = append(parts, fmt.Sprintf("%d msgs", totalMsgs))
		}

		if !m.project.LastModified.IsZero() {
			parts = append(parts, "last: "+m.project.LastModified.Local().Format("Jan 02 15:04"))
		}

		projectInfo = headerLabelStyle.Render("Project: ") + strings.Join(parts, " | ")
	} else {
		projectInfo = headerDimStyle.Render("No project selected")
	}

	// Truncate if needed
	projectInfo = truncateWithWidth(projectInfo, availWidth)

	// Pad to fill width, placing brand on far right
	infoWidth := lipgloss.Width(projectInfo)
	padding := max(1, m.width-infoWidth-brandWidth)

	return projectInfo + strings.Repeat(" ", padding) + brand
}

func (m headerModel) renderSessionLine() string {
	var sessionInfo string

	if m.session != nil {
		sessionInfo = m.renderFullSessionInfo()
	} else if m.sessionMeta != nil {
		sessionInfo = m.renderMetaSessionInfo()
	} else {
		sessionInfo = headerDimStyle.Render("No session selected")
	}

	// Truncate to fit width
	sessionInfo = truncateWithWidth(sessionInfo, m.width-2)

	// Pad to fill full width
	infoWidth := lipgloss.Width(sessionInfo)
	padding := max(0, m.width-infoWidth)

	return sessionInfo + strings.Repeat(" ", padding)
}

func (m headerModel) renderFullSessionInfo() string {
	s := m.session
	var parts []string

	// Session ID
	if s.ID != "" {
		id := s.ID
		if len(id) > 8 {
			id = id[:8]
		}
		parts = append(parts, headerLabelStyle.Render("Session: ")+id)
	}

	// Duration
	if !s.StartTime.IsZero() && !s.EndTime.IsZero() {
		parts = append(parts, fmt.Sprintf("duration: %s", formatSessionDuration(s.Duration())))
	}

	// Turns and entries
	parts = append(parts, fmt.Sprintf("%d turns, %d entries", s.TurnCount(), len(s.Entries)))

	// Model
	if s.Model != "" {
		model := s.Model
		if strings.Contains(model, "opus") {
			model = "opus"
		} else if strings.Contains(model, "sonnet") {
			model = "sonnet"
		} else if strings.Contains(model, "haiku") {
			model = "haiku"
		}
		parts = append(parts, headerLabelStyle.Render("model: ")+model)
	}

	// Branch
	if s.Branch != "" {
		branch := s.Branch
		if len(branch) > 15 {
			branch = branch[:15] + "..."
		}
		parts = append(parts, headerLabelStyle.Render("branch: ")+branch)
	}

	return strings.Join(parts, " | ")
}

func (m headerModel) renderMetaSessionInfo() string {
	meta := m.sessionMeta
	var parts []string

	// Session ID
	if meta.SessionID != "" {
		id := meta.SessionID
		if len(id) > 8 {
			id = id[:8]
		}
		parts = append(parts, headerLabelStyle.Render("Session: ")+id)
	}

	// First prompt preview
	if meta.FirstPrompt != "" {
		prompt := meta.FirstPrompt
		if len(prompt) > 30 {
			prompt = prompt[:30] + "..."
		}
		parts = append(parts, fmt.Sprintf("\"%s\"", prompt))
	}

	// Message count
	if meta.MessageCount > 0 {
		parts = append(parts, fmt.Sprintf("%d msgs", meta.MessageCount))
	}

	// Created time
	if !meta.Created.IsZero() {
		parts = append(parts, meta.Created.Local().Format("Jan 02 15:04"))
	}

	return strings.Join(parts, " | ")
}

func formatSessionDuration(d interface{ Seconds() float64 }) string {
	secs := d.Seconds()
	if secs < 60 {
		return fmt.Sprintf("%.0fs", secs)
	}
	mins := int(secs) / 60
	if mins < 60 {
		return fmt.Sprintf("%dm%ds", mins, int(secs)%60)
	}
	hours := mins / 60
	return fmt.Sprintf("%dh%dm", hours, mins%60)
}

// truncateWithWidth truncates a string to fit within maxWidth, accounting for ANSI codes.
func truncateWithWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	width := lipgloss.Width(s)
	if width <= maxWidth {
		return s
	}
	// Simple truncation - strip from end until it fits
	runes := []rune(s)
	for len(runes) > 0 {
		s = string(runes)
		if lipgloss.Width(s) <= maxWidth-3 {
			return s + "..."
		}
		runes = runes[:len(runes)-1]
	}
	return "..."
}

// Header styles
var (
	// headerBoxStyle wraps the entire header in a borderless box
	headerBoxStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#2d2d44")).
			Foreground(lipgloss.Color("#e0e0e0")).
			Padding(0)

	headerLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#9d7aff"))

	headerBrandStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#9d7aff"))

	headerDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))
)
