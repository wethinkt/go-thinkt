package tui

import (
	"fmt"
	"path/filepath"
	"sort"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
)

// pickerProjectItem wraps a thinkt.Project for the picker list.
type pickerProjectItem struct {
	project thinkt.Project
}

func (i pickerProjectItem) Title() string {
	return i.project.Name
}

func (i pickerProjectItem) Description() string {
	var parts []string

	// Path
	if i.project.DisplayPath != "" {
		displayPath := i.project.DisplayPath
		// Keep it reasonable
		if len(displayPath) > 50 {
			displayPath = "..." + displayPath[len(displayPath)-47:]
		}
		parts = append(parts, displayPath)
	}

	// Session count
	if i.project.SessionCount > 0 {
		parts = append(parts, fmt.Sprintf("%d sessions", i.project.SessionCount))
	}

	// Source
	if i.project.Source != "" {
		parts = append(parts, string(i.project.Source))
	}

	// Last modified
	if !i.project.LastModified.IsZero() {
		parts = append(parts, i.project.LastModified.Local().Format("Jan 02, 3:04 PM"))
	}

	result := ""
	for idx, p := range parts {
		if idx > 0 {
			result += "  •  "
		}
		result += p
	}
	return result
}

func (i pickerProjectItem) FilterValue() string {
	return i.project.Name + " " + i.project.Path + " " + string(i.project.Source)
}

// ProjectPickerResult holds the result of the project picker.
type ProjectPickerResult struct {
	Selected  *thinkt.Project
	Cancelled bool
}

// ProjectPickerModel is a standalone project picker TUI.
type ProjectPickerModel struct {
	list     list.Model
	projects []thinkt.Project
	result   ProjectPickerResult
	quitting bool
	width    int
	height   int
	ready    bool
}

type projectPickerKeyMap struct {
	Enter key.Binding
	Quit  key.Binding
}

func defaultProjectPickerKeyMap() projectPickerKeyMap {
	return projectPickerKeyMap{
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "esc", "ctrl+c"),
			key.WithHelp("esc", "cancel"),
		),
	}
}

// NewProjectPickerModel creates a new project picker with projects sorted by last modified (newest first).
func NewProjectPickerModel(projects []thinkt.Project) ProjectPickerModel {
	// Sort by last modified descending (newest first)
	sorted := make([]thinkt.Project, len(projects))
	copy(sorted, projects)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].LastModified.After(sorted[j].LastModified)
	})

	// Create list items
	items := make([]list.Item, len(sorted))
	for i, p := range sorted {
		items[i] = pickerProjectItem{project: p}
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "Select a Project"
	l.SetShowStatusBar(true)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)

	return ProjectPickerModel{
		list:     l,
		projects: sorted,
	}
}

func (m ProjectPickerModel) Init() tea.Cmd {
	return nil
}

func (m ProjectPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keys := defaultProjectPickerKeyMap()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-2)
		m.ready = true
		return m, nil

	case tea.KeyMsg:
		// Don't handle keys if filtering
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, keys.Quit):
			m.result.Cancelled = true
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, keys.Enter):
			if item := m.list.SelectedItem(); item != nil {
				if pi, ok := item.(pickerProjectItem); ok {
					m.result.Selected = &pi.project
				}
			}
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

var projectPickerStyle = lipgloss.NewStyle().Padding(1, 2)

func (m ProjectPickerModel) View() tea.View {
	if !m.ready {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	if m.quitting {
		v := tea.NewView("")
		return v
	}

	content := projectPickerStyle.Render(m.list.View())
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// Result returns the picker result after the program exits.
func (m ProjectPickerModel) Result() ProjectPickerResult {
	return m.result
}

// PickProject runs the project picker and returns the selected project.
func PickProject(projects []thinkt.Project) (*thinkt.Project, error) {
	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects available")
	}

	model := NewProjectPickerModel(projects)
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	result := finalModel.(ProjectPickerModel).Result()
	if result.Cancelled {
		return nil, nil // User cancelled
	}
	return result.Selected, nil
}

// formatFileSize returns a human-readable file size string.
// This is duplicated from session_picker.go for internal use.
func formatFileSizeForProject(size int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
	)
	switch {
	case size >= MB:
		return fmt.Sprintf("%.1f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.1f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

//nolint:unused
func getProjectPathName(path string) string {
	return filepath.Base(path)
}
