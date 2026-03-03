// Package setup implements the first-run setup wizard TUI for thinkt.
//
// It guides the user through language selection, home directory creation,
// source discovery, indexer configuration, and embedding setup.
package setup

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// step represents the current wizard step.
type step int

const (
	stepWelcome step = iota
	stepHome
	stepSourceConsent
	stepSourceApproval
	stepIndexer
	stepEmbeddings
	stepSuggestions
	stepDone
)

const (
	setupMinWidth = 48
	setupMaxWidth = 96
)

// sourceMode controls how sources are approved.
type sourceMode int

const (
	sourceModeOneByOne sourceMode = iota
	sourceModeAll
	sourceModeDisableAll
)

// sourceResult holds the scan result for a single source with its approval state.
type sourceResult struct {
	Info     thinkt.DetailedSourceInfo
	Approved bool
}

// Result holds the final output of the setup wizard.
type Result struct {
	Language   string
	HomeDir    string
	Sources    map[string]bool
	Indexer    bool
	Embeddings bool
	Completed  bool
}

// sourceDiscoveredMsg carries a single source discovery result.
type sourceDiscoveredMsg struct {
	info thinkt.DetailedSourceInfo
}

// scanCompleteMsg signals that all sources have been scanned.
type scanCompleteMsg struct{}

// Model is the BubbleTea model for the setup wizard.
type Model struct {
	step   step
	width  int
	height int
	result Result

	// Source selection
	sourceMode    sourceMode
	sources       []sourceResult
	approvalIdx   int
	consentCursor int // 0-3 for the 4 consent choices

	// Discovery
	factories       []thinkt.StoreFactory
	scanCh          chan tea.Msg
	scanning        bool
	scanDone        bool
	pendingApproval bool

	// Language selection
	langs  []thinktI18n.LangInfo
	cursor int

	// Yes/No button selector (true = Yes focused)
	confirm bool

	// Theme colors
	accent  string
	muted   string
	primary string
}

// New creates a new setup wizard model.
func New(factories []thinkt.StoreFactory) Model {
	t := theme.Current()
	langs := thinktI18n.AvailableLanguages(thinktI18n.ActiveTag())

	// Find cursor position for active language
	cursor := 0
	for i, l := range langs {
		if l.Active {
			cursor = i
			break
		}
	}

	return Model{
		step:      stepWelcome,
		result:    Result{Sources: make(map[string]bool)},
		factories: factories,
		langs:     langs,
		cursor:    cursor,
		confirm:   true, // default Yes for home directory step
		accent:    t.GetAccent(),
		muted:     t.TextMuted.Fg,
		primary:   t.TextPrimary.Fg,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		resized := m.width > 0 && m.height > 0 && (m.width != msg.Width || m.height != msg.Height)
		m.width = msg.Width
		m.height = msg.Height
		if resized {
			return m, clearScreenCmd()
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q", "Q":
			m.result.Completed = false
			return m, tea.Quit
		}

	case sourceDiscoveredMsg:
		m.sources = append(m.sources, sourceResult{
			Info:     msg.info,
			Approved: m.sourceMode == sourceModeAll,
		})
		if m.sourceMode == sourceModeOneByOne {
			m.pendingApproval = true
			m.approvalIdx = len(m.sources) - 1
			m.confirm = true
			// Don't call waitForScan — wait for user to approve first
			return m, nil
		}
		return m, m.waitForScan()

	case scanCompleteMsg:
		m.scanning = false
		m.scanDone = true
		return m, nil
	}

	switch m.step {
	case stepWelcome:
		return m.updateWelcome(msg)
	case stepHome:
		return m.updateHome(msg)
	case stepSourceConsent:
		return m.updateSourceConsent(msg)
	case stepSourceApproval:
		return m.updateSourceApproval(msg)
	case stepIndexer:
		return m.updateIndexer(msg)
	case stepEmbeddings:
		return m.updateEmbeddings(msg)
	case stepSuggestions:
		return m.updateSuggestions(msg)
	case stepDone:
		return m, tea.Quit
	}

	return m, nil
}

func clearScreenCmd() tea.Cmd {
	return func() tea.Msg {
		return tea.ClearScreen()
	}
}

// View implements tea.Model.
func (m Model) View() tea.View {
	var content string

	switch m.step {
	case stepWelcome:
		content = m.viewWelcome()
	case stepHome:
		content = m.viewHome()
	case stepSourceConsent:
		content = m.viewSourceConsent()
	case stepSourceApproval:
		content = m.viewSourceApproval()
	case stepIndexer:
		content = m.viewIndexer()
	case stepEmbeddings:
		content = m.viewEmbeddings()
	case stepSuggestions:
		content = m.viewSuggestions()
	case stepDone:
		content = ""
	}

	content = strings.Trim(content, "\n")
	if width := m.inlineWidth(); width > 0 {
		content = lipgloss.Wrap(content, width, "")
		// Hard-truncate wrapped lines to avoid terminal auto-wrap drift on very
		// long unbroken tokens (for example long filesystem paths).
		lines := strings.Split(content, "\n")
		for i := range lines {
			lines[i] = ansi.Truncate(lines[i], width, "")
		}
		content = strings.Join(lines, "\n")
	}
	if m.height > 0 {
		// Setup renders in the primary screen buffer, so keep output within a
		// fixed top-aligned viewport and reserve one row at the bottom. This helps
		// avoid terminal scrollback drift when content updates near full height.
		viewHeight := m.height
		if viewHeight > 1 {
			viewHeight--
		}
		content = lipgloss.NewStyle().
			MaxHeight(viewHeight).
			Height(viewHeight).
			AlignVertical(lipgloss.Top).
			Render(content)
	}

	v := tea.NewView(content)
	v.AltScreen = false
	return v
}

// GetResult returns the wizard result.
func (m Model) GetResult() Result {
	return m.result
}

// startProgressiveScan starts a background goroutine that scans sources
// and sends results one at a time through scanCh.
func (m *Model) startProgressiveScan() tea.Cmd {
	m.scanning = true
	m.scanDone = false
	m.sources = nil
	m.approvalIdx = 0
	ch := make(chan tea.Msg, len(m.factories))
	m.scanCh = ch
	factories := m.factories
	go func() {
		d := thinkt.NewDiscovery(factories...)
		_, _ = d.DiscoverDetailed(context.Background(), func(info thinkt.DetailedSourceInfo) {
			ch <- sourceDiscoveredMsg{info: info}
		})
		close(ch)
	}()
	return m.waitForScan()
}

// waitForScan returns a tea.Cmd that reads one message from scanCh.
func (m *Model) waitForScan() tea.Cmd {
	ch := m.scanCh
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return scanCompleteMsg{}
		}
		return msg
	}
}

// padRight pads a (possibly styled) string to width using its visible width.
func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// renderCLIHint renders a "CLI:  command" line in muted/accent style.
func (m Model) renderCLIHint(cmd string) string {
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.muted))
	codeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.accent))
	return fmt.Sprintf("  %s  %s",
		mutedStyle.Render(thinktI18n.T("tui.setup.hint.useCLICommand", "use CLI command:")),
		codeStyle.Render(cmd),
	)
}

func (m Model) withEscQ(helpText string) string {
	if strings.Contains(helpText, "esc/q") || strings.Contains(helpText, "ESC/Q") {
		return helpText
	}
	if strings.Contains(helpText, "esc") {
		return strings.Replace(helpText, "esc", "esc/q", 1)
	}
	if strings.Contains(helpText, "ESC") {
		return strings.Replace(helpText, "ESC", "ESC/Q", 1)
	}
	return helpText + " · esc/q"
}

// renderStepHeader renders a consistent left-justified step heading with divider.
func (m Model) renderStepHeader(title string) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.accent))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.muted))

	head := "  " + titleStyle.Render(title)
	if step := m.stepIndicator(); step != "" {
		head += " " + step
	}

	lineWidth := m.inlineWidth() - 2
	if lineWidth < 1 {
		lineWidth = 1
	} else if lineWidth < 16 && m.inlineWidth() >= 18 {
		lineWidth = 16
	}
	divider := "  " + mutedStyle.Render(strings.Repeat("─", lineWidth))

	var b strings.Builder
	if m.step != stepWelcome && m.step != stepDone {
		b.WriteString(m.renderStickyContext())
		b.WriteString("\n")
	}
	b.WriteString(head)
	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n")
	return b.String()
}

// renderStickyContext shows key selections from earlier steps so users keep context.
func (m Model) renderStickyContext() string {
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.muted))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.primary))
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.accent))

	var lines []string
	lines = append(lines, "  "+titleStyle.Render(
		m.welcomeHeader(),
	))

	if lang := m.selectedLanguageSummary(); lang != "" {
		lines = append(lines, fmt.Sprintf("  %s %s",
			labelStyle.Render(thinktI18n.T("tui.setup.context.languageLabel", "Language:")),
			valueStyle.Render(lang),
		))
	}

	if m.result.HomeDir != "" {
		lines = append(lines, fmt.Sprintf("  %s %s",
			labelStyle.Render(thinktI18n.T("tui.setup.context.homeDirLabel", "Home Dir:")),
			valueStyle.Render(m.result.HomeDir),
		))
	}

	if m.scanning || m.scanDone || len(m.sources) > 0 {
		discovered := len(m.sources)
		enabled := 0
		for _, src := range m.sources {
			if src.Approved {
				enabled++
			}
		}
		sourceSummary := thinktI18n.Tf("tui.setup.context.sourcesDiscovered", "%d discovered", discovered)
		if m.scanDone && discovered > 0 {
			sourceSummary = thinktI18n.Tf(
				"tui.setup.context.sourcesDiscoveredEnabled",
				"%d discovered, %d enabled",
				discovered,
				enabled,
			)
		}
		lines = append(lines, fmt.Sprintf("  %s %s",
			labelStyle.Render(thinktI18n.T("tui.setup.context.sourcesLabel", "Sources:")),
			valueStyle.Render(sourceSummary),
		))
	}

	if m.step > stepIndexer {
		indexerStatus := thinktI18n.T("tui.setup.suggestions.disabled", "disabled")
		if m.result.Indexer {
			indexerStatus = thinktI18n.T("tui.setup.suggestions.enabled", "enabled")
		}
		lines = append(lines, fmt.Sprintf("  %s %s",
			labelStyle.Render(thinktI18n.T("tui.setup.context.indexerLabel", "Indexer:")),
			valueStyle.Render(indexerStatus),
		))
	}

	if m.step > stepEmbeddings {
		embeddingStatus := thinktI18n.T("tui.setup.suggestions.disabled", "disabled")
		if m.result.Embeddings {
			embeddingStatus = thinktI18n.T("tui.setup.suggestions.enabled", "enabled")
		}
		lines = append(lines, fmt.Sprintf("  %s %s",
			labelStyle.Render(thinktI18n.T("tui.setup.context.embeddingsLabel", "Embeddings:")),
			valueStyle.Render(embeddingStatus),
		))
	}

	return strings.Join(lines, "\n")
}

func (m Model) welcomeHeader() string {
	return thinktI18n.T("tui.setup.welcome.header", "Welcome to 🧠 thinkt")
}

func (m Model) selectedLanguageSummary() string {
	tag := m.result.Language
	if tag == "" && m.cursor >= 0 && m.cursor < len(m.langs) {
		tag = m.langs[m.cursor].Tag
	}
	if tag == "" {
		return ""
	}

	for _, l := range m.langs {
		if l.Tag == tag {
			name := l.Name
			if name == "" {
				name = l.EnglishName
			}
			if name == "" {
				return tag
			}
			return fmt.Sprintf("%s (%s)", name, tag)
		}
	}

	return tag
}

// formatBytes formats a byte count into a human-readable string.
func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// renderVerticalConfirm renders a vertical Yes/No selector with Y/N hotkey hints.
// An optional noLabel overrides the default "No" text.
func (m Model) renderVerticalConfirm(noLabel ...string) string {
	yes := thinktI18n.T("common.yes", "Yes")
	no := thinktI18n.T("common.no", "No")
	if len(noLabel) > 0 {
		no = noLabel[0]
	}

	active := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.accent))
	inactive := lipgloss.NewStyle().Foreground(lipgloss.Color(m.muted))
	hotkey := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.accent))
	hotkeyDim := lipgloss.NewStyle().Foreground(lipgloss.Color(m.muted))
	help := lipgloss.NewStyle().Foreground(lipgloss.Color(m.muted))

	helpText := thinktI18n.T("tui.setup.confirm.help",
		"↑/↓ or tab: select · Y/N: choose · Enter: confirm · esc: exit")
	helpRendered := "  " + help.Render(m.withEscQ(helpText))

	renderLine := func(selected bool, key string, label string) string {
		pointer := "  "
		keyStyled := hotkeyDim.Render(key)
		labelStyled := inactive.Render(label)
		if selected {
			pointer = "▸ "
			keyStyled = hotkey.Render(key)
			labelStyled = active.Render(label)
		}
		return fmt.Sprintf("  %s%s  %s", pointer, keyStyled, labelStyled)
	}

	return fmt.Sprintf("%s\n%s\n\n%s\n",
		renderLine(m.confirm, "Y", yes),
		renderLine(!m.confirm, "N", no),
		helpRendered,
	)
}

// inlineWidth returns a readable content width for inline, non-fullscreen rendering.
func (m Model) inlineWidth() int {
	if m.width <= 0 {
		return setupMaxWidth
	}
	w := m.width - 2
	if w < setupMinWidth {
		return w
	}
	if w > setupMaxWidth {
		return setupMaxWidth
	}
	return w
}

// stepIndicator returns a step progress indicator like "[2/9]".
func (m Model) stepIndicator() string {
	// Don't show for welcome, final setup-complete screen, or done.
	if m.step == stepWelcome || m.step == stepSuggestions || m.step == stepDone {
		return ""
	}
	total := 6 // home through suggestions (excluding welcome and done)
	current := int(m.step)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(m.muted))
	return style.Render(fmt.Sprintf("[%d/%d]", current, total))
}
