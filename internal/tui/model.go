package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
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
	return loadProjectsCmd(m.baseDir)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateSizes()
		return m, nil

	case ProjectsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
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
			m.err = msg.Err
			return m, nil
		}
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
			m.err = msg.Err
			return m, nil
		}
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
			m.content.setLoadingMore(true)
			cmds = append(cmds, loadMoreSessionCmd(m.content.sessionPath, m.content.window.BytesRead))
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
	// Reserve lines for header (2) and status bar (1)
	headerHeight := m.header.height()
	contentHeight := m.height - headerHeight - 1

	// Column widths: calculate proportionally, col3 gets remainder
	// Each column border adds 2 chars (left + right), so 6 total for 3 columns
	availableWidth := m.width - 6
	col1Width := max(18, availableWidth*20/100)
	col2Width := max(23, availableWidth*25/100)
	col3Width := availableWidth - col1Width - col2Width // remainder ensures exact fit

	m.projects.setSize(col1Width, contentHeight-2)
	m.sessions.setSize(col2Width, contentHeight-2)
	m.content.setSize(col3Width, contentHeight-2)
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
	contentHeight := m.height - headerHeight - 1

	// Column widths: calculate proportionally, then col3 gets remainder
	// Each column border adds 2 chars (left + right), so 6 total for 3 columns
	availableWidth := m.width - 6
	col1Width := max(18, availableWidth*20/100)
	col2Width := max(23, availableWidth*25/100)
	col3Width := availableWidth - col1Width - col2Width // remainder ensures exact fit

	// Render columns with borders, include sort indicator in projects title
	projectsTitle := "Projects " + m.projects.sortIndicator()
	col1 := renderColumnBorder(m.projects.view(), projectsTitle, col1Width, contentHeight, m.activeColumn == colProjects)
	col2 := renderColumnBorder(m.sessions.view(), "Sessions", col2Width, contentHeight, m.activeColumn == colSessions)
	col3 := renderColumnBorder(m.content.view(), "Content", col3Width, contentHeight, m.activeColumn == colContent)

	// Join columns horizontally
	columns := lipgloss.JoinHorizontal(lipgloss.Top, col1, col2, col3)

	// Build layout: header, columns, status bar
	header := m.header.view()
	status := statusBarStyle.Width(m.width).Render("Tab: columns | Enter: select | s: sort | r: reverse | T: tracer | q: quit")

	// Join all parts vertically
	content := lipgloss.JoinVertical(lipgloss.Left, header, columns, status)

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// Commands

func loadProjectsCmd(baseDir string) tea.Cmd {
	return func() tea.Msg {
		projects, err := claude.ListProjects(baseDir)
		return ProjectsLoadedMsg{Projects: projects, Err: err}
	}
}

func loadSessionsCmd(projectDir string) tea.Cmd {
	return func() tea.Msg {
		sessions, err := claude.ListProjectSessions(projectDir)
		return SessionsLoadedMsg{Sessions: sessions, Err: err}
	}
}

// windowContentBytes is the target content size per window.
// This is roughly enough to fill a typical terminal screen with some buffer.
// Using content bytes as proxy for lines since entries vary wildly in size.
const windowContentBytes = 32 * 1024 // 32KB of displayable content

// loadSessionCmd loads an initial window of session content.
func loadSessionCmd(sessionPath string) tea.Cmd {
	return func() tea.Msg {
		window, err := claude.LoadSessionWindow(sessionPath, 0, windowContentBytes)
		if err != nil {
			return SessionWindowMsg{Err: err}
		}

		return SessionWindowMsg{
			Window:     window,
			Path:       sessionPath,
			IsContinue: false,
		}
	}
}

// loadMoreSessionCmd loads more session content from a given offset.
func loadMoreSessionCmd(sessionPath string, offset int64) tea.Cmd {
	return func() tea.Msg {
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
