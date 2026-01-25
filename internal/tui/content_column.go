package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

// contentModel manages the content viewport (column 3).
type contentModel struct {
	viewport viewport.Model
	width    int
	height   int

	// Current session window
	sessionPath string
	window      *claude.SessionWindow
	rendered    string

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

// setWindow sets the initial session window
func (m *contentModel) setWindow(window *claude.SessionWindow, path string) {
	m.window = window
	m.sessionPath = path
	m.loading = false
	m.loadingMore = false

	if window != nil && window.Session != nil {
		m.rendered = RenderSession(window.Session, m.width)
		m.updateViewportContent()
		m.viewport.GotoTop()
	} else {
		m.rendered = ""
		m.viewport.SetContent("Select a session to view")
	}
}

// appendWindow appends more content from a continuation window
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
	if m.window != nil {
		status := m.statusLine()
		if status != "" {
			content += "\n\n" + status
		}
	}

	m.viewport.SetContent(content)
}

func (m *contentModel) statusLine() string {
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
	if m.window == nil || !m.window.HasMore || m.loadingMore {
		return false
	}

	// Check if we're within 5 lines of bottom
	atBottom := m.viewport.AtBottom()
	return atBottom
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
