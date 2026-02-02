// Package cli provides CLI output formatting utilities.
package cli

import (
	"context"
	"fmt"
	"sort"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// SessionPickerItem represents a selectable session in the picker.
type SessionPickerItem struct {
	Session thinkt.SessionMeta
	Project thinkt.Project
}

func (i SessionPickerItem) Title() string {
	if i.Session.FirstPrompt != "" {
		text := i.Session.FirstPrompt
		if len(text) > 50 {
			text = text[:50] + "..."
		}
		return text
	}
	return i.Session.ID[:8]
}

func (i SessionPickerItem) Description() string {
	proj := i.Project.Name
	if len(proj) > 20 {
		proj = proj[:20] + "..."
	}
	source := string(i.Session.Source)
	return fmt.Sprintf("%s | %s | %d msgs", proj, source, i.Session.EntryCount)
}

func (i SessionPickerItem) FilterValue() string {
	return i.Session.FirstPrompt + " " + i.Project.Name + " " + i.Session.ID
}

// sessionPickerModel is the bubbletea model for session selection.
type sessionPickerModel struct {
	list     list.Model
	selected *SessionPickerItem
	quitting bool
}

func newSessionPickerModel(sessions []SessionPickerItem) sessionPickerModel {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = s
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("#9d7aff")).
		Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("#666666"))

	l := list.New(items, delegate, 80, 20)
	l.SetShowTitle(true)
	l.Title = "Select a session (or press / to search)"
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)

	return sessionPickerModel{list: l}
}

func (m sessionPickerModel) Init() tea.Cmd {
	return nil
}

func (m sessionPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if item, ok := m.list.SelectedItem().(SessionPickerItem); ok {
				m.selected = &item
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m sessionPickerModel) View() tea.View {
	if m.quitting && m.selected == nil {
		v := tea.NewView("Cancelled.\n")
		return v
	}
	v := tea.NewView(m.list.View())
	return v
}

// PickSessionInteractive shows an interactive picker for sessions across all sources.
func PickSessionInteractive(registry *thinkt.StoreRegistry) (*SessionPickerItem, error) {
	ctx := context.Background()

	// Collect all sessions from all sources
	var allSessions []SessionPickerItem

	for _, store := range registry.All() {
		projects, err := store.ListProjects(ctx)
		if err != nil {
			continue
		}

		for _, proj := range projects {
			sessions, err := store.ListSessions(ctx, proj.ID)
			if err != nil {
				continue
			}

			for _, sess := range sessions {
				allSessions = append(allSessions, SessionPickerItem{
					Session: sess,
					Project: proj,
				})
			}
		}
	}

	if len(allSessions) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}

	// Sort by modified time (newest first)
	sort.Slice(allSessions, func(i, j int) bool {
		return allSessions[i].Session.ModifiedAt.After(allSessions[j].Session.ModifiedAt)
	})

	// Run the picker
	model := newSessionPickerModel(allSessions)
	p := tea.NewProgram(model)
	
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	m := finalModel.(sessionPickerModel)
	if m.selected == nil {
		return nil, fmt.Errorf("no session selected")
	}

	return m.selected, nil
}
