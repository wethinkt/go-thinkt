package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// themeBrowserItem holds a theme and its metadata for the browser.
type themeBrowserItem struct {
	meta   theme.ThemeMeta
	theme  theme.Theme
	active bool // currently active theme
}

// ThemeBrowserModel is the model for the theme browser TUI.
type ThemeBrowserModel struct {
	items       []themeBrowserItem
	cursor      int
	preview     viewport.Model
	previewData []thinkt.Entry
	width       int
	height      int
	ready       bool

	// Result fields (checked after Run)
	selected string // theme name to activate ("" if cancelled)
	editName string // theme name to launch builder for
	newTheme bool   // true if user pressed 'n'
}

// NewThemeBrowserModel creates a new theme browser model.
func NewThemeBrowserModel() ThemeBrowserModel {
	themes, _ := theme.ListAvailable()
	activeName := theme.ActiveName()

	var items []themeBrowserItem
	activeCursor := 0
	for i, meta := range themes {
		t, err := theme.LoadByName(meta.Name)
		if err != nil {
			t = theme.DefaultTheme()
		}
		isActive := meta.Name == activeName
		if isActive {
			activeCursor = i
		}
		items = append(items, themeBrowserItem{
			meta:   meta,
			theme:  t,
			active: isActive,
		})
	}

	return ThemeBrowserModel{
		items:       items,
		cursor:      activeCursor,
		previewData: theme.MockEntries(),
	}
}

func (m ThemeBrowserModel) Init() tea.Cmd {
	return nil
}

func (m ThemeBrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.preview = viewport.New()
			m.ready = true
		}
		previewWidth := m.width * 55 / 100
		m.preview.SetWidth(previewWidth - 4)
		m.preview.SetHeight(m.height - 6)
		m.updatePreview()

	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.updatePreview()
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				m.updatePreview()
			}
		case "enter":
			if len(m.items) > 0 {
				m.selected = m.items[m.cursor].meta.Name
			}
			return m, tea.Quit
		case "e":
			if len(m.items) > 0 {
				m.editName = m.items[m.cursor].meta.Name
			}
			return m, tea.Quit
		case "n":
			m.newTheme = true
			return m, tea.Quit
		}
	}

	// Allow scrolling the preview viewport
	var cmd tea.Cmd
	m.preview, cmd = m.preview.Update(msg)
	return m, cmd
}

func (m *ThemeBrowserModel) updatePreview() {
	if !m.ready || len(m.items) == 0 {
		return
	}

	t := m.items[m.cursor].theme
	previewWidth := m.preview.Width()
	styles := buildPreviewStyles(t)

	var b strings.Builder

	// Color swatches header
	b.WriteString(m.renderSwatches(t))
	b.WriteString("\n\n")

	// Mock conversation entries
	for _, entry := range m.previewData {
		rendered := renderPreviewEntry(&entry, previewWidth, styles)
		if rendered != "" {
			b.WriteString(rendered)
			b.WriteString("\n")
		}
	}

	m.preview.SetContent(b.String())
}

func (m *ThemeBrowserModel) renderSwatches(t theme.Theme) string {
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))

	var parts []string

	if accent := t.GetAccent(); accent != "" {
		swatch := lipgloss.NewStyle().Background(lipgloss.Color(accent)).Render("  ")
		parts = append(parts, mutedStyle.Render("accent ")+swatch)
	}
	if border := t.GetBorderActive(); border != "" {
		swatch := lipgloss.NewStyle().Background(lipgloss.Color(border)).Render("  ")
		parts = append(parts, mutedStyle.Render("border ")+swatch)
	}
	if fg := t.TextPrimary.Fg; fg != "" {
		swatch := lipgloss.NewStyle().Background(lipgloss.Color(fg)).Render("  ")
		parts = append(parts, mutedStyle.Render("text ")+swatch)
	}
	if fg := t.TextMuted.Fg; fg != "" {
		swatch := lipgloss.NewStyle().Background(lipgloss.Color(fg)).Render("  ")
		parts = append(parts, mutedStyle.Render("muted ")+swatch)
	}

	return strings.Join(parts, "  ")
}

func (m ThemeBrowserModel) View() tea.View {
	if !m.ready {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	listWidth := m.width * 35 / 100
	previewWidth := m.width - listWidth - 3

	// Determine accent from highlighted theme
	var accentColor string
	var borderActive, borderInactive string
	if len(m.items) > 0 {
		t := m.items[m.cursor].theme
		accentColor = t.GetAccent()
		borderActive = t.GetBorderActive()
		borderInactive = t.GetBorderInactive()
	} else {
		accentColor = "#7D56F4"
		borderActive = "#7D56F4"
		borderInactive = "#444444"
	}

	// Header
	listTitle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accentColor)).Render("Themes")
	previewTitle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accentColor)).Render("Preview")
	header := listTitle + strings.Repeat(" ", max(0, listWidth-lipgloss.Width(listTitle)+3)) + previewTitle

	// Left pane: theme list
	listContent := m.renderThemeList(accentColor, borderInactive)
	listBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderActive)).
		Width(listWidth).
		Height(m.height - 6)
	listPane := listBorder.Render(listContent)

	// Right pane: preview
	previewBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderInactive)).
		Width(previewWidth).
		Height(m.height - 6)
	previewPane := previewBorder.Render(m.preview.View())

	// Footer
	helpText := "↑/↓: navigate • enter: activate • e: edit • n: new theme • q/esc: cancel"
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color(borderInactive)).Render(helpText)

	content := header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, listPane, " ", previewPane) + "\n" + footer
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m ThemeBrowserModel) renderThemeList(accentColor, mutedColor string) string {
	var b strings.Builder

	for i, item := range m.items {
		prefix := "  "
		if i == m.cursor {
			prefix = "▸ "
		}

		nameStyle := lipgloss.NewStyle()
		if i == m.cursor {
			nameStyle = nameStyle.Bold(true).Foreground(lipgloss.Color(accentColor))
		}

		name := item.meta.Name
		if item.active {
			name += " *"
		}

		line := prefix + nameStyle.Render(name)

		// Show description on second line for cursor item
		if i == m.cursor && item.meta.Description != "" {
			descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(mutedColor))
			line += "\n    " + descStyle.Render(item.meta.Description)
		}

		b.WriteString(line + "\n")
	}

	return b.String()
}

// RunThemeBrowser runs the theme browser TUI and handles the result.
func RunThemeBrowser() error {
	model := NewThemeBrowserModel()
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	result, ok := finalModel.(ThemeBrowserModel)
	if !ok {
		return nil
	}

	// Handle result
	if result.selected != "" {
		if err := theme.SetActive(result.selected); err != nil {
			return fmt.Errorf("failed to set theme: %w", err)
		}
		fmt.Printf("Theme set to: %s\n", result.selected)
		return nil
	}

	if result.editName != "" {
		return RunThemeBuilder(result.editName)
	}

	if result.newTheme {
		// Prompt for name, then launch builder
		fmt.Print("New theme name: ")
		var name string
		if _, err := fmt.Scanln(&name); err != nil || name == "" {
			return nil
		}
		return RunThemeBuilder(name)
	}

	return nil
}
