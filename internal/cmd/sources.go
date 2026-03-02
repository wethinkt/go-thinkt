package cmd

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/config"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// Source management commands
var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "Manage and view available session sources",
	Long: `View and manage available AI assistant session sources.

Sources are the AI coding assistants that store session data
on this machine (e.g., Claude Code, Kimi Code, Gemini CLI, Copilot CLI, Codex CLI).

Examples:
  thinkt sources list           # List all available sources
  thinkt sources status         # Show detailed source status
  thinkt sources enable claude  # Enable a source
  thinkt sources disable kimi   # Disable a source
  thinkt sources enable --all   # Enable all sources
  thinkt sources disable --all  # Disable all sources`,
	RunE: runSourcesList,
}

var sourcesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available session sources",
	Long: `List all session sources and their availability.

Shows which sources have session data available on this machine.

Sources include:
  - Kimi Code (~/.kimi)
  - Claude Code (~/.claude)
  - Gemini CLI (~/.gemini)
  - GitHub Copilot (~/.copilot)
  - Codex CLI (~/.codex)
  - Qwen Code (~/.qwen)`,
	RunE: runSourcesList,
}

var sourcesAllFlag bool

var sourcesEnableCmd = &cobra.Command{
	Use:          "enable [source]",
	Short:        "Enable a source",
	Long:         "Enable a previously disabled source. Use --all to enable all sources.",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE:         runSourcesEnable,
}

var sourcesDisableCmd = &cobra.Command{
	Use:          "disable [source]",
	Short:        "Disable a source",
	Long:         "Disable a source so it is excluded from all commands. Use --all to disable all sources.",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE:         runSourcesDisable,
}

var sourcesStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show detailed source status",
	Long: `Show detailed information about each session source including
workspace ID, base path, and project count.`,
	RunE: runSourcesStatus,
}

// formatSize formats a byte count as a human-readable string.
func formatSize(b int64) string {
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

// padRight pads s with spaces to the given display width.
func padRight(s string, width int) string {
	sw := lipgloss.Width(s)
	if sw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-sw)
}

// runSourcesList lists available sources.
func runSourcesList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	registry := CreateSourceRegistry()

	ctx := context.Background()
	srcs := registry.SourceStatus(ctx)

	slices.SortFunc(srcs, func(a, b thinkt.SourceInfo) int {
		return cmp.Compare(a.Name, b.Name)
	})

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(srcs)
	}

	if len(srcs) == 0 {
		fmt.Println(thinktI18n.T("cmd.sources.noSources", "No sources found."))
		fmt.Println(thinktI18n.T("cmd.sources.expectedSources", "\nExpected sources:"))
		fmt.Println("  - Kimi Code: ~/.kimi/")
		fmt.Println("  - Claude Code: ~/.claude/")
		fmt.Println("  - Gemini CLI: ~/.gemini/")
		fmt.Println("  - Copilot CLI: ~/.copilot/")
		fmt.Println("  - Codex CLI: ~/.codex/")
		fmt.Println("  - Qwen Code: ~/.qwen/")
		return nil
	}

	const gap = 2

	// Headers
	hSource := thinktI18n.T("common.header.source", "SOURCE")
	hEnabled := thinktI18n.T("cmd.sources.header.enabled", "ENABLED")
	hStatus := thinktI18n.T("common.header.status", "STATUS")
	hProjects := thinktI18n.T("cmd.sources.header.projects", "PROJECTS")
	hSessions := thinktI18n.T("cmd.sources.header.sessions", "SESSIONS")
	hSize := thinktI18n.T("cmd.sources.header.size", "SIZE")
	hBasePath := thinktI18n.T("cmd.sources.header.basePath", "BASE PATH")

	// Compute column widths
	colSource := lipgloss.Width(hSource)
	colEnabled := lipgloss.Width(hEnabled)
	colStatus := lipgloss.Width(hStatus)
	colProjects := lipgloss.Width(hProjects)
	colSessions := lipgloss.Width(hSessions)
	colSize := lipgloss.Width(hSize)
	colBasePath := lipgloss.Width(hBasePath)

	type row struct {
		name, enabled, status, projects, sessions, size, basePath string
		disabled                                                  bool
	}

	rows := make([]row, len(srcs))
	for i, s := range srcs {
		r := row{
			name:     s.Name,
			projects: fmt.Sprintf("%d", s.ProjectCount),
			sessions: fmt.Sprintf("%d", s.SessionCount),
			size:     formatSize(s.TotalSize),
			basePath: s.BasePath,
		}
		r.status = thinktI18n.T("common.status.noData", "no data")
		if s.Available {
			r.status = thinktI18n.T("common.status.available", "available")
		}
		r.disabled = isSourceDisabled(cfg, string(s.Source))
		r.enabled = thinktI18n.T("common.yes", "yes")
		if r.disabled {
			r.enabled = thinktI18n.T("common.no", "no")
		}
		rows[i] = r

		if w := lipgloss.Width(r.name); w > colSource {
			colSource = w
		}
		if w := lipgloss.Width(r.enabled); w > colEnabled {
			colEnabled = w
		}
		if w := lipgloss.Width(r.status); w > colStatus {
			colStatus = w
		}
		if w := lipgloss.Width(r.projects); w > colProjects {
			colProjects = w
		}
		if w := lipgloss.Width(r.sessions); w > colSessions {
			colSessions = w
		}
		if w := lipgloss.Width(r.size); w > colSize {
			colSize = w
		}
		if w := lipgloss.Width(r.basePath); w > colBasePath {
			colBasePath = w
		}
	}

	if !isTTY() {
		header := padRight(hSource, colSource+gap) +
			padRight(hEnabled, colEnabled+gap) +
			padRight(hStatus, colStatus+gap) +
			padRight(hProjects, colProjects+gap) +
			padRight(hSessions, colSessions+gap) +
			padRight(hSize, colSize+gap) +
			hBasePath
		fmt.Println(header)
		for _, r := range rows {
			fmt.Println(
				padRight(r.name, colSource+gap) +
					padRight(r.enabled, colEnabled+gap) +
					padRight(r.status, colStatus+gap) +
					padRight(r.projects, colProjects+gap) +
					padRight(r.sessions, colSessions+gap) +
					padRight(r.size, colSize+gap) +
					r.basePath)
		}
		return nil
	}

	t := theme.Current()
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true)
	enabledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent()))
	disabledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	availableStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent()))
	noDataStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))

	header := headerStyle.Render(
		padRight(hSource, colSource+gap) +
			padRight(hEnabled, colEnabled+gap) +
			padRight(hStatus, colStatus+gap) +
			padRight(hProjects, colProjects+gap) +
			padRight(hSessions, colSessions+gap) +
			padRight(hSize, colSize+gap) +
			hBasePath)
	fmt.Println(header)

	for _, r := range rows {
		eStyle := enabledStyle
		if r.disabled {
			eStyle = disabledStyle
		}
		sStyle := availableStyle
		if r.status != thinktI18n.T("common.status.available", "available") {
			sStyle = noDataStyle
		}

		line := nameStyle.Render(padRight(r.name, colSource+gap)) +
			eStyle.Render(padRight(r.enabled, colEnabled+gap)) +
			sStyle.Render(padRight(r.status, colStatus+gap)) +
			valueStyle.Render(padRight(r.projects, colProjects+gap)) +
			valueStyle.Render(padRight(r.sessions, colSessions+gap)) +
			valueStyle.Render(padRight(r.size, colSize+gap)) +
			mutedStyle.Render(r.basePath)
		fmt.Println(line)
	}

	return nil
}

// runSourcesEnable enables one or all sources.
func runSourcesEnable(cmd *cobra.Command, args []string) error {
	if sourcesAllFlag {
		return setAllSourcesEnabled(true)
	}
	if len(args) == 1 {
		return setSourceEnabled(args[0], true)
	}
	// Interactive picker: show disabled sources
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	var disabled []string
	for _, s := range thinkt.AllSources {
		if isSourceDisabled(cfg, string(s)) {
			disabled = append(disabled, string(s))
		}
	}
	if len(disabled) == 0 {
		fmt.Println(thinktI18n.T("cmd.sources.allEnabled", "All sources are already enabled."))
		return nil
	}
	picked, err := pickSource(disabled, "Enable which source?")
	if err != nil {
		return err
	}
	if picked == "" {
		return nil
	}
	return setSourceEnabled(picked, true)
}

// runSourcesDisable disables one or all sources.
func runSourcesDisable(cmd *cobra.Command, args []string) error {
	if sourcesAllFlag {
		return setAllSourcesEnabled(false)
	}
	if len(args) == 1 {
		return setSourceEnabled(args[0], false)
	}
	// Interactive picker: show enabled sources
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	var enabled []string
	for _, s := range thinkt.AllSources {
		if !isSourceDisabled(cfg, string(s)) {
			enabled = append(enabled, string(s))
		}
	}
	if len(enabled) == 0 {
		fmt.Println(thinktI18n.T("cmd.sources.allDisabled", "All sources are already disabled."))
		return nil
	}
	picked, err := pickSource(enabled, "Disable which source?")
	if err != nil {
		return err
	}
	if picked == "" {
		return nil
	}
	return setSourceEnabled(picked, false)
}

// isSourceDisabled returns true if the source is disabled in config.Sources.
// When Sources is nil, all sources are treated as enabled.
func isSourceDisabled(cfg config.Config, name string) bool {
	if cfg.Sources == nil {
		return false
	}
	sc, ok := cfg.Sources[name]
	if !ok {
		// Missing entry in explicit source config means excluded.
		return true
	}
	return !sc.Enabled
}

// validSourceName returns true if name matches a known source.
func validSourceName(name string) bool {
	for _, s := range thinkt.AllSources {
		if string(s) == name {
			return true
		}
	}
	return false
}

// setSourceEnabled enables or disables a single source in the config.
func setSourceEnabled(name string, enabled bool) error {
	if !validSourceName(name) {
		if outputJSON {
			return jsonError(fmt.Sprintf("unknown source: %q", name))
		}
		return fmt.Errorf("unknown source: %q (available: %s)", name, sourceNames())
	}

	cfg, err := config.Load()
	if err != nil {
		if outputJSON {
			return jsonError("failed to load config: " + err.Error())
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	setSourceEnabledInConfig(&cfg, name, enabled)

	if err := config.Save(cfg); err != nil {
		if outputJSON {
			return jsonError("failed to save config: " + err.Error())
		}
		return fmt.Errorf("failed to save config: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]bool{"enabled": enabled})
	}

	action := thinktI18n.T("common.status.enabled", "enabled")
	if !enabled {
		action = thinktI18n.T("common.status.disabled", "disabled")
	}
	fmt.Println(thinktI18n.Tf("cmd.sources.sourceToggled", "Source %q %s.", name, action))
	return nil
}

// setAllSourcesEnabled enables or disables all sources.
func setAllSourcesEnabled(enabled bool) error {
	cfg, err := config.Load()
	if err != nil {
		if outputJSON {
			return jsonError("failed to load config: " + err.Error())
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	setAllSourcesEnabledInConfig(&cfg, enabled)

	if err := config.Save(cfg); err != nil {
		if outputJSON {
			return jsonError("failed to save config: " + err.Error())
		}
		return fmt.Errorf("failed to save config: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]bool{"enabled": enabled})
	}

	action := thinktI18n.T("common.status.enabled", "enabled")
	if !enabled {
		action = thinktI18n.T("common.status.disabled", "disabled")
	}
	fmt.Println(thinktI18n.Tf("cmd.sources.allToggled", "All sources %s.", action))
	return nil
}

// setSourceEnabledInConfig toggles a single source in config.Sources.
func setSourceEnabledInConfig(cfg *config.Config, name string, enabled bool) {
	if cfg.Sources == nil {
		cfg.Sources = sourceStateMap(true)
	}
	sc := cfg.Sources[name]
	sc.Enabled = enabled
	cfg.Sources[name] = sc
}

// setAllSourcesEnabledInConfig toggles all known sources in config.Sources.
func setAllSourcesEnabledInConfig(cfg *config.Config, enabled bool) {
	if cfg.Sources == nil {
		cfg.Sources = make(map[string]config.SourceConfig, len(thinkt.AllSources))
	}
	for _, s := range thinkt.AllSources {
		name := string(s)
		sc := cfg.Sources[name]
		sc.Enabled = enabled
		cfg.Sources[name] = sc
	}
}

func sourceStateMap(enabled bool) map[string]config.SourceConfig {
	m := make(map[string]config.SourceConfig, len(thinkt.AllSources))
	for _, s := range thinkt.AllSources {
		m[string(s)] = config.SourceConfig{Enabled: enabled}
	}
	return m
}

// sourceNames returns a comma-separated list of valid source names.
func sourceNames() string {
	names := make([]string, len(thinkt.AllSources))
	for i, s := range thinkt.AllSources {
		names[i] = string(s)
	}
	return fmt.Sprintf("%s", names)
}

// pickSource launches an interactive picker for source selection.
func pickSource(sourceList []string, title string) (string, error) {
	if !isTTY() {
		return "", fmt.Errorf("interactive picker requires a terminal; pass source name as argument")
	}

	// Build AppConfig-compatible items for the existing picker
	apps := make([]config.AppConfig, len(sourceList))
	for i, name := range sourceList {
		src := thinkt.Source(name)
		apps[i] = config.AppConfig{
			ID:   name,
			Name: src.DisplayName(),
		}
	}
	return runAppPicker(apps, title, "")
}

// runSourcesStatus shows detailed source status.
func runSourcesStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	registry := CreateSourceRegistry()

	ctx := context.Background()
	srcs := registry.SourceStatus(ctx)

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(srcs)
	}

	if len(srcs) == 0 {
		fmt.Println(thinktI18n.T("cmd.sources.noSources", "No sources found."))
		return nil
	}

	// Labels
	lSource := thinktI18n.T("cmd.sources.detail.labelSource", "Source:")
	lID := thinktI18n.T("cmd.sources.detail.labelID", "ID:")
	lDescription := thinktI18n.T("cmd.sources.detail.labelDescription", "Description:")
	lEnabled := thinktI18n.T("cmd.sources.detail.labelEnabled", "Enabled:")
	lStatus := thinktI18n.T("cmd.sources.detail.labelStatus", "Status:")
	lWorkspace := thinktI18n.T("cmd.sources.detail.labelWorkspace", "Workspace:")
	lBasePath := thinktI18n.T("cmd.sources.detail.labelBasePath", "Base Path:")
	lProjects := thinktI18n.T("cmd.sources.detail.labelProjects", "Projects:")
	lSessions := thinktI18n.T("cmd.sources.detail.labelSessions", "Sessions:")
	lSize := thinktI18n.T("cmd.sources.detail.labelSize", "Size:")

	// Compute label column width
	labelWidth := 0
	for _, l := range []string{lSource, lID, lDescription, lEnabled, lStatus, lWorkspace, lBasePath, lProjects, lSessions, lSize} {
		if w := lipgloss.Width(l); w > labelWidth {
			labelWidth = w
		}
	}
	labelWidth += 1 // gap

	themed := isTTY()
	var labelStyle, valueStyle, accentStyle, mutedStyle, separatorStyle lipgloss.Style
	if themed {
		t := theme.Current()
		labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
		valueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true)
		accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent()))
		mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg))
		separatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	}

	printField := func(label, value string, style lipgloss.Style) {
		if themed {
			fmt.Printf("%s %s\n", labelStyle.Render(padRight(label, labelWidth)), style.Render(value))
		} else {
			fmt.Printf("%s %s\n", padRight(label, labelWidth), value)
		}
	}

	// Compute separator width from the widest value line we'll print.
	separatorWidth := labelWidth + 1 // label + gap
	for _, s := range srcs {
		for _, v := range []string{s.Name, string(s.Source), s.Description, s.WorkspaceID, s.BasePath} {
			if total := labelWidth + 1 + lipgloss.Width(v); total > separatorWidth {
				separatorWidth = total
			}
		}
	}
	separator := strings.Repeat("─", separatorWidth)

	for i, s := range srcs {
		if i > 0 {
			fmt.Println()
			if themed {
				fmt.Println(separatorStyle.Render(separator))
			} else {
				fmt.Println(separator)
			}
		}

		printField(lSource, s.Name, valueStyle)
		printField(lID, string(s.Source), accentStyle)
		printField(lDescription, s.Description, mutedStyle)

		disabled := isSourceDisabled(cfg, string(s.Source))
		enabledStr := thinktI18n.T("common.yes", "yes")
		enabledValStyle := accentStyle
		if disabled {
			enabledStr = thinktI18n.T("common.no", "no")
			enabledValStyle = mutedStyle
		}
		printField(lEnabled, enabledStr, enabledValStyle)

		statusStr := thinktI18n.T("common.status.noData", "no data")
		statusValStyle := mutedStyle
		if s.Available {
			statusStr = thinktI18n.T("common.status.available", "available")
			statusValStyle = accentStyle
		}
		printField(lStatus, statusStr, statusValStyle)

		if s.Available {
			printField(lWorkspace, s.WorkspaceID, mutedStyle)
			printField(lBasePath, s.BasePath, mutedStyle)
			printField(lProjects, fmt.Sprintf("%d", s.ProjectCount), valueStyle)
			printField(lSessions, fmt.Sprintf("%d", s.SessionCount), valueStyle)
			printField(lSize, formatSize(s.TotalSize), valueStyle)
		}
	}

	return nil
}
