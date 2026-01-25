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
	baseDir            string
	width              int
	height             int
	activeColumn       column
	projects           projectsModel
	sessions           sessionsModel
	content            contentModel
	summary            summaryModel
	selectedProject    *claude.Project
	currentSessions    []claude.SessionMeta
	loadedSessionPath  string // Track which session is currently loaded in content
	err                error
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
		summary:      newSummaryModel(),
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
			m.summary.setProject(m.selectedProject)
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
		m.summary.setSessions(msg.Sessions)
		// Auto-load first session
		if len(msg.Sessions) > 0 {
			sess := &msg.Sessions[0]
			m.summary.setSessionMeta(sess)
			// Load session content
			return m, loadSessionCmd(sess.FullPath)
		}
		return m, nil

	case SessionLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.content.setSession(msg.Session, msg.IsPreview, msg.FileSize)
		m.summary.setSession(msg.Session)
		if msg.Session != nil {
			m.loadedSessionPath = msg.Session.Path
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

	case key.Matches(msg, keys.ToggleInfo):
		m.summary.toggle()
		m.updateSizes()
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
			m.summary.setProject(proj)
			// Batch the list's command with loading sessions
			return m, tea.Batch(cmd, loadSessionsCmd(proj.DirPath))
		}
		return m, cmd
	case colSessions:
		var cmd tea.Cmd
		m.sessions, cmd = m.sessions.update(msg)
		// Check if selection changed and auto-load session
		if sess := m.sessions.selectedSession(); sess != nil {
			m.summary.setSessionMeta(sess)
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
			m.summary.setProject(proj)
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
	// Reserve lines for status bar and summary pane
	summaryHeight := m.summary.height()
	contentHeight := m.height - 3 - summaryHeight

	// Column widths: ~20% / ~25% / ~55%
	col1Width := m.width * 20 / 100
	col2Width := m.width * 25 / 100
	col3Width := m.width - col1Width - col2Width - 6 // account for borders

	// Minimum widths
	if col1Width < 20 {
		col1Width = 20
	}
	if col2Width < 25 {
		col2Width = 25
	}
	if col3Width < 30 {
		col3Width = 30
	}

	m.projects.setSize(col1Width-2, contentHeight-2)
	m.sessions.setSize(col2Width-2, contentHeight-2)
	m.content.setSize(col3Width-2, contentHeight-2)
	m.summary.setSize(m.width - 2)
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

	summaryHeight := m.summary.height()
	contentHeight := m.height - 3 - summaryHeight

	// Column widths
	col1Width := m.width * 20 / 100
	col2Width := m.width * 25 / 100
	col3Width := m.width - col1Width - col2Width - 6

	if col1Width < 20 {
		col1Width = 20
	}
	if col2Width < 25 {
		col2Width = 25
	}
	if col3Width < 30 {
		col3Width = 30
	}

	// Render columns with borders, include sort indicator in projects title
	projectsTitle := "Projects " + m.projects.sortIndicator()
	col1 := renderColumnBorder(m.projects.view(), projectsTitle, col1Width, contentHeight, m.activeColumn == colProjects)
	col2 := renderColumnBorder(m.sessions.view(), "Sessions", col2Width, contentHeight, m.activeColumn == colSessions)
	col3 := renderColumnBorder(m.content.view(), "Content", col3Width, contentHeight, m.activeColumn == colContent)

	// Join columns horizontally
	columns := lipgloss.JoinHorizontal(lipgloss.Top, col1, col2, col3)

	// Summary pane (if visible)
	var parts []string
	if m.summary.isVisible() {
		parts = append(parts, m.summary.view())
	}
	parts = append(parts, columns)

	// Status bar with info toggle indicator
	infoStatus := "i: show info"
	if m.summary.isVisible() {
		infoStatus = "i: hide info"
	}
	status := statusBarStyle.Render("Tab: columns | Enter: select | s: sort | r: reverse | " + infoStatus + " | T: tracer | q: quit")
	parts = append(parts, status)

	// Join all parts
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)

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

const (
	// maxPreviewEntries limits how many entries we load for the preview.
	maxPreviewEntries = 50
	// maxPreviewEntriesLarge is used for files > 10MB - more aggressive limit.
	maxPreviewEntriesLarge = 20
	// largeFileThreshold is 10MB - files larger than this get extra limits.
	largeFileThreshold = 10 * 1024 * 1024
)

// loadSessionCmd stats the file for size, shows loading state, then loads preview.
func loadSessionCmd(sessionPath string) tea.Cmd {
	return func() tea.Msg {
		// Stat file to get size (only this one file, not all files)
		fileSize, err := claude.GetSessionFileInfo(sessionPath)
		if err != nil {
			return SessionLoadedMsg{Err: err}
		}

		// Determine entry limit based on file size
		maxEntries := maxPreviewEntries
		if fileSize > largeFileThreshold {
			maxEntries = maxPreviewEntriesLarge
		}

		// Load session with appropriate limit
		session, loadErr := claude.LoadSessionPreview(sessionPath, maxEntries)
		isPreview := session != nil && len(session.Entries) >= maxEntries

		return SessionLoadedMsg{
			Session:   session,
			IsPreview: isPreview,
			FileSize:  fileSize,
			Err:       loadErr,
		}
	}
}
