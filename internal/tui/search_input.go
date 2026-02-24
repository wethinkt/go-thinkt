package tui

import (
	"fmt"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/indexer/db"
	"github.com/wethinkt/go-thinkt/internal/indexer/search"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// SearchInputResult holds the result of the search input.
type SearchInputResult struct {
	Query     string
	Cancelled bool
}

// SearchInputModel is a simple input field for entering search queries.
type SearchInputModel struct {
	input      textinput.Model
	result     SearchInputResult
	quitting   bool
	width      int
	height     int
	standalone bool
}

// NewSearchInputModel creates a new search input model.
func NewSearchInputModel() SearchInputModel {
	ti := textinput.New()
	ti.Placeholder = "Enter search query..."
	ti.Focus()
	ti.CharLimit = 156

	return SearchInputModel{
		input: ti,
	}
}

func (m SearchInputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m SearchInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			m.result.Query = m.input.Value()
			m.quitting = true
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return m.result }

		case "esc":
			m.result.Cancelled = true
			m.quitting = true
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return m.result }

		case "ctrl+c":
			m.result.Cancelled = true
			m.quitting = true
			return m, tea.Quit
		}
	}

	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m SearchInputModel) viewContent() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.Current().TextPrimary.Fg))

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Current().TextMuted.Fg))

	title := titleStyle.Render("Search Sessions")
	help := helpStyle.Render("enter: search  ·  esc: cancel")

	content := fmt.Sprintf("%s\n\n%s\n\n%s", title, m.input.View(), help)

	// Center the content
	contentWidth := 60
	contentHeight := 6

	leftPadding := (m.width - contentWidth) / 2
	if leftPadding < 0 {
		leftPadding = 0
	}
	topPadding := (m.height - contentHeight) / 2
	if topPadding < 0 {
		topPadding = 0
	}

	containerStyle := lipgloss.NewStyle().
		Padding(topPadding, leftPadding)

	return containerStyle.Render(content)
}

func (m SearchInputModel) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	return v
}

// Result returns the search input result.
func (m SearchInputModel) Result() SearchInputResult {
	return m.result
}

// PickSearchQuery runs the search input and returns the query.
func PickSearchQuery() (string, error) {
	model := NewSearchInputModel()
	model.standalone = true
	p := tea.NewProgram(model, termSizeOpts()...)
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	result := finalModel.(SearchInputModel).Result()
	if result.Cancelled {
		return "", nil
	}
	return result.Query, nil
}

// PerformSearch performs a search with the given query and returns the results.
// This is a helper function that can be used by both the indexer CLI and the TUI.
func PerformSearch(query string, opts search.SearchOptions) ([]search.SessionResult, error) {
	if query == "" {
		return nil, fmt.Errorf("empty search query")
	}

	// Set the query in options
	opts.Query = query

	// Open the database
	dbPath, err := DefaultDBPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get default DB path: %w", err)
	}

	database, err := db.OpenReadOnly(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Perform the search
	svc := search.NewService(database, nil)
	results, _, err := svc.Search(opts)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// OpenSearchPicker opens the search picker with results for the given query.
// This can be used by the main TUI when the user presses the search key.
func OpenSearchPicker(query string) (tea.Model, tea.Cmd) {
	results, err := PerformSearch(query, search.DefaultSearchOptions())
	if err != nil {
		tuilog.Log.Error("OpenSearchPicker: search failed", "error", err)
		return nil, nil
	}

	if len(results) == 0 {
		tuilog.Log.Info("OpenSearchPicker: no results found")
		return nil, nil
	}

	picker := NewSearchPickerModel(results, query)
	return picker, picker.Init()
}
