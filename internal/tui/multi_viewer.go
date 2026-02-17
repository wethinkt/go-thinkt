package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// MultiViewerModel displays multiple sessions in time order with lazy loading.
type MultiViewerModel struct {
	sessionPaths  []string
	sessions      []thinkt.LazySession
	registry      *thinkt.StoreRegistry
	loadErrors    []string
	viewport      viewport.Model
	width         int
	height        int
	ready         bool
	title         string
	keys          viewerKeyMap
	rendered      string
	loadedCount   int
	loadingMore   bool
	currentIdx    int  // Index of session currently being loaded
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

	// Search state
	searchMode    bool            // true when search input is visible
	searchInput   textinput.Model // the text input widget
	searchQuery   string          // current active search query
	searchMatches []int           // line numbers (0-indexed) of matches
	currentMatch  int             // index into searchMatches (-1 = none)

	// Input settling: ignore key events until a real render has occurred.
	// This prevents stray terminal escape sequences (from Kitty keyboard
	// protocol queries, cursor position reports, etc.) from being interpreted
	// as user input during view transitions.
	inputSettled bool
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

// entriesRenderedMsg is sent when a batch of entries has been rendered in a background goroutine.
type entriesRenderedMsg struct {
	origIdx  int      // session index
	startIdx int      // first entry index in this batch
	entries  []string // rendered strings
}

// NewMultiViewerModel creates a new multi-session viewer.
func NewMultiViewerModel(sessionPaths []string) MultiViewerModel {
	return NewMultiViewerModelWithRegistry(sessionPaths, nil)
}

// NewMultiViewerModelWithRegistry creates a new multi-session viewer with source-aware loading.
func NewMultiViewerModelWithRegistry(sessionPaths []string, registry *thinkt.StoreRegistry) MultiViewerModel {
	return MultiViewerModel{
		sessionPaths:     sessionPaths,
		sessions:         make([]thinkt.LazySession, len(sessionPaths)),
		registry:         registry,
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
		ls, err := OpenLazySessionWithRegistry(path, m.registry)
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

// renderForViewport sets up the viewport with current content and kicks off
// async rendering for entries. The UI shows immediately; entries appear progressively.
func (m *MultiViewerModel) renderForViewport() tea.Cmd {
	tuilog.Log.Info("MultiViewer.renderForViewport: starting", "width", m.width, "height", m.height)

	// Build sorted session indices if not already done
	if m.sortedSessions == nil {
		m.rebuildSortedSessions()
	}

	if len(m.sortedSessions) == 0 {
		m.rendered = m.renderNoSessionContent()
		m.setViewportContent()
		return nil
	}

	// Show what we have so far (may be empty initially)
	m.rebuildRenderedOutput()
	m.setViewportContent()

	// Kick off async rendering for the first session that has unrendered entries
	return m.asyncRenderNextBatch()
}

// asyncRenderNextBatch returns a tea.Cmd that renders a batch of entries in a goroutine.
// Returns nil if there are no more entries to render up to the current target.
func (m *MultiViewerModel) asyncRenderNextBatch() tea.Cmd {
	targetLines := m.height + m.renderBuffer
	if targetLines < 50 {
		targetLines = 50
	}

	if m.totalLines >= targetLines {
		return nil // already have enough
	}

	// Find next session with unrendered entries
	for _, origIdx := range m.sortedSessions {
		s := m.sessions[origIdx]
		if s == nil {
			continue
		}
		entries := s.Entries()
		displayed := m.displayedEntries[origIdx]
		if displayed >= len(entries) {
			continue
		}

		// Determine batch size: render enough to fill ~1 viewport
		batchSize := 20 // entries per batch
		remaining := len(entries) - displayed
		if remaining < batchSize {
			batchSize = remaining
		}

		// Capture values for the goroutine
		width := m.width - 4
		filters := m.filters
		startIdx := displayed
		entriesToRender := make([]thinkt.Entry, batchSize)
		copy(entriesToRender, entries[startIdx:startIdx+batchSize])
		capturedOrigIdx := origIdx

		return func() tea.Msg {
			rendered := make([]string, batchSize)
			for i, entry := range entriesToRender {
				rendered[i] = RenderThinktEntry(&entry, width, &filters)
			}
			return entriesRenderedMsg{
				origIdx:  capturedOrigIdx,
				startIdx: startIdx,
				entries:  rendered,
			}
		}
	}
	return nil
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
		m.setViewportContent()
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
	if len(m.loadErrors) > 0 {
		parts = append(parts, moreStyle.Render(fmt.Sprintf("Debug: %d session load error(s)", len(m.loadErrors))))
		parts = append(parts, moreStyle.Render("Last error: "+truncateDebugLine(m.loadErrors[len(m.loadErrors)-1], m.width-8)))
		parts = append(parts, "")
	}
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

// invalidateCache clears the rendered entry cache and starts async re-rendering.
func (m *MultiViewerModel) invalidateCache() tea.Cmd {
	m.entryCache = make([][]string, len(m.sessionPaths))
	m.displayedEntries = make([]int, len(m.sessionPaths))
	m.totalLines = 0
	var renderCmd tea.Cmd
	if m.ready {
		renderCmd = m.renderForViewport()
	}
	// Re-execute search against new rendered content
	if m.searchQuery != "" {
		m.executeSearch()
	}
	return renderCmd
}

// setViewportContent sets the viewport content, applying search highlighting if active.
func (m *MultiViewerModel) setViewportContent() {
	if m.searchQuery != "" && len(m.searchMatches) > 0 {
		m.viewport.SetContent(m.buildHighlightedContent())
	} else {
		m.viewport.SetContent(m.rendered)
	}
}

// buildHighlightedContent returns m.rendered with search matches highlighted.
func (m *MultiViewerModel) buildHighlightedContent() string {
	queryLower := strings.ToLower(m.searchQuery)
	lines := strings.Split(m.rendered, "\n")

	matchSet := make(map[int]bool, len(m.searchMatches))
	for _, ln := range m.searchMatches {
		matchSet[ln] = true
	}

	currentLine := -1
	if m.currentMatch >= 0 && m.currentMatch < len(m.searchMatches) {
		currentLine = m.searchMatches[m.currentMatch]
	}

	for i, line := range lines {
		if !matchSet[i] {
			continue
		}
		lines[i] = highlightLineMatches(line, queryLower, i == currentLine)
	}

	return strings.Join(lines, "\n")
}

// highlightLineMatches highlights all case-insensitive occurrences of queryLower
// within line, handling ANSI escape sequences correctly.
// isCurrent uses a bolder highlight for the current match line.
func highlightLineMatches(line, queryLower string, isCurrent bool) string {
	stripped := ansi.Strip(line)
	strippedLower := strings.ToLower(stripped)

	// Find all match byte-positions in the stripped text
	type span struct{ start, end int }
	var matches []span
	start := 0
	for {
		idx := strings.Index(strippedLower[start:], queryLower)
		if idx < 0 {
			break
		}
		mStart := start + idx
		mEnd := mStart + len(queryLower)
		matches = append(matches, span{mStart, mEnd})
		start = mEnd
	}
	if len(matches) == 0 {
		return line
	}

	// ANSI codes: reverse video for matches, reverse+bold for current match line
	hlOn := "\033[7m"
	hlOff := "\033[27m"
	if isCurrent {
		hlOn = "\033[1;7m"
		hlOff = "\033[27;22m"
	}

	// Walk the original line, tracking visible byte position (position in stripped text).
	// Insert highlight codes at the right spots.
	var buf strings.Builder
	buf.Grow(len(line) + len(matches)*16)
	visPos := 0 // byte offset in stripped text
	mi := 0     // index into matches
	inHL := false

	for i := 0; i < len(line); {
		// Skip over ANSI escape sequences
		if line[i] == '\033' && i+1 < len(line) && line[i+1] == '[' {
			j := i + 2
			for j < len(line) && !isAnsiTerminator(line[j]) {
				j++
			}
			if j < len(line) {
				j++ // include the terminator byte
			}
			buf.WriteString(line[i:j])
			i = j
			continue
		}

		// Start highlight at match boundary
		if mi < len(matches) && visPos == matches[mi].start && !inHL {
			buf.WriteString(hlOn)
			inHL = true
		}

		buf.WriteByte(line[i])
		i++
		visPos++

		// End highlight at match boundary
		if inHL && mi < len(matches) && visPos == matches[mi].end {
			buf.WriteString(hlOff)
			inHL = false
			mi++
		}
	}

	if inHL {
		buf.WriteString(hlOff)
	}
	return buf.String()
}

// isAnsiTerminator returns true if b is the final byte of a CSI escape sequence.
func isAnsiTerminator(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '~'
}

// executeSearch finds all lines matching the current search query.
func (m *MultiViewerModel) executeSearch() {
	if m.searchQuery == "" || m.rendered == "" {
		m.searchMatches = nil
		m.currentMatch = -1
		return
	}

	query := strings.ToLower(m.searchQuery)
	lines := strings.Split(m.rendered, "\n")
	m.searchMatches = nil

	for i, line := range lines {
		stripped := ansi.Strip(line)
		if strings.Contains(strings.ToLower(stripped), query) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}

	if len(m.searchMatches) > 0 {
		// Jump to the first match at or after the current viewport position
		yOffset := m.viewport.YOffset()
		m.currentMatch = 0
		for i, lineNum := range m.searchMatches {
			if lineNum >= yOffset {
				m.currentMatch = i
				break
			}
		}
		m.jumpToCurrentMatch()
	} else {
		m.currentMatch = -1
		m.setViewportContent() // remove stale highlights
	}
}

// jumpToCurrentMatch scrolls the viewport to show the current match.
func (m *MultiViewerModel) jumpToCurrentMatch() {
	if m.currentMatch < 0 || m.currentMatch >= len(m.searchMatches) {
		return
	}
	lineNum := m.searchMatches[m.currentMatch]

	// Update highlighting to reflect new current match
	m.setViewportContent()

	// Center the match in the viewport
	viewportHeight := m.viewport.Height()
	offset := lineNum - viewportHeight/2
	if offset < 0 {
		offset = 0
	}
	m.viewport.SetYOffset(offset)
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

func (m *MultiViewerModel) appendLoadError(idx int, err error) {
	path := "<unknown>"
	if idx >= 0 && idx < len(m.sessionPaths) {
		path = m.sessionPaths[idx]
	}
	msg := fmt.Sprintf("%s: %v", path, err)
	m.loadErrors = append(m.loadErrors, msg)
	if len(m.loadErrors) > 8 {
		m.loadErrors = m.loadErrors[len(m.loadErrors)-8:]
	}
}

func (m MultiViewerModel) renderNoSessionContent() string {
	if len(m.loadErrors) == 0 {
		return "No sessions loaded successfully"
	}

	lines := []string{
		"No sessions loaded successfully",
		"",
		"Debug (load errors):",
	}
	for _, errLine := range m.loadErrors {
		lines = append(lines, " - "+truncateDebugLine(errLine, m.width-6))
	}
	lines = append(lines, "")
	lines = append(lines, "Set THINKT_LOG_FILE=/tmp/thinkt.log for full logs.")
	return strings.Join(lines, "\n")
}

func truncateDebugLine(s string, max int) string {
	if max < 16 {
		max = 16
	}
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func (m MultiViewerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case entriesRenderedMsg:
		tuilog.Log.Info("MultiViewer.Update: entriesRenderedMsg", "origIdx", msg.origIdx, "startIdx", msg.startIdx, "count", len(msg.entries))
		origIdx := msg.origIdx
		// Append rendered entries to cache
		if m.entryCache[origIdx] == nil {
			m.entryCache[origIdx] = make([]string, 0, len(msg.entries))
		}
		for i, rendered := range msg.entries {
			entryIdx := msg.startIdx + i
			// Only append if this is the next expected entry (avoid duplicates from stale batches)
			if entryIdx == len(m.entryCache[origIdx]) {
				m.entryCache[origIdx] = append(m.entryCache[origIdx], rendered)
				lines := strings.Count(rendered, "\n") + 1
				m.displayedEntries[origIdx] = entryIdx + 1
				m.totalLines += lines
			}
		}
		// Rebuild output and update viewport
		m.rebuildRenderedOutput()
		m.setViewportContent()

		// Settle input after first render batch
		if !m.inputSettled {
			m.inputSettled = true
		}

		// Queue another batch if we need more
		if renderCmd := m.asyncRenderNextBatch(); renderCmd != nil {
			cmds = append(cmds, renderCmd)
		}

	case multiSessionLoadedMsg:
		tuilog.Log.Info("MultiViewer.Update: multiSessionLoadedMsg received", "index", msg.index, "hasError", msg.err != nil)
		if msg.err != nil {
			// Log error but continue loading other sessions
			tuilog.Log.Error("MultiViewer.Update: session load failed", "index", msg.index, "error", msg.err)
			m.appendLoadError(msg.index, msg.err)
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
			// All sessions opened, start async rendering
			m.loadingMore = false
			m.updateHasMoreData()
			if m.ready {
				tuilog.Log.Info("MultiViewer.Update: all sessions opened, starting async render", "hasMoreData", m.hasMoreData)
				if renderCmd := m.renderForViewport(); renderCmd != nil {
					cmds = append(cmds, renderCmd)
				}
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
			if m.allSessionsAttempted() && m.loadedCount > 0 {
				tuilog.Log.Info("MultiViewer.Update: viewport ready, starting async render")
				if renderCmd := m.renderForViewport(); renderCmd != nil {
					cmds = append(cmds, renderCmd)
				}
			}
		} else {
			m.viewport.SetWidth(m.width - 2)
			m.viewport.SetHeight(contentHeight)
		}

	case tea.KeyMsg:
		// Don't process key input until the view has settled. During view
		// transitions, stray terminal responses (e.g. Kitty keyboard protocol
		// queries, cursor position reports) can arrive as key events — a split
		// CSI sequence like \x1b[2;11R can have the \x1b parsed as Escape
		// triggering Back, or "/" triggering search with junk filling the bar.
		// Only allow ctrl+c (unambiguous, never part of escape sequences).
		if !m.inputSettled {
			if msg.String() == "ctrl+c" {
				for _, s := range m.sessions {
					if s != nil {
						s.Close()
					}
				}
				return m, tea.Quit
			}
			return m, nil
		}

		if m.searchMode {
			switch msg.String() {
			case "enter":
				query := m.searchInput.Value()
				if query != "" {
					m.searchQuery = query
					m.searchMode = false
					m.executeSearch()
				} else {
					m.searchMode = false
				}
				return m, nil
			case "esc":
				m.searchMode = false
				return m, nil
			case "ctrl+c":
				for _, s := range m.sessions {
					if s != nil {
						s.Close()
					}
				}
				return m, tea.Quit
			default:
				var tiCmd tea.Cmd
				m.searchInput, tiCmd = m.searchInput.Update(msg)
				return m, tiCmd
			}
		}

		switch {
		case key.Matches(msg, m.keys.Search):
			m.searchMode = true
			m.searchInput = textinput.New()
			m.searchInput.Prompt = ""
			m.searchInput.Placeholder = "Search..."
			m.searchInput.Focus()
			m.searchInput.CharLimit = 256
			m.searchInput.SetWidth(m.width - 4)
			if m.searchQuery != "" {
				m.searchInput.SetValue(m.searchQuery)
			}
			return m, textinput.Blink
		case key.Matches(msg, m.keys.NextMatch):
			if len(m.searchMatches) > 0 {
				m.currentMatch = (m.currentMatch + 1) % len(m.searchMatches)
				m.jumpToCurrentMatch()
			}
		case key.Matches(msg, m.keys.PrevMatch):
			if len(m.searchMatches) > 0 {
				m.currentMatch--
				if m.currentMatch < 0 {
					m.currentMatch = len(m.searchMatches) - 1
				}
				m.jumpToCurrentMatch()
			}
		case key.Matches(msg, m.keys.Back):
			if m.searchQuery != "" {
				// First ESC clears search and removes highlights
				m.searchQuery = ""
				m.searchMatches = nil
				m.currentMatch = -1
				m.setViewportContent()
				return m, nil
			}
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
			if c := m.invalidateCache(); c != nil {
				cmds = append(cmds, c)
			}
		case key.Matches(msg, m.keys.ToggleOutput):
			m.filters.Output = !m.filters.Output
			if c := m.invalidateCache(); c != nil {
				cmds = append(cmds, c)
			}
		case key.Matches(msg, m.keys.ToggleTools):
			m.filters.Tools = !m.filters.Tools
			if c := m.invalidateCache(); c != nil {
				cmds = append(cmds, c)
			}
		case key.Matches(msg, m.keys.ToggleThinking):
			m.filters.Thinking = !m.filters.Thinking
			if c := m.invalidateCache(); c != nil {
				cmds = append(cmds, c)
			}
		case key.Matches(msg, m.keys.ToggleOther):
			m.filters.Other = !m.filters.Other
			if c := m.invalidateCache(); c != nil {
				cmds = append(cmds, c)
			}
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


// allSessionsAttempted returns true when every session path has either loaded
// successfully or failed with an error.
func (m MultiViewerModel) allSessionsAttempted() bool {
	return m.loadedCount+len(m.loadErrors) >= len(m.sessionPaths)
}

func (m MultiViewerModel) viewContent() string {
	allDone := m.allSessionsAttempted()

	// Show loading screen until viewport is ready and sessions are done
	if !m.ready || !allDone {
		progress := ""
		if len(m.sessionPaths) > 1 && m.currentIdx > 0 {
			progress = fmt.Sprintf(" (%d/%d)", m.currentIdx, len(m.sessionPaths))
		}
		tuilog.Log.Debug("MultiViewer.View: still loading", "ready", m.ready, "renderedLen", len(m.rendered), "loadedCount", m.loadedCount, "allDone", allDone)
		return "Loading..." + progress
	}

	// Handle case where content couldn't be rendered (e.g., all sessions failed)
	if m.rendered == "" {
		tuilog.Log.Warn("MultiViewer.View: no content to display", "loadedCount", m.loadedCount)
		return m.renderNoSessionContent()
	}

	// Header
	title := viewerTitleStyle.Render(m.title)
	loadInfo := ""
	if !allDone {
		loadInfo = viewerInfoStyle.Render(fmt.Sprintf("  Loading %d/%d...", m.loadedCount+len(m.loadErrors), len(m.sessionPaths)))
	} else if m.loadingMore {
		loadInfo = viewerInfoStyle.Render("  Loading more...")
	} else if m.hasMoreData {
		loadInfo = viewerInfoStyle.Render("  (scroll for more)")
	} else {
		loadInfo = viewerInfoStyle.Render(fmt.Sprintf("  %d sessions", m.loadedCount))
	}
	if len(m.loadErrors) > 0 {
		loadInfo += viewerInfoStyle.Render(fmt.Sprintf("  %d failed", len(m.loadErrors)))
	}
	header := title + loadInfo + "  " + m.renderFilterStatus()

	// Footer
	var footer string
	if m.searchMode {
		// Show search input
		prompt := viewerInfoStyle.Render("/")
		inputView := m.searchInput.View()
		footer = prompt + inputView
	} else {
		scrollPercent := m.viewport.ScrollPercent() * 100
		position := viewerInfoStyle.Render(fmt.Sprintf("%3.0f%%", scrollPercent))

		var helpText string
		if m.searchQuery != "" && len(m.searchMatches) > 0 {
			matchInfo := fmt.Sprintf("Match %d/%d", m.currentMatch+1, len(m.searchMatches))
			helpText = fmt.Sprintf("%s  ·  n/N: next/prev  ·  /: search  ·  esc: clear", matchInfo)
		} else if m.searchQuery != "" {
			helpText = "No matches  ·  /: search  ·  esc: clear"
		} else if m.hasMoreData {
			helpText = "↑/↓: scroll • /: search • 1-5: filters • G: load all • esc: back • q: quit"
		} else {
			helpText = "↑/↓: scroll • /: search • 1-5: filters • g/G: top/bottom • esc: back • q: quit"
		}
		help := viewerHelpStyle.Render(helpText)
		footerWidth := m.width - lipgloss.Width(position) - 4
		footer = help + lipgloss.NewStyle().Width(footerWidth).Align(lipgloss.Right).Render(position)
	}

	// Content
	contentHeight := m.height - 4
	content := viewerBorderStyle.
		Width(m.width - 2).
		Height(contentHeight).
		Render(m.viewport.View())

	return header + "\n" + content + "\n" + footer
}

func (m MultiViewerModel) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	return v
}
