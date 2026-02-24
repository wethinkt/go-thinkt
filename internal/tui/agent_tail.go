package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/agents"
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
	ready         bool
	entries       []agents.StreamEntry
	autoScroll    bool
	connected     bool
	streamErr     error
	cancel        context.CancelFunc
}

// NewAgentTailModel creates a tail page for the given agent.
func NewAgentTailModel(hub *agents.AgentHub, agent agents.UnifiedAgent) AgentTailModel {
	return AgentTailModel{
		hub:        hub,
		agent:      agent,
		autoScroll: true,
		connected:  true,
	}
}

func (m AgentTailModel) Init() tea.Cmd {
	return m.connectStream()
}

func (m AgentTailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keys := tailKeys()

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

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Back):
			if m.cancel != nil {
				m.cancel()
			}
			return m, func() tea.Msg { return AgentTailResult{Cancelled: true} }
		case key.Matches(msg, keys.Quit):
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case key.Matches(msg, keys.Bottom):
			m.autoScroll = true
			if m.ready {
				m.viewport.GotoBottom()
			}
			return m, nil
		case key.Matches(msg, keys.Up):
			m.autoScroll = false
		case key.Matches(msg, keys.PageUp):
			m.autoScroll = false
		}
	}

	if m.ready {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m AgentTailModel) View() tea.View {
	if !m.ready {
		v := tea.NewView("Connecting to agent stream...")
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

	connStatus := connectedStyle.Render("connected")
	if !m.connected {
		connStatus = disconnectedStyle.Render("disconnected")
	}

	header := title + info + "  " + connStatus

	help := helpStyle.Render("G/end: resume scroll  esc: back  q: quit  j/k: scroll")

	content := header + "\n" + m.viewport.View() + "\n" + help
	v := tea.NewView(padStyle.Render(content))
	v.AltScreen = true
	return v
}

// connectStream starts the stream and returns the channel in a message.
func (m *AgentTailModel) connectStream() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel

		ch, err := m.hub.Stream(ctx, m.agent.SessionID)
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
	t := theme.Current()
	userStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5a9fd4"))
	assistantStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7fcc5a"))
	systemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)).Italic(true)
	toolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#d4a55a"))
	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))

	if len(m.entries) == 0 {
		return systemStyle.Render("Waiting for entries...")
	}

	var b strings.Builder
	for _, e := range m.entries {
		ts := timeStyle.Render(e.Timestamp.Format("15:04:05"))
		switch e.Role {
		case "user":
			fmt.Fprintf(&b, "\n%s %s\n%s\n", userStyle.Render("[user]"), ts, e.Text)
		case "assistant":
			model := ""
			if e.Model != "" {
				model = " " + timeStyle.Render(e.Model)
			}
			fmt.Fprintf(&b, "\n%s%s %s\n%s\n", assistantStyle.Render("[assistant]"), model, ts, e.Text)
		case "system":
			fmt.Fprintf(&b, "\n%s\n", systemStyle.Render("--- "+e.Text+" ---"))
		default:
			if e.ToolName != "" {
				fmt.Fprintf(&b, "\n%s %s %s\n", toolStyle.Render("["+e.Role+"]"), toolStyle.Render(e.ToolName), ts)
			} else {
				fmt.Fprintf(&b, "\n%s %s\n%s\n", toolStyle.Render("["+e.Role+"]"), ts, e.Text)
			}
		}
	}

	if m.streamErr != nil {
		fmt.Fprintf(&b, "\n%s\n", systemStyle.Render("--- Error: "+m.streamErr.Error()+" ---"))
	}

	return b.String()
}

type tailKeyBindings struct {
	Back   key.Binding
	Quit   key.Binding
	Bottom key.Binding
	Up     key.Binding
	PageUp key.Binding
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
	}
}
