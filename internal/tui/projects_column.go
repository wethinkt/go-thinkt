package tui

import (
	"fmt"
	"sort"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tuilog"
)

// SortField defines what field to sort projects by.
type SortField int

const (
	SortByName SortField = iota
	SortByRecent
)

func (s SortField) String() string {
	switch s {
	case SortByName:
		return "name"
	case SortByRecent:
		return "recent"
	default:
		return "name"
	}
}

// projectItem wraps a claude.Project for the list component.
type projectItem struct {
	project claude.Project
}

func (i projectItem) Title() string       { return i.project.DisplayName }
func (i projectItem) Description() string { return i.project.FullPath }
func (i projectItem) FilterValue() string { return i.project.DisplayName + " " + i.project.FullPath }

// projectsModel manages the projects list (column 1).
type projectsModel struct {
	list      list.Model
	items     []claude.Project
	sortField SortField
	sortAsc   bool
	width     int
	height    int
}

func newProjectsModel() projectsModel {
	delegate := list.NewDefaultDelegate()
	l := list.New(nil, delegate, 0, 0)
	l.SetShowTitle(false) // We render title in the column border
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowFilter(false)      // Hide filter bar to save space
	l.SetFilteringEnabled(true) // But keep filtering functional (/ to search)
	return projectsModel{
		list:      l,
		sortField: SortByName,
		sortAsc:   true,
	}
}

func (m *projectsModel) setItems(projects []claude.Project) {
	tuilog.Log.Debug("projectsModel.setItems", "count", len(projects), "currentHeight", m.height)
	m.items = projects
	m.applySort()
}

func (m *projectsModel) applySort() {
	sorted := make([]claude.Project, len(m.items))
	copy(sorted, m.items)

	switch m.sortField {
	case SortByName:
		sort.Slice(sorted, func(i, j int) bool {
			if m.sortAsc {
				return sorted[i].DisplayName < sorted[j].DisplayName
			}
			return sorted[i].DisplayName > sorted[j].DisplayName
		})
	case SortByRecent:
		sort.Slice(sorted, func(i, j int) bool {
			if m.sortAsc {
				return sorted[i].LastModified.Before(sorted[j].LastModified)
			}
			return sorted[i].LastModified.After(sorted[j].LastModified)
		})
	}

	items := make([]list.Item, len(sorted))
	for i, p := range sorted {
		items[i] = projectItem{project: p}
	}
	m.list.SetItems(items)
}

func (m *projectsModel) toggleSortField() {
	if m.sortField == SortByName {
		m.sortField = SortByRecent
	} else {
		m.sortField = SortByName
	}
	m.applySort()
}

func (m *projectsModel) toggleSortOrder() {
	m.sortAsc = !m.sortAsc
	m.applySort()
}

func (m *projectsModel) sortIndicator() string {
	order := "asc"
	if !m.sortAsc {
		order = "desc"
	}
	return fmt.Sprintf("[%s %s]", m.sortField.String(), order)
}

func (m *projectsModel) setSize(w, h int) {
	tuilog.Log.Debug("projectsModel.setSize", "width", w, "height", h, "itemCount", len(m.items))
	m.width = w
	m.height = h
	m.list.SetSize(w, h)
	// Re-apply items to ensure proper pagination with new dimensions
	if len(m.items) > 0 {
		m.applySort()
	}
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
	// Show loading/empty state if no items
	// if len(m.items) == 0 {
	// 	if m.height > 0 {
	// 		// Return empty lines to fill the space (border will show "Projects" title)
	// 		lines := make([]string, m.height)
	// 		for i := range lines {
	// 			lines[i] = ""
	// 		}
	// 		return strings.Join(lines, "\n")
	// 	}
	// 	return ""
	// }

	content := m.list.View()
	// // Constrain to our dimensions in case list renders too much
	// if m.height > 0 {
	// 	lines := strings.Split(content, "\n")
	// 	if len(lines) > m.height {
	// 		lines = lines[:m.height]
	// 		content = strings.Join(lines, "\n")
	// 	}
	// }
	return content
}
