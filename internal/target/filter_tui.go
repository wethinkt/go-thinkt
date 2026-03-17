package target

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// FilterPickerResult is emitted in embedded mode when the user confirms or cancels.
type FilterPickerResult struct {
	Filter    ContentFilter
	Cancelled bool
}

type filterItem struct {
	label   string
	enabled bool
}

type FilterPickerModel struct {
	items      []filterItem
	cursor     int
	cancelled  bool
	standalone bool

	titleStyle  lipgloss.Style
	cursorStyle lipgloss.Style
	checkStyle  lipgloss.Style
	labelStyle  lipgloss.Style
	mutedStyle  lipgloss.Style
	helpStyle   lipgloss.Style
}

func NewFilterPicker(filter ContentFilter) FilterPickerModel {
	t := theme.Current()
	return FilterPickerModel{
		items: []filterItem{
			{label: "Thinking", enabled: filter.IncludeThinking},
			{label: "Tool Calls", enabled: filter.IncludeToolUse},
			{label: "Tool Results", enabled: filter.IncludeToolResults},
			{label: "Media", enabled: filter.IncludeMedia},
			{label: "System", enabled: filter.IncludeSystem},
		},
		titleStyle:  lipgloss.NewStyle().Bold(true).MarginBottom(1),
		cursorStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		checkStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())),
		labelStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)),
		mutedStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		helpStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)).MarginTop(1),
	}
}

func (m FilterPickerModel) ToFilter() ContentFilter {
	return ContentFilter{
		IncludeThinking:    m.items[0].enabled,
		IncludeToolUse:     m.items[1].enabled,
		IncludeToolResults: m.items[2].enabled,
		IncludeMedia:       m.items[3].enabled,
		IncludeSystem:      m.items[4].enabled,
	}
}

func (m FilterPickerModel) Init() tea.Cmd { return nil }

func (m FilterPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg {
				return FilterPickerResult{Cancelled: true}
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "space", "x":
			m.items[m.cursor].enabled = !m.items[m.cursor].enabled
		case "enter":
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg {
				return FilterPickerResult{Filter: m.ToFilter()}
			}
		}
	}
	return m, nil
}

func (m FilterPickerModel) ViewContent() string {
	var b strings.Builder

	b.WriteString(m.titleStyle.Render("Include in output:"))
	b.WriteString("\n\n")

	for i, item := range m.items {
		if i == m.cursor {
			b.WriteString(m.cursorStyle.Render("> "))
		} else {
			b.WriteString("  ")
		}

		if item.enabled {
			b.WriteString(m.checkStyle.Render("[x]"))
		} else {
			b.WriteString(m.mutedStyle.Render("[ ]"))
		}

		label := item.label
		if i == m.cursor {
			b.WriteString(" " + m.labelStyle.Render(label))
		} else {
			b.WriteString(" " + m.mutedStyle.Render(label))
		}
		b.WriteString("\n")
	}

	b.WriteString(m.helpStyle.Render("↑/↓ move • space toggle • enter confirm • esc cancel"))
	b.WriteString("\n")

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m FilterPickerModel) View() tea.View {
	return tea.NewView(m.ViewContent())
}

// PickContentFilter shows an interactive checklist for selecting content types.
func PickContentFilter(defaults ContentFilter) (ContentFilter, error) {
	m := NewFilterPicker(defaults)
	m.standalone = true
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return ContentFilter{}, err
	}
	result := final.(FilterPickerModel)
	if result.cancelled {
		return ContentFilter{}, fmt.Errorf("cancelled")
	}
	return result.ToFilter(), nil
}
