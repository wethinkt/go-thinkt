// Package tui provides terminal UI components.
package tui

import (
	"fmt"
	"io"
	"os"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ConfirmResult represents the outcome of a confirmation dialog.
type ConfirmResult int

const (
	ConfirmYes ConfirmResult = iota
	ConfirmNo
	ConfirmCancelled
)

// ConfirmOptions configures the confirm dialog.
type ConfirmOptions struct {
	Prompt      string    // The question to ask
	Affirmative string    // Text for yes button (default "Yes")
	Negative    string    // Text for no button (default "No")
	Default     bool      // Default selection (true = affirmative)
	Output      io.Writer // Where to write output (default os.Stdout)
}

// Confirm displays an interactive confirmation dialog and returns the result.
// This is the main entry point for using the confirm component.
func Confirm(opts ConfirmOptions) (ConfirmResult, error) {
	if opts.Affirmative == "" {
		opts.Affirmative = "Yes"
	}
	if opts.Negative == "" {
		opts.Negative = "No"
	}
	if opts.Output == nil {
		opts.Output = os.Stdout
	}

	m := newConfirmModel(opts)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return ConfirmCancelled, err
	}

	result := finalModel.(confirmModel)
	return result.result, nil
}

// confirmModel is the Bubbletea model for the confirm dialog.
type confirmModel struct {
	prompt      string
	affirmative string
	negative    string
	selection   bool // true = affirmative selected
	result      ConfirmResult
	quitting    bool
	keys        confirmKeyMap

	// Styles
	promptStyle     lipgloss.Style
	selectedStyle   lipgloss.Style
	unselectedStyle lipgloss.Style
}

type confirmKeyMap struct {
	Toggle      key.Binding
	Submit      key.Binding
	Affirmative key.Binding
	Negative    key.Binding
	Quit        key.Binding
	Abort       key.Binding
}

func defaultConfirmKeyMap(affirmative, negative string) confirmKeyMap {
	return confirmKeyMap{
		Toggle: key.NewBinding(
			key.WithKeys("left", "right", "h", "l", "tab", "shift+tab"),
			key.WithHelp("←/→", "toggle"),
		),
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "submit"),
		),
		Affirmative: key.NewBinding(
			key.WithKeys("y", "Y"),
			key.WithHelp("y", affirmative),
		),
		Negative: key.NewBinding(
			key.WithKeys("n", "N"),
			key.WithHelp("n", negative),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "esc"),
			key.WithHelp("esc", "cancel"),
		),
		Abort: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "abort"),
		),
	}
}

func newConfirmModel(opts ConfirmOptions) confirmModel {
	return confirmModel{
		prompt:      opts.Prompt,
		affirmative: opts.Affirmative,
		negative:    opts.Negative,
		selection:   opts.Default,
		result:      ConfirmCancelled,
		keys:        defaultConfirmKeyMap(opts.Affirmative, opts.Negative),

		promptStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")),
		selectedStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("212")).
			Padding(0, 2),
		unselectedStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("247")).
			Padding(0, 2),
	}
}

func (m confirmModel) Init() tea.Cmd {
	return nil
}

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Abort):
			m.result = ConfirmCancelled
			m.quitting = true
			return m, tea.Interrupt

		case key.Matches(msg, m.keys.Quit):
			m.result = ConfirmCancelled
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, m.keys.Affirmative):
			m.result = ConfirmYes
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, m.keys.Negative):
			m.result = ConfirmNo
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, m.keys.Toggle):
			m.selection = !m.selection

		case key.Matches(msg, m.keys.Submit):
			if m.selection {
				m.result = ConfirmYes
			} else {
				m.result = ConfirmNo
			}
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m confirmModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var aff, neg string
	if m.selection {
		aff = m.selectedStyle.Render(m.affirmative)
		neg = m.unselectedStyle.Render(m.negative)
	} else {
		aff = m.unselectedStyle.Render(m.affirmative)
		neg = m.selectedStyle.Render(m.negative)
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, aff, "  ", neg)

	content := fmt.Sprintf("\n%s\n\n%s\n",
		m.promptStyle.Render(m.prompt),
		buttons,
	)

	return tea.NewView(content)
}
