package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tuilog"
)

// contentModel manages the content viewport (column 3).
type contentModel struct {
	viewport viewport.Model
	width    int
	height   int

	// Current session - lazy loaded
	sessionPath  string
	lazySession  *claude.LazySession
	renderedCount int // number of entries already rendered
	rendered     string

	// Legacy window support (for backwards compatibility)
	window *claude.SessionWindow

	// Loading state
	loading     bool
	loadingMore bool
}

func newContentModel() contentModel {
	vp := viewport.New()
	return contentModel{viewport: vp}
}

func (m *contentModel) setSize(w, h int) {
	m.width = w
	m.height = h
	m.viewport.SetWidth(w)
	m.viewport.SetHeight(h)
}

// setLazySession sets a lazy session for incremental content loading.
// Returns a command to render content asynchronously.
func (m *contentModel) setLazySession(ls *claude.LazySession) tea.Cmd {
	// Close previous session if any
	if m.lazySession != nil {
		m.lazySession.Close()
	}

	m.lazySession = ls
	m.sessionPath = ls.Path
	m.loading = false
	m.loadingMore = false
	m.renderedCount = 0
	m.rendered = ""
	m.window = nil
	m.viewport.GotoTop()

	// Return command to render content asynchronously
	return m.renderEntriesCmd()
}

// renderEntriesCmd returns a command that renders entries asynchronously.
func (m *contentModel) renderEntriesCmd() tea.Cmd {
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
		newRendered := RenderSession(newSession, width)

		var result string
		if prevCount == 0 {
			result = newRendered
		} else {
			result = prevRendered + "\n" + newRendered
		}

		return ContentRenderedMsg{
			Rendered:      result,
			RenderedCount: prevCount + len(newEntries),
		}
	}
}

// applyRenderedContent applies the result of async rendering.
func (m *contentModel) applyRenderedContent(rendered string, count int) {
	m.rendered = rendered
	m.renderedCount = count
	m.updateViewportContent()
}

// setWindow sets the initial session window (legacy method)
func (m *contentModel) setWindow(window *claude.SessionWindow, path string) {
	// Close lazy session if switching to window mode
	if m.lazySession != nil {
		m.lazySession.Close()
		m.lazySession = nil
	}

	m.window = window
	m.sessionPath = path
	m.loading = false
	m.loadingMore = false
	m.renderedCount = 0

	if window != nil && window.Session != nil {
		m.rendered = RenderSession(window.Session, m.width)
		m.updateViewportContent()
		m.viewport.GotoTop()
	} else {
		m.rendered = ""
		m.viewport.SetContent("Select a session to view")
	}
}

// appendWindow appends more content from a continuation window (legacy method)
func (m *contentModel) appendWindow(window *claude.SessionWindow) {
	if window == nil || window.Session == nil {
		m.loadingMore = false
		return
	}

	// Render new entries and append
	newRendered := RenderSession(window.Session, m.width)
	m.rendered += "\n" + newRendered

	// Update window state
	m.window.BytesRead = window.BytesRead
	m.window.HasMore = window.HasMore
	m.window.EntryCount += window.EntryCount
	m.window.Session.Entries = append(m.window.Session.Entries, window.Session.Entries...)

	m.loadingMore = false
	m.updateViewportContent()
}

func (m *contentModel) updateViewportContent() {
	content := m.rendered

	// Add status line at bottom
	status := m.statusLine()
	if status != "" {
		content += "\n\n" + status
	}

	m.viewport.SetContent(content)
}

func (m *contentModel) statusLine() string {
	// LazySession mode
	if m.lazySession != nil {
		sizeInfo := formatBytes(m.lazySession.FileSize)
		readInfo := formatBytes(m.lazySession.BytesRead())
		entryCount := m.lazySession.EntryCount()

		if m.loadingMore {
			return fmt.Sprintf("--- Loading more... (%s / %s) ---", readInfo, sizeInfo)
		}

		if m.lazySession.HasMore() {
			pct := m.lazySession.Progress() * 100
			return fmt.Sprintf("--- %d entries loaded (%.0f%% of %s) | scroll down for more ---",
				entryCount, pct, sizeInfo)
		}

		return fmt.Sprintf("--- %d entries (%s) ---", entryCount, sizeInfo)
	}

	// Legacy window mode
	if m.window == nil {
		return ""
	}

	sizeInfo := formatBytes(m.window.TotalSize)
	readInfo := formatBytes(m.window.BytesRead)

	if m.loadingMore {
		return fmt.Sprintf("--- Loading more... (%s / %s) ---", readInfo, sizeInfo)
	}

	if m.window.HasMore {
		pct := float64(m.window.BytesRead) / float64(m.window.TotalSize) * 100
		return fmt.Sprintf("--- %d entries loaded (%.0f%% of %s) | scroll down for more ---",
			m.window.EntryCount, pct, sizeInfo)
	}

	return fmt.Sprintf("--- %d entries (%s) ---", m.window.EntryCount, sizeInfo)
}

// needsMore returns true if user has scrolled near bottom and there's more to load
func (m *contentModel) needsMore() bool {
	if m.loadingMore {
		return false
	}

	// LazySession mode
	if m.lazySession != nil {
		return m.lazySession.HasMore() && m.viewport.AtBottom()
	}

	// Legacy window mode
	if m.window == nil || !m.window.HasMore {
		return false
	}

	return m.viewport.AtBottom()
}

// loadMoreFromLazySession loads more entries from the lazy session
func (m *contentModel) loadMoreFromLazySession() tea.Cmd {
	if m.lazySession == nil || !m.lazySession.HasMore() {
		return nil
	}

	m.loadingMore = true
	m.updateViewportContent()

	ls := m.lazySession
	return func() tea.Msg {
		defer tuilog.Log.Timed("loadMoreLazy")()
		n, err := ls.LoadMore(32 * 1024) // Load 32KB more
		tuilog.Log.Debug("loadMoreLazy", "loaded", n, "error", err)
		return LazyLoadedMsg{Count: n, Err: err}
	}
}

func (m *contentModel) setLoadingMore(loading bool) {
	m.loadingMore = loading
	m.updateViewportContent()
}

func (m contentModel) update(msg tea.Msg) (contentModel, tea.Cmd) {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m contentModel) view() string {
	if m.loading {
		return "Loading..."
	}
	return m.viewport.View()
}

// formatBytes formats bytes into human-readable format
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
