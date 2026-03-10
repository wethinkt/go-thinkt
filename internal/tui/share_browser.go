package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/wethinkt/go-thinkt/internal/share"
)

// ShareBrowserMode controls whether we show the user's traces or public explore.
type ShareBrowserMode int

const (
	ShareBrowserMine    ShareBrowserMode = iota
	ShareBrowserExplore ShareBrowserMode = iota
)

// ShareBrowserResult is what the TUI returns when a trace is selected.
type ShareBrowserResult struct {
	Slug   string
	Action string // "open", "quit"
}

type shareItem struct {
	trace share.Trace
}

func (i shareItem) Title() string { return i.trace.Title }
func (i shareItem) Description() string {
	vis := i.trace.Visibility
	size := formatShareSize(i.trace.SizeBytes)
	likes := ""
	if i.trace.LikesCount > 0 {
		likes = fmt.Sprintf(" | %d likes", i.trace.LikesCount)
	}
	owner := ""
	if i.trace.OwnerName != "" {
		owner = fmt.Sprintf(" | @%s", i.trace.OwnerName)
	}
	return fmt.Sprintf("%s | %s | %s%s%s", i.trace.Slug, vis, size, likes, owner)
}
func (i shareItem) FilterValue() string { return i.trace.Title + " " + i.trace.Slug }

// ShareBrowserModel is the bubbletea model for browsing shared traces.
type ShareBrowserModel struct {
	list   list.Model
	result *ShareBrowserResult
}

// NewShareBrowser creates a new share browser TUI.
func NewShareBrowser(traces []share.Trace, mode ShareBrowserMode) ShareBrowserModel {
	items := make([]list.Item, len(traces))
	for i, t := range traces {
		items[i] = shareItem{trace: t}
	}

	title := "My Traces"
	if mode == ShareBrowserExplore {
		title = "Explore Public Traces"
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = title
	l.SetShowHelp(true)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open in browser")),
		}
	}

	return ShareBrowserModel{list: l}
}

func (m ShareBrowserModel) Init() tea.Cmd { return nil }

func (m ShareBrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "enter":
			if item, ok := m.list.SelectedItem().(shareItem); ok {
				m.result = &ShareBrowserResult{Slug: item.trace.Slug, Action: "open"}
				return m, tea.Quit
			}
		case "q", "esc":
			m.result = &ShareBrowserResult{Action: "quit"}
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m ShareBrowserModel) View() tea.View {
	v := tea.NewView(m.list.View())
	v.AltScreen = true
	return v
}

// Result returns the selected trace result, or nil if quit.
func (m ShareBrowserModel) Result() *ShareBrowserResult {
	return m.result
}

func formatShareSize(b int) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
