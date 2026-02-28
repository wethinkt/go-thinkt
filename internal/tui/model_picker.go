package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// ModelOption represents an embedding model in the picker.
type ModelOption struct {
	ID     string
	Detail string // e.g. "768-dim, mean pooling"
	Active bool   // currently configured model
}

// ModelPickerResult holds the result of the model picker.
type ModelPickerResult struct {
	Selected  string // model ID
	Cancelled bool
}

// ModelPickerModel is a model selection TUI.
type ModelPickerModel struct {
	options  []ModelOption
	cursor   int
	result   ModelPickerResult
	quitting bool

	// styles
	cursorStyle  lipgloss.Style
	activeStyle  lipgloss.Style
	normalStyle  lipgloss.Style
	detailStyle  lipgloss.Style
	helpStyle    lipgloss.Style
}

// NewModelPickerModel creates a model picker.
func NewModelPickerModel(options []ModelOption) ModelPickerModel {
	t := theme.Current()

	// Start cursor on the active model.
	cursor := 0
	for i, o := range options {
		if o.Active {
			cursor = i
			break
		}
	}

	return ModelPickerModel{
		options: options,
		cursor:  cursor,
		cursorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		activeStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextPrimary.Fg)),
		normalStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		detailStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextMuted.Fg)),
		helpStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextMuted.Fg)),
	}
}

func (m ModelPickerModel) Init() tea.Cmd { return nil }

func (m ModelPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keys := modelPickerKeyMap()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = len(m.options) - 1
			}

		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.options)-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}

		case key.Matches(msg, keys.Quit):
			m.result.Cancelled = true
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, keys.Enter):
			m.result.Selected = m.options[m.cursor].ID
			m.quitting = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m ModelPickerModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var b strings.Builder

	// Compute column width: longest ID + " (active)" tag.
	maxIDLen := 0
	for _, o := range m.options {
		if len(o.ID) > maxIDLen {
			maxIDLen = len(o.ID)
		}
	}
	activeTag := " " + thinktI18n.T("tui.modelPicker.active", "(active)")
	nameCol := maxIDLen + len(activeTag) + 1 // +1 for spacing before tag

	for i, o := range m.options {
		isCursor := i == m.cursor

		if isCursor {
			b.WriteString(m.cursorStyle.Render("> "))
		} else {
			b.WriteString("  ")
		}

		// Model name + active tag, padded to fixed column width.
		nameStr := o.ID
		if o.Active {
			nameStr += " " + thinktI18n.T("tui.modelPicker.active", "(active)")
		}
		nameStr = fmt.Sprintf("%-*s", nameCol, nameStr)
		if isCursor {
			b.WriteString(m.activeStyle.Render(nameStr))
		} else {
			b.WriteString(m.normalStyle.Render(nameStr))
		}

		// Detail
		b.WriteString("  ")
		b.WriteString(m.detailStyle.Render(o.Detail))
		b.WriteString("\n")
	}

	b.WriteString(m.helpStyle.Render("  " + thinktI18n.T("tui.modelPicker.helpText", "↑/↓ navigate • enter select • esc cancel")))

	return tea.NewView(b.String())
}

// Result returns the picker result.
func (m ModelPickerModel) Result() ModelPickerResult {
	return m.result
}

type modelPickerKeys struct {
	Up    key.Binding
	Down  key.Binding
	Enter key.Binding
	Quit  key.Binding
}

func modelPickerKeyMap() modelPickerKeys {
	return modelPickerKeys{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
		),
		Quit: key.NewBinding(
			key.WithKeys("esc", "q", "ctrl+c"),
		),
	}
}

// PickModel runs an inline model picker and returns the selected model ID.
// Returns nil if cancelled.
func PickModel(options []ModelOption) (*string, error) {
	model := NewModelPickerModel(options)
	p := tea.NewProgram(model)
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	result := final.(ModelPickerModel).Result()
	if result.Cancelled {
		return nil, nil
	}
	return &result.Selected, nil
}
