package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/agents"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// AgentTailResult is returned when the tail page is dismissed.
type AgentTailResult struct {
	Cancelled bool
}

// agentStreamEntryMsg delivers a stream entry to the TUI.
type agentStreamEntryMsg struct {
	Entry agents.StreamEntry
	Ch    <-chan agents.StreamEntry // pass channel back for next read
}

// agentStreamErrorMsg signals a stream error.
type agentStreamErrorMsg struct {
	Err error
}

// agentStreamStartedMsg is sent once the stream channel is established.
type agentStreamStartedMsg struct {
	Ch <-chan agents.StreamEntry
}

// AgentTailModel shows a live stream of an agent's conversation.
type AgentTailModel struct {
	hub           *agents.AgentHub
	agent         agents.UnifiedAgent
	width, height int
	viewport      viewport.Model
	spinner       spinner.Model
	filters       RoleFilterSet
	keys          tailKeyBindings
	ready         bool
	entries       []agents.StreamEntry
	autoScroll    bool
	connected     bool
	streamErr     error
	cancel        context.CancelFunc
	flashTicks    int // counts down on spinner ticks; >0 means show new-data indicator
}

// NewAgentTailModel creates a tail page for the given agent.
func NewAgentTailModel(hub *agents.AgentHub, agent agents.UnifiedAgent) AgentTailModel {
	t := theme.Current()
	s := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent()))),
	)
	return AgentTailModel{
		hub:        hub,
		agent:      agent,
		spinner:    s,
		filters:    NewRoleFilterSet(),
		keys:       tailKeys(),
		autoScroll: true,
		connected:  true,
	}
}

func (m AgentTailModel) Init() tea.Cmd {
	return tea.Batch(m.connectStream(), m.spinner.Tick)
}

func (m AgentTailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 3
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
		m.viewport.SetContent(m.renderEntries())
		if m.autoScroll {
			m.viewport.GotoBottom()
		}
		return m, nil

	case agentStreamStartedMsg:
		m.connected = true
		return m, waitForStreamEntry(msg.Ch)

	case agentStreamEntryMsg:
		m.entries = append(m.entries, msg.Entry)
		m.connected = true
		m.flashTicks = 4
		if m.ready {
			m.viewport.SetContent(m.renderEntries())
			if m.autoScroll {
				m.viewport.GotoBottom()
			}
		}
		return m, waitForStreamEntry(msg.Ch)

	case agentStreamErrorMsg:
		m.streamErr = msg.Err
		m.connected = false
		if m.ready {
			m.viewport.SetContent(m.renderEntries())
			if m.autoScroll {
				m.viewport.GotoBottom()
			}
		}
		return m, nil

	case spinner.TickMsg:
		if m.flashTicks > 0 {
			m.flashTicks--
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Back):
			if m.cancel != nil {
				m.cancel()
			}
			return m, func() tea.Msg { return AgentTailResult{Cancelled: true} }
		case key.Matches(msg, m.keys.Quit):
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case key.Matches(msg, m.keys.Bottom):
			m.autoScroll = true
			if m.ready {
				m.viewport.GotoBottom()
			}
			return m, nil
		case key.Matches(msg, m.keys.ToggleInput):
			m.filters.User = !m.filters.User
			return m, m.refreshViewport()
		case key.Matches(msg, m.keys.ToggleOutput):
			m.filters.Assistant = !m.filters.Assistant
			return m, m.refreshViewport()
		case key.Matches(msg, m.keys.ToggleTools):
			m.filters.Tools = !m.filters.Tools
			return m, m.refreshViewport()
		case key.Matches(msg, m.keys.ToggleThinking):
			m.filters.Thinking = !m.filters.Thinking
			return m, m.refreshViewport()
		case key.Matches(msg, m.keys.ToggleOther):
			m.filters.Other = !m.filters.Other
			return m, m.refreshViewport()
		}
	}

	if m.ready {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		m.autoScroll = m.viewport.AtBottom()
		return m, cmd
	}
	return m, nil
}

func (m *AgentTailModel) refreshViewport() tea.Cmd {
	if m.ready {
		m.viewport.SetContent(m.renderEntries())
		if m.autoScroll {
			m.viewport.GotoBottom()
		}
	}
	return nil
}

func (m AgentTailModel) View() tea.View {
	if !m.ready {
		v := tea.NewView(m.spinner.View() + " Connecting to agent stream...")
		v.AltScreen = true
		return v
	}

	t := theme.Current()
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.GetAccent()))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	padStyle := lipgloss.NewStyle().Padding(1, 2)
	connectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7fcc5a"))
	disconnectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b"))

	// Header
	sessionID := m.agent.SessionID
	if len(sessionID) > 8 {
		sessionID = sessionID[:8]
	}
	title := titleStyle.Render(fmt.Sprintf("Agent Tail: %s", sessionID))
	info := mutedStyle.Render(fmt.Sprintf(" %s  %s", m.agent.Source, shortenPath(m.agent.ProjectPath)))

	var status string
	if !m.connected {
		status = disconnectedStyle.Render("disconnected")
	} else if m.flashTicks > 0 {
		status = m.spinner.View() + " " + connectedStyle.Render("new data")
	} else {
		status = m.spinner.View() + " " + connectedStyle.Render("live")
	}

	header := title + info + "  " + status

	filterStatus := m.renderFilterStatus()
	help := helpStyle.Render("G/end: resume scroll  esc: back  q: quit  j/k: scroll")

	content := header + "\n" + filterStatus + "\n" + m.viewport.View() + "\n" + help
	v := tea.NewView(padStyle.Render(content))
	v.AltScreen = true
	return v
}

// connectStream starts the stream and returns the channel in a message.
func (m *AgentTailModel) connectStream() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel

		ch, err := m.hub.Stream(ctx, m.agent.SessionID, 50)
		if err != nil {
			return agentStreamErrorMsg{Err: err}
		}

		return agentStreamStartedMsg{Ch: ch}
	}
}

// waitForStreamEntry returns a command that blocks until the next entry arrives.
func waitForStreamEntry(ch <-chan agents.StreamEntry) tea.Cmd {
	return func() tea.Msg {
		entry, ok := <-ch
		if !ok {
			return agentStreamErrorMsg{Err: fmt.Errorf("stream closed")}
		}
		return agentStreamEntryMsg{Entry: entry, Ch: ch}
	}
}

func (m AgentTailModel) renderEntries() string {
	s := GetStyles()

	if len(m.entries) == 0 {
		systemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current().TextMuted.Fg)).Italic(true)
		return systemStyle.Render("Waiting for entries...")
	}

	contentWidth := max(20, m.width-8)
	var b strings.Builder
	for _, e := range m.entries {
		te := e.ToThinktEntry()
		rendered := RenderThinktEntry(&te, contentWidth, &m.filters)
		if rendered != "" {
			b.WriteString(rendered)
			b.WriteString("\n")
		}
	}

	if m.streamErr != nil {
		label := s.ThinkingLabel.Render("Error")
		content := s.ThinkingBlock.Width(contentWidth).Render(m.streamErr.Error())
		b.WriteString(label + "\n" + content + "\n")
	}

	return b.String()
}

func (m AgentTailModel) renderFilterStatus() string {
	type filterItem struct {
		key   string
		label string
		on    bool
	}
	items := []filterItem{
		{"1", thinktI18n.T("tui.filter.user", "User"), m.filters.User},
		{"2", thinktI18n.T("tui.filter.assistant", "Assistant"), m.filters.Assistant},
		{"3", thinktI18n.T("tui.filter.tools", "Tools"), m.filters.Tools},
		{"4", thinktI18n.T("tui.filter.thinking", "Thinking"), m.filters.Thinking},
		{"5", thinktI18n.T("tui.filter.other", "Other"), m.filters.Other},
	}

	active := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)

	var parts []string
	for _, it := range items {
		label := fmt.Sprintf("%s:%s", it.key, it.label)
		if it.on {
			parts = append(parts, active.Render(label))
		} else {
			parts = append(parts, dim.Render(label))
		}
	}
	return strings.Join(parts, " ")
}

type tailKeyBindings struct {
	Back           key.Binding
	Quit           key.Binding
	Bottom         key.Binding
	Up             key.Binding
	PageUp         key.Binding
	ToggleInput    key.Binding
	ToggleOutput   key.Binding
	ToggleTools    key.Binding
	ToggleThinking key.Binding
	ToggleOther    key.Binding
}

func tailKeys() tailKeyBindings {
	return tailKeyBindings{
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G", "end"),
			key.WithHelp("G", "resume scroll"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("k", "up"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		ToggleInput: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "toggle input"),
		),
		ToggleOutput: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "toggle output"),
		),
		ToggleTools: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "toggle tools"),
		),
		ToggleThinking: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "toggle thinking"),
		),
		ToggleOther: key.NewBinding(
			key.WithKeys("5"),
			key.WithHelp("5", "toggle other"),
		),
	}
}
