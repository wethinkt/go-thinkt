package tui

import (
	"context"
	"fmt"
	"os"
	"sort"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"golang.org/x/term"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/kimi"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tuilog"
)

// column identifies which column is currently active.
type column int

const (
	colProjects column = iota
	colSessions
	colContent
)

// Model is the top-level bubbletea model for the TUI.
type Model struct {
	width             int
	height            int
	activeColumn      column
	projects          projectsModel
	sessions          sessionsModel
	content           contentModel
	header            headerModel
	selectedProject   *thinkt.Project
	currentSessions   []thinkt.SessionMeta
	loadedSessionPath string // Track which session is currently loaded in content
	err               error
	
	// Multi-source support
	registry *thinkt.StoreRegistry
}

// NewModel creates a new TUI model.
func NewModel(baseDir string) Model {
	// Create registry with all available sources
	registry := thinkt.NewRegistry()
	
	// Try to add Kimi store
	kimiStore := kimi.NewStore("")
	if projects, err := kimiStore.ListProjects(context.Background()); err == nil && len(projects) > 0 {
		registry.Register(kimiStore)
		tuilog.Log.Info("Kimi store registered", "projects", len(projects))
	}
	
	// Try to add Claude store
	claudeStore := claude.NewStore(baseDir)
	if projects, err := claudeStore.ListProjects(context.Background()); err == nil && len(projects) > 0 {
		registry.Register(claudeStore)
		tuilog.Log.Info("Claude store registered", "projects", len(projects))
	}
	
	return Model{
		activeColumn: colProjects,
		projects:     newProjectsModel(),
		sessions:     newSessionsModel(),
		content:      newContentModel(),
		header:       newHeaderModel(),
		registry:     registry,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	tuilog.Log.Info("Init", "sources", len(m.registry.All()))
	return loadProjectsCmd(m.registry)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		tuilog.Log.Debug("WindowSizeMsg", "width", msg.Width, "height", msg.Height)
		// Use fallback dimensions if we get 0x0 (can happen on startup or in non-TTY contexts)
		width, height := msg.Width, msg.Height
		if width == 0 || height == 0 {
			// Try to get size from terminal directly
			if fd := int(os.Stdout.Fd()); term.IsTerminal(fd) {
				if w, h, err := term.GetSize(fd); err == nil && w > 0 && h > 0 {
					width, height = w, h
					tuilog.Log.Debug("WindowSizeMsg fallback from stdout", "width", width, "height", height)
				}
			}
		}
		// Still 0? Use sensible defaults
		if width == 0 || height == 0 {
			width, height = 80, 24
			tuilog.Log.Debug("WindowSizeMsg using defaults", "width", width, "height", height)
		}
		m.width = width
		m.height = height
		m.updateSizes()
		return m, nil

	case ProjectsLoadedMsg:
		if msg.Err != nil {
			tuilog.Log.Error("ProjectsLoadedMsg", "error", msg.Err)
			m.err = msg.Err
			return m, nil
		}
		tuilog.Log.Info("ProjectsLoadedMsg", "count", len(msg.Projects))
		m.projects.setItems(msg.Projects)
		// Auto-select first project and load its sessions
		if len(msg.Projects) > 0 {
			m.selectedProject = &msg.Projects[0]
			m.header.setProject(m.selectedProject)
			return m, loadSessionsCmd(m.registry, m.selectedProject)
		}
		return m, nil

	case SessionsLoadedMsg:
		if msg.Err != nil {
			tuilog.Log.Error("SessionsLoadedMsg", "error", msg.Err)
			m.err = msg.Err
			return m, nil
		}
		tuilog.Log.Info("SessionsLoadedMsg", "count", len(msg.Sessions))
		m.sessions.setItems(msg.Sessions)
		m.currentSessions = msg.Sessions
		m.header.setSessions(msg.Sessions)
		// Auto-load first session
		if len(msg.Sessions) > 0 {
			sess := &msg.Sessions[0]
			m.header.setSessionMeta(sess)
			// Load session if different from currently loaded
			if sess.FullPath != m.loadedSessionPath {
				return m, nil //tea.Batch(cmd, loadSessionCmd(sess.FullPath))
			}
		}
		return m, nil

	case SessionWindowMsg:
		if msg.Err != nil {
			tuilog.Log.Error("SessionWindowMsg", "error", msg.Err)
			m.err = msg.Err
			return m, nil
		}
		tuilog.Log.Info("SessionWindowMsg", "path", msg.Path, "isContinue", msg.IsContinue,
			"entries", msg.Window.EntryCount, "bytesRead", msg.Window.BytesRead, "hasMore", msg.Window.HasMore)
		if msg.IsContinue {
			// Append more content
			m.content.appendWindow(msg.Window)
		} else {
			// Initial load
			m.content.setWindow(msg.Window, msg.Path)
			if msg.Window != nil && msg.Window.Session != nil {
				m.header.setSession(msg.Window.Session)
				m.loadedSessionPath = msg.Path
			}
		}
		return m, nil

	case LazySessionMsg:
		if msg.Err != nil {
			tuilog.Log.Error("LazySessionMsg", "error", msg.Err)
			m.err = msg.Err
			return m, nil
		}
		meta := msg.Session.Metadata()
		tuilog.Log.Info("LazySessionMsg", "path", msg.Path,
			"entries", msg.Session.EntryCount(), "hasMore", msg.Session.HasMore())
		cmd := m.content.setLazySession(msg.Session, msg.Path)
		m.header.setThinktSession(meta)
		m.loadedSessionPath = msg.Path
		return m, cmd

	case LazyLoadedMsg:
		if msg.Err != nil {
			tuilog.Log.Error("LazyLoadedMsg", "error", msg.Err)
		}
		tuilog.Log.Info("LazyLoadedMsg", "newEntries", msg.Count)
		m.content.setLoadingMore(false)
		// Update header with new session state
		if m.content.lazySession != nil {
			meta := m.content.lazySession.Metadata()
			m.header.setThinktSession(meta)
		}
		// Render newly loaded entries asynchronously
		return m, m.content.renderEntriesCmd()

	case ContentRenderedMsg:
		tuilog.Log.Info("ContentRenderedMsg", "renderedCount", msg.RenderedCount)
		m.content.applyRenderedContent(msg.Rendered, msg.RenderedCount)
		return m, nil
	}

	// Forward to active column
	switch m.activeColumn {
	case colProjects:
		var cmd tea.Cmd
		m.projects, cmd = m.projects.update(msg)
		cmds = append(cmds, cmd)
	case colSessions:
		var cmd tea.Cmd
		m.sessions, cmd = m.sessions.update(msg)
		cmds = append(cmds, cmd)
	case colContent:
		var cmd tea.Cmd
		m.content, cmd = m.content.update(msg)
		cmds = append(cmds, cmd)
		// Check if we need to load more content
		if m.content.needsMore() {
			if m.content.lazySession != nil {
				// Use lazy loading
				cmds = append(cmds, m.content.loadMoreFromLazySession())
			} else if m.content.window != nil {
				// Legacy window mode
				m.content.setLoadingMore(true)
				cmds = append(cmds, loadMoreSessionCmd(m.content.sessionPath, m.content.window.BytesRead))
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, keys.Tab):
		m.activeColumn = (m.activeColumn + 1) % 3
		return m, nil

	case key.Matches(msg, keys.ShiftTab):
		m.activeColumn = (m.activeColumn + 2) % 3
		return m, nil

	case key.Matches(msg, keys.Enter):
		return m.handleEnter()

	case key.Matches(msg, keys.OpenTracer):
		// TODO: implement tracer opening with permission dialog
		return m, nil

	case key.Matches(msg, keys.ToggleSort):
		if m.activeColumn == colProjects {
			m.projects.toggleSortField()
		}
		return m, nil

	case key.Matches(msg, keys.ToggleOrder):
		if m.activeColumn == colProjects {
			m.projects.toggleSortOrder()
		}
		return m, nil
	}

	// Forward to active column for navigation
	switch m.activeColumn {
	case colProjects:
		var cmd tea.Cmd
		m.projects, cmd = m.projects.update(msg)
		// Check if selection changed
		if proj := m.projects.selectedProject(); proj != nil && (m.selectedProject == nil || proj.ID != m.selectedProject.ID) {
			m.selectedProject = proj
			m.header.setProject(proj)
			// Load sessions for the newly selected project
			return m, tea.Batch(cmd, loadSessionsCmd(m.registry, proj))
		}
		return m, cmd
	case colSessions:
		var cmd tea.Cmd
		m.sessions, cmd = m.sessions.update(msg)
		// Check if selection changed and auto-load session
		if sess := m.sessions.selectedSession(); sess != nil {
			m.header.setSessionMeta(sess)
			// Load session if different from currently loaded
			if sess.FullPath != m.loadedSessionPath {
				return m, cmd //tea.Batch(cmd, loadSessionCmd(sess.FullPath))
			}
		}
		return m, cmd
	case colContent:
		var cmd tea.Cmd
		m.content, cmd = m.content.update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.activeColumn {
	case colProjects:
		proj := m.projects.selectedProject()
		if proj != nil {
			m.selectedProject = proj
			m.header.setProject(proj)
			// Move focus to sessions column
			m.activeColumn = colSessions
			return m, loadSessionsCmd(m.registry, proj)
		}
	case colSessions:
		// Session is already auto-loaded, just move focus to content
		m.activeColumn = colContent
		return m, nil
	}
	return m, nil
}

func (m *Model) updateSizes() {
	// Calculate column dimensions (same as View() for consistency)
	headerHeight := m.header.height()
	statusHeight := 1
	// Total height: header + columns + status bar = m.height
	availableHeight := max(3, m.height-headerHeight-statusHeight)

	// Width distribution: projects takes ~38%, sessions takes remaining ~62%
	// Sessions column fills remaining space to right edge
	projectsWidth := int(float64(m.width) * 0.38)
	sessionsWidth := m.width - projectsWidth
	if projectsWidth < 30 {
		projectsWidth = 30
	}
	if sessionsWidth < 30 {
		sessionsWidth = 30
	}

	// List height = available height - borders (2) - title (1) = height - 3
	// Border frame: top(1) + title(1) + content(N) + bottom(1) = N + 3 = availableHeight
	// Therefore N = availableHeight - 3
	listHeight := max(1, availableHeight-3)
	tuilog.Log.Debug("updateSizes", "termHeight", m.height, "headerHeight", headerHeight,
		"availableHeight", availableHeight, "listHeight", listHeight,
		"projectsWidth", projectsWidth, "sessionsWidth", sessionsWidth)
	m.projects.setSize(projectsWidth, listHeight)
	m.sessions.setSize(sessionsWidth, listHeight)
	m.header.setWidth(m.width)
}

// View implements tea.Model.
func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	if m.err != nil {
		v := tea.NewView("Error: " + m.err.Error())
		v.AltScreen = true
		return v
	}

	headerHeight := m.header.height()
	statusHeight := 1 // Status bar is 1 line
	
	// Total height: header + columns + status bar = m.height
	// Available height for columns (accounting for header and status bar)
	availableHeight := max(3, m.height-headerHeight-statusHeight)

	// Width distribution: projects takes ~38%, sessions takes remaining ~62%
	// Sessions column fills remaining space to right edge
	projectsWidth := int(float64(m.width) * 0.38)
	sessionsWidth := m.width - projectsWidth
	if projectsWidth < 30 {
		projectsWidth = 30
	}
	if sessionsWidth < 30 {
		sessionsWidth = 30
	}

	// Border frame: top(1) + title(1) + content(N) + bottom(1) = N + 3
	// So content height = availableHeight - 3 (accounting for border + title)
	colHeight := max(1, availableHeight-3)

	statusText := "Tab: columns | Enter: select | s: sort | r: reverse | T: tracer | q: quit"

	// Render columns with border
	col1 := renderColumnBorder(m.projects.view(), projectsWidth, colHeight, m.activeColumn == colProjects)
	col2 := renderColumnBorder(m.sessions.view(), sessionsWidth, colHeight, m.activeColumn == colSessions)
	body := lipgloss.JoinHorizontal(lipgloss.Top, col1, col2)
	// Build layout: header, projects column, status bar

	header := m.header.view()
	status := statusBarStyle.Width(m.width).Render(statusText)

	// Join all parts vertically
	content := lipgloss.JoinVertical(lipgloss.Left, header, body, status)

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// Commands

func loadProjectsCmd(registry *thinkt.StoreRegistry) tea.Cmd {
	return func() tea.Msg {
		defer tuilog.Log.Timed("loadProjects")()
		tuilog.Log.Debug("loadProjects", "sources", len(registry.All()))
		
		ctx := context.Background()
		projects, err := registry.ListAllProjects(ctx)
		if err != nil {
			return ProjectsLoadedMsg{Err: err}
		}
		
		// Sort by path for consistent display
		sort.Slice(projects, func(i, j int) bool {
			return projects[i].Path < projects[j].Path
		})
		
		return ProjectsLoadedMsg{Projects: projects, Err: nil}
	}
}

func loadSessionsCmd(registry *thinkt.StoreRegistry, project *thinkt.Project) tea.Cmd {
	return func() tea.Msg {
		defer tuilog.Log.Timed("loadSessions")()
		tuilog.Log.Debug("loadSessions", "projectID", project.ID, "source", project.Source)
		
		store, ok := registry.Get(project.Source)
		if !ok {
			return SessionsLoadedMsg{Err: fmt.Errorf("source not found: %s", project.Source)}
		}
		
		ctx := context.Background()
		sessions, err := store.ListSessions(ctx, project.ID)
		if err != nil {
			return SessionsLoadedMsg{Err: err}
		}
		
		return SessionsLoadedMsg{Sessions: sessions, Err: nil}
	}
}

// windowContentBytes is the target content size per load operation.
const windowContentBytes = 32 * 1024 // 32KB of displayable content

// loadSessionCmd opens a lazy session for incremental content loading.
func loadSessionCmd(sessionPath string) tea.Cmd {
	return func() tea.Msg {
		defer tuilog.Log.Timed("loadSession")()
		tuilog.Log.Debug("loadSession", "path", sessionPath)

		ls, err := OpenLazySession(sessionPath)
		if err != nil {
			return LazySessionMsg{Err: err}
		}

		return LazySessionMsg{Session: ls, Path: sessionPath}
	}
}

// loadMoreSessionCmd loads more session content from a given offset.
func loadMoreSessionCmd(sessionPath string, offset int64) tea.Cmd {
	return func() tea.Msg {
		defer tuilog.Log.Timed("loadMoreSession")()
		tuilog.Log.Debug("loadMoreSession", "path", sessionPath, "offset", offset)
		window, err := claude.LoadSessionWindow(sessionPath, offset, windowContentBytes)
		if err != nil {
			return SessionWindowMsg{Err: err, IsContinue: true}
		}

		return SessionWindowMsg{
			Window:     window,
			Path:       sessionPath,
			IsContinue: true,
		}
	}
}
