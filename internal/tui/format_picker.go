package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

type formatOption struct {
	value string
	label string
}

type formatPickerModel struct {
	options   []formatOption
	cursor    int
	cancelled bool

	titleStyle    lipgloss.Style
	cursorStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	normalStyle   lipgloss.Style
	helpStyle     lipgloss.Style
}

func newFormatPicker() formatPickerModel {
	t := theme.Current()
	return formatPickerModel{
		options: []formatOption{
			{value: "md", label: "Markdown"},
			{value: "html", label: "HTML"},
			{value: "json", label: "JSON"},
		},
		titleStyle:    lipgloss.NewStyle().Bold(true).MarginBottom(1),
		cursorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		selectedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true),
		normalStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		helpStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)).MarginTop(1),
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
	var b strings.Builder

	b.WriteString(m.titleStyle.Render("Export format:"))
	b.WriteString("\n\n")

	for i, opt := range m.options {
		if i == m.cursor {
			b.WriteString(m.cursorStyle.Render("> "))
			b.WriteString(m.selectedStyle.Render(opt.label))
		} else {
			b.WriteString("  ")
			b.WriteString(m.normalStyle.Render(opt.label))
		}
		b.WriteString("\n")
	}

	b.WriteString(m.helpStyle.Render("↑/↓ to move • enter to select • esc to cancel"))
	b.WriteString("\n")

	inner := lipgloss.NewStyle().Padding(1, 2).Render(b.String())
	return tea.NewView(inner)
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
