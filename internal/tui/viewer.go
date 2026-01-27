package tui

import (
	"fmt"
	"path/filepath"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

// ViewerModel is a standalone session viewer with scrolling and lazy loading.
type ViewerModel struct {
	sessionPath   string
	lazySession   *claude.LazySession
	viewport      viewport.Model
	width         int
	height        int
	ready         bool
	title         string
	keys          viewerKeyMap
	rendered      string
	renderedCount int
	loadingMore   bool
	err           error
}

type viewerKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding
	Quit     key.Binding
}

func defaultViewerKeyMap() viewerKeyMap {
	return viewerKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+u"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+d"),
			key.WithHelp("pgdn", "page down"),
		),
		Home: key.NewBinding(
			key.WithKeys("home", "g"),
			key.WithHelp("home/g", "top"),
		),
		End: key.NewBinding(
			key.WithKeys("end", "G"),
			key.WithHelp("end/G", "bottom"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "esc", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// NewViewerModel creates a new session viewer with lazy loading.
func NewViewerModel(sessionPath string) ViewerModel {
	return ViewerModel{
		sessionPath: sessionPath,
		title:       filepath.Base(sessionPath),
		keys:        defaultViewerKeyMap(),
	}
}

// sessionLoadedMsg is sent when the session has been loaded.
type sessionLoadedMsg struct {
	session *claude.LazySession
	err     error
}

// loadMoreMsg is sent when more content should be loaded.
type loadMoreMsg struct{}

// moreLoadedMsg is sent when more content has been loaded.
type moreLoadedMsg struct {
	count int
	err   error
}

// contentRenderedMsg is sent when content has been rendered.
type contentRenderedMsg struct {
	rendered      string
	renderedCount int
}

func (m ViewerModel) Init() tea.Cmd {
	// Start loading the session
	return m.loadSession()
}

func (m ViewerModel) loadSession() tea.Cmd {
	return func() tea.Msg {
		ls, err := claude.OpenLazySession(m.sessionPath)
		return sessionLoadedMsg{session: ls, err: err}
	}
}

func (m ViewerModel) loadMore() tea.Cmd {
	return func() tea.Msg {
		if m.lazySession == nil {
			return moreLoadedMsg{err: fmt.Errorf("no session")}
		}
		count, err := m.lazySession.LoadMore(64 * 1024) // Load 64KB chunks
		return moreLoadedMsg{count: count, err: err}
	}
}

// renderEntriesCmd returns a command that renders entries asynchronously.
func (m ViewerModel) renderEntriesCmd() tea.Cmd {
	if m.lazySession == nil {
		return nil
	}

	entries := m.lazySession.Entries()
	if len(entries) <= m.renderedCount {
		return nil
	}

	// Capture current state for the goroutine
	newEntries := make([]claude.Entry, len(entries)-m.renderedCount)
	copy(newEntries, entries[m.renderedCount:])
	prevRendered := m.rendered
	prevCount := m.renderedCount
	width := m.width

	return func() tea.Msg {
		// Do expensive rendering in background
		newSession := &claude.Session{Entries: newEntries}
		newRendered := RenderSession(newSession, width-4)

		var result string
		if prevCount == 0 {
			result = newRendered
		} else {
			result = prevRendered + "\n" + newRendered
		}

		return contentRenderedMsg{
			rendered:      result,
			renderedCount: prevCount + len(newEntries),
		}
	}
}

func (m ViewerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case sessionLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.lazySession = msg.session
		// Start async rendering
		if cmd := m.renderEntriesCmd(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		// Load more if available
		if m.lazySession.HasMore() {
			cmds = append(cmds, m.loadMore())
		}

	case contentRenderedMsg:
		m.rendered = msg.rendered
		m.renderedCount = msg.renderedCount
		m.viewport.SetContent(m.rendered)
		// Return early - don't pass this message to the viewport
		return m, nil

	case moreLoadedMsg:
		m.loadingMore = false
		if msg.err == nil && msg.count > 0 {
			// Start async rendering for new content
			if cmd := m.renderEntriesCmd(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Reserve space for header (2 lines) and footer (2 lines)
		headerHeight := 2
		footerHeight := 2
		contentHeight := m.height - headerHeight - footerHeight

		if !m.ready {
			// Initialize viewport
			m.viewport = viewport.New()
			m.viewport.SetWidth(m.width - 2)
			m.viewport.SetHeight(contentHeight)
			m.ready = true

			// Re-render if session already loaded
			if m.lazySession != nil {
				m.renderedCount = 0
				m.rendered = ""
				if cmd := m.renderEntriesCmd(); cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		} else {
			// Update viewport size
			m.viewport.SetWidth(m.width - 2)
			m.viewport.SetHeight(contentHeight)
		}

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			if m.lazySession != nil {
				m.lazySession.Close()
			}
			return m, tea.Quit
		case key.Matches(msg, m.keys.Home):
			m.viewport.GotoTop()
		case key.Matches(msg, m.keys.End):
			m.viewport.GotoBottom()
			// Load more if at bottom
			if m.lazySession != nil && m.lazySession.HasMore() && !m.loadingMore {
				m.loadingMore = true
				cmds = append(cmds, m.loadMore())
			}
		}
	}

	// Let viewport handle its own scrolling keys
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	// Check if we need to load more (near bottom)
	if m.lazySession != nil && m.lazySession.HasMore() && !m.loadingMore && m.viewport.AtBottom() {
		m.loadingMore = true
		cmds = append(cmds, m.loadMore())
	}

	return m, tea.Batch(cmds...)
}

// Viewer styles
var (
	viewerBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#7D56F4"))

	viewerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#7D56F4")).
				Padding(0, 1)

	viewerHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262")).
			Padding(0, 1)

	viewerInfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))
)

func (m ViewerModel) View() tea.View {
	if m.err != nil {
		v := tea.NewView(fmt.Sprintf("Error: %v", m.err))
		v.AltScreen = true
		return v
	}

	// Don't show the frame until both viewport is ready AND session is loaded
	if !m.ready || m.lazySession == nil {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	// Header: title and session info
	title := viewerTitleStyle.Render(m.title)
	info := ""
	if m.lazySession != nil {
		session := m.lazySession.ToSession()
		entryCount := m.lazySession.EntryCount()
		more := ""
		if m.lazySession.HasMore() {
			more = "+"
		}
		info = viewerInfoStyle.Render(fmt.Sprintf(
			"  %d%s entries | %s",
			entryCount,
			more,
			session.Model,
		))
	}
	header := title + info

	// Footer: help and scroll position
	scrollPercent := m.viewport.ScrollPercent() * 100
	position := viewerInfoStyle.Render(fmt.Sprintf("%3.0f%%", scrollPercent))
	loadingIndicator := ""
	if m.loadingMore {
		loadingIndicator = " loading..."
	}
	help := viewerHelpStyle.Render("↑/↓: scroll • pgup/pgdn: page • g/G: top/bottom • q: quit" + loadingIndicator)
	footerWidth := m.width - lipgloss.Width(position) - 4
	footer := help + lipgloss.NewStyle().Width(footerWidth).Align(lipgloss.Right).Render(position)

	// Content with border
	contentHeight := m.height - 4 // header + footer
	content := viewerBorderStyle.
		Width(m.width - 2).
		Height(contentHeight).
		Render(m.viewport.View())

	result := header + "\n" + content + "\n" + footer

	v := tea.NewView(result)
	v.AltScreen = true
	return v
}

// RunViewer launches the session viewer for a session file path.
func RunViewer(sessionPath string) error {
	model := NewViewerModel(sessionPath)
	p := tea.NewProgram(model)
	_, err := p.Run()
	return err
}

// RunMultiViewer launches the viewer for multiple session files.
// Sessions are displayed in time order (oldest first).
func RunMultiViewer(sessionPaths []string) error {
	model := NewMultiViewerModel(sessionPaths)
	p := tea.NewProgram(model)
	_, err := p.Run()
	return err
}
