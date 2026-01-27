package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

// sessionItem wraps a claude.SessionMeta for the list component.
type sessionItem struct {
	meta claude.SessionMeta
}

func (i sessionItem) Title() string {
	if i.meta.FirstPrompt != "" {
		text := i.meta.FirstPrompt
		if len(text) > 60 {
			text = text[:60] + "..."
		}
		return text
	}
	return i.meta.SessionID[:8]
}

func (i sessionItem) Description() string {
	if !i.meta.Created.IsZero() {
		ts := i.meta.Created.Local().Format("Jan 02, 3:04 PM")
		if i.meta.MessageCount > 0 {
			return fmt.Sprintf("%s  (%d msgs)", ts, i.meta.MessageCount)
		}
		return ts
	}
	return ""
}

func (i sessionItem) FilterValue() string {
	return i.meta.FirstPrompt + " " + i.meta.SessionID
}

// sessionsModel manages the sessions list (column 2).
type sessionsModel struct {
	list   list.Model
	items  []claude.SessionMeta
	width  int
	height int
}

func newSessionsModel() sessionsModel {
	delegate := list.NewDefaultDelegate()
	l := list.New(nil, delegate, 0, 0)
	l.SetShowTitle(false)       // We render title in the column border
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowFilter(false)      // Hide filter bar to save space
	l.SetFilteringEnabled(true) // But keep filtering functional (/ to search)
	return sessionsModel{list: l}
}

func (m *sessionsModel) setItems(sessions []claude.SessionMeta) {
	m.items = sessions
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = sessionItem{meta: s}
	}
	m.list.SetItems(items)
}

func (m *sessionsModel) setSize(w, h int) {
	m.width = w
	m.height = h
	m.list.SetWidth(w)
	m.list.SetHeight(h)
}

func (m *sessionsModel) selectedSession() *claude.SessionMeta {
	item := m.list.SelectedItem()
	if item == nil {
		return nil
	}
	si, ok := item.(sessionItem)
	if !ok {
		return nil
	}
	return &si.meta
}

func (m sessionsModel) update(msg tea.Msg) (sessionsModel, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m sessionsModel) view() string {
	content := m.list.View()
	// Constrain to our dimensions in case list renders too much
	if m.height > 0 {
		lines := strings.Split(content, "\n")
		if len(lines) > m.height {
			lines = lines[:m.height]
			content = strings.Join(lines, "\n")
		}
	}
	return content
}
