package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/export"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// ExporterPageResult is returned when the exporter page is dismissed.
type ExporterPageResult struct {
	Cancelled bool
}

// ExporterPageModel shows exporter status and configuration.
type ExporterPageModel struct {
	width, height int
	stats         export.ExporterStats
	err           error
	loading       bool
	viewport      viewport.Model
	ready         bool
}

// NewExporterPageModel creates an exporter status page with the given stats.
func NewExporterPageModel(stats export.ExporterStats) ExporterPageModel {
	return ExporterPageModel{
		stats: stats,
	}
}

func (m ExporterPageModel) Init() tea.Cmd {
	return nil
}

func (m ExporterPageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keys := exporterPageKeyMap()

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

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Back):
			return m, func() tea.Msg { return ExporterPageResult{Cancelled: true} }
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		}
	}

	if m.ready {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m ExporterPageModel) View() tea.View {
	if !m.ready {
		v := tea.NewView("Loading exporter status...")
		v.AltScreen = true
		return v
	}

	t := theme.Current()
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.GetAccent()))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	padStyle := lipgloss.NewStyle().Padding(1, 2)

	title := titleStyle.Render("Exporter Status")
	help := helpStyle.Render("esc: back  q: quit  j/k: scroll")

	content := title + "\n" + m.viewport.View() + "\n" + help
	v := tea.NewView(padStyle.Render(content))
	v.AltScreen = true
	return v
}

// renderContent builds the scrollable content string.
func (m ExporterPageModel) renderContent() string {
	t := theme.Current()
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.TextPrimary.Fg))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)).Width(20)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7fcc5a"))

	var b strings.Builder

	// Connection
	b.WriteString(headerStyle.Render("Connection"))
	b.WriteString("\n")
	collectorURL := m.stats.CollectorURL
	if collectorURL == "" {
		collectorURL = mutedStyle.Render("(not configured)")
	}
	b.WriteString(labelStyle.Render("Collector URL") + valueStyle.Render(collectorURL) + "\n")
	b.WriteString(labelStyle.Render("Status") + activeStyle.Render("connected") + "\n")
	b.WriteString("\n")

	// Watched directories
	b.WriteString(headerStyle.Render(fmt.Sprintf("Watched Directories (%d)", len(m.stats.Watching))))
	b.WriteString("\n")
	if len(m.stats.Watching) == 0 {
		b.WriteString(mutedStyle.Render("  None."))
		b.WriteString("\n")
	} else {
		for _, wd := range m.stats.Watching {
			b.WriteString("  " + valueStyle.Render(fmt.Sprintf("[%s] %s", wd.Source, shortenPath(wd.Path))) + "\n")
		}
	}
	b.WriteString("\n")

	// Buffer status
	b.WriteString(headerStyle.Render("Buffer"))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Buffered Traces") + valueStyle.Render(fmt.Sprintf("%d", m.stats.TracesBuffered)) + "\n")
	b.WriteString(labelStyle.Render("Buffer Size") + valueStyle.Render(formatFileSize(m.stats.BufferSizeBytes)) + "\n")
	b.WriteString("\n")

	// Export stats
	b.WriteString(headerStyle.Render("Export Stats"))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Traces Shipped") + valueStyle.Render(fmt.Sprintf("%d", m.stats.TracesShipped)) + "\n")
	b.WriteString(labelStyle.Render("Traces Failed") + valueStyle.Render(fmt.Sprintf("%d", m.stats.TracesFailed)) + "\n")
	lastShip := "never"
	if !m.stats.LastShipTime.IsZero() {
		lastShip = relativeDate(m.stats.LastShipTime)
	}
	b.WriteString(labelStyle.Render("Last Ship") + mutedStyle.Render(lastShip) + "\n")

	return b.String()
}

type exporterKeyMap struct {
	Back key.Binding
	Quit key.Binding
}

func exporterPageKeyMap() exporterKeyMap {
	return exporterKeyMap{
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}
