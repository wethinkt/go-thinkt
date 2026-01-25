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
	baseDir      string
	width        int
	height       int
	activeColumn column
	projects     projectsModel
	sessions     sessionsModel
	content      contentModel
	err          error
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
			return m, loadSessionsCmd(msg.Projects[0].DirPath)
		}
		return m, nil

	case SessionsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.sessions.setItems(msg.Sessions)
		return m, nil

	case SessionLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.content.setSession(msg.Session)
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
	}

	// Forward to active column for navigation
	switch m.activeColumn {
	case colProjects:
		var cmd tea.Cmd
		m.projects, cmd = m.projects.update(msg)
		return m, cmd
	case colSessions:
		var cmd tea.Cmd
		m.sessions, cmd = m.sessions.update(msg)
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
			return m, loadSessionsCmd(proj.DirPath)
		}
	case colSessions:
		sess := m.sessions.selectedSession()
		if sess != nil {
			return m, loadSessionCmd(sess.FullPath)
		}
	}
	return m, nil
}

func (m *Model) updateSizes() {
	// Reserve 1 line for status bar
	contentHeight := m.height - 3

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

	contentHeight := m.height - 3

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

	// Render columns with borders
	col1 := renderColumnBorder(m.projects.view(), "Projects", col1Width, contentHeight, m.activeColumn == colProjects)
	col2 := renderColumnBorder(m.sessions.view(), "Sessions", col2Width, contentHeight, m.activeColumn == colSessions)
	col3 := renderColumnBorder(m.content.view(), "Content", col3Width, contentHeight, m.activeColumn == colContent)

	// Join columns horizontally
	columns := lipgloss.JoinHorizontal(lipgloss.Top, col1, col2, col3)

	// Status bar
	status := statusBarStyle.Render("Tab: switch columns | Enter: select | T: open tracer | q: quit")

	// Join with status bar
	content := lipgloss.JoinVertical(lipgloss.Left, columns, status)

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

func loadSessionCmd(sessionPath string) tea.Cmd {
	return func() tea.Msg {
		session, err := claude.LoadSession(sessionPath)
		return SessionLoadedMsg{Session: session, Err: err}
	}
}
