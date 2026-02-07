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

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// MultiViewerModel displays multiple sessions in time order with lazy loading.
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
	hasMoreData   bool // True if any session has more content to load
	prefetchBytes int  // How many bytes to prefetch when scrolling near bottom

	// Lazy rendering state
	sortedSessions   []int      // Indices into sessions slice, sorted by time
	entryCache       [][]string // Cached rendered entries: entryCache[origIdx][entryIdx]
	displayedEntries []int      // Number of entries included in viewport per session (by origIdx)
	totalLines       int        // Total lines currently in rendered output
	renderBuffer     int        // Extra lines to render beyond viewport (buffer for smooth scroll)

	// Entry type filters
	filters RoleFilterSet
}

// multiSessionLoadedMsg is sent when a session has been loaded (initial open).
type multiSessionLoadedMsg struct {
	session thinkt.LazySession
	index   int
	err     error
}

// moreContentLoadedMsg is sent when additional content is loaded via LoadMore.
type moreContentLoadedMsg struct {
	loaded int
}

// NewMultiViewerModel creates a new multi-session viewer.
func NewMultiViewerModel(sessionPaths []string) MultiViewerModel {
	return MultiViewerModel{
		sessionPaths:     sessionPaths,
		sessions:         make([]thinkt.LazySession, len(sessionPaths)),
		title:            fmt.Sprintf("All Sessions (%d)", len(sessionPaths)),
		keys:             defaultViewerKeyMap(),
		prefetchBytes:    32 * 1024, // Load 32KB chunks when scrolling
		entryCache:       make([][]string, len(sessionPaths)),
		displayedEntries: make([]int, len(sessionPaths)),
		renderBuffer:     50, // Render 50 extra lines beyond viewport
		filters:          NewRoleFilterSet(),
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
		// Don't call LoadAll - OpenLazySession preloads first 8KB
		tuilog.Log.Info("MultiViewer: session opened", "idx", idx, "entries", ls.EntryCount(), "hasMore", ls.HasMore())
		return multiSessionLoadedMsg{session: ls, index: idx, err: nil}
	}
}

// loadMoreContent loads additional content from sessions that have more data.
func (m MultiViewerModel) loadMoreContent() tea.Cmd {
	// Find sessions that have more content
	sessions := m.sessions
	prefetchBytes := m.prefetchBytes
	return func() tea.Msg {
		totalLoaded := 0
		for _, s := range sessions {
			if s != nil && s.HasMore() {
				n, err := s.LoadMore(prefetchBytes)
				if err != nil {
					tuilog.Log.Error("MultiViewer: LoadMore failed", "error", err)
				}
				totalLoaded += n
				tuilog.Log.Info("MultiViewer: loaded more content", "entries", n)
			}
		}
		return moreContentLoadedMsg{loaded: totalLoaded}
	}
}

// renderForViewport renders only enough content to fill the viewport + buffer.
// It uses cached entry renders and only renders new entries as needed.
func (m *MultiViewerModel) renderForViewport() {
	tuilog.Log.Info("MultiViewer.renderForViewport: starting", "width", m.width, "height", m.height)

	// Build sorted session indices if not already done
	if m.sortedSessions == nil {
		m.rebuildSortedSessions()
	}

	if len(m.sortedSessions) == 0 {
		m.rendered = "No sessions loaded successfully"
		m.viewport.SetContent(m.rendered)
		return
	}

	// Calculate target lines: viewport height + buffer
	targetLines := m.height + m.renderBuffer
	if targetLines < 50 {
		targetLines = 50 // Minimum
	}

	// Render entries until we have enough lines
	m.renderUntilLines(targetLines)

	// Build final output from displayed entries
	m.rebuildRenderedOutput()
	tuilog.Log.Info("MultiViewer.renderForViewport: complete", "totalLines", m.totalLines, "contentLen", len(m.rendered))
	m.viewport.SetContent(m.rendered)
}

// renderUntilLines renders entries until we have at least targetLines of content.
func (m *MultiViewerModel) renderUntilLines(targetLines int) {
	// Already have enough?
	if m.totalLines >= targetLines {
		return
	}

	for _, origIdx := range m.sortedSessions {
		s := m.sessions[origIdx]
		if s == nil {
			continue
		}

		entries := s.Entries()
		displayed := m.displayedEntries[origIdx]

		// Render more entries from this session
		for displayed < len(entries) && m.totalLines < targetLines {
			// Ensure cache is large enough
			if m.entryCache[origIdx] == nil {
				m.entryCache[origIdx] = make([]string, 0, len(entries))
			}

			// Render this entry if not cached
			if displayed >= len(m.entryCache[origIdx]) {
				entry := entries[displayed]
				rendered := RenderThinktEntry(&entry, m.width-4, &m.filters)
				m.entryCache[origIdx] = append(m.entryCache[origIdx], rendered)
			}

			// Count lines in this entry
			entryContent := m.entryCache[origIdx][displayed]
			lines := strings.Count(entryContent, "\n") + 1

			m.displayedEntries[origIdx] = displayed + 1
			m.totalLines += lines
			displayed++

			tuilog.Log.Debug("MultiViewer.renderUntilLines: rendered entry",
				"origIdx", origIdx, "entryIdx", displayed-1, "lines", lines, "totalLines", m.totalLines)
		}

		// If we have enough lines, stop
		if m.totalLines >= targetLines {
			break
		}
	}
}

// renderMoreForScroll renders additional entries when user scrolls near bottom.
func (m *MultiViewerModel) renderMoreForScroll() bool {
	oldLines := m.totalLines
	targetLines := m.totalLines + m.height // Add another viewport worth

	m.renderUntilLines(targetLines)

	if m.totalLines > oldLines {
		m.rebuildRenderedOutput()
		m.viewport.SetContent(m.rendered)
		return true
	}
	return false
}

// hasUnrenderedEntries returns true if there are loaded but not yet rendered entries.
func (m *MultiViewerModel) hasUnrenderedEntries() bool {
	for _, origIdx := range m.sortedSessions {
		s := m.sessions[origIdx]
		if s == nil {
			continue
		}
		if m.displayedEntries[origIdx] < len(s.Entries()) {
			return true
		}
	}
	return false
}

// rebuildSortedSessions creates a sorted list of session indices by start time.
func (m *MultiViewerModel) rebuildSortedSessions() {
	type sessionWithIdx struct {
		idx   int
		start int64
	}
	var toSort []sessionWithIdx
	for i, s := range m.sessions {
		if s != nil {
			meta := s.Metadata()
			start := int64(0)
			if !meta.CreatedAt.IsZero() {
				start = meta.CreatedAt.Unix()
			}
			toSort = append(toSort, sessionWithIdx{idx: i, start: start})
		}
	}

	sort.Slice(toSort, func(i, j int) bool {
		return toSort[i].start < toSort[j].start
	})

	m.sortedSessions = make([]int, len(toSort))
	for i, s := range toSort {
		m.sortedSessions[i] = s.idx
	}
	tuilog.Log.Info("MultiViewer.rebuildSortedSessions: sorted", "count", len(m.sortedSessions))
}

// rebuildRenderedOutput combines displayed entries with headers into final output.
func (m *MultiViewerModel) rebuildRenderedOutput() {
	s := GetStyles()
	separatorStyle := s.Separator
	moreStyle := s.MoreText

	var parts []string
	for _, origIdx := range m.sortedSessions {
		s := m.sessions[origIdx]
		if s == nil {
			continue
		}

		// Add separator between sessions
		if len(parts) > 0 {
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

		// Add displayed entries from cache
		displayed := m.displayedEntries[origIdx]
		for i := 0; i < displayed && i < len(m.entryCache[origIdx]); i++ {
			if m.entryCache[origIdx][i] != "" {
				parts = append(parts, m.entryCache[origIdx][i])
			}
		}

		// Show indicator if there's more content (either unrendered or unloaded)
		hasMoreToRender := displayed < len(s.Entries())
		hasMoreToLoad := s.HasMore()
		if hasMoreToRender || hasMoreToLoad {
			parts = append(parts, "")
			parts = append(parts, moreStyle.Render("  ▼ scroll down for more content..."))
		}
	}

	m.rendered = strings.Join(parts, "\n")
	m.totalLines = strings.Count(m.rendered, "\n") + 1
}

// invalidateCache clears the rendered entry cache and re-renders the viewport.
func (m *MultiViewerModel) invalidateCache() {
	m.entryCache = make([][]string, len(m.sessionPaths))
	m.displayedEntries = make([]int, len(m.sessionPaths))
	m.totalLines = 0
	if m.ready {
		m.renderForViewport()
	}
}

func (m *MultiViewerModel) updateHasMoreData() {
	m.hasMoreData = false
	for _, s := range m.sessions {
		if s != nil && s.HasMore() {
			m.hasMoreData = true
			return
		}
	}
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
			// Invalidate sorted sessions so they get rebuilt with the new session
			m.sortedSessions = nil
			// Reset display counts but keep entry cache (entries don't change identity)
			m.displayedEntries = make([]int, len(m.sessionPaths))
			m.totalLines = 0
			tuilog.Log.Info("MultiViewer.Update: session stored", "index", msg.index, "loadedCount", m.loadedCount, "hasMore", msg.session.HasMore())
		}

		// Load next session if any
		nextIdx := msg.index + 1
		if nextIdx < len(m.sessionPaths) {
			m.currentIdx = nextIdx
			cmds = append(cmds, m.loadSessionAt(nextIdx))
		} else {
			// All sessions opened, render initial viewport content
			m.loadingMore = false
			m.updateHasMoreData()
			if m.ready {
				tuilog.Log.Info("MultiViewer.Update: all sessions opened, rendering", "hasMoreData", m.hasMoreData)
				m.renderForViewport()
			} else {
				tuilog.Log.Info("MultiViewer.Update: all sessions opened but viewport not ready yet")
			}
		}

	case moreContentLoadedMsg:
		tuilog.Log.Info("MultiViewer.Update: moreContentLoadedMsg received", "loaded", msg.loaded)
		m.loadingMore = false
		m.updateHasMoreData()
		if msg.loaded > 0 && m.ready {
			// New entries loaded, render more if we need them
			m.renderMoreForScroll()
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
				m.renderForViewport()
			}
		} else {
			m.viewport.SetWidth(m.width - 2)
			m.viewport.SetHeight(contentHeight)
		}

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Back):
			// Close all sessions and go back to previous page
			for _, s := range m.sessions {
				if s != nil {
					s.Close()
				}
			}
			return m, func() tea.Msg { return PopPageMsg{} }
		case key.Matches(msg, m.keys.Quit):
			// Close all sessions and exit
			for _, s := range m.sessions {
				if s != nil {
					s.Close()
				}
			}
			return m, tea.Quit
		case key.Matches(msg, m.keys.Home):
			m.viewport.GotoTop()
		case key.Matches(msg, m.keys.End):
			// When jumping to end, load all remaining content first if there's more
			if m.hasMoreData && !m.loadingMore {
				m.loadingMore = true
				cmds = append(cmds, m.loadMoreContent())
			}
			m.viewport.GotoBottom()
		case key.Matches(msg, m.keys.ToggleInput):
			m.filters.Input = !m.filters.Input
			m.invalidateCache()
		case key.Matches(msg, m.keys.ToggleOutput):
			m.filters.Output = !m.filters.Output
			m.invalidateCache()
		case key.Matches(msg, m.keys.ToggleTools):
			m.filters.Tools = !m.filters.Tools
			m.invalidateCache()
		case key.Matches(msg, m.keys.ToggleThinking):
			m.filters.Thinking = !m.filters.Thinking
			m.invalidateCache()
		case key.Matches(msg, m.keys.ToggleOther):
			m.filters.Other = !m.filters.Other
			m.invalidateCache()
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	// Check if we need more content when scrolled past 80%
	if m.ready {
		scrollPercent := m.viewport.ScrollPercent()
		if scrollPercent > 0.8 {
			// First, try rendering more already-loaded entries
			if m.hasUnrenderedEntries() {
				tuilog.Log.Info("MultiViewer.Update: scroll threshold, rendering more entries", "scrollPercent", scrollPercent)
				m.renderMoreForScroll()
			} else if m.hasMoreData && !m.loadingMore {
				// No unrendered entries, need to load more from disk
				tuilog.Log.Info("MultiViewer.Update: scroll threshold, loading more from disk", "scrollPercent", scrollPercent)
				m.loadingMore = true
				cmds = append(cmds, m.loadMoreContent())
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// renderFilterStatus returns a styled string showing which filters are active.
func (m MultiViewerModel) renderFilterStatus() string {
	type filterItem struct {
		key   string
		label string
		on    bool
	}
	items := []filterItem{
		{"1", "Input", m.filters.Input},
		{"2", "Output", m.filters.Output},
		{"3", "Tools", m.filters.Tools},
		{"4", "Thinking", m.filters.Thinking},
		{"5", "Other", m.filters.Other},
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

func (m MultiViewerModel) View() tea.View {
	// Check if we're still loading sessions
	allSessionsLoaded := m.currentIdx >= len(m.sessionPaths)-1 || m.loadedCount >= len(m.sessionPaths)

	// Don't show the frame until viewport is ready AND either content is rendered or all sessions loaded
	if !m.ready || (!allSessionsLoaded && m.rendered == "") {
		progress := ""
		if m.currentIdx > 0 {
			progress = fmt.Sprintf(" (%d/%d)", m.currentIdx, len(m.sessionPaths))
		}
		tuilog.Log.Debug("MultiViewer.View: still loading", "ready", m.ready, "renderedLen", len(m.rendered), "currentIdx", m.currentIdx, "allSessionsLoaded", allSessionsLoaded)
		v := tea.NewView("Loading..." + progress)
		v.AltScreen = true
		return v
	}

	// Handle case where content couldn't be rendered (e.g., all sessions failed)
	if m.rendered == "" && allSessionsLoaded {
		tuilog.Log.Warn("MultiViewer.View: no content to display", "loadedCount", m.loadedCount)
		v := tea.NewView("No content to display")
		v.AltScreen = true
		return v
	}

	// Header
	title := viewerTitleStyle.Render(m.title)
	loadInfo := ""
	if m.currentIdx < len(m.sessionPaths)-1 {
		loadInfo = viewerInfoStyle.Render(fmt.Sprintf("  Loading %d/%d...", m.currentIdx+1, len(m.sessionPaths)))
	} else if m.loadingMore {
		loadInfo = viewerInfoStyle.Render("  Loading more...")
	} else if m.hasMoreData {
		loadInfo = viewerInfoStyle.Render("  (scroll for more)")
	} else {
		loadInfo = viewerInfoStyle.Render(fmt.Sprintf("  %d sessions", m.loadedCount))
	}
	header := title + loadInfo + "  " + m.renderFilterStatus()

	// Footer
	scrollPercent := m.viewport.ScrollPercent() * 100
	position := viewerInfoStyle.Render(fmt.Sprintf("%3.0f%%", scrollPercent))
	helpText := "↑/↓: scroll • 1-5: filters • g/G: top/bottom • esc: back • q: quit"
	if m.hasMoreData {
		helpText = "↑/↓: scroll • 1-5: filters • G: load all • esc: back • q: quit"
	}
	help := viewerHelpStyle.Render(helpText)
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
