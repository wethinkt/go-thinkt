package tui

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// AppPickerResult is the message returned when an app is selected.
type AppPickerResult struct {
	App       *config.AppConfig
	Cancelled bool
}

// appItem implements list.Item for an AppConfig.
type appItem struct {
	app config.AppConfig
}

func (i appItem) Title() string       { return i.app.Name }
func (i appItem) Description() string { return i.app.ID }
func (i appItem) FilterValue() string { return i.app.Name + " " + i.app.ID }

// appDelegate renders the app list items.
type appDelegate struct {
	normalStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	dimmedStyle   lipgloss.Style
}

func newAppDelegate() appDelegate {
	t := theme.Current()
	return appDelegate{
		normalStyle:   lipgloss.NewStyle().PaddingLeft(2),
		selectedStyle: lipgloss.NewStyle().PaddingLeft(1).Bold(true).Border(lipgloss.NormalBorder(), false, false, false, true).BorderLeftForeground(lipgloss.Color(t.GetAccent())).Foreground(lipgloss.Color(t.TextPrimary.Fg)),
		dimmedStyle:   lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color(t.TextMuted.Fg)),
	}
}

func (d appDelegate) Height() int  { return 1 }
func (d appDelegate) Spacing() int { return 0 }
func (d appDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d appDelegate) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc/q", "cancel")),
	}
}

func (d appDelegate) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}

func (d appDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ai, ok := item.(appItem)
	if !ok {
		return
	}

	isSelected := index == m.Index()
	emptyFilter := m.FilterState() == list.Filtering && m.FilterValue() == ""

	// Use a bullet to make it clear it's a list
	bullet := "• "
	if isSelected {
		bullet = "> "
	}

	var line string
	if emptyFilter {
		line = d.dimmedStyle.Render(bullet + ai.app.Name)
	} else if isSelected {
		line = d.selectedStyle.Render(bullet + ai.app.Name)
	} else {
		line = d.normalStyle.Render(bullet + ai.app.Name)
	}

	fmt.Fprint(w, line)
}

// AppPickerModel is a TUI overlay for selecting an application.
type AppPickerModel struct {
	list     list.Model
	target   string // The path we are opening
	width    int
	height   int
	quitting bool
	result   AppPickerResult
}

// NewAppPickerModel creates a new app picker.
func NewAppPickerModel(apps []config.AppConfig, targetPath string) AppPickerModel {
	items := make([]list.Item, 0, len(apps))
	for _, app := range apps {
		if app.Enabled {
			items = append(items, appItem{app: app})
		}
	}

	l := list.New(items, newAppDelegate(), 0, 0)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowTitle(false)
	l.SetShowPagination(false)
	
	// Ensure the list doesn't have internal padding that eats space
	l.Styles.NoItems = lipgloss.NewStyle().Margin(0, 2)

	return AppPickerModel{
		list:   l,
		target: targetPath,
	}
}

func (m AppPickerModel) Init() tea.Cmd {
	return nil
}

// SetSize updates the model's dimensions and resizes internal components.
func (m *AppPickerModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	listWidth := 60
	if listWidth > width-10 {
		listWidth = width - 10
	}

	// Bubbles/list needs significant vertical buffer for the filter bar
	// and internal viewport management even when title/help are hidden.
	itemCount := len(m.list.Items())
	listHeight := itemCount + 4 // 1 for filter + 3 for internal buffer

	if listHeight > height-10 {
		listHeight = height - 10
	}

	m.list.SetSize(listWidth, listHeight)
}

func (m AppPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch msg.String() {
		case "esc", "q":
			m.result.Cancelled = true
			m.quitting = true
			return m, nil

		case "enter":
			if item := m.list.SelectedItem(); item != nil {
				ai := item.(appItem)
				m.result.App = &ai.app
				m.quitting = true
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m AppPickerModel) viewContent() string {
	t := theme.Current()
	accent := lipgloss.Color(t.GetAccent())

	// Styles
	overlayStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(1, 2)

	if t.TextPrimary.Bg != "" {
		overlayStyle = overlayStyle.Background(lipgloss.Color(t.TextPrimary.Bg))
	}

	pathStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.TextMuted.Fg)).
		Italic(true).
		MaxWidth(56)

	labelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(accent)

	// Build Content
	title := labelStyle.Render("Open Project In:")
	path := pathStyle.Render(m.target)

	// Add a distinct border or separator for the list area
	listArea := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderTopForeground(lipgloss.Color(t.GetBorderInactive())).
		MarginTop(1).
		PaddingTop(1).
		Render(m.list.View())

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		path,
		listArea,
	)

	// Center the overlay
	overlay := overlayStyle.Render(content)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		overlay,
	)
}

func (m AppPickerModel) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	return v
}

func (m AppPickerModel) Result() AppPickerResult {
	return m.result
}