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
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tuilog"
)

// pickerSessionItem wraps a thinkt.SessionMeta for the picker list.
type pickerSessionItem struct {
	meta thinkt.SessionMeta
}

func (i pickerSessionItem) Title() string {
	if i.meta.FirstPrompt != "" {
		text := i.meta.FirstPrompt
		if len(text) > 70 {
			text = text[:70] + "..."
		}
		return text
	}
	if len(i.meta.ID) > 8 {
		return i.meta.ID[:8]
	}
	return i.meta.ID
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
	if !i.meta.ModifiedAt.IsZero() {
		parts = append(parts, i.meta.ModifiedAt.Local().Format("Jan 02, 3:04 PM"))
	} else if !i.meta.CreatedAt.IsZero() {
		parts = append(parts, i.meta.CreatedAt.Local().Format("Jan 02, 3:04 PM"))
	}

	// File size
	if i.meta.FileSize > 0 {
		parts = append(parts, formatFileSize(i.meta.FileSize))
	}

	// Source indicator
	if i.meta.Source != "" {
		parts = append(parts, string(i.meta.Source))
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
	return i.meta.FirstPrompt + " " + i.meta.ID + " " + string(i.meta.Source)
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
	Selected  *thinkt.SessionMeta
	Cancelled bool
}

// SessionPickerModel is a standalone session picker TUI.
type SessionPickerModel struct {
	list       list.Model
	sessions   []thinkt.SessionMeta
	result     SessionPickerResult
	quitting   bool
	width      int
	height     int
	ready      bool
	standalone bool // true when run via PickSession(), false when embedded in Shell
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
func NewSessionPickerModel(sessions []thinkt.SessionMeta) SessionPickerModel {
	// Sort by modified time descending (newest first)
	sorted := make([]thinkt.SessionMeta, len(sessions))
	copy(sorted, sessions)
	sort.Slice(sorted, func(i, j int) bool {
		ti := sorted[i].ModifiedAt
		if ti.IsZero() {
			ti = sorted[i].CreatedAt
		}
		tj := sorted[j].ModifiedAt
		if tj.IsZero() {
			tj = sorted[j].CreatedAt
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
	tuilog.Log.Info("SessionPicker.Init", "sessionCount", len(m.sessions))
	return nil
}

func (m SessionPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keys := defaultPickerKeyMap()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		tuilog.Log.Info("SessionPicker.Update: WindowSizeMsg", "width", msg.Width, "height", msg.Height)
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
			tuilog.Log.Info("SessionPicker.Update: Quit key pressed")
			m.result.Cancelled = true
			m.quitting = true
			// In standalone mode, quit the program; in Shell mode, return result for navigation
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return m.result }

		case key.Matches(msg, keys.Enter):
			tuilog.Log.Info("SessionPicker.Update: Enter key pressed")
			if item := m.list.SelectedItem(); item != nil {
				if si, ok := item.(pickerSessionItem); ok {
					tuilog.Log.Info("SessionPicker.Update: session selected", "sessionID", si.meta.ID)
					m.result.Selected = &si.meta
				} else {
					tuilog.Log.Error("SessionPicker.Update: selected item is not a pickerSessionItem", "type", fmt.Sprintf("%T", item))
				}
			} else {
				tuilog.Log.Warn("SessionPicker.Update: no item selected")
			}
			m.quitting = true
			tuilog.Log.Info("SessionPicker.Update: returning result")
			// In standalone mode, quit the program; in Shell mode, return result for navigation
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return m.result }
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
func PickSession(sessions []thinkt.SessionMeta) (*thinkt.SessionMeta, error) {
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions available")
	}

	model := NewSessionPickerModel(sessions)
	model.standalone = true // Mark as standalone so it returns tea.Quit
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
