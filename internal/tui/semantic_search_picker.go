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

// semanticResultItem wraps a search.SemanticResult for the list.
type semanticResultItem struct {
	result search.SemanticResult
}

func (i semanticResultItem) Title() string {
	return i.resultTitle(80)
}

func (i semanticResultItem) Description() string {
	return ""
}

func (i semanticResultItem) FilterValue() string {
	return i.result.ProjectName + " " + i.result.SessionID + " " + i.result.Source
}

func (i semanticResultItem) resultTitle(maxLen int) string {
	if maxLen <= 0 {
		maxLen = 80
	}

	title := fmt.Sprintf("%s · %s · %s (distance: %.4f)",
		i.result.ProjectName,
		shortenID(i.result.SessionID),
		i.result.Source,
		i.result.Distance,
	)

	runes := []rune(title)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return title
}

func (i semanticResultItem) renderTitle(maxLen int, muted bool) string {
	if maxLen <= 0 {
		maxLen = 80
	}

	res := i.result

	projectPart := res.ProjectName
	sessionPart := shortenID(res.SessionID)
	distPart := fmt.Sprintf("(distance: %.4f)", res.Distance)

	sourceStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(SourceColorHex(thinkt.Source(res.Source))))
	if muted {
		sourceStyle = sourceStyle.Faint(true)
	}
	sourcePart := sourceStyle.Render(res.Source)

	title := fmt.Sprintf("%s · %s · %s %s", projectPart, sessionPart, sourcePart, distPart)
	return truncateStyled(title, maxLen)
}

func (i semanticResultItem) renderPreview(maxLen int, muted bool) string {
	res := i.result

	roleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current().TextMuted.Fg))
	if muted {
		roleStyle = roleStyle.Faint(true)
	}

	var normalStyle lipgloss.Style
	if muted {
		normalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current().TextMuted.Fg))
	} else {
		normalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current().TextSecondary.Fg))
	}

	parts := roleStyle.Render(fmt.Sprintf("[%s]:", res.Role))

	if res.FirstPrompt != "" {
		prompt := res.FirstPrompt
		if len(prompt) > maxLen-20 {
			prompt = prompt[:maxLen-20] + "..."
		}
		parts += " " + normalStyle.Render(prompt)
	}

	if res.TotalChunks > 1 {
		chunkInfo := fmt.Sprintf(" [chunk %d/%d]", res.ChunkIndex+1, res.TotalChunks)
		parts += normalStyle.Render(chunkInfo)
	}

	return parts
}

// semanticResultDelegate renders each semantic search result.
type semanticResultDelegate struct {
	normalStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	dimmedStyle   lipgloss.Style
	cursorStyle   lipgloss.Style
	sepStyle      lipgloss.Style
}

func newSemanticResultDelegate() semanticResultDelegate {
	t := theme.Current()
	return semanticResultDelegate{
		normalStyle:   lipgloss.NewStyle().PaddingLeft(4),
		selectedStyle: lipgloss.NewStyle().PaddingLeft(1).Bold(true).Foreground(lipgloss.Color(t.TextPrimary.Fg)),
		dimmedStyle:   lipgloss.NewStyle().PaddingLeft(4).Foreground(lipgloss.Color(t.TextMuted.Fg)),
		cursorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		sepStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetBorderInactive())),
	}
}

func (d semanticResultDelegate) Height() int                             { return 3 }
func (d semanticResultDelegate) Spacing() int                            { return 0 }
func (d semanticResultDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d semanticResultDelegate) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "view")),
	}
}

func (d semanticResultDelegate) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}

func (d semanticResultDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(semanticResultItem)
	if !ok {
		return
	}
	if m.Width() <= 0 {
		return
	}

	isSelected := index == m.Index()
	emptyFilter := m.FilterState() == list.Filtering && m.FilterValue() == ""

	textWidth := m.Width() - 6
	if textWidth < 20 {
		textWidth = 20
	}

	// Line 1: Title
	var titleStr string
	if emptyFilter {
		titleStr = d.dimmedStyle.Render(si.renderTitle(textWidth, true))
	} else if isSelected {
		marker := d.cursorStyle.Render(">  ")
		titleStr = marker + d.selectedStyle.Render(si.renderTitle(textWidth, false))
	} else {
		titleStr = d.normalStyle.Render(si.renderTitle(textWidth, false))
	}

	// Line 2: Preview
	previewStr := si.renderPreview(textWidth, emptyFilter && !isSelected)

	// Separator
	sepWidth := m.Width() - 6
	if sepWidth < 1 {
		sepWidth = 1
	}
	sep := d.sepStyle.Render(strings.Repeat("─", sepWidth))

	fmt.Fprintf(w, "%s\n%s\n%s", titleStr, previewStr, "    "+sep) //nolint: errcheck
}

// SemanticPickerResult holds the result of the semantic search picker.
type SemanticPickerResult struct {
	Selected  *search.SemanticResult
	Cancelled bool
}

// SemanticPickerModel is a TUI picker for semantic search results.
type SemanticPickerModel struct {
	list       list.Model
	results    []search.SemanticResult
	result     SemanticPickerResult
	quitting   bool
	width      int
	height     int
	ready      bool
	standalone bool
	query      string
}

// NewSemanticPickerModel creates a new semantic search result picker.
func NewSemanticPickerModel(results []search.SemanticResult, query string) SemanticPickerModel {
	items := make([]list.Item, len(results))
	for i, r := range results {
		items[i] = semanticResultItem{result: r}
	}

	delegate := newSemanticResultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = fmt.Sprintf("Semantic Search Results for %q (%d)", query, len(results))
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(false)

	return SemanticPickerModel{
		list:    l,
		results: results,
		query:   query,
	}
}

func (m SemanticPickerModel) Init() tea.Cmd {
	tuilog.Log.Info("SemanticPicker.Init", "resultCount", len(m.results))
	return nil
}

func (m SemanticPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keys := defaultSearchPickerKeyMap()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-2)
		m.ready = true
		return m, nil

	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, keys.Back):
			m.result.Cancelled = true
			m.quitting = true
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return m.result }

		case key.Matches(msg, keys.Quit):
			m.result.Cancelled = true
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, keys.Enter):
			if item := m.list.SelectedItem(); item != nil {
				if si, ok := item.(semanticResultItem); ok {
					m.result.Selected = &si.result
				}
			}
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

var semanticPickerStyle = lipgloss.NewStyle().Padding(1, 2)

func (m SemanticPickerModel) View() tea.View {
	var content string
	if !m.ready {
		content = "Loading..."
	} else if m.quitting {
		content = ""
	} else {
		content = semanticPickerStyle.Render(m.list.View())
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// Result returns the picker result after the program exits.
func (m SemanticPickerModel) Result() SemanticPickerResult {
	return m.result
}

// PickSemanticResult runs the semantic search result picker and returns the selected result.
func PickSemanticResult(results []search.SemanticResult, query string) (*search.SemanticResult, error) {
	if len(results) == 0 {
		return nil, fmt.Errorf("no semantic search results available")
	}

	model := NewSemanticPickerModel(results, query)
	model.standalone = true
	p := tea.NewProgram(model, termSizeOpts()...)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	result := finalModel.(SemanticPickerModel).Result()
	if result.Cancelled {
		return nil, nil
	}
	return result.Selected, nil
}
