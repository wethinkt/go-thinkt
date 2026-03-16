package target

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

type filterItem struct {
	label   string
	enabled bool
}

type filterPickerModel struct {
	items     []filterItem
	cursor    int
	cancelled bool

	titleStyle  lipgloss.Style
	cursorStyle lipgloss.Style
	checkStyle  lipgloss.Style
	labelStyle  lipgloss.Style
	mutedStyle  lipgloss.Style
	helpStyle   lipgloss.Style
}

func newFilterPicker(filter ContentFilter) filterPickerModel {
	t := theme.Current()
	return filterPickerModel{
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

func (m filterPickerModel) toFilter() ContentFilter {
	return ContentFilter{
		IncludeThinking:    m.items[0].enabled,
		IncludeToolUse:     m.items[1].enabled,
		IncludeToolResults: m.items[2].enabled,
		IncludeMedia:       m.items[3].enabled,
		IncludeSystem:      m.items[4].enabled,
	}
}

func (m filterPickerModel) Init() tea.Cmd { return nil }

func (m filterPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case " ", "x":
			m.items[m.cursor].enabled = !m.items[m.cursor].enabled
		case "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m filterPickerModel) View() tea.View {
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

	inner := lipgloss.NewStyle().Padding(1, 2).Render(b.String())
	return tea.NewView(inner)
}

// PickContentFilter shows an interactive checklist for selecting content types.
func PickContentFilter(defaults ContentFilter) (ContentFilter, error) {
	m := newFilterPicker(defaults)
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return ContentFilter{}, err
	}
	result := final.(filterPickerModel)
	if result.cancelled {
		return ContentFilter{}, fmt.Errorf("cancelled")
	}
	return result.toFilter(), nil
}
