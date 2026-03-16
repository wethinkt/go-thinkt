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

type outputPickerModel struct {
	options   []string
	cursor    int
	cancelled bool

	titleStyle    lipgloss.Style
	cursorStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	normalStyle   lipgloss.Style
	helpStyle     lipgloss.Style
}

func newOutputPicker() outputPickerModel {
	t := theme.Current()
	return outputPickerModel{
		options: []string{"Enter filename", "Browse...", "stdout"},
		titleStyle:    lipgloss.NewStyle().Bold(true).MarginBottom(1),
		cursorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		selectedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true),
		normalStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		helpStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)).MarginTop(1),
	}
}

func (m outputPickerModel) Init() tea.Cmd { return nil }

func (m outputPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m outputPickerModel) View() tea.View {
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

	inner := lipgloss.NewStyle().Padding(1, 2).Render(b.String())
	return tea.NewView(inner)
}

// --- filename input ---

type filenameInputModel struct {
	value     string
	cursor    int
	cancelled bool

	promptStyle lipgloss.Style
	inputStyle  lipgloss.Style
	helpStyle   lipgloss.Style
}

func newFilenameInput(suggestion string) filenameInputModel {
	t := theme.Current()
	return filenameInputModel{
		value:       suggestion,
		cursor:      len(suggestion),
		promptStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true),
		inputStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())),
		helpStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)).MarginTop(1),
	}
}

func (m filenameInputModel) Init() tea.Cmd { return nil }

func (m filenameInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			return m, tea.Quit
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

func (m filenameInputModel) View() tea.View {
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

	inner := lipgloss.NewStyle().Padding(1, 2).Render(b.String())
	return tea.NewView(inner)
}

// --- file browser ---

type fileBrowserModel struct {
	dir       string
	entries   []os.DirEntry
	cursor    int
	cancelled bool
	selected  string

	titleStyle    lipgloss.Style
	cursorStyle   lipgloss.Style
	dirStyle      lipgloss.Style
	fileStyle     lipgloss.Style
	mutedStyle    lipgloss.Style
	helpStyle     lipgloss.Style
}

func newFileBrowser() fileBrowserModel {
	t := theme.Current()
	dir, _ := os.Getwd()
	m := fileBrowserModel{
		dir:           dir,
		titleStyle:    lipgloss.NewStyle().Bold(true).MarginBottom(1),
		cursorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		dirStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		fileStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		mutedStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)),
		helpStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)).MarginTop(1),
	}
	m.loadDir()
	return m
}

func (m *fileBrowserModel) loadDir() {
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

func (m fileBrowserModel) Init() tea.Cmd { return nil }

func (m fileBrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m fileBrowserModel) View() tea.View {
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

	inner := lipgloss.NewStyle().Padding(1, 2).Render(b.String())
	return tea.NewView(inner)
}

// PickOutput shows a three-option picker for output destination, then handles
// the sub-flow for each option. Returns the chosen output path ("" for stdout).
// The formatExt should include the dot, e.g. ".md", ".html", ".json".
func PickOutput(suggestedName string) (*OutputChoice, error) {
	// Step 1: Pick destination type
	m := newOutputPicker()
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	result := final.(outputPickerModel)
	if result.cancelled {
		return nil, fmt.Errorf("cancelled")
	}

	switch result.cursor {
	case 0: // Enter filename
		fm := newFilenameInput(suggestedName)
		fp := tea.NewProgram(fm)
		fFinal, err := fp.Run()
		if err != nil {
			return nil, err
		}
		fResult := fFinal.(filenameInputModel)
		if fResult.cancelled {
			return nil, fmt.Errorf("cancelled")
		}
		return &OutputChoice{Mode: "file", Path: fResult.value}, nil

	case 1: // Browse...
		bm := newFileBrowser()
		bp := tea.NewProgram(bm)
		bFinal, err := bp.Run()
		if err != nil {
			return nil, err
		}
		bResult := bFinal.(fileBrowserModel)
		if bResult.cancelled {
			return nil, fmt.Errorf("cancelled")
		}
		if bResult.selected == "" {
			return nil, fmt.Errorf("cancelled")
		}
		// After selecting a directory, prompt for filename
		suggestion := filepath.Join(bResult.selected, suggestedName)
		fm := newFilenameInput(suggestion)
		fp := tea.NewProgram(fm)
		fFinal, err := fp.Run()
		if err != nil {
			return nil, err
		}
		fResult := fFinal.(filenameInputModel)
		if fResult.cancelled {
			return nil, fmt.Errorf("cancelled")
		}
		return &OutputChoice{Mode: "file", Path: fResult.value}, nil

	default: // stdout
		return &OutputChoice{Mode: "stdout"}, nil
	}
}
