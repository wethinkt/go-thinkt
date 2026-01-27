package tui

import (
	"fmt"
	"path/filepath"
	"sort"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

// pickerSessionItem wraps a claude.SessionMeta for the picker list.
type pickerSessionItem struct {
	meta claude.SessionMeta
}

func (i pickerSessionItem) Title() string {
	if i.meta.FirstPrompt != "" {
		text := i.meta.FirstPrompt
		if len(text) > 70 {
			text = text[:70] + "..."
		}
		return text
	}
	if len(i.meta.SessionID) > 8 {
		return i.meta.SessionID[:8]
	}
	return i.meta.SessionID
}

func (i pickerSessionItem) Description() string {
	var parts []string

	// File name
	if i.meta.FullPath != "" {
		filename := filepath.Base(i.meta.FullPath)
		// Trim .jsonl extension for cleaner display
		if len(filename) > 6 && filename[len(filename)-6:] == ".jsonl" {
			filename = filename[:len(filename)-6]
		}
		// Truncate long filenames (GUIDs are 36 chars, so allow 37)
		if len(filename) > 37 {
			filename = filename[:34] + "..."
		}
		parts = append(parts, filename)
	}

	// Modified/created time
	if !i.meta.Modified.IsZero() {
		parts = append(parts, i.meta.Modified.Local().Format("Jan 02, 3:04 PM"))
	} else if !i.meta.Created.IsZero() {
		parts = append(parts, i.meta.Created.Local().Format("Jan 02, 3:04 PM"))
	}

	// File size
	if i.meta.FileSize > 0 {
		parts = append(parts, formatFileSize(i.meta.FileSize))
	}

	// Message count
	if i.meta.MessageCount > 0 {
		parts = append(parts, fmt.Sprintf("%d msgs", i.meta.MessageCount))
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

func (i pickerSessionItem) FilterValue() string {
	return i.meta.FirstPrompt + " " + i.meta.SessionID
}

func formatFileSize(size int64) string {
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

// SessionPickerResult holds the result of the session picker.
type SessionPickerResult struct {
	Selected  *claude.SessionMeta
	Cancelled bool
}

// SessionPickerModel is a standalone session picker TUI.
type SessionPickerModel struct {
	list     list.Model
	sessions []claude.SessionMeta
	result   SessionPickerResult
	quitting bool
	width    int
	height   int
	ready    bool
}

type pickerKeyMap struct {
	Enter key.Binding
	Quit  key.Binding
}

func defaultPickerKeyMap() pickerKeyMap {
	return pickerKeyMap{
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

// NewSessionPickerModel creates a new session picker with sessions sorted by newest first.
func NewSessionPickerModel(sessions []claude.SessionMeta) SessionPickerModel {
	// Sort by modified time descending (newest first)
	sorted := make([]claude.SessionMeta, len(sessions))
	copy(sorted, sessions)
	sort.Slice(sorted, func(i, j int) bool {
		ti := sorted[i].Modified
		if ti.IsZero() {
			ti = sorted[i].Created
		}
		tj := sorted[j].Modified
		if tj.IsZero() {
			tj = sorted[j].Created
		}
		return ti.After(tj)
	})

	// Create list items
	items := make([]list.Item, len(sorted))
	for i, s := range sorted {
		items[i] = pickerSessionItem{meta: s}
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "Select a Session"
	l.SetShowStatusBar(true)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)

	return SessionPickerModel{
		list:     l,
		sessions: sorted,
	}
}

func (m SessionPickerModel) Init() tea.Cmd {
	return nil
}

func (m SessionPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keys := defaultPickerKeyMap()

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
				if si, ok := item.(pickerSessionItem); ok {
					m.result.Selected = &si.meta
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

var pickerStyle = lipgloss.NewStyle().Padding(1, 2)

func (m SessionPickerModel) View() tea.View {
	if !m.ready {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	if m.quitting {
		v := tea.NewView("")
		return v
	}

	content := pickerStyle.Render(m.list.View())
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// Result returns the picker result after the program exits.
func (m SessionPickerModel) Result() SessionPickerResult {
	return m.result
}

// PickSession runs the session picker and returns the selected session.
func PickSession(sessions []claude.SessionMeta) (*claude.SessionMeta, error) {
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions available")
	}

	model := NewSessionPickerModel(sessions)
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	result := finalModel.(SessionPickerModel).Result()
	if result.Cancelled {
		return nil, nil // User cancelled
	}
	return result.Selected, nil
}
