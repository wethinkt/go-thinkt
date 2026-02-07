package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui/colorpicker"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// editMode represents what is being edited
type editMode int

const (
	editNone editMode = iota
	editFg
	editBg
)

// styleEntry represents a single editable style in the theme.
type styleEntry struct {
	Name     string
	Category string
	GetStyle func(*theme.Theme) *theme.Style
	// For simple color fields (accent, border colors)
	GetColor func(*theme.Theme) string
	SetColor func(*theme.Theme, string)
}

// ThemeBuilderModel is the model for the theme builder TUI.
type ThemeBuilderModel struct {
	theme       theme.Theme
	themeName   string
	entries     []styleEntry
	selected    int
	editMode    editMode
	picker      colorpicker.Model
	preview     viewport.Model
	previewData []thinkt.Entry
	width       int
	height      int
	ready       bool
	focusPane   int // 0 = style list, 1 = preview
	dirty       bool
	message     string
	messageType string // "success", "error", ""
}

// NewThemeBuilderModel creates a new theme builder.
func NewThemeBuilderModel(themeName string) ThemeBuilderModel {
	t, err := theme.LoadByName(themeName)
	if err != nil {
		t = theme.DefaultTheme()
	}

	return ThemeBuilderModel{
		theme:       t,
		themeName:   themeName,
		entries:     buildStyleEntries(),
		previewData: theme.MockEntries(),
		picker:      colorpicker.New("#000000"),
	}
}

func buildStyleEntries() []styleEntry {
	return []styleEntry{
		// Accent colors
		{Name: "Accent", Category: "Accent Colors",
			GetColor: func(t *theme.Theme) string { return t.Accent },
			SetColor: func(t *theme.Theme, c string) { t.Accent = c }},
		{Name: "Border Active", Category: "Accent Colors",
			GetColor: func(t *theme.Theme) string { return t.BorderActive },
			SetColor: func(t *theme.Theme, c string) { t.BorderActive = c }},
		{Name: "Border Inactive", Category: "Accent Colors",
			GetColor: func(t *theme.Theme) string { return t.BorderInactive },
			SetColor: func(t *theme.Theme, c string) { t.BorderInactive = c }},

		// Text styles
		{Name: "Text Primary", Category: "Text",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.TextPrimary }},
		{Name: "Text Secondary", Category: "Text",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.TextSecondary }},
		{Name: "Text Muted", Category: "Text",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.TextMuted }},

		// Conversation blocks
		{Name: "User Block", Category: "Blocks",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.UserBlock }},
		{Name: "Assistant Block", Category: "Blocks",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.AssistantBlock }},
		{Name: "Thinking Block", Category: "Blocks",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.ThinkingBlock }},
		{Name: "Tool Call Block", Category: "Blocks",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.ToolCallBlock }},
		{Name: "Tool Result Block", Category: "Blocks",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.ToolResultBlock }},

		// Labels
		{Name: "User Label", Category: "Labels",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.UserLabel }},
		{Name: "Assistant Label", Category: "Labels",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.AssistantLabel }},
		{Name: "Thinking Label", Category: "Labels",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.ThinkingLabel }},
		{Name: "Tool Label", Category: "Labels",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.ToolLabel }},

		// Confirm dialog
		{Name: "Confirm Prompt", Category: "Confirm Dialog",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.ConfirmPrompt }},
		{Name: "Confirm Selected", Category: "Confirm Dialog",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.ConfirmSelected }},
		{Name: "Confirm Unselected", Category: "Confirm Dialog",
			GetStyle: func(t *theme.Theme) *theme.Style { return &t.ConfirmUnselected }},
	}
}

func (m ThemeBuilderModel) Init() tea.Cmd {
	return nil
}

func (m ThemeBuilderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.preview = viewport.New()
			m.ready = true
		}
		// Preview takes right 55% of screen
		previewWidth := m.width * 55 / 100
		m.preview.SetWidth(previewWidth - 4)
		m.preview.SetHeight(m.height - 6)
		m.updatePreview()

	case tea.KeyMsg:
		// Clear message on any key
		m.message = ""
		m.messageType = ""

		key := msg.String()

		if m.editMode != editNone {
			// Handle color picker
			m.picker.HandleKey(key)

			// Apply color change in real-time
			m.applyPickerColor()

			if m.picker.Confirmed {
				m.dirty = true
				m.editMode = editNone
				m.picker.Confirmed = false
			} else if m.picker.Cancelled {
				// Restore original color
				m.applyPickerOriginalColor()
				m.editMode = editNone
				m.picker.Cancelled = false
			}

			m.updatePreview()
		} else {
			// Check for quit first
			if m.handleQuit(key) {
				return m, tea.Quit
			}
			m.handleNormalKeys(key)
		}
	}

	// Update preview viewport when focused
	if m.focusPane == 1 && m.editMode == editNone {
		m.preview, cmd = m.preview.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *ThemeBuilderModel) handleQuit(key string) bool {
	switch key {
	case "q", "ctrl+c":
		return true
	case "esc":
		return m.editMode == editNone
	}
	return false
}

func (m *ThemeBuilderModel) handleNormalKeys(key string) {
	switch key {
	case "up", "k":
		if m.focusPane == 0 && m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.focusPane == 0 && m.selected < len(m.entries)-1 {
			m.selected++
		}
	case "tab":
		m.focusPane = (m.focusPane + 1) % 2
	case "enter", "e":
		if m.focusPane == 0 {
			m.startEditingFg()
		}
	case "E": // Shift+e for background
		if m.focusPane == 0 && m.entries[m.selected].GetStyle != nil {
			m.startEditingBg()
		}
	case "left", "h":
		if m.focusPane == 0 {
			m.startEditingFg()
		}
	case "right", "l":
		if m.focusPane == 0 && m.entries[m.selected].GetStyle != nil {
			m.startEditingBg()
		}
	case "b": // Toggle bold
		m.toggleBold()
	case "i": // Toggle italic
		m.toggleItalic()
	case "u": // Toggle underline
		m.toggleUnderline()
	case "ctrl+s":
		m.saveTheme()
	}
}

func (m *ThemeBuilderModel) startEditingFg() {
	entry := m.entries[m.selected]
	var currentColor string

	if entry.GetStyle != nil {
		style := entry.GetStyle(&m.theme)
		currentColor = style.Fg
	} else if entry.GetColor != nil {
		currentColor = entry.GetColor(&m.theme)
	}

	if currentColor == "" {
		currentColor = "#808080"
	}

	m.picker = colorpicker.New(currentColor)
	m.picker.AccentColor = m.theme.GetAccent()
	m.picker.MutedColor = m.theme.TextMuted.Fg
	m.editMode = editFg
}

func (m *ThemeBuilderModel) startEditingBg() {
	entry := m.entries[m.selected]
	if entry.GetStyle == nil {
		return
	}

	style := entry.GetStyle(&m.theme)
	currentColor := style.Bg
	if currentColor == "" {
		currentColor = "#000000"
	}

	m.picker = colorpicker.New(currentColor)
	m.picker.AccentColor = m.theme.GetAccent()
	m.picker.MutedColor = m.theme.TextMuted.Fg
	m.editMode = editBg
}

func (m *ThemeBuilderModel) applyPickerColor() {
	color := m.picker.Value()
	entry := m.entries[m.selected]
	if entry.GetStyle != nil {
		style := entry.GetStyle(&m.theme)
		if m.editMode == editFg {
			style.Fg = color
		} else {
			style.Bg = color
		}
	} else if entry.SetColor != nil {
		entry.SetColor(&m.theme, color)
	}
}

func (m *ThemeBuilderModel) applyPickerOriginalColor() {
	origColor := colorpicker.RGBToHex(m.picker.OrigR, m.picker.OrigG, m.picker.OrigB)
	entry := m.entries[m.selected]
	if entry.GetStyle != nil {
		style := entry.GetStyle(&m.theme)
		if m.editMode == editFg {
			style.Fg = origColor
		} else {
			style.Bg = origColor
		}
	} else if entry.SetColor != nil {
		entry.SetColor(&m.theme, origColor)
	}
}

func (m *ThemeBuilderModel) toggleBold() {
	entry := m.entries[m.selected]
	if entry.GetStyle == nil {
		return
	}
	style := entry.GetStyle(&m.theme)
	style.Bold = !style.Bold
	m.dirty = true
	m.updatePreview()
}

func (m *ThemeBuilderModel) toggleItalic() {
	entry := m.entries[m.selected]
	if entry.GetStyle == nil {
		return
	}
	style := entry.GetStyle(&m.theme)
	style.Italic = !style.Italic
	m.dirty = true
	m.updatePreview()
}

func (m *ThemeBuilderModel) toggleUnderline() {
	entry := m.entries[m.selected]
	if entry.GetStyle == nil {
		return
	}
	style := entry.GetStyle(&m.theme)
	style.Underline = !style.Underline
	m.dirty = true
	m.updatePreview()
}

func (m *ThemeBuilderModel) saveTheme() {
	if err := theme.Save(m.themeName, m.theme); err != nil {
		m.message = fmt.Sprintf("Error saving: %v", err)
		m.messageType = "error"
		return
	}
	m.dirty = false
	m.message = fmt.Sprintf("Theme '%s' saved!", m.themeName)
	m.messageType = "success"
}

func (m *ThemeBuilderModel) updatePreview() {
	if !m.ready {
		return
	}

	var b strings.Builder
	previewWidth := m.preview.Width()
	styles := buildPreviewStyles(m.theme)

	for _, entry := range m.previewData {
		rendered := renderPreviewEntry(&entry, previewWidth, styles)
		if rendered != "" {
			b.WriteString(rendered)
			b.WriteString("\n")
		}
	}

	m.preview.SetContent(b.String())
}

// previewStyles holds lipgloss styles built from the theme for preview.
type previewStyles struct {
	UserBlock       lipgloss.Style
	AssistantBlock  lipgloss.Style
	ThinkingBlock   lipgloss.Style
	ToolCallBlock   lipgloss.Style
	ToolResultBlock lipgloss.Style
	UserLabel       lipgloss.Style
	AssistantLabel  lipgloss.Style
	ThinkingLabel   lipgloss.Style
	ToolLabel       lipgloss.Style
}

func buildPreviewStyles(t theme.Theme) previewStyles {
	return previewStyles{
		UserBlock:       applyThemeStyle(lipgloss.NewStyle(), t.UserBlock).Padding(0, 1).MarginBottom(1),
		AssistantBlock:  applyThemeStyle(lipgloss.NewStyle(), t.AssistantBlock).Padding(0, 1).MarginBottom(1),
		ThinkingBlock:   applyThemeStyle(lipgloss.NewStyle(), t.ThinkingBlock).Padding(0, 1).MarginBottom(1),
		ToolCallBlock:   applyThemeStyle(lipgloss.NewStyle(), t.ToolCallBlock).Padding(0, 1).MarginBottom(1),
		ToolResultBlock: applyThemeStyle(lipgloss.NewStyle(), t.ToolResultBlock).Padding(0, 1).MarginBottom(1),
		UserLabel:       applyThemeStyle(lipgloss.NewStyle(), t.UserLabel),
		AssistantLabel:  applyThemeStyle(lipgloss.NewStyle(), t.AssistantLabel),
		ThinkingLabel:   applyThemeStyle(lipgloss.NewStyle(), t.ThinkingLabel),
		ToolLabel:       applyThemeStyle(lipgloss.NewStyle(), t.ToolLabel),
	}
}

func applyThemeStyle(s lipgloss.Style, ts theme.Style) lipgloss.Style {
	if ts.Fg != "" {
		s = s.Foreground(lipgloss.Color(ts.Fg))
	}
	if ts.Bg != "" {
		s = s.Background(lipgloss.Color(ts.Bg))
	}
	if ts.Bold {
		s = s.Bold(true)
	}
	if ts.Italic {
		s = s.Italic(true)
	}
	if ts.Underline {
		s = s.Underline(true)
	}
	return s
}

func renderPreviewEntry(entry *thinkt.Entry, width int, styles previewStyles) string {
	switch entry.Role {
	case thinkt.RoleUser:
		text := entry.Text
		if text == "" {
			for _, cb := range entry.ContentBlocks {
				if cb.Type == "text" && cb.Text != "" {
					text = cb.Text
					break
				}
			}
		}
		if text == "" {
			return ""
		}
		label := styles.UserLabel.Render("User")
		content := styles.UserBlock.Width(width).Render(text)
		return label + "\n" + content

	case thinkt.RoleAssistant:
		var parts []string
		for _, block := range entry.ContentBlocks {
			switch block.Type {
			case "thinking":
				if block.Thinking != "" {
					label := styles.ThinkingLabel.Render("Thinking")
					text := block.Thinking
					if len(text) > 200 {
						text = text[:200] + "..."
					}
					content := styles.ThinkingBlock.Width(width).Render(text)
					parts = append(parts, label+"\n"+content)
				}
			case "text":
				if block.Text != "" {
					label := styles.AssistantLabel.Render("Assistant")
					content := styles.AssistantBlock.Width(width).Render(block.Text)
					parts = append(parts, label+"\n"+content)
				}
			case "tool_use":
				label := styles.ToolLabel.Render(fmt.Sprintf("Tool: %s", block.ToolName))
				summary := fmt.Sprintf("id: %s", block.ToolUseID)
				content := styles.ToolCallBlock.Width(width).Render(summary)
				parts = append(parts, label+"\n"+content)
			}
		}
		if len(parts) == 0 && entry.Text != "" {
			label := styles.AssistantLabel.Render("Assistant")
			content := styles.AssistantBlock.Width(width).Render(entry.Text)
			parts = append(parts, label+"\n"+content)
		}
		return strings.Join(parts, "\n")

	case thinkt.RoleTool:
		for _, block := range entry.ContentBlocks {
			if block.Type == "tool_result" {
				label := styles.ToolLabel.Render("Tool Result")
				text := "(result)"
				if block.IsError {
					text = "(error)"
				}
				content := styles.ToolResultBlock.Width(width).Render(text)
				return label + "\n" + content
			}
		}
	}
	return ""
}

func (m ThemeBuilderModel) View() tea.View {
	if !m.ready {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	// Layout: left side = style list or picker, right side = preview
	listWidth := m.width * 45 / 100
	previewWidth := m.width - listWidth - 3

	var leftContent string
	if m.editMode != editNone {
		leftContent = m.renderPickerPane()
	} else {
		leftContent = m.renderStyleList()
	}

	// Build preview pane
	accentColor := m.theme.GetAccent()
	previewTitle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accentColor)).Render("Preview")
	previewBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.theme.GetBorderInactive())).
		Width(previewWidth).
		Height(m.height - 6)

	previewContent := previewBorder.Render(m.preview.View())

	// Build list pane
	listTitle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accentColor)).Render("Theme: " + m.themeName)
	if m.dirty {
		listTitle += lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Render(" *")
	}
	listBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.theme.GetBorderInactive())).
		Width(listWidth).
		Height(m.height - 6)

	if m.focusPane == 0 || m.editMode != editNone {
		listBorder = listBorder.BorderForeground(lipgloss.Color(m.theme.GetBorderActive()))
	} else {
		previewBorder = previewBorder.BorderForeground(lipgloss.Color(m.theme.GetBorderActive()))
		previewContent = previewBorder.Render(m.preview.View())
	}

	listPane := listBorder.Render(leftContent)

	// Header
	header := listTitle + strings.Repeat(" ", max(0, listWidth-lipgloss.Width(listTitle)+3)) + previewTitle

	// Footer with help
	var helpText string
	if m.editMode != editNone {
		helpText = m.picker.View()
		// Just show the help line from the picker
		lines := strings.Split(helpText, "\n")
		if len(lines) > 0 {
			helpText = lines[len(lines)-1]
		}
	} else {
		helpText = "↑/↓: select • e/E: edit fg/bg • b/i/u: bold/italic/underline • tab: pane • ctrl+s: save • esc/q: quit"
	}
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.TextMuted.Fg)).Render(helpText)

	// Combine
	content := header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, listPane, " ", previewContent) + "\n" + footer

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m ThemeBuilderModel) renderStyleList() string {
	var listContent strings.Builder
	currentCategory := ""
	accentColor := m.theme.GetAccent()

	for i, entry := range m.entries {
		// Category header
		if entry.Category != currentCategory {
			if currentCategory != "" {
				listContent.WriteString("\n")
			}
			categoryStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accentColor))
			listContent.WriteString(categoryStyle.Render(entry.Category) + "\n")
			currentCategory = entry.Category
		}

		// Entry line
		prefix := "  "
		if i == m.selected {
			prefix = "▸ "
		}

		nameStyle := lipgloss.NewStyle().Width(18)
		if i == m.selected {
			nameStyle = nameStyle.Bold(true).Foreground(lipgloss.Color(accentColor))
		}

		// Show color values and attributes
		var info strings.Builder
		if entry.GetStyle != nil {
			style := entry.GetStyle(&m.theme)

			// Foreground color with swatch
			if style.Fg != "" {
				fgSwatch := lipgloss.NewStyle().Background(lipgloss.Color(style.Fg)).Render("  ")
				info.WriteString(fgSwatch + " ")
			}

			// Background color with swatch (if set)
			if style.Bg != "" {
				bgSwatch := lipgloss.NewStyle().Background(lipgloss.Color(style.Bg)).Render("  ")
				info.WriteString(bgSwatch + " ")
			}

			// Text attributes
			attrs := ""
			if style.Bold {
				attrs += "B"
			}
			if style.Italic {
				attrs += "I"
			}
			if style.Underline {
				attrs += "U"
			}
			if attrs != "" {
				attrStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.TextMuted.Fg))
				info.WriteString(attrStyle.Render("[" + attrs + "]"))
			}
		} else if entry.GetColor != nil {
			color := entry.GetColor(&m.theme)
			swatch := lipgloss.NewStyle().Background(lipgloss.Color(color)).Render("  ")
			colorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.TextMuted.Fg))
			info.WriteString(swatch + " " + colorStyle.Render(color))
		}

		listContent.WriteString(prefix + nameStyle.Render(entry.Name) + " " + info.String() + "\n")
	}

	// Message
	if m.message != "" {
		msgStyle := lipgloss.NewStyle()
		switch m.messageType {
		case "error":
			msgStyle = msgStyle.Foreground(lipgloss.Color("#ff5555"))
		case "success":
			msgStyle = msgStyle.Foreground(lipgloss.Color("#50fa7b"))
		}
		listContent.WriteString("\n" + msgStyle.Render(m.message) + "\n")
	}

	return listContent.String()
}

func (m ThemeBuilderModel) renderPickerPane() string {
	var b strings.Builder

	// Show what we're editing
	entry := m.entries[m.selected]
	targetLabel := "Foreground"
	if m.editMode == editBg {
		targetLabel = "Background"
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.GetAccent()))
	b.WriteString(headerStyle.Render(fmt.Sprintf("Editing: %s (%s)", entry.Name, targetLabel)) + "\n\n")

	// Render the color picker
	b.WriteString(m.picker.View())

	return b.String()
}

// RunThemeBuilder runs the theme builder TUI.
func RunThemeBuilder(themeName string) error {
	model := NewThemeBuilderModel(themeName)
	p := tea.NewProgram(model)
	_, err := p.Run()
	return err
}
