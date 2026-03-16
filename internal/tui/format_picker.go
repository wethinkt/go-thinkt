package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

type formatOption struct {
	value string
	label string
}

type formatPickerModel struct {
	options   []formatOption
	cursor    int
	cancelled bool
}

func newFormatPicker() formatPickerModel {
	return formatPickerModel{
		options: []formatOption{
			{value: "md", label: "Markdown"},
			{value: "html", label: "HTML"},
			{value: "json", label: "JSON"},
		},
	}
}

func (m formatPickerModel) Init() tea.Cmd { return nil }

func (m formatPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m formatPickerModel) View() tea.View {
	s := "\nExport format:\n\n"
	for i, opt := range m.options {
		cursor := "  "
		label := opt.label
		if i == m.cursor {
			cursor = "> "
			label = fmt.Sprintf("\033[1m%s\033[0m", label)
		}
		s += fmt.Sprintf("%s%s\n", cursor, label)
	}
	s += "\n↑/↓ to move, enter to select, esc to cancel\n"
	return tea.NewView(s)
}

// PickFormat shows a picker for export format (md, html, json).
func PickFormat() (string, error) {
	m := newFormatPicker()
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	result := final.(formatPickerModel)
	if result.cancelled {
		return "", fmt.Errorf("cancelled")
	}
	return result.options[result.cursor].value, nil
}
