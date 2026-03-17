package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/target"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// wizardStep identifies the current step in the export wizard.
type wizardStep int

const (
	stepProject wizardStep = iota
	stepSession
	stepFilter
	stepFormat
	stepOutput
	stepOutputFilename
	stepOutputBrowse
	stepOutputBrowseFilename
	stepDone
)

// exportWizardResult holds all the configuration collected by the wizard.
type exportWizardResult struct {
	Cancelled   bool
	ProjectName string
	ProjectID   string
	Session     *thinkt.SessionMeta
	Filter      target.ContentFilter
	Format      string
	Output      *tui.OutputChoice
}

// exportWizardConfig holds pre-resolved values and skip flags.
type exportWizardConfig struct {
	// Pre-resolved project (skip project picker)
	ProjectID   string
	ProjectName string
	// Pre-loaded sessions (skip project picker, go straight to session)
	Sessions []thinkt.SessionMeta
	// Pre-selected session (skip session picker)
	Session *thinkt.SessionMeta
	// Pre-selected filter (skip filter picker)
	Filter *target.ContentFilter
	// Pre-selected format (skip format picker)
	Format string
	// Whether --view was set (skip output picker)
	ViewMode bool
	// Suggested filename for output picker
	SuggestedFilename string
}

type exportWizardModel struct {
	config   exportWizardConfig
	registry *thinkt.StoreRegistry
	flags    target.Flags

	step   wizardStep
	result exportWizardResult
	err    error

	// Sub-models
	projectPicker tui.ProjectPickerModel
	sessionPicker tui.SessionPickerModel
	filterPicker  target.FilterPickerModel
	formatPicker  tui.FormatPickerModel
	outputPicker  tui.OutputPickerModel
	filenameInput tui.FilenameInputModel
	fileBrowser   tui.FileBrowserModel

	// Completed step labels (for display)
	completedSteps []stepLine

	width  int
	height int
	ready  bool

	labelStyle lipgloss.Style
	valueStyle lipgloss.Style
}

type stepLine struct {
	label string
	value string
}

// sessionLoadedMsg is sent when session data is loaded after picker selection.
type sessionLoadedMsg struct {
	sessions []thinkt.SessionMeta
	err      error
}

func newExportWizard(registry *thinkt.StoreRegistry, flags target.Flags, config exportWizardConfig) exportWizardModel {
	t := theme.Current()
	m := exportWizardModel{
		config:   config,
		registry: registry,
		flags:    flags,
		labelStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)),
		valueStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true),
	}

	// Determine starting step based on what's pre-resolved
	switch {
	case config.Session != nil:
		// Project and session already resolved
		m.result.ProjectID = config.ProjectID
		m.result.ProjectName = config.ProjectName
		m.result.Session = config.Session
		m.addStep("Project", config.ProjectName)
		m.addStep("Session", sessionLabel(config.Session))
		if config.Filter != nil {
			m.result.Filter = *config.Filter
			m.addStep("Include", filterSummary(m.result.Filter))
			if config.Format != "" {
				m.result.Format = config.Format
				m.addStep("Format", config.Format)
				if config.ViewMode {
					m.step = stepDone
				} else {
					m.step = stepOutput
					m.initOutputPicker()
				}
			} else {
				m.step = stepFormat
				m.formatPicker = tui.NewFormatPicker()
			}
		} else {
			m.step = stepFilter
			m.filterPicker = target.NewFilterPicker(target.DefaultFilter())
		}
	case len(config.Sessions) > 0:
		// Project resolved, need session picker
		m.result.ProjectID = config.ProjectID
		m.result.ProjectName = config.ProjectName
		m.addStep("Project", config.ProjectName)
		m.step = stepSession
		m.initSessionPicker(config.Sessions)
	default:
		// Need project picker
		m.step = stepProject
		// projectPicker will be set in Init
	}

	return m
}

func (m *exportWizardModel) addStep(label, value string) {
	m.completedSteps = append(m.completedSteps, stepLine{label: label, value: value})
}

func (m *exportWizardModel) initSessionPicker(sessions []thinkt.SessionMeta) {
	m.config.Sessions = sessions
	m.sessionPicker = tui.NewSessionPickerModel(sessions, nil)
	m.sessionPicker.SetDisableResume(true)
}

func (m *exportWizardModel) initOutputPicker() {
	m.outputPicker = tui.NewOutputPicker()
}

func (m exportWizardModel) Init() tea.Cmd {
	if m.step == stepProject {
		return func() tea.Msg {
			res, err := target.ResolveProjectNonInteractive(m.registry, m.flags)
			if err != nil {
				return errMsg{err}
			}
			return projectResolvedMsg{res: res}
		}
	}
	return nil
}

type errMsg struct{ err error }
type projectResolvedMsg struct{ res *target.ProjectResolution }

func (m exportWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		// Forward adjusted size to current sub-model
		subMsg := tea.WindowSizeMsg{Width: m.width, Height: m.availableHeight()}
		return m.updateCurrentStep(subMsg)

	case errMsg:
		m.err = msg.err
		m.result.Cancelled = true
		return m, tea.Quit

	case projectResolvedMsg:
		if msg.res.Resolved {
			m.result.ProjectID = msg.res.ProjectID
			m.result.ProjectName = msg.res.ProjectName
			m.addStep("Project", msg.res.ProjectName)
			return m, m.loadSessions(msg.res.ProjectID)
		}
		m.projectPicker = tui.NewProjectPickerModel(msg.res.Projects)
		return m, m.resizeCurrentStep()

	case sessionLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.result.Cancelled = true
			return m, tea.Quit
		}
		if len(msg.sessions) == 0 {
			m.err = fmt.Errorf("no sessions found in project %s", m.result.ProjectName)
			m.result.Cancelled = true
			return m, tea.Quit
		}
		m.step = stepSession
		m.initSessionPicker(msg.sessions)
		return m, m.resizeCurrentStep()

	case tui.ProjectPickerResult:
		if msg.Cancelled {
			m.result.Cancelled = true
			return m, tea.Quit
		}
		if msg.Selected == nil {
			m.result.Cancelled = true
			return m, tea.Quit
		}
		m.result.ProjectID = msg.Selected.ID
		m.result.ProjectName = msg.Selected.Name
		m.addStep("Project", msg.Selected.Name)
		return m, m.loadSessions(msg.Selected.ID)

	case tui.SessionPickerResult:
		if msg.Cancelled || msg.Selected == nil {
			m.result.Cancelled = true
			return m, tea.Quit
		}
		m.result.Session = msg.Selected
		m.addStep("Session", sessionLabel(msg.Selected))
		if m.config.Filter != nil {
			m.result.Filter = *m.config.Filter
			m.addStep("Include", filterSummary(m.result.Filter))
			return m, m.advanceFromFilter()
		}
		m.step = stepFilter
		m.filterPicker = target.NewFilterPicker(target.DefaultFilter())
		return m, nil

	case target.FilterPickerResult:
		if msg.Cancelled {
			m.result.Cancelled = true
			return m, tea.Quit
		}
		m.result.Filter = msg.Filter
		m.addStep("Include", filterSummary(msg.Filter))
		return m, m.advanceFromFilter()

	case tui.FormatPickerResult:
		if msg.Cancelled {
			m.result.Cancelled = true
			return m, tea.Quit
		}
		m.result.Format = msg.Format
		m.addStep("Format", msg.Format)
		if m.config.ViewMode {
			m.step = stepDone
			return m, tea.Quit
		}
		m.step = stepOutput
		m.initOutputPicker()
		return m, nil

	case tui.OutputPickerResult:
		if msg.Cancelled {
			m.result.Cancelled = true
			return m, tea.Quit
		}
		switch msg.Cursor {
		case 0: // Enter filename
			m.step = stepOutputFilename
			m.filenameInput = tui.NewFilenameInput(m.suggestedFilename())
			return m, nil
		case 1: // Browse
			m.step = stepOutputBrowse
			m.fileBrowser = tui.NewFileBrowser()
			return m, nil
		default: // stdout
			m.result.Output = &tui.OutputChoice{Mode: "stdout"}
			m.addStep("Output", "stdout")
			m.step = stepDone
			return m, tea.Quit
		}

	case tui.FilenameInputResult:
		if msg.Cancelled {
			m.result.Cancelled = true
			return m, tea.Quit
		}
		m.result.Output = &tui.OutputChoice{Mode: "file", Path: msg.Value}
		m.addStep("Output", msg.Value)
		m.step = stepDone
		return m, tea.Quit

	case tui.FileBrowserResult:
		if msg.Cancelled {
			m.result.Cancelled = true
			return m, tea.Quit
		}
		// After selecting a directory, prompt for filename
		m.step = stepOutputBrowseFilename
		suggestion := filepath.Join(msg.Dir, m.suggestedFilename())
		m.filenameInput = tui.NewFilenameInput(suggestion)
		return m, nil
	}

	// Delegate to current sub-model
	return m.updateCurrentStep(msg)
}

func (m exportWizardModel) updateCurrentStep(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.step {
	case stepProject:
		updated, cmd := m.projectPicker.Update(msg)
		m.projectPicker = updated.(tui.ProjectPickerModel)
		return m, cmd

	case stepSession:
		updated, cmd := m.sessionPicker.Update(msg)
		m.sessionPicker = updated.(tui.SessionPickerModel)
		return m, cmd

	case stepFilter:
		updated, cmd := m.filterPicker.Update(msg)
		m.filterPicker = updated.(target.FilterPickerModel)
		return m, cmd

	case stepFormat:
		updated, cmd := m.formatPicker.Update(msg)
		m.formatPicker = updated.(tui.FormatPickerModel)
		return m, cmd

	case stepOutput:
		updated, cmd := m.outputPicker.Update(msg)
		m.outputPicker = updated.(tui.OutputPickerModel)
		return m, cmd

	case stepOutputFilename, stepOutputBrowseFilename:
		updated, cmd := m.filenameInput.Update(msg)
		m.filenameInput = updated.(tui.FilenameInputModel)
		return m, cmd

	case stepOutputBrowse:
		updated, cmd := m.fileBrowser.Update(msg)
		m.fileBrowser = updated.(tui.FileBrowserModel)
		return m, cmd
	}

	return m, nil
}

func (m exportWizardModel) View() tea.View {
	if !m.ready {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	var sections []string

	// Header bar with breadcrumb
	breadcrumb := m.buildBreadcrumb()
	detail := ""
	if m.step == stepSession {
		detail = fmt.Sprintf("(%d sessions)", len(m.config.Sessions))
	}
	header := tui.RenderHeaderBar(breadcrumb, detail, m.width)
	if header != "" {
		sections = append(sections, header)
	}

	// Completed steps
	if len(m.completedSteps) > 0 {
		sections = append(sections, m.renderCompletedSteps())
	}

	// Current step content
	sections = append(sections, m.renderCurrentStep())

	v := tea.NewView(strings.Join(sections, "\n"))
	v.AltScreen = true
	return v
}

func (m exportWizardModel) buildBreadcrumb() string {
	parts := []string{"export"}
	if m.result.ProjectName != "" && m.step > stepProject {
		parts = append(parts, m.result.ProjectName)
	}
	return strings.Join(parts, " > ")
}

func (m exportWizardModel) renderCompletedSteps() string {
	var b strings.Builder
	for _, s := range m.completedSteps {
		fmt.Fprintf(&b, "  %s  %s\n", m.labelStyle.Render(s.label+":"), m.valueStyle.Render(s.value))
	}
	return b.String()
}

func (m exportWizardModel) renderCurrentStep() string {
	switch m.step {
	case stepProject:
		return m.projectPicker.ViewContent()
	case stepSession:
		return m.sessionPicker.ViewContent()
	case stepFilter:
		return m.filterPicker.ViewContent()
	case stepFormat:
		return m.formatPicker.ViewContent()
	case stepOutput:
		return m.outputPicker.ViewContent()
	case stepOutputFilename, stepOutputBrowseFilename:
		return m.filenameInput.ViewContent()
	case stepOutputBrowse:
		return m.fileBrowser.ViewContent()
	}
	return ""
}

func (m exportWizardModel) availableHeight() int {
	overhead := tui.HeaderBarHeight + 1 // header bar + newline
	overhead += len(m.completedSteps)   // one line per completed step
	if len(m.completedSteps) > 0 {
		overhead++ // blank line after steps
	}
	h := m.height - overhead
	if h < 5 {
		h = 5
	}
	return h
}

func (m exportWizardModel) resizeCurrentStep() tea.Cmd {
	if m.width <= 0 {
		return nil
	}
	w, h := m.width, m.availableHeight()
	return func() tea.Msg {
		return tea.WindowSizeMsg{Width: w, Height: h}
	}
}

func (m exportWizardModel) loadSessions(projectID string) tea.Cmd {
	registry := m.registry
	sources := m.flags.Sources
	return func() tea.Msg {
		sessions, err := target.GetSessionsForProject(registry, projectID, sources)
		return sessionLoadedMsg{sessions: sessions, err: err}
	}
}

func (m *exportWizardModel) advanceFromFilter() tea.Cmd {
	if m.config.Format != "" {
		m.result.Format = m.config.Format
		m.addStep("Format", m.config.Format)
		if m.config.ViewMode {
			m.step = stepDone
			return tea.Quit
		}
		m.step = stepOutput
		m.initOutputPicker()
		return nil
	}
	m.step = stepFormat
	m.formatPicker = tui.NewFormatPicker()
	return nil
}

func (m exportWizardModel) suggestedFilename() string {
	title := "export"
	if m.result.Session != nil {
		title = buildExportTitle(thinkt.SessionMeta{
			FirstPrompt: m.result.Session.FirstPrompt,
			ProjectPath: m.result.Session.ProjectPath,
		})
	}
	ext := "." + m.result.Format
	return sanitizeFilename(title) + ext
}

func sessionLabel(meta *thinkt.SessionMeta) string {
	if meta == nil {
		return ""
	}
	title := meta.FirstPrompt
	if title == "" {
		title = meta.Summary
	}
	if title == "" {
		title = meta.ID
	}
	runes := []rune(title)
	if len(runes) > 60 {
		title = string(runes[:57]) + "..."
	}
	return title
}
