package tui

import (
	"os"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"golang.org/x/term"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
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
	baseDir           string
	width             int
	height            int
	activeColumn      column
	projects          projectsModel
	sessions          sessionsModel
	content           contentModel
	header            headerModel
	selectedProject   *claude.Project
	currentSessions   []claude.SessionMeta
	loadedSessionPath string // Track which session is currently loaded in content
	err               error
}

// NewModel creates a new TUI model.
func NewModel(baseDir string) Model {
	if baseDir == "" {
		defaultDir, err := claude.DefaultDir()
		if err == nil {
			baseDir = defaultDir
		} else {
			baseDir = "~/.claude" // fallback
		}
	}
	return Model{
		baseDir:      baseDir,
		activeColumn: colProjects,
		projects:     newProjectsModel(),
		sessions:     newSessionsModel(),
		content:      newContentModel(),
		header:       newHeaderModel(),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	tuilog.Log.Info("Init", "baseDir", m.baseDir)
	return loadProjectsCmd(m.baseDir)
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
		// Auto-select first project if available
		if len(msg.Projects) > 0 {
			m.selectedProject = &msg.Projects[0]
			m.header.setProject(m.selectedProject)
			return m, loadSessionsCmd(msg.Projects[0].DirPath)
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
			// Load session content
			return m, loadSessionCmd(sess.FullPath)
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
		tuilog.Log.Info("LazySessionMsg", "path", msg.Session.Path,
			"entries", msg.Session.EntryCount(), "hasMore", msg.Session.HasMore())
		cmd := m.content.setLazySession(msg.Session)
		m.header.setSession(msg.Session.ToSession())
		m.loadedSessionPath = msg.Session.Path
		return m, cmd

	case LazyLoadedMsg:
		if msg.Err != nil {
			tuilog.Log.Error("LazyLoadedMsg", "error", msg.Err)
		}
		tuilog.Log.Info("LazyLoadedMsg", "newEntries", msg.Count)
		m.content.setLoadingMore(false)
		// Update header with new session state
		if m.content.lazySession != nil {
			m.header.setSession(m.content.lazySession.ToSession())
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
		if proj := m.projects.selectedProject(); proj != nil && (m.selectedProject == nil || proj.DirPath != m.selectedProject.DirPath) {
			m.selectedProject = proj
			m.header.setProject(proj)
			// Batch the list's command with loading sessions
			return m, tea.Batch(cmd, loadSessionsCmd(proj.DirPath))
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
				return m, tea.Batch(cmd, loadSessionCmd(sess.FullPath))
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
			return m, loadSessionsCmd(proj.DirPath)
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
	// Total height minus header, status bar (1), and column border (2 top+bottom)
	columnContentHeight := max(3, m.height-headerHeight-1-2)

	// Column widths: calculate proportionally, col3 gets remainder
	// Each column border adds 2 chars (left + right), so 6 total for 3 columns
	availableWidth := m.width - 6
	col1Width := max(18, availableWidth*20/100)
	col2Width := max(23, availableWidth*25/100)
	col3Width := availableWidth - col1Width - col2Width // remainder ensures exact fit

	// List/viewport height = column content height - title line (1)
	// The border renders: title + "\n" + content, so content gets height-1 lines
	listHeight := max(1, columnContentHeight-1)
	tuilog.Log.Debug("updateSizes", "termHeight", m.height, "headerHeight", headerHeight,
		"columnContentHeight", columnContentHeight, "listHeight", listHeight)
	m.projects.setSize(col1Width, listHeight)
	m.sessions.setSize(col2Width, listHeight)
	m.content.setSize(col3Width, listHeight)
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
	// Total height minus header, status bar (1), and column border (2 for top+bottom)
	columnContentHeight := max(3, m.height-headerHeight-1-2)

	// Column widths: calculate proportionally to fill the full terminal width
	// Each column border adds 2 chars (left + right), so 6 total for 3 columns
	// We set content width such that total (content + borders) = m.width
	availableWidth := m.width - 6 // content width for all columns combined
	col1Width := availableWidth - 2
	// col1Width := max(18, availableWidth*20/100)
	// col2Width := max(23, availableWidth*25/100)
	// col3Width := availableWidth - col1Width - col2Width // remainder ensures exact fit

	statusText := "Tab: columns | Enter: select | s: sort | r: reverse | T: tracer | q: quit"

	// Render columns with borders, include sort indicator in projects title
	projectsTitle := "Projects " + m.projects.sortIndicator()
	col1 := renderColumnBorder(m.projects.view(), projectsTitle, col1Width, columnContentHeight, m.activeColumn == colProjects)
	// col2 := renderColumnBorder(m.sessions.view(), "Sessions", col2Width, columnContentHeight, m.activeColumn == colSessions)
	// col3 := renderColumnBorder(m.content.view(), "Content", col3Width, columnContentHeight, m.activeColumn == colContent)

	// Join columns horizontally
	// columns := lipgloss.JoinHorizontal(lipgloss.Top, col1, col2, col3)
	columns := col1

	// Build layout: header, columns, status bar
	header := m.header.view()

	status := statusBarStyle.Width(m.width).Render(statusText)

	// Join all parts vertically
	content := lipgloss.JoinVertical(lipgloss.Left, header, columns, status)

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// Commands

func loadProjectsCmd(baseDir string) tea.Cmd {
	return func() tea.Msg {
		defer tuilog.Log.Timed("loadProjects")()
		tuilog.Log.Debug("loadProjects", "baseDir", baseDir)
		projects, err := claude.ListProjects(baseDir)
		return ProjectsLoadedMsg{Projects: projects, Err: err}
	}
}

func loadSessionsCmd(projectDir string) tea.Cmd {
	return func() tea.Msg {
		defer tuilog.Log.Timed("loadSessions")()
		tuilog.Log.Debug("loadSessions", "projectDir", projectDir)
		sessions, err := claude.ListProjectSessions(projectDir)
		return SessionsLoadedMsg{Sessions: sessions, Err: err}
	}
}

// windowContentBytes is the target content size per load operation.
const windowContentBytes = 32 * 1024 // 32KB of displayable content

// loadSessionCmd opens a lazy session for incremental content loading.
func loadSessionCmd(sessionPath string) tea.Cmd {
	return func() tea.Msg {
		defer tuilog.Log.Timed("loadSession")()
		tuilog.Log.Debug("loadSession", "path", sessionPath)

		ls, err := claude.OpenLazySession(sessionPath)
		if err != nil {
			return LazySessionMsg{Err: err}
		}

		return LazySessionMsg{Session: ls}
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
