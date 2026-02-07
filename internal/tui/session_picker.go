package tui

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// pickerSessionItem wraps a thinkt.SessionMeta for the list.
type pickerSessionItem struct {
	meta thinkt.SessionMeta
}

func (i pickerSessionItem) Title() string {
	return i.sessionTitle(80)
}

func (i pickerSessionItem) Description() string { return "" }

func (i pickerSessionItem) FilterValue() string {
	return i.meta.FirstPrompt + " " + i.meta.ID + " " + string(i.meta.Source)
}

// sessionTitle returns the first prompt truncated to maxLen, or the session ID.
func (i pickerSessionItem) sessionTitle(maxLen int) string {
	if maxLen <= 0 {
		maxLen = 80
	}
	if i.meta.FirstPrompt != "" {
		text := i.meta.FirstPrompt
		if len(text) > maxLen {
			text = text[:maxLen] + "..."
		}
		return text
	}
	if len(i.meta.ID) > 8 {
		return i.meta.ID[:8]
	}
	return i.meta.ID
}

func formatFileSize(size int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
	)
	switch {
	case size >= MB:
		return fmt.Sprintf("%.1fMB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.0fKB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%dB", size)
	}
}

// sessionDelegate renders each session as two lines plus a separator.
// Line 1: first prompt (as much as fits)
// Line 2: size, time ago, source, model, ID
// Line 3: separator
type sessionDelegate struct {
	normalStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	dimmedStyle   lipgloss.Style
	mutedStyle    lipgloss.Style
	cursorStyle   lipgloss.Style
	sepStyle      lipgloss.Style
}

func newSessionDelegate() sessionDelegate {
	t := theme.Current()
	return sessionDelegate{
		normalStyle:   lipgloss.NewStyle().PaddingLeft(4),
		selectedStyle: lipgloss.NewStyle().PaddingLeft(1).Bold(true).Foreground(lipgloss.Color(t.TextPrimary.Fg)),
		dimmedStyle:   lipgloss.NewStyle().PaddingLeft(4).Foreground(lipgloss.Color(t.TextMuted.Fg)),
		mutedStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		cursorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		sepStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetBorderInactive())),
	}
}

func (d sessionDelegate) Height() int                             { return 3 }
func (d sessionDelegate) Spacing() int                            { return 0 }
func (d sessionDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// ShortHelp returns key bindings for the help bar.
func (d sessionDelegate) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sources")),
	}
}

// FullHelp returns key bindings for the full help view.
func (d sessionDelegate) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}

func (d sessionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(pickerSessionItem)
	if !ok {
		return
	}
	if m.Width() <= 0 {
		return
	}

	isSelected := index == m.Index()
	emptyFilter := m.FilterState() == list.Filtering && m.FilterValue() == ""
	meta := si.meta

	// Available width for text (account for padding/cursor)
	textWidth := m.Width() - 6
	if textWidth < 20 {
		textWidth = 20
	}

	title := si.sessionTitle(textWidth)

	// Build detail parts for line 2
	var detailParts []string
	if meta.FileSize > 0 {
		detailParts = append(detailParts, formatFileSize(meta.FileSize))
	}
	ts := meta.ModifiedAt
	if ts.IsZero() {
		ts = meta.CreatedAt
	}
	if !ts.IsZero() {
		detailParts = append(detailParts, relativeDate(ts))
	}
	if meta.Source != "" {
		sourceStr := lipgloss.NewStyle().
			Foreground(lipgloss.Color(SourceColorHex(meta.Source))).
			Render(string(meta.Source))
		detailParts = append(detailParts, sourceStr)
	}
	if meta.Model != "" {
		detailParts = append(detailParts, meta.Model)
	}
	if meta.ID != "" {
		id := meta.ID
		if len(id) > 8 {
			id = id[:8]
		}
		detailParts = append(detailParts, id)
	}
	detailStr := strings.Join(detailParts, "  ")

	// Separator line
	sepWidth := m.Width() - 6
	if sepWidth < 1 {
		sepWidth = 1
	}
	sep := d.sepStyle.Render(strings.Repeat("─", sepWidth))

	// Render based on state
	if emptyFilter {
		line1 := d.dimmedStyle.Render(title)
		line2 := d.dimmedStyle.Render(detailStr)
		fmt.Fprintf(w, "%s\n%s\n%s", line1, line2, "    "+sep) //nolint: errcheck
	} else if isSelected {
		marker := d.cursorStyle.Render(">  ")
		line1 := marker + d.selectedStyle.Render(title)
		line2 := "    " + d.mutedStyle.Render(detailStr)
		fmt.Fprintf(w, "%s\n%s\n%s", line1, line2, "    "+sep) //nolint: errcheck
	} else {
		line1 := d.normalStyle.Render(title)
		line2 := d.normalStyle.Render(d.mutedStyle.Render(detailStr))
		fmt.Fprintf(w, "%s\n%s\n%s", line1, line2, "    "+sep) //nolint: errcheck
	}
}

// SessionPickerResult holds the result of the session picker.
type SessionPickerResult struct {
	Selected  *thinkt.SessionMeta
	Cancelled bool
}

// SessionPickerModel is a standalone session picker TUI.
type SessionPickerModel struct {
	list         list.Model
	allSessions  []thinkt.SessionMeta // unfiltered sessions
	sessions     []thinkt.SessionMeta // currently displayed (after filter)
	result       SessionPickerResult
	quitting     bool
	width        int
	height       int
	ready        bool
	standalone   bool // true when run via PickSession(), false when embedded in Shell
	sourceFilter []thinkt.Source
	showSources  bool
	sourcePicker SourcePickerModel
}

type pickerKeyMap struct {
	Enter   key.Binding
	Back    key.Binding
	Quit    key.Binding
	Sources key.Binding
}

func defaultPickerKeyMap() pickerKeyMap {
	return pickerKeyMap{
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Sources: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "sources"),
		),
	}
}

// NewSessionPickerModel creates a new session picker with sessions sorted by newest first.
// sourceFilter may be nil (show all sources).
func NewSessionPickerModel(sessions []thinkt.SessionMeta, sourceFilter []thinkt.Source) SessionPickerModel {
	filtered := filterSessionsBySource(sessions, sourceFilter)
	sortSessionsByDate(filtered)

	items := make([]list.Item, len(filtered))
	for i, s := range filtered {
		items[i] = pickerSessionItem{meta: s}
	}

	delegate := newSessionDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = sessionPickerTitle(len(filtered), sourceFilter)
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)

	return SessionPickerModel{
		list:         l,
		allSessions:  sessions,
		sessions:     filtered,
		sourceFilter: sourceFilter,
	}
}

func sessionPickerTitle(count int, sourceFilter []thinkt.Source) string {
	title := fmt.Sprintf("Select a Session (%d)", count)
	if len(sourceFilter) > 0 {
		names := make([]string, len(sourceFilter))
		for i, s := range sourceFilter {
			names[i] = string(s)
		}
		title += " · " + strings.Join(names, ",")
	}
	return title
}

func filterSessionsBySource(sessions []thinkt.SessionMeta, filter []thinkt.Source) []thinkt.SessionMeta {
	if len(filter) == 0 {
		result := make([]thinkt.SessionMeta, len(sessions))
		copy(result, sessions)
		return result
	}
	allowed := make(map[thinkt.Source]bool, len(filter))
	for _, s := range filter {
		allowed[s] = true
	}
	var result []thinkt.SessionMeta
	for _, s := range sessions {
		if allowed[s.Source] {
			result = append(result, s)
		}
	}
	return result
}

func sortSessionsByDate(sessions []thinkt.SessionMeta) {
	sort.Slice(sessions, func(i, j int) bool {
		ti := sessions[i].ModifiedAt
		if ti.IsZero() {
			ti = sessions[i].CreatedAt
		}
		tj := sessions[j].ModifiedAt
		if tj.IsZero() {
			tj = sessions[j].CreatedAt
		}
		return ti.After(tj)
	})
}

// rebuildAndRefresh re-filters from allSessions and updates the list.
func (m *SessionPickerModel) rebuildAndRefresh() tea.Cmd {
	m.sessions = filterSessionsBySource(m.allSessions, m.sourceFilter)
	sortSessionsByDate(m.sessions)

	items := make([]list.Item, len(m.sessions))
	for i, s := range m.sessions {
		items[i] = pickerSessionItem{meta: s}
	}
	m.list.Title = sessionPickerTitle(len(m.sessions), m.sourceFilter)
	return m.list.SetItems(items)
}

func (m SessionPickerModel) Init() tea.Cmd {
	tuilog.Log.Info("SessionPicker.Init", "sessionCount", len(m.sessions))
	return nil
}

func (m SessionPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Source picker overlay
	if m.showSources {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			updated, cmd := m.sourcePicker.Update(msg)
			m.sourcePicker = updated.(SourcePickerModel)

			if m.sourcePicker.quitting {
				m.showSources = false
				result := m.sourcePicker.Result()
				if !result.Cancelled {
					m.sourceFilter = result.Sources
					rebuildCmd := m.rebuildAndRefresh()
					return m, tea.Batch(cmd, rebuildCmd)
				}
			}
			return m, cmd

		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.list.SetSize(msg.Width, msg.Height-2)
			updated, cmd := m.sourcePicker.Update(msg)
			m.sourcePicker = updated.(SourcePickerModel)
			return m, cmd
		}
		return m, nil
	}

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
		case key.Matches(msg, keys.Back):
			tuilog.Log.Info("SessionPicker.Update: Back key pressed")
			m.result.Cancelled = true
			m.quitting = true
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return m.result }

		case key.Matches(msg, keys.Quit):
			tuilog.Log.Info("SessionPicker.Update: Quit key pressed")
			m.result.Cancelled = true
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, keys.Sources):
			// Build source options from allSessions
			seen := make(map[thinkt.Source]bool)
			for _, s := range m.allSessions {
				if s.Source != "" {
					seen[s.Source] = true
				}
			}
			selectedSet := make(map[thinkt.Source]bool)
			for _, s := range m.sourceFilter {
				selectedSet[s] = true
			}
			if len(m.sourceFilter) == 0 {
				for s := range seen {
					selectedSet[s] = true
				}
			}

			allSources := []thinkt.Source{
				thinkt.SourceClaude,
				thinkt.SourceKimi,
				thinkt.SourceGemini,
				thinkt.SourceCopilot,
			}
			var options []SourceOption
			for _, s := range allSources {
				options = append(options, SourceOption{
					Source:   s,
					Enabled:  seen[s],
					Selected: selectedSet[s],
				})
			}

			m.sourcePicker = NewSourcePickerModel(options, true)
			m.sourcePicker.width = m.width
			m.sourcePicker.height = m.height
			m.showSources = true
			return m, nil

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
			tuilog.Log.Info("SessionPicker.Update: returning result")
			if m.standalone {
				m.quitting = true
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

	// Source picker overlay
	if m.showSources {
		return m.sourcePicker.View()
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

	model := NewSessionPickerModel(sessions, nil)
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
