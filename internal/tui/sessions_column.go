package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
)

// sessionItem wraps a thinkt.SessionMeta for the list component.
type sessionItem struct {
	meta thinkt.SessionMeta
}

func (i sessionItem) Title() string {
	// Add source indicator prefix
	sourcePrefix := ""
	if i.meta.Source == "kimi" {
		sourcePrefix = "[K] "
	} else if i.meta.Source == "claude" {
		sourcePrefix = "[C] "
	}
	
	// Use FirstPrompt if available, otherwise show truncated ID
	if i.meta.FirstPrompt != "" {
		text := i.meta.FirstPrompt
		// Limit to reasonable display length
		if len(text) > 50 {
			text = text[:50] + "..."
		}
		return sourcePrefix + text
	}
	
	// Fallback: show short ID with indicator it's empty
	id := i.meta.ID
	if len(id) > 8 {
		id = id[:8]
	}
	return sourcePrefix + "[" + id + "] (no preview)"
}

func (i sessionItem) Description() string {
	parts := []string{}
	
	// Timestamp
	if !i.meta.CreatedAt.IsZero() {
		parts = append(parts, i.meta.CreatedAt.Local().Format("Jan 02, 3:04 PM"))
	}
	
	// Entry count
	if i.meta.EntryCount > 0 {
		parts = append(parts, fmt.Sprintf("%d msgs", i.meta.EntryCount))
	}
	
	// Show partial ID for reference
	id := i.meta.ID
	if len(id) > 6 {
		id = id[:6]
	}
	parts = append(parts, "id:"+id)
	
	return strings.Join(parts, " | ")
}

func (i sessionItem) FilterValue() string {
	return i.meta.FirstPrompt + " " + i.meta.ID
}

// sessionsModel manages the sessions list (column 2).
type sessionsModel struct {
	list   list.Model
	items  []thinkt.SessionMeta
	width  int
	height int
}

func newSessionsModel() sessionsModel {
	delegate := list.NewDefaultDelegate()
	l := list.New(nil, delegate, 0, 0)
	l.SetShowTitle(true)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowFilter(false)      // Hide filter bar to save space
	l.SetFilteringEnabled(true) // But keep filtering functional (/ to search)
	l.Title = "Sessions"
	return sessionsModel{list: l}
}

func (m *sessionsModel) setItems(sessions []thinkt.SessionMeta) {
	m.items = sessions
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = sessionItem{meta: s}
	}
	m.list.SetItems(items)
	// Update title with count and source breakdown
	m.updateTitle()
}

func (m *sessionsModel) updateTitle() {
	kimiCount := 0
	claudeCount := 0
	for _, s := range m.items {
		if s.Source == "kimi" {
			kimiCount++
		} else if s.Source == "claude" {
			claudeCount++
		}
	}
	
	sourceInfo := ""
	if kimiCount > 0 && claudeCount > 0 {
		sourceInfo = fmt.Sprintf(" [K:%d C:%d]", kimiCount, claudeCount)
	}
	
	m.list.Title = fmt.Sprintf("Sessions (%d)%s", len(m.items), sourceInfo)
}

func (m *sessionsModel) setSize(w, h int) {
	m.width = w
	m.height = h
	m.list.SetWidth(w)
	m.list.SetHeight(h)
}

func (m *sessionsModel) selectedSession() *thinkt.SessionMeta {
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
