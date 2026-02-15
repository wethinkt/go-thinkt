package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/collect"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// CollectorPageResult is returned when the collector page is dismissed.
type CollectorPageResult struct {
	Cancelled bool
}

// CollectorPageModel shows collector server status by querying its REST API.
type CollectorPageModel struct {
	width, height int
	stats         *collect.CollectorStats
	agents        []collect.AgentInfo
	sessions      []collect.SessionSummary
	err           error
	loading       bool
	collectorURL  string
	viewport      viewport.Model
	ready         bool
}

// NewCollectorPageModel creates a collector status page for the given collector URL.
func NewCollectorPageModel(collectorURL string) CollectorPageModel {
	return CollectorPageModel{
		collectorURL: collectorURL,
		loading:      true,
	}
}

// collectorDataMsg holds the result of fetching collector data.
type collectorDataMsg struct {
	stats    *collect.CollectorStats
	agents   []collect.AgentInfo
	sessions []collect.SessionSummary
	err      error
}

// collectorTickMsg triggers a periodic refresh.
type collectorTickMsg struct{}

func (m CollectorPageModel) Init() tea.Cmd {
	return tea.Batch(fetchCollectorData(m.collectorURL), collectorTick())
}

func (m CollectorPageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keys := collectorPageKeyMap()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 2 // padding
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

	case collectorDataMsg:
		m.loading = false
		m.err = msg.err
		m.stats = msg.stats
		m.agents = msg.agents
		m.sessions = msg.sessions
		if m.ready {
			m.viewport.SetContent(m.renderContent())
		}
		return m, nil

	case collectorTickMsg:
		return m, tea.Batch(fetchCollectorData(m.collectorURL), collectorTick())

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Back):
			return m, func() tea.Msg { return CollectorPageResult{Cancelled: true} }
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Refresh):
			m.loading = true
			return m, fetchCollectorData(m.collectorURL)
		}
	}

	if m.ready {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m CollectorPageModel) View() tea.View {
	if !m.ready {
		v := tea.NewView("Loading collector status...")
		v.AltScreen = true
		return v
	}

	t := theme.Current()
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.GetAccent()))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	padStyle := lipgloss.NewStyle().Padding(1, 2)

	title := titleStyle.Render("Collector Status")
	if m.loading {
		title += " (refreshing...)"
	}

	help := helpStyle.Render("r: refresh  esc: back  q: quit  j/k: scroll")

	content := title + "\n" + m.viewport.View() + "\n" + help
	v := tea.NewView(padStyle.Render(content))
	v.AltScreen = true
	return v
}

// renderContent builds the scrollable content string.
func (m CollectorPageModel) renderContent() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v", m.err)
	}
	if m.stats == nil {
		return "No data yet."
	}

	t := theme.Current()
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.TextPrimary.Fg))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)).Width(20)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7fcc5a"))
	staleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b"))

	var b strings.Builder

	// Server info
	b.WriteString(headerStyle.Render("Server"))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("URL") + valueStyle.Render(m.collectorURL) + "\n")
	b.WriteString(labelStyle.Render("Status") + activeStyle.Render("running") + "\n")
	uptime := time.Duration(m.stats.UptimeSeconds * float64(time.Second))
	b.WriteString(labelStyle.Render("Uptime") + valueStyle.Render(formatDuration(uptime)) + "\n")
	b.WriteString(labelStyle.Render("Started") + valueStyle.Render(m.stats.StartedAt.Format("2006-01-02 15:04:05")) + "\n")
	b.WriteString("\n")

	// Stats
	b.WriteString(headerStyle.Render("Statistics"))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Total Traces") + valueStyle.Render(fmt.Sprintf("%d", m.stats.TotalTraces)) + "\n")
	b.WriteString(labelStyle.Render("Total Sessions") + valueStyle.Render(fmt.Sprintf("%d", m.stats.TotalSessions)) + "\n")
	b.WriteString(labelStyle.Render("Active Agents") + valueStyle.Render(fmt.Sprintf("%d / %d", m.stats.ActiveAgents, m.stats.TotalAgents)) + "\n")
	b.WriteString(labelStyle.Render("DB Size") + valueStyle.Render(formatFileSize(m.stats.DBSizeBytes)) + "\n")
	b.WriteString("\n")

	// Agents
	b.WriteString(headerStyle.Render(fmt.Sprintf("Agents (%d)", len(m.agents))))
	b.WriteString("\n")
	if len(m.agents) == 0 {
		b.WriteString(mutedStyle.Render("  No agents registered."))
		b.WriteString("\n")
	} else {
		for _, a := range m.agents {
			status := activeStyle.Render(a.Status)
			if a.Status == "stale" {
				status = staleStyle.Render(a.Status)
			}
			id := a.InstanceID
			if len(id) > 12 {
				id = id[:12]
			}
			heartbeat := relativeDate(a.LastHeartbeat)
			b.WriteString(fmt.Sprintf("  %s  %s  %s  traces:%d  %s  %s\n",
				status,
				valueStyle.Render(id),
				mutedStyle.Render(a.Platform),
				a.TraceCount,
				mutedStyle.Render(a.Hostname),
				mutedStyle.Render(heartbeat),
			))
		}
	}
	b.WriteString("\n")

	// Sessions
	b.WriteString(headerStyle.Render(fmt.Sprintf("Recent Sessions (%d)", len(m.sessions))))
	b.WriteString("\n")
	if len(m.sessions) == 0 {
		b.WriteString(mutedStyle.Render("  No sessions collected."))
		b.WriteString("\n")
	} else {
		for _, s := range m.sessions {
			id := s.ID
			if len(id) > 8 {
				id = id[:8]
			}
			updated := relativeDate(s.LastUpdated)
			b.WriteString(fmt.Sprintf("  %s  %s  entries:%d  %s  %s\n",
				valueStyle.Render(id),
				mutedStyle.Render(s.Source),
				s.EntryCount,
				mutedStyle.Render(shortenPath(s.ProjectPath)),
				mutedStyle.Render(updated),
			))
		}
	}

	return b.String()
}

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

func fetchCollectorData(baseURL string) tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 5 * time.Second}

		// Fetch stats
		stats, err := fetchJSON[collect.CollectorStats](client, baseURL+"/v1/traces/stats")
		if err != nil {
			return collectorDataMsg{err: fmt.Errorf("fetch stats: %w", err)}
		}

		// Fetch agents
		agents, err := fetchJSONSlice[collect.AgentInfo](client, baseURL+"/v1/agents")
		if err != nil {
			agents = nil // non-fatal
		}

		// Fetch sessions via stats (the collector may not expose a sessions list endpoint yet)
		// For now we leave sessions empty; they can be added when the endpoint exists.
		var sessions []collect.SessionSummary

		return collectorDataMsg{
			stats:    stats,
			agents:   agents,
			sessions: sessions,
		}
	}
}

func collectorTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return collectorTickMsg{}
	})
}

// fetchJSON performs a GET and decodes a single JSON object.
func fetchJSON[T any](client *http.Client, url string) (*T, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// fetchJSONSlice performs a GET and decodes a JSON array.
func fetchJSONSlice[T any](client *http.Client, url string) ([]T, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result []T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

type collectorKeyMap struct {
	Back    key.Binding
	Quit    key.Binding
	Refresh key.Binding
}

func collectorPageKeyMap() collectorKeyMap {
	return collectorKeyMap{
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
	}
}
