package tui

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

// projectItem wraps a claude.Project for the list component.
type projectItem struct {
	project claude.Project
}

func (i projectItem) Title() string       { return i.project.DisplayName }
func (i projectItem) Description() string { return i.project.FullPath }
func (i projectItem) FilterValue() string { return i.project.DisplayName + " " + i.project.FullPath }

// projectsModel manages the projects list (column 1).
type projectsModel struct {
	list  list.Model
	items []claude.Project
}

func newProjectsModel() projectsModel {
	delegate := list.NewDefaultDelegate()
	l := list.New(nil, delegate, 0, 0)
	l.Title = "Projects"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	return projectsModel{list: l}
}

func (m *projectsModel) setItems(projects []claude.Project) {
	m.items = projects
	items := make([]list.Item, len(projects))
	for i, p := range projects {
		items[i] = projectItem{project: p}
	}
	m.list.SetItems(items)
}

func (m *projectsModel) setSize(w, h int) {
	m.list.SetSize(w, h)
}

func (m *projectsModel) selectedProject() *claude.Project {
	item := m.list.SelectedItem()
	if item == nil {
		return nil
	}
	pi, ok := item.(projectItem)
	if !ok {
		return nil
	}
	return &pi.project
}

func (m projectsModel) update(msg tea.Msg) (projectsModel, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m projectsModel) view() string {
	return m.list.View()
}
