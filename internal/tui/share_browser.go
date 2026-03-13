package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/wethinkt/go-thinkt/internal/share"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ShareBrowserMode controls whether we show the user's sessions or public explore.
type ShareBrowserMode int

const (
	ShareBrowserMine ShareBrowserMode = iota
	ShareBrowserExplore
)

// ShareBrowserResult is what the TUI returns when a session is selected.
type ShareBrowserResult struct {
	Slug   string
	Action string // "open", "quit"
}

type shareItem struct {
	session share.Session
}

func (i shareItem) Title() string { return i.session.Title }
func (i shareItem) Description() string {
	vis := i.session.Visibility
	size := thinkt.FormatBytes(int64(i.session.SizeBytes))
	likes := ""
	if i.session.LikesCount > 0 {
		likes = fmt.Sprintf(" | %d likes", i.session.LikesCount)
	}
	owner := ""
	if i.session.OwnerName != "" {
		owner = fmt.Sprintf(" | @%s", i.session.OwnerName)
	}
	return fmt.Sprintf("%s | %s | %s%s%s", i.session.Slug, vis, size, likes, owner)
}
func (i shareItem) FilterValue() string { return i.session.Title + " " + i.session.Slug }

// ShareBrowserModel is the bubbletea model for browsing shared sessions.
type ShareBrowserModel struct {
	list   list.Model
	result *ShareBrowserResult
}

// NewShareBrowser creates a new share browser TUI.
func NewShareBrowser(sessions []share.Session, mode ShareBrowserMode) ShareBrowserModel {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = shareItem{session: s}
	}

	title := "My Sessions"
	if mode == ShareBrowserExplore {
		title = "Explore Public Sessions"
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
				m.result = &ShareBrowserResult{Slug: item.session.Slug, Action: "open"}
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

// Result returns the selected session result, or nil if quit.
func (m ShareBrowserModel) Result() *ShareBrowserResult {
	return m.result
}
