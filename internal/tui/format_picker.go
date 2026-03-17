package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// FormatPickerResult is emitted by FormatPickerModel in embedded mode
// when the user confirms or cancels the selection.
type FormatPickerResult struct {
	Format    string
	Cancelled bool
}

type FormatOption struct {
	value string
	label string
}

type FormatPickerModel struct {
	options    []FormatOption
	cursor     int
	cancelled  bool
	standalone bool

	titleStyle    lipgloss.Style
	cursorStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	normalStyle   lipgloss.Style
	helpStyle     lipgloss.Style
}

func NewFormatPicker() FormatPickerModel {
	t := theme.Current()
	return FormatPickerModel{
		options: []FormatOption{
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

func (m FormatPickerModel) Init() tea.Cmd { return nil }

func (m FormatPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg {
				return FormatPickerResult{Cancelled: true}
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter":
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg {
				return FormatPickerResult{Format: m.options[m.cursor].value}
			}
		}
	}
	return m, nil
}

func (m FormatPickerModel) ViewContent() string {
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

	return b.String()
}

func (m FormatPickerModel) View() tea.View {
	inner := lipgloss.NewStyle().Padding(1, 2).Render(m.ViewContent())
	return tea.NewView(inner)
}

// PickFormat shows a picker for export format (md, html, json).
func PickFormat() (string, error) {
	m := NewFormatPicker()
	m.standalone = true
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	result := final.(FormatPickerModel)
	if result.cancelled {
		return "", fmt.Errorf("cancelled")
	}
	return result.options[result.cursor].value, nil
}
