package target

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

type filterItem struct {
	label   string
	enabled bool
}

type filterPickerModel struct {
	items     []filterItem
	cursor    int
	cancelled bool
}

func newFilterPicker(filter ContentFilter) filterPickerModel {
	return filterPickerModel{
		items: []filterItem{
			{label: "Thinking", enabled: filter.IncludeThinking},
			{label: "Tool Calls", enabled: filter.IncludeToolUse},
			{label: "Tool Results", enabled: filter.IncludeToolResults},
			{label: "Media", enabled: filter.IncludeMedia},
			{label: "System", enabled: filter.IncludeSystem},
		},
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
	s := "\nInclude in output:\n\n"
	for i, item := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		check := "[ ]"
		if item.enabled {
			check = "[x]"
		}
		s += fmt.Sprintf("%s%s %s\n", cursor, check, item.label)
	}
	s += "\n↑/↓ move, space toggle, enter confirm, esc cancel\n"
	return tea.NewView(s)
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
