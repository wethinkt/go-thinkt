// Package discover implements the first-run setup wizard TUI for thinkt.
//
// It guides the user through language selection, home directory creation,
// source discovery, indexer configuration, and embedding setup.
package discover

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// step represents the current wizard step.
type step int

const (
	stepWelcome step = iota
	stepLanguage
	stepHome
	stepSourceConsent
	stepSourceApproval
	stepSourceSummary
	stepIndexer
	stepEmbeddings
	stepSuggestions
	stepDone
)

// sourceMode controls how sources are approved.
type sourceMode int

const (
	sourceModeOneByOne sourceMode = iota
	sourceModeAll
	sourceModeSkip
)

// sourceResult holds the scan result for a single source with its approval state.
type sourceResult struct {
	Info     thinkt.DetailedSourceInfo
	Approved bool
}

// Result holds the final output of the discover wizard.
type Result struct {
	Language   string
	HomeDir    string
	Sources    map[string]bool
	Indexer    bool
	Embeddings bool
	Completed  bool
}

// scanResultMsg carries the async scan results back to the model.
type scanResultMsg struct {
	results []thinkt.DetailedSourceInfo
	err     error
}

// Model is the BubbleTea model for the discover wizard.
type Model struct {
	step   step
	width  int
	height int
	result Result

	// Source selection
	sourceMode  sourceMode
	sources     []sourceResult
	approvalIdx int

	// Discovery
	factories   []thinkt.StoreFactory
	scanning    bool
	scanDone    bool
	scanResults []thinkt.DetailedSourceInfo

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

// New creates a new discover wizard model.
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
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "esc" {
			m.result.Completed = false
			m.step = stepDone
			return m, tea.Quit
		}

	case scanResultMsg:
		m.scanning = false
		m.scanDone = true
		m.scanResults = msg.results
		return m, nil
	}

	switch m.step {
	case stepWelcome:
		return m.updateWelcome(msg)
	case stepLanguage:
		return m.updateLanguage(msg)
	case stepHome:
		return m.updateHome(msg)
	case stepSourceConsent:
		return m.updateSourceConsent(msg)
	case stepSourceApproval:
		return m.updateSourceApproval(msg)
	case stepSourceSummary:
		return m.updateSourceSummary(msg)
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

// View implements tea.Model.
func (m Model) View() tea.View {
	var content string

	switch m.step {
	case stepWelcome:
		content = m.viewWelcome()
	case stepLanguage:
		content = m.viewLanguage()
	case stepHome:
		content = m.viewHome()
	case stepSourceConsent:
		content = m.viewSourceConsent()
	case stepSourceApproval:
		content = m.viewSourceApproval()
	case stepSourceSummary:
		content = m.viewSourceSummary()
	case stepIndexer:
		content = m.viewIndexer()
	case stepEmbeddings:
		content = m.viewEmbeddings()
	case stepSuggestions:
		content = m.viewSuggestions()
	case stepDone:
		content = ""
	}

	if m.width > 0 && m.height > 0 {
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// GetResult returns the wizard result.
func (m Model) GetResult() Result {
	return m.result
}

// startScan returns a tea.Cmd that runs DiscoverDetailed asynchronously.
func (m *Model) startScan() tea.Cmd {
	m.scanning = true
	m.scanDone = false
	factories := m.factories
	return func() tea.Msg {
		d := thinkt.NewDiscovery(factories...)
		results, err := d.DiscoverDetailed(context.Background(), nil)
		return scanResultMsg{results: results, err: err}
	}
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

// renderConfirm renders a   [ Yes ]  No   or   Yes  [ No ]   button pair.
func (m Model) renderConfirm() string {
	yes := thinktI18n.T("common.yes", "Yes")
	no := thinktI18n.T("common.no", "No")

	active := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.accent))
	inactive := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))

	var yesStr, noStr string
	if m.confirm {
		yesStr = active.Render("▸ " + yes)
		noStr = inactive.Render("  " + no)
	} else {
		yesStr = inactive.Render("  " + yes)
		noStr = active.Render("▸ " + no)
	}
	return fmt.Sprintf("%s    %s    %s", yesStr, noStr,
		help.Render("←/→: select · Enter: confirm · esc: exit"))
}

// stepIndicator returns a step progress indicator like "[2/9]".
func (m Model) stepIndicator() string {
	// Don't show for welcome or done
	if m.step == stepWelcome || m.step == stepDone {
		return ""
	}
	total := 8 // welcome through suggestions (excluding done)
	current := int(m.step)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(m.muted))
	return style.Render(fmt.Sprintf("[%d/%d]", current, total))
}
