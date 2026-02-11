package tui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/indexer/search"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// searchResultItem wraps a search.SessionResult for the list.
type searchResultItem struct {
	result search.SessionResult
}

func (i searchResultItem) Title() string {
	return i.resultTitle(80)
}

func (i searchResultItem) Description() string {
	return ""
}

func (i searchResultItem) FilterValue() string {
	return i.result.ProjectName + " " + i.result.SessionID + " " + i.result.Source
}

// resultTitle returns a formatted title for the search result.
func (i searchResultItem) resultTitle(maxLen int) string {
	if maxLen <= 0 {
		maxLen = 80
	}

	// Format: "ProjectName · SessionID (N matches)"
	matches := len(i.result.Matches)
	title := fmt.Sprintf("%s · %s (%d %s)",
		i.result.ProjectName,
		shortenID(i.result.SessionID),
		matches,
		pluralize("match", "matches", matches),
	)

	runes := []rune(title)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return title
}

func shortenID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func pluralize(singular, plural string, count int) string {
	if count == 1 {
		return singular
	}
	return plural
}

// searchResultDelegate renders each search result.
type searchResultDelegate struct {
	normalStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	dimmedStyle   lipgloss.Style
	mutedStyle    lipgloss.Style
	cursorStyle   lipgloss.Style
	sepStyle      lipgloss.Style
	sourceStyle   func(string) lipgloss.Style
}

func newSearchResultDelegate() searchResultDelegate {
	t := theme.Current()
	return searchResultDelegate{
		normalStyle:   lipgloss.NewStyle().PaddingLeft(4),
		selectedStyle: lipgloss.NewStyle().PaddingLeft(1).Bold(true).Foreground(lipgloss.Color(t.TextPrimary.Fg)),
		dimmedStyle:   lipgloss.NewStyle().PaddingLeft(4).Foreground(lipgloss.Color(t.TextMuted.Fg)),
		mutedStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		cursorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		sepStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetBorderInactive())),
		sourceStyle: func(source string) lipgloss.Style {
			return lipgloss.NewStyle().
				Foreground(lipgloss.Color(SourceColorHex(thinkt.Source(source))))
		},
	}
}

func (d searchResultDelegate) Height() int                             { return 4 }
func (d searchResultDelegate) Spacing() int                            { return 0 }
func (d searchResultDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// ShortHelp returns key bindings for the help bar.
func (d searchResultDelegate) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "view")),
	}
}

// FullHelp returns key bindings for the full help view.
func (d searchResultDelegate) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}

func (d searchResultDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(searchResultItem)
	if !ok {
		return
	}
	if m.Width() <= 0 {
		return
	}

	isSelected := index == m.Index()
	emptyFilter := m.FilterState() == list.Filtering && m.FilterValue() == ""
	res := si.result

	// Available width for text (account for padding/cursor)
	textWidth := m.Width() - 6
	if textWidth < 20 {
		textWidth = 20
	}

	// Line 1: Title (Project · SessionID)
	title := si.resultTitle(textWidth)

	// Line 2: Source and match count
	sourceStr := d.sourceStyle(res.Source).Render(res.Source)
	metaStr := fmt.Sprintf("%s  ·  %d %s", sourceStr, len(res.Matches), pluralize("match", "matches", len(res.Matches)))

	// Line 3: First match preview (if any)
	previewStr := ""
	if len(res.Matches) > 0 {
		preview := res.Matches[0].Preview
		if len(preview) > textWidth-4 {
			preview = preview[:textWidth-4] + "..."
		}
		previewStr = d.mutedStyle.Render(fmt.Sprintf("[%s]: %s", res.Matches[0].Role, preview))
	}

	// Separator line
	sepWidth := m.Width() - 6
	if sepWidth < 1 {
		sepWidth = 1
	}
	sep := d.sepStyle.Render(strings.Repeat("─", sepWidth))

	// Render based on state
	if emptyFilter {
		line1 := d.dimmedStyle.Render(title)
		line2 := d.dimmedStyle.Render(metaStr)
		fmt.Fprintf(w, "%s\n%s\n%s\n%s", line1, line2, previewStr, "    "+sep) //nolint: errcheck
	} else if isSelected {
		marker := d.cursorStyle.Render(">  ")
		line1 := marker + d.selectedStyle.Render(title)
		line2 := "    " + d.mutedStyle.Render(metaStr)
		fmt.Fprintf(w, "%s\n%s\n%s\n%s", line1, line2, previewStr, "    "+sep) //nolint: errcheck
	} else {
		line1 := d.normalStyle.Render(title)
		line2 := d.normalStyle.Render(d.mutedStyle.Render(metaStr))
		fmt.Fprintf(w, "%s\n%s\n%s\n%s", line1, line2, previewStr, "    "+sep) //nolint: errcheck
	}
}

// SearchPickerResult holds the result of the search picker.
type SearchPickerResult struct {
	Selected  *search.SessionResult
	Cancelled bool
}

// SearchPickerModel is a TUI picker for search results.
type SearchPickerModel struct {
	list       list.Model
	results    []search.SessionResult
	result     SearchPickerResult
	quitting   bool
	width      int
	height     int
	ready      bool
	standalone bool // true when run via PickSearchResult(), false when embedded in Shell
	query      string // the search query (for display)
}

// searchPickerKeyMap defines key bindings for the search picker.
type searchPickerKeyMap struct {
	Enter key.Binding
	Back  key.Binding
	Quit  key.Binding
}

func defaultSearchPickerKeyMap() searchPickerKeyMap {
	return searchPickerKeyMap{
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "view session"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// NewSearchPickerModel creates a new search result picker.
func NewSearchPickerModel(results []search.SessionResult, query string) SearchPickerModel {
	items := make([]list.Item, len(results))
	for i, r := range results {
		items[i] = searchResultItem{result: r}
	}

	delegate := newSearchResultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = searchPickerTitle(len(results), query)
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)

	return SearchPickerModel{
		list:    l,
		results: results,
		query:   query,
	}
}

func searchPickerTitle(count int, query string) string {
	if query != "" {
		return fmt.Sprintf("Search Results for %q (%d)", query, count)
	}
	return fmt.Sprintf("Search Results (%d)", count)
}

func (m SearchPickerModel) Init() tea.Cmd {
	tuilog.Log.Info("SearchPicker.Init", "resultCount", len(m.results))
	return nil
}

func (m SearchPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keys := defaultSearchPickerKeyMap()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		tuilog.Log.Info("SearchPicker.Update: WindowSizeMsg", "width", msg.Width, "height", msg.Height)
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
			tuilog.Log.Info("SearchPicker.Update: Back key pressed")
			m.result.Cancelled = true
			m.quitting = true
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return m.result }

		case key.Matches(msg, keys.Quit):
			tuilog.Log.Info("SearchPicker.Update: Quit key pressed")
			m.result.Cancelled = true
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, keys.Enter):
			tuilog.Log.Info("SearchPicker.Update: Enter key pressed")
			if item := m.list.SelectedItem(); item != nil {
				if si, ok := item.(searchResultItem); ok {
					tuilog.Log.Info("SearchPicker.Update: result selected", "sessionID", si.result.SessionID)
					m.result.Selected = &si.result
				} else {
					tuilog.Log.Error("SearchPicker.Update: selected item is not a searchResultItem", "type", fmt.Sprintf("%T", item))
				}
			} else {
				tuilog.Log.Warn("SearchPicker.Update: no item selected")
			}
			tuilog.Log.Info("SearchPicker.Update: returning result")
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

var searchPickerStyle = lipgloss.NewStyle().Padding(1, 2)

func (m SearchPickerModel) View() tea.View {
	if !m.ready {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	if m.quitting {
		v := tea.NewView("")
		return v
	}

	content := searchPickerStyle.Render(m.list.View())
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// Result returns the picker result after the program exits.
func (m SearchPickerModel) Result() SearchPickerResult {
	return m.result
}

// PickSearchResult runs the search result picker and returns the selected result.
func PickSearchResult(results []search.SessionResult, query string) (*search.SessionResult, error) {
	if len(results) == 0 {
		return nil, fmt.Errorf("no search results available")
	}

	model := NewSearchPickerModel(results, query)
	model.standalone = true // Mark as standalone so it returns tea.Quit
	p := tea.NewProgram(model, termSizeOpts()...)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	result := finalModel.(SearchPickerModel).Result()
	if result.Cancelled {
		return nil, nil // User cancelled
	}
	return result.Selected, nil
}

// GetSessionPaths returns the session paths from the search results.
// Useful for feeding into MultiViewerModel.
func GetSessionPaths(results []search.SessionResult) []string {
	paths := make([]string, len(results))
	for i, r := range results {
		paths[i] = r.Path
	}
	return paths
}
