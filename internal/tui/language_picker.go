package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

type langPickerItem struct {
	info thinktI18n.LangInfo
}

// LanguagePickerModel is the model for the language picker TUI.
type LanguagePickerModel struct {
	items    []langPickerItem
	cursor   int
	preview  viewport.Model
	width    int
	height   int
	ready    bool
	selected string // tag selected by user ("" if cancelled)

	// Theme colors
	accentColor    string
	borderActive   string
	borderInactive string
	textPrimary    string
	textMuted      string
}

// NewLanguagePickerModel creates a new language picker model.
func NewLanguagePickerModel(activeTag string) LanguagePickerModel {
	langs := thinktI18n.AvailableLanguages(activeTag)
	t := theme.Current()

	var items []langPickerItem
	activeCursor := 0
	for i, l := range langs {
		if l.Active {
			activeCursor = i
		}
		items = append(items, langPickerItem{info: l})
	}

	return LanguagePickerModel{
		items:          items,
		cursor:         activeCursor,
		accentColor:    t.GetAccent(),
		borderActive:   t.GetBorderActive(),
		borderInactive: t.GetBorderInactive(),
		textPrimary:    t.TextPrimary.Fg,
		textMuted:      t.TextMuted.Fg,
	}
}

func (m LanguagePickerModel) Init() tea.Cmd {
	return nil
}

func (m LanguagePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.preview = viewport.New()
			m.ready = true
		}

		listWidth := m.width * listWidthPercent / 100
		previewWidth := m.width - listWidth - 2
		m.preview.SetWidth(previewWidth)
		m.preview.SetHeight(m.height - 4)
		m.updatePreview()

	case tea.KeyMsg:
		switch msg.String() {
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
				m.selected = m.items[m.cursor].info.Tag
			}
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.preview, cmd = m.preview.Update(msg)
	return m, cmd
}

func (m *LanguagePickerModel) updatePreview() {
	if !m.ready || len(m.items) == 0 {
		return
	}

	tag := m.items[m.cursor].info.Tag
	strings_ := thinktI18n.PreviewStrings(tag)
	keys := thinktI18n.PreviewKeys()

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.textMuted))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.textPrimary)).Bold(true)

	// Group preview strings by category
	type row struct {
		label  string
		values []string
	}

	rows := []row{
		{"Filter labels", nil},
		{"Search", nil},
		{"Loading", nil},
		{"Time", nil},
		{"Help", nil},
	}

	for _, kv := range keys {
		v := strings_[kv[0]]
		switch {
		case strings.HasPrefix(kv[0], "tui.filter."):
			rows[0].values = append(rows[0].values, v)
		case kv[0] == "tui.search.title":
			rows[1].values = append(rows[1].values, v)
		case kv[0] == "common.loading":
			rows[2].values = append(rows[2].values, v)
		case strings.HasPrefix(kv[0], "common.time."):
			rows[3].values = append(rows[3].values, v)
		case strings.HasPrefix(kv[0], "tui.help."):
			rows[4].values = append(rows[4].values, v)
		}
	}

	var b strings.Builder
	b.WriteString("\n")
	for _, r := range rows {
		if len(r.values) == 0 {
			continue
		}
		label := labelStyle.Render(fmt.Sprintf("  %-16s", r.label))
		val := valueStyle.Render(strings.Join(r.values, " · "))
		b.WriteString(label + val + "\n\n")
	}

	m.preview.SetContent(b.String())
}

func (m LanguagePickerModel) View() tea.View {
	if !m.ready {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	listWidth := m.width * listWidthPercent / 100
	previewWidth := m.width - listWidth - 2

	// Header
	listTitle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.accentColor)).Render("Languages")
	previewTitle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.accentColor)).Render("Preview")
	brand := lipgloss.NewStyle().Foreground(lipgloss.Color(m.borderInactive)).Render("🧠 thinkt")
	midGap := strings.Repeat(" ", max(0, listWidth-lipgloss.Width(listTitle)+3))
	rightGap := strings.Repeat(" ", max(0, m.width-lipgloss.Width(listTitle)-lipgloss.Width(midGap)-lipgloss.Width(previewTitle)-lipgloss.Width(brand)))
	header := listTitle + midGap + previewTitle + rightGap + brand

	// Left pane: language list
	listContent := m.renderLanguageList()
	listBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.borderActive)).
		Width(listWidth).
		Height(m.height - 2)
	listPane := listBorder.Render(listContent)

	// Right pane: preview
	previewBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.borderInactive)).
		Width(previewWidth).
		Height(m.height - 2)
	previewPane := previewBorder.Render(m.preview.View())

	// Footer
	helpText := "↑/↓: navigate • enter: select • q/esc: cancel"
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color(m.borderInactive)).Render(helpText)

	content := header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, listPane, " ", previewPane) + "\n" + footer
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m LanguagePickerModel) renderLanguageList() string {
	var b strings.Builder

	for i, item := range m.items {
		prefix := "  "
		if i == m.cursor {
			prefix = "▸ "
		}

		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.textPrimary))
		if i == m.cursor {
			nameStyle = nameStyle.Bold(true).Foreground(lipgloss.Color(m.accentColor))
		}

		name := item.info.Name
		if item.info.Active {
			name += " *"
		}

		line := prefix + nameStyle.Render(name)

		// Show tag + English name on second line for cursor item
		if i == m.cursor {
			descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.textMuted))
			desc := item.info.Tag
			if item.info.EnglishName != item.info.Name {
				desc += " — " + item.info.EnglishName
			}
			line += "\n    " + descStyle.Render(desc)
		}

		b.WriteString(line + "\n")
	}

	return b.String()
}

// RunLanguagePicker runs the language picker TUI and returns the selected tag.
// Returns "" if the user cancelled.
func RunLanguagePicker(activeTag string) (string, error) {
	model := NewLanguagePickerModel(activeTag)
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	result, ok := finalModel.(LanguagePickerModel)
	if !ok {
		return "", nil
	}

	return result.selected, nil
}
