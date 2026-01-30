package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tuilog"
)

// MultiViewerModel displays multiple sessions in time order.
type MultiViewerModel struct {
	sessionPaths  []string
	sessions      []thinkt.LazySession
	viewport      viewport.Model
	width         int
	height        int
	ready         bool
	title         string
	keys          viewerKeyMap
	rendered      string
	loadedCount   int
	loadingMore   bool
	currentIdx    int // Index of session currently being loaded
	err           error
}

// multiSessionLoadedMsg is sent when a session has been loaded.
type multiSessionLoadedMsg struct {
	session thinkt.LazySession
	index   int
	err     error
}

// NewMultiViewerModel creates a new multi-session viewer.
func NewMultiViewerModel(sessionPaths []string) MultiViewerModel {
	return MultiViewerModel{
		sessionPaths: sessionPaths,
		sessions:     make([]thinkt.LazySession, len(sessionPaths)),
		title:        fmt.Sprintf("All Sessions (%d)", len(sessionPaths)),
		keys:         defaultViewerKeyMap(),
	}
}

func (m MultiViewerModel) Init() tea.Cmd {
	tuilog.Log.Info("MultiViewer.Init", "sessionCount", len(m.sessionPaths))
	// Start loading the first session
	if len(m.sessionPaths) > 0 {
		return m.loadSessionAt(0)
	}
	return nil
}

func (m MultiViewerModel) loadSessionAt(idx int) tea.Cmd {
	if idx >= len(m.sessionPaths) {
		tuilog.Log.Info("MultiViewer.loadSessionAt: idx out of range", "idx", idx, "total", len(m.sessionPaths))
		return nil
	}
	path := m.sessionPaths[idx]
	tuilog.Log.Info("MultiViewer.loadSessionAt", "idx", idx, "path", path)
	return func() tea.Msg {
		tuilog.Log.Info("MultiViewer: opening lazy session", "idx", idx, "path", path)
		ls, err := OpenLazySession(path)
		if err != nil {
			tuilog.Log.Error("MultiViewer: OpenLazySession failed", "idx", idx, "path", path, "error", err)
			return multiSessionLoadedMsg{index: idx, err: err}
		}
		tuilog.Log.Info("MultiViewer: lazy session opened", "idx", idx, "path", path)
		// Load all content for this session
		tuilog.Log.Info("MultiViewer: loading all content", "idx", idx)
		if err := ls.LoadAll(); err != nil {
			tuilog.Log.Error("MultiViewer: LoadAll failed", "idx", idx, "error", err)
			ls.Close()
			return multiSessionLoadedMsg{index: idx, err: err}
		}
		tuilog.Log.Info("MultiViewer: content loaded successfully", "idx", idx, "entries", len(ls.Entries()))
		return multiSessionLoadedMsg{session: ls, index: idx, err: nil}
	}
}

func (m *MultiViewerModel) renderAllSessions() {
	tuilog.Log.Info("MultiViewer.renderAllSessions: starting", "sessionCount", len(m.sessions))
	// Sort sessions by start time
	type sessionWithTime struct {
		session thinkt.LazySession
		start   int64
	}
	var sessionsToSort []sessionWithTime
	for _, s := range m.sessions {
		if s != nil {
			meta := s.Metadata()
			start := int64(0)
			if !meta.CreatedAt.IsZero() {
				start = meta.CreatedAt.Unix()
			}
			sessionsToSort = append(sessionsToSort, sessionWithTime{session: s, start: start})
		}
	}

	sort.Slice(sessionsToSort, func(i, j int) bool {
		return sessionsToSort[i].start < sessionsToSort[j].start
	})

	// Render each session with a separator
	var parts []string
	separatorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4")).
		Bold(true)

	for i, st := range sessionsToSort {
		s := st.session
		session := &thinkt.Session{
			Meta:    s.Metadata(),
			Entries: s.Entries(),
		}

		// Add separator between sessions
		if i > 0 {
			parts = append(parts, "")
		}

		// Session header
		meta := s.Metadata()
		sessionName := filepath.Base(meta.FullPath)
		if len(sessionName) > 40 {
			sessionName = sessionName[:40] + "..."
		}
		timestamp := ""
		if !meta.CreatedAt.IsZero() {
			timestamp = meta.CreatedAt.Local().Format("Jan 02, 2006 3:04 PM")
		}
		header := separatorStyle.Render(fmt.Sprintf("━━━ %s (%s) ━━━", sessionName, timestamp))
		parts = append(parts, header)
		parts = append(parts, "")

		// Render session content
		tuilog.Log.Info("MultiViewer.renderAllSessions: rendering session", "idx", i, "entries", len(session.Entries))
		rendered := RenderThinktSession(session, m.width-4)
		parts = append(parts, rendered)
	}

	m.rendered = strings.Join(parts, "\n")
	tuilog.Log.Info("MultiViewer.renderAllSessions: setting viewport content", "contentLength", len(m.rendered))
	m.viewport.SetContent(m.rendered)
	tuilog.Log.Info("MultiViewer.renderAllSessions: complete")
}

func (m MultiViewerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case multiSessionLoadedMsg:
		tuilog.Log.Info("MultiViewer.Update: multiSessionLoadedMsg received", "index", msg.index, "hasError", msg.err != nil)
		if msg.err != nil {
			// Log error but continue loading other sessions
			tuilog.Log.Error("MultiViewer.Update: session load failed", "index", msg.index, "error", msg.err)
			fmt.Printf("Warning: failed to load session %d: %v\n", msg.index, msg.err)
		} else {
			m.sessions[msg.index] = msg.session
			m.loadedCount++
			tuilog.Log.Info("MultiViewer.Update: session stored", "index", msg.index, "loadedCount", m.loadedCount)
		}

		// Load next session if any
		nextIdx := msg.index + 1
		if nextIdx < len(m.sessionPaths) {
			m.currentIdx = nextIdx
			cmds = append(cmds, m.loadSessionAt(nextIdx))
		} else {
			// All sessions loaded, render
			m.loadingMore = false
			if m.ready {
				tuilog.Log.Info("MultiViewer.Update: all sessions loaded, rendering")
				m.renderAllSessions()
			} else {
				tuilog.Log.Info("MultiViewer.Update: all sessions loaded but viewport not ready yet")
			}
		}

	case tea.WindowSizeMsg:
		tuilog.Log.Info("MultiViewer.Update: WindowSizeMsg", "width", msg.Width, "height", msg.Height, "wasReady", m.ready)
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 2
		footerHeight := 2
		contentHeight := m.height - headerHeight - footerHeight

		if !m.ready {
			m.viewport = viewport.New()
			m.viewport.SetWidth(m.width - 2)
			m.viewport.SetHeight(contentHeight)
			m.ready = true

			// Render if sessions already loaded
			if m.loadedCount > 0 && m.currentIdx >= len(m.sessionPaths)-1 {
				tuilog.Log.Info("MultiViewer.Update: viewport ready, rendering loaded sessions")
				m.renderAllSessions()
			}
		} else {
			m.viewport.SetWidth(m.width - 2)
			m.viewport.SetHeight(contentHeight)
		}

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			// Close all sessions
			for _, s := range m.sessions {
				if s != nil {
					s.Close()
				}
			}
			return m, tea.Quit
		case key.Matches(msg, m.keys.Home):
			m.viewport.GotoTop()
		case key.Matches(msg, m.keys.End):
			m.viewport.GotoBottom()
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m MultiViewerModel) View() tea.View {
	if m.err != nil {
		tuilog.Log.Error("MultiViewer.View: error state", "error", m.err)
		v := tea.NewView(fmt.Sprintf("Error: %v", m.err))
		v.AltScreen = true
		return v
	}

	// Don't show the frame until viewport is ready AND content is rendered
	if !m.ready || m.rendered == "" {
		progress := ""
		if m.currentIdx > 0 {
			progress = fmt.Sprintf(" (%d/%d)", m.currentIdx, len(m.sessionPaths))
		}
		tuilog.Log.Debug("MultiViewer.View: still loading", "ready", m.ready, "renderedLen", len(m.rendered), "currentIdx", m.currentIdx)
		v := tea.NewView("Loading..." + progress)
		v.AltScreen = true
		return v
	}

	// Header
	title := viewerTitleStyle.Render(m.title)
	loadInfo := ""
	if m.currentIdx < len(m.sessionPaths)-1 {
		loadInfo = viewerInfoStyle.Render(fmt.Sprintf("  Loading %d/%d...", m.currentIdx+1, len(m.sessionPaths)))
	} else {
		loadInfo = viewerInfoStyle.Render(fmt.Sprintf("  %d sessions loaded", m.loadedCount))
	}
	header := title + loadInfo

	// Footer
	scrollPercent := m.viewport.ScrollPercent() * 100
	position := viewerInfoStyle.Render(fmt.Sprintf("%3.0f%%", scrollPercent))
	help := viewerHelpStyle.Render("↑/↓: scroll • pgup/pgdn: page • g/G: top/bottom • q: quit")
	footerWidth := m.width - lipgloss.Width(position) - 4
	footer := help + lipgloss.NewStyle().Width(footerWidth).Align(lipgloss.Right).Render(position)

	// Content
	contentHeight := m.height - 4
	content := viewerBorderStyle.
		Width(m.width - 2).
		Height(contentHeight).
		Render(m.viewport.View())

	result := header + "\n" + content + "\n" + footer

	v := tea.NewView(result)
	v.AltScreen = true
	return v
}
