package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/agents"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// AgentsPageResult is returned when the agents page is dismissed.
type AgentsPageResult struct {
	Cancelled bool
	Selected  *agents.UnifiedAgent
}

// agentsFilterMode cycles through filter states.
type agentsFilterMode int

const (
	agentsFilterAll agentsFilterMode = iota
	agentsFilterLocal
	agentsFilterRemote
)

func (m agentsFilterMode) String() string {
	switch m {
	case agentsFilterLocal:
		return "local"
	case agentsFilterRemote:
		return "remote"
	default:
		return "all"
	}
}

// AgentsPageModel shows a list of active agents from the hub.
type AgentsPageModel struct {
	hub           *agents.AgentHub
	width, height int
	viewport      viewport.Model
	ready         bool
	filter        agentsFilterMode
	agentList     []agents.UnifiedAgent
	selected      int
}

// NewAgentsPageModel creates an agents list page.
func NewAgentsPageModel(hub *agents.AgentHub) AgentsPageModel {
	return AgentsPageModel{
		hub: hub,
	}
}

// agentsRefreshMsg triggers a periodic refresh.
type agentsRefreshMsg struct{}

func (m AgentsPageModel) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), agentsTickCmd())
}

func (m AgentsPageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keys := agentsPageKeyMap()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 2
		contentWidth := msg.Width - 4
		contentHeight := msg.Height - headerHeight - 4
		if !m.ready {
			m.viewport = viewport.New()
			m.viewport.SetWidth(contentWidth)
			m.viewport.SetHeight(contentHeight)
			m.ready = true
		} else {
			m.viewport.SetWidth(contentWidth)
			m.viewport.SetHeight(contentHeight)
		}
		m.viewport.SetContent(m.renderContent())
		return m, nil

	case agentsRefreshMsg:
		m.refreshAgents()
		if m.ready {
			m.viewport.SetContent(m.renderContent())
		}
		return m, agentsTickCmd()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Back):
			return m, func() tea.Msg { return AgentsPageResult{Cancelled: true} }
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Filter):
			m.filter = (m.filter + 1) % 3
			m.refreshAgents()
			if m.ready {
				m.viewport.SetContent(m.renderContent())
			}
			return m, nil
		case key.Matches(msg, keys.Up):
			if m.selected > 0 {
				m.selected--
				if m.ready {
					m.viewport.SetContent(m.renderContent())
				}
			}
			return m, nil
		case key.Matches(msg, keys.Down):
			if m.selected < len(m.agentList)-1 {
				m.selected++
				if m.ready {
					m.viewport.SetContent(m.renderContent())
				}
			}
			return m, nil
		case key.Matches(msg, keys.Enter):
			if m.selected >= 0 && m.selected < len(m.agentList) {
				a := m.agentList[m.selected]
				return m, func() tea.Msg { return AgentsPageResult{Selected: &a} }
			}
			return m, nil
		case key.Matches(msg, keys.Refresh):
			return m, m.refreshCmd()
		}
	}

	if m.ready {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m AgentsPageModel) View() tea.View {
	if !m.ready {
		v := tea.NewView("Loading agents...")
		v.AltScreen = true
		return v
	}

	t := theme.Current()
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.GetAccent()))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	padStyle := lipgloss.NewStyle().Padding(1, 2)

	title := titleStyle.Render(fmt.Sprintf("Agents [%s]", m.filter))

	help := helpStyle.Render("f: filter  enter: tail  r: refresh  esc: back  q: quit  j/k: scroll")

	content := title + "\n" + m.viewport.View() + "\n" + help
	v := tea.NewView(padStyle.Render(content))
	v.AltScreen = true
	return v
}

func (m *AgentsPageModel) refreshAgents() {
	if m.hub == nil {
		return
	}
	filter := agents.AgentFilter{}
	switch m.filter {
	case agentsFilterLocal:
		filter.LocalOnly = true
	case agentsFilterRemote:
		filter.RemoteOnly = true
	}
	m.agentList = m.hub.List(filter)
	if m.selected >= len(m.agentList) {
		m.selected = max(0, len(m.agentList)-1)
	}
}

func (m AgentsPageModel) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		if m.hub != nil {
			m.hub.PollOnce(context.Background())
		}
		return agentsRefreshMsg{}
	}
}

func (m AgentsPageModel) renderContent() string {
	t := theme.Current()
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.TextSecondary.Fg))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7fcc5a"))
	staleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b"))
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.GetAccent()))

	if len(m.agentList) == 0 {
		return mutedStyle.Render("No active agents found.")
	}

	var b strings.Builder

	// Header row
	b.WriteString(headerStyle.Render(fmt.Sprintf("  %-8s  %-8s  %-30s  %-10s  %-15s  %s", "STATUS", "SOURCE", "PROJECT", "SESSION", "MACHINE", "AGE")))
	b.WriteString("\n")

	for i, a := range m.agentList {
		sessionID := a.SessionID
		if len(sessionID) > 8 {
			sessionID = sessionID[:8]
		}
		project := shortenPath(a.ProjectPath)
		if len(project) > 30 {
			project = "..." + project[len(project)-27:]
		}
		age := time.Since(a.DetectedAt).Truncate(time.Second).String()
		machine := a.MachineName
		if machine == "" {
			machine = a.Hostname
		}
		if len(machine) > 15 {
			machine = machine[:15]
		}

		statusStr := activeStyle.Render(a.Status)
		if a.Status == "stale" {
			statusStr = staleStyle.Render(a.Status)
		}

		cursor := "  "
		lineStyle := mutedStyle
		if i == m.selected {
			cursor = "> "
			lineStyle = selectedStyle
		}

		line := fmt.Sprintf("%s%-8s  %-8s  %-30s  %-10s  %-15s  %s",
			cursor, a.Status, a.Source, project, sessionID, machine, age)

		if i == m.selected {
			b.WriteString(lineStyle.Render(line))
		} else {
			// Render status colored, rest muted
			b.WriteString(fmt.Sprintf("%s%s  %s  %s  %s  %s  %s",
				cursor, statusStr,
				mutedStyle.Render(fmt.Sprintf("%-8s", a.Source)),
				mutedStyle.Render(fmt.Sprintf("%-30s", project)),
				mutedStyle.Render(fmt.Sprintf("%-10s", sessionID)),
				mutedStyle.Render(fmt.Sprintf("%-15s", machine)),
				mutedStyle.Render(age)))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func agentsTickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return agentsRefreshMsg{}
	})
}

type agentsKeyMap struct {
	Back    key.Binding
	Quit    key.Binding
	Filter  key.Binding
	Up      key.Binding
	Down    key.Binding
	Enter   key.Binding
	Refresh key.Binding
}

func agentsPageKeyMap() agentsKeyMap {
	return agentsKeyMap{
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Filter: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "filter"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("j", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "tail"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
	}
}
