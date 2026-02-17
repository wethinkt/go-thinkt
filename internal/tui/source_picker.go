package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// SourceOption represents a source in the picker.
type SourceOption struct {
	Source   thinkt.Source
	Enabled bool // whether this source has data
	Selected bool // current selection state
}

// SourcePickerResult holds the result of the source picker.
type SourcePickerResult struct {
	Sources   []thinkt.Source
	Cancelled bool
}

// SourcePickerModel is a source selection TUI.
type SourcePickerModel struct {
	options     []SourceOption
	cursor      int
	multiSelect bool
	result      SourcePickerResult
	quitting    bool
	width       int
	height      int
	standalone  bool
}

type sourcePickerKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Toggle key.Binding
	All    key.Binding
	None   key.Binding
	Enter  key.Binding
	Quit   key.Binding
}

func defaultSourcePickerKeyMap(multi bool) sourcePickerKeyMap {
	km := sourcePickerKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm"),
		),
		Quit: key.NewBinding(
			key.WithKeys("esc", "q"),
			key.WithHelp("esc", "cancel"),
		),
	}
	if multi {
		km.Toggle = key.NewBinding(
			key.WithKeys("space"),
			key.WithHelp("space", "toggle"),
		)
		km.All = key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "all"),
		)
		km.None = key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "none"),
		)
	}
	return km
}

// NewSourcePickerModel creates a source picker.
// If multiSelect is true, space toggles and multiple sources can be selected.
// If false, enter selects the current item.
func NewSourcePickerModel(options []SourceOption, multiSelect bool) SourcePickerModel {
	// Copy options to avoid mutating caller's slice
	opts := make([]SourceOption, len(options))
	copy(opts, options)

	// Start cursor on first enabled option
	cursor := 0
	for i, o := range opts {
		if o.Enabled {
			cursor = i
			break
		}
	}

	return SourcePickerModel{
		options:     opts,
		cursor:      cursor,
		multiSelect: multiSelect,
	}
}

func (m SourcePickerModel) Init() tea.Cmd { return nil }

// SetSize updates the model's dimensions.
func (m *SourcePickerModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m SourcePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keys := defaultSourcePickerKeyMap(m.multiSelect)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Up):
			m.moveCursor(-1)

		case key.Matches(msg, keys.Down):
			m.moveCursor(1)

		case key.Matches(msg, keys.Quit):
			m.result.Cancelled = true
			m.quitting = true
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return m.result }

		case m.multiSelect && key.Matches(msg, keys.Toggle):
			if m.options[m.cursor].Enabled {
				m.options[m.cursor].Selected = !m.options[m.cursor].Selected
			}

		case m.multiSelect && key.Matches(msg, keys.All):
			for i := range m.options {
				if m.options[i].Enabled {
					m.options[i].Selected = true
				}
			}

		case m.multiSelect && key.Matches(msg, keys.None):
			for i := range m.options {
				m.options[i].Selected = false
			}

		case key.Matches(msg, keys.Enter):
			m.quitting = true
			if m.multiSelect {
				for _, o := range m.options {
					if o.Selected {
						m.result.Sources = append(m.result.Sources, o.Source)
					}
				}
			} else {
				if m.options[m.cursor].Enabled {
					m.result.Sources = []thinkt.Source{m.options[m.cursor].Source}
				}
			}
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return m.result }
		}
	}

	return m, nil
}

// moveCursor moves to the next enabled option in the given direction.
func (m *SourcePickerModel) moveCursor(dir int) {
	n := len(m.options)
	for i := 0; i < n; i++ {
		next := (m.cursor + dir + n) % n
		m.cursor = next
		if m.options[next].Enabled {
			return
		}
		dir = dir / abs(dir) // normalize to +1 or -1 for wrapping
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

var (
	spTitleStyle    lipgloss.Style
	spCursorStyle   lipgloss.Style
	spDisabledStyle lipgloss.Style
	spCheckStyle    lipgloss.Style
	spHelpStyle     lipgloss.Style
)

func init() {
	t := theme.Current()
	spTitleStyle = lipgloss.NewStyle().Bold(true).MarginBottom(1)
	spCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true)
	spDisabledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	spCheckStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent()))
	spHelpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)).MarginTop(1)
}

func (m SourcePickerModel) viewContent() string {
	var b strings.Builder

	if m.multiSelect {
		b.WriteString(spTitleStyle.Render("Filter by Source"))
	} else {
		b.WriteString(spTitleStyle.Render("Select a Source"))
	}
	b.WriteString("\n")

	for i, o := range m.options {
		isCursor := i == m.cursor

		// Cursor marker
		if isCursor {
			b.WriteString(spCursorStyle.Render("> "))
		} else {
			b.WriteString("  ")
		}

		// Checkbox (multi-select only)
		if m.multiSelect {
			if o.Selected {
				b.WriteString(spCheckStyle.Render("[x] "))
			} else {
				b.WriteString("[ ] ")
			}
		}

		// Source name with color
		name := string(o.Source)
		if !o.Enabled {
			b.WriteString(spDisabledStyle.Render(name + " (no data)"))
		} else {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(SourceColorHex(o.Source)))
			if isCursor {
				style = style.Bold(true)
			}
			b.WriteString(style.Render(name))
		}

		b.WriteString("\n")
	}

	// Help line
	if m.multiSelect {
		b.WriteString(spHelpStyle.Render("space toggle • a all • n none • enter confirm • esc cancel"))
	} else {
		b.WriteString(spHelpStyle.Render("enter select • esc cancel"))
	}

	inner := lipgloss.NewStyle().Padding(1, 3).Render(b.String())

	// Center in terminal if we have dimensions
	if m.width > 0 && m.height > 0 {
		inner = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
	}

	return inner
}

func (m SourcePickerModel) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	return v
}

// Result returns the picker result.
func (m SourcePickerModel) Result() SourcePickerResult {
	return m.result
}

// PickSource runs a single-select source picker.
func PickSource(options []SourceOption) (*thinkt.Source, error) {
	model := NewSourcePickerModel(options, false)
	model.standalone = true
	p := tea.NewProgram(model)
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	result := final.(SourcePickerModel).Result()
	if result.Cancelled || len(result.Sources) == 0 {
		return nil, nil
	}
	return &result.Sources[0], nil
}

// PickSources runs a multi-select source picker.
func PickSources(options []SourceOption) ([]thinkt.Source, error) {
	model := NewSourcePickerModel(options, true)
	model.standalone = true
	p := tea.NewProgram(model)
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	result := final.(SourcePickerModel).Result()
	if result.Cancelled {
		return nil, nil
	}
	return result.Sources, nil
}

// SourceOptionsFromRegistry builds SourceOption list from a registry.
func SourceOptionsFromRegistry(registry *thinkt.StoreRegistry, selected []thinkt.Source) []SourceOption {
	registered := make(map[thinkt.Source]bool)
	for _, s := range registry.Sources() {
		registered[s] = true
	}

	selectedSet := make(map[thinkt.Source]bool)
	for _, s := range selected {
		selectedSet[s] = true
	}

	var options []SourceOption
	for _, s := range thinkt.AllSources {
		options = append(options, SourceOption{
			Source:   s,
			Enabled:  registered[s],
			Selected: selectedSet[s],
		})
	}
	return options
}
