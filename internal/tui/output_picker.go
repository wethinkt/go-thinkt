package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// OutputChoice represents the user's output destination selection.
type OutputChoice struct {
	// Mode is "file", "picker", or "stdout".
	Mode string
	// Path is the selected file path (for "file" and "picker" modes).
	Path string
}

// --- output destination picker ---

// OutputPickerResult is emitted by OutputPickerModel in embedded mode.
type OutputPickerResult struct {
	Cursor    int
	Cancelled bool
}

type OutputPickerModel struct {
	options    []string
	cursor     int
	cancelled  bool
	standalone bool

	titleStyle    lipgloss.Style
	cursorStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	normalStyle   lipgloss.Style
	helpStyle     lipgloss.Style
}

func NewOutputPicker() OutputPickerModel {
	t := theme.Current()
	return OutputPickerModel{
		options: []string{"Enter filename", "Browse...", "stdout"},
		titleStyle:    lipgloss.NewStyle().Bold(true).MarginBottom(1),
		cursorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		selectedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true),
		normalStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		helpStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)).MarginTop(1),
	}
}

func (m OutputPickerModel) Init() tea.Cmd { return nil }

func (m OutputPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return OutputPickerResult{Cancelled: true} }
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
			cursor := m.cursor
			return m, func() tea.Msg { return OutputPickerResult{Cursor: cursor} }
		}
	}
	return m, nil
}

func (m OutputPickerModel) ViewContent() string {
	var b strings.Builder

	b.WriteString(m.titleStyle.Render("Output destination:"))
	b.WriteString("\n\n")

	for i, opt := range m.options {
		if i == m.cursor {
			b.WriteString(m.cursorStyle.Render("> "))
			b.WriteString(m.selectedStyle.Render(opt))
		} else {
			b.WriteString("  ")
			b.WriteString(m.normalStyle.Render(opt))
		}
		b.WriteString("\n")
	}

	b.WriteString(m.helpStyle.Render("↑/↓ to move • enter to select • esc to cancel"))
	b.WriteString("\n")

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m OutputPickerModel) View() tea.View {
	return tea.NewView(m.ViewContent())
}

// --- filename input ---

// FilenameInputResult is emitted by FilenameInputModel in embedded mode.
type FilenameInputResult struct {
	Value     string
	Cancelled bool
}

type FilenameInputModel struct {
	value      string
	cursor     int
	cancelled  bool
	standalone bool

	promptStyle lipgloss.Style
	inputStyle  lipgloss.Style
	helpStyle   lipgloss.Style
}

func NewFilenameInput(suggestion string) FilenameInputModel {
	t := theme.Current()
	return FilenameInputModel{
		value:       suggestion,
		cursor:      len(suggestion),
		promptStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true),
		inputStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())),
		helpStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)).MarginTop(1),
	}
}

func (m FilenameInputModel) Init() tea.Cmd { return nil }

func (m FilenameInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return FilenameInputResult{Cancelled: true} }
		case "enter":
			if m.standalone {
				return m, tea.Quit
			}
			value := m.value
			return m, func() tea.Msg { return FilenameInputResult{Value: value} }
		case "backspace":
			if m.cursor > 0 {
				m.value = m.value[:m.cursor-1] + m.value[m.cursor:]
				m.cursor--
			}
		case "left":
			if m.cursor > 0 {
				m.cursor--
			}
		case "right":
			if m.cursor < len(m.value) {
				m.cursor++
			}
		default:
			if len(msg.String()) == 1 {
				m.value = m.value[:m.cursor] + msg.String() + m.value[m.cursor:]
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m FilenameInputModel) ViewContent() string {
	display := m.inputStyle.Render(m.value[:m.cursor])
	if m.cursor < len(m.value) {
		display += lipgloss.NewStyle().Reverse(true).Render(string(m.value[m.cursor]))
		display += m.inputStyle.Render(m.value[m.cursor+1:])
	} else {
		display += lipgloss.NewStyle().Reverse(true).Render(" ")
	}

	var b strings.Builder
	b.WriteString(m.promptStyle.Render("Filename: "))
	b.WriteString(display)
	b.WriteString("\n")
	b.WriteString(m.helpStyle.Render("enter to confirm • esc to cancel"))
	b.WriteString("\n")

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m FilenameInputModel) View() tea.View {
	return tea.NewView(m.ViewContent())
}

// --- file browser ---

// FileBrowserResult is emitted by FileBrowserModel in embedded mode.
type FileBrowserResult struct {
	Dir       string
	Cancelled bool
}

type FileBrowserModel struct {
	dir        string
	entries    []os.DirEntry
	cursor     int
	cancelled  bool
	selected   string
	standalone bool

	titleStyle  lipgloss.Style
	cursorStyle lipgloss.Style
	dirStyle    lipgloss.Style
	fileStyle   lipgloss.Style
	mutedStyle  lipgloss.Style
	helpStyle   lipgloss.Style
}

func NewFileBrowser() FileBrowserModel {
	t := theme.Current()
	dir, _ := os.Getwd()
	m := FileBrowserModel{
		dir:         dir,
		titleStyle:  lipgloss.NewStyle().Bold(true).MarginBottom(1),
		cursorStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		dirStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		fileStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		mutedStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)),
		helpStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)).MarginTop(1),
	}
	m.loadDir()
	return m
}

func (m *FileBrowserModel) loadDir() {
	entries, _ := os.ReadDir(m.dir)
	// Filter to only directories and writable locations
	m.entries = nil
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			m.entries = append(m.entries, e)
		}
	}
	m.cursor = 0
}

func (m FileBrowserModel) Init() tea.Cmd { return nil }

func (m FileBrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return FileBrowserResult{Cancelled: true} }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.entries) { // +1 for ".."
				m.cursor++
			}
		case "enter":
			if m.cursor == 0 {
				// ".." — go up
				m.dir = filepath.Dir(m.dir)
				m.loadDir()
				return m, nil
			}
			idx := m.cursor - 1
			if idx < len(m.entries) && m.entries[idx].IsDir() {
				m.dir = filepath.Join(m.dir, m.entries[idx].Name())
				m.loadDir()
				return m, nil
			}
		case "s":
			// Select current directory
			m.selected = m.dir
			if m.standalone {
				return m, tea.Quit
			}
			dir := m.dir
			return m, func() tea.Msg { return FileBrowserResult{Dir: dir} }
		}
	}
	return m, nil
}

func (m FileBrowserModel) ViewContent() string {
	var b strings.Builder

	b.WriteString(m.titleStyle.Render("Select directory:"))
	b.WriteString("\n")
	b.WriteString(m.mutedStyle.Render(m.dir))
	b.WriteString("\n\n")

	// ".." entry
	if m.cursor == 0 {
		b.WriteString(m.cursorStyle.Render("> "))
		b.WriteString(m.dirStyle.Render(".."))
	} else {
		b.WriteString("  ")
		b.WriteString(m.mutedStyle.Render(".."))
	}
	b.WriteString("\n")

	for i, entry := range m.entries {
		if i+1 == m.cursor {
			b.WriteString(m.cursorStyle.Render("> "))
			b.WriteString(m.dirStyle.Render(entry.Name() + "/"))
		} else {
			b.WriteString("  ")
			b.WriteString(m.fileStyle.Render(entry.Name() + "/"))
		}
		b.WriteString("\n")
	}

	b.WriteString(m.helpStyle.Render("↑/↓ move • enter open dir • s select here • esc cancel"))
	b.WriteString("\n")

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m FileBrowserModel) View() tea.View {
	return tea.NewView(m.ViewContent())
}

// PickOutput shows a three-option picker for output destination, then handles
// the sub-flow for each option. Returns the chosen output path ("" for stdout).
// The formatExt should include the dot, e.g. ".md", ".html", ".json".
func PickOutput(suggestedName string) (*OutputChoice, error) {
	// Step 1: Pick destination type
	m := NewOutputPicker()
	m.standalone = true
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	result := final.(OutputPickerModel)
	if result.cancelled {
		return nil, fmt.Errorf("cancelled")
	}

	switch result.cursor {
	case 0: // Enter filename
		fm := NewFilenameInput(suggestedName)
		fm.standalone = true
		fp := tea.NewProgram(fm)
		fFinal, err := fp.Run()
		if err != nil {
			return nil, err
		}
		fResult := fFinal.(FilenameInputModel)
		if fResult.cancelled {
			return nil, fmt.Errorf("cancelled")
		}
		return &OutputChoice{Mode: "file", Path: fResult.value}, nil

	case 1: // Browse...
		bm := NewFileBrowser()
		bm.standalone = true
		bp := tea.NewProgram(bm)
		bFinal, err := bp.Run()
		if err != nil {
			return nil, err
		}
		bResult := bFinal.(FileBrowserModel)
		if bResult.cancelled {
			return nil, fmt.Errorf("cancelled")
		}
		if bResult.selected == "" {
			return nil, fmt.Errorf("cancelled")
		}
		// After selecting a directory, prompt for filename
		suggestion := filepath.Join(bResult.selected, suggestedName)
		fm := NewFilenameInput(suggestion)
		fm.standalone = true
		fp := tea.NewProgram(fm)
		fFinal, err := fp.Run()
		if err != nil {
			return nil, err
		}
		fResult := fFinal.(FilenameInputModel)
		if fResult.cancelled {
			return nil, fmt.Errorf("cancelled")
		}
		return &OutputChoice{Mode: "file", Path: fResult.value}, nil

	default: // stdout
		return &OutputChoice{Mode: "stdout"}, nil
	}
}
