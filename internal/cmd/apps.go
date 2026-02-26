package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

var appsCmd = &cobra.Command{
	Use:   "apps",
	Short: "Manage open-in apps and default terminal",
	Long: `Manage the apps available for "open in" actions and the default terminal.

Apps are configured in ~/.thinkt/config.json.

Examples:
  thinkt apps                        # List all apps
  thinkt apps list                   # List all apps
  thinkt apps enable vscode          # Enable an app
  thinkt apps disable finder         # Disable an app
  thinkt apps get-terminal           # Show default terminal
  thinkt apps set-terminal ghostty   # Set default terminal
  thinkt apps set-terminal           # Interactive terminal picker`,
	RunE: runAppsList,
}

var appsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all apps with enabled/disabled status",
	RunE:  runAppsList,
}

var appsEnableCmd = &cobra.Command{
	Use:          "enable [id]",
	Short:        "Enable an app",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE:         runAppsEnable,
}

var appsDisableCmd = &cobra.Command{
	Use:          "disable [id]",
	Short:        "Disable an app",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE:         runAppsDisable,
}

var appsGetTermCmd = &cobra.Command{
	Use:   "get-terminal",
	Short: "Show the configured default terminal app",
	RunE:  runAppsGetTerminal,
}

var appsSetTermCmd = &cobra.Command{
	Use:          "set-terminal [id]",
	Short:        "Set the default terminal app",
	Long:         "Set the default terminal app. Without an argument, launches an interactive picker.",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE:         runAppsSetTerminal,
}

func runAppsList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if outputJSON {
		out := make([]config.AppInfo, len(cfg.AllowedApps))
		for i, app := range cfg.AllowedApps {
			out[i] = app.Info()
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}

	if len(cfg.AllowedApps) == 0 {
		fmt.Println("No apps configured.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tENABLED\tTERMINAL")
	for _, app := range cfg.AllowedApps {
		enabled := "no"
		if app.Enabled {
			enabled = "yes"
		}
		terminal := "no"
		if len(app.ExecRun) > 0 {
			terminal = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", app.ID, app.Name, enabled, terminal)
	}
	return w.Flush()
}

func runAppsEnable(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		return setAppEnabled(args[0], true)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	var disabled []config.AppConfig
	for _, app := range cfg.AllowedApps {
		if !app.Enabled {
			disabled = append(disabled, app)
		}
	}
	if len(disabled) == 0 {
		fmt.Println("All apps are already enabled.")
		return nil
	}
	picked, err := pickApp(disabled, "Enable which app?")
	if err != nil {
		return err
	}
	if picked == "" {
		return nil
	}
	return setAppEnabled(picked, true)
}

func runAppsDisable(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		return setAppEnabled(args[0], false)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	var enabled []config.AppConfig
	for _, app := range cfg.AllowedApps {
		if app.Enabled {
			enabled = append(enabled, app)
		}
	}
	if len(enabled) == 0 {
		fmt.Println("All apps are already disabled.")
		return nil
	}
	picked, err := pickApp(enabled, "Disable which app?")
	if err != nil {
		return err
	}
	if picked == "" {
		return nil
	}
	return setAppEnabled(picked, false)
}

func setAppEnabled(id string, enabled bool) error {
	cfg, err := config.Load()
	if err != nil {
		if outputJSON {
			return jsonError("failed to load config: " + err.Error())
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	found := false
	for i := range cfg.AllowedApps {
		if cfg.AllowedApps[i].ID == id {
			cfg.AllowedApps[i].Enabled = enabled
			found = true
			break
		}
	}

	if !found {
		if outputJSON {
			return jsonError(fmt.Sprintf("unknown app: %q", id))
		}
		return fmt.Errorf("unknown app: %q", id)
	}

	if err := config.Save(cfg); err != nil {
		if outputJSON {
			return jsonError("failed to save config: " + err.Error())
		}
		return fmt.Errorf("failed to save config: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]bool{"enabled": enabled})
	}

	action := "enabled"
	if !enabled {
		action = "disabled"
	}
	fmt.Printf("App %q %s.\n", id, action)
	return nil
}

func jsonError(msg string) error {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": msg})
	return nil
}

func runAppsGetTerminal(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	terminal := cfg.Terminal
	if terminal == "" {
		terminal = "terminal"
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"terminal": terminal})
	}

	fmt.Println(terminal)
	return nil
}

// terminalApps returns the subset of AllowedApps that are enabled and have ExecRun.
func terminalApps(cfg config.Config) []config.AppConfig {
	var terminals []config.AppConfig
	for _, app := range cfg.AllowedApps {
		if app.Enabled && len(app.ExecRun) > 0 {
			terminals = append(terminals, app)
		}
	}
	return terminals
}

func runAppsSetTerminal(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	var id string
	if len(args) == 1 {
		id = args[0]
	} else {
		// Interactive picker
		terminals := terminalApps(cfg)
		if len(terminals) == 0 {
			return fmt.Errorf("no terminal apps available (apps need ExecRun capability)")
		}

		picked, err := pickTerminal(terminals, cfg.Terminal)
		if err != nil {
			return err
		}
		if picked == "" {
			return nil // cancelled
		}
		id = picked
	}

	// Validate: app must exist, be enabled, and have ExecRun
	found := false
	for _, app := range cfg.AllowedApps {
		if app.ID == id {
			if !app.Enabled {
				return fmt.Errorf("app %q is disabled; enable it first with: thinkt apps enable %s", id, id)
			}
			if len(app.ExecRun) == 0 {
				return fmt.Errorf("app %q is not a terminal (no ExecRun capability)", id)
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("unknown app: %q", id)
	}

	cfg.Terminal = id
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Default terminal set to %q.\n", id)
	return nil
}

// --- app picker TUI ---

type appPickItem struct {
	app    config.AppConfig
	marker string // e.g. "*" for currently active
}

func (i appPickItem) Title() string {
	if i.marker != "" {
		return i.app.Name + " " + i.marker
	}
	return i.app.Name
}
func (i appPickItem) Description() string { return i.app.ID }
func (i appPickItem) FilterValue() string { return i.app.Name + " " + i.app.ID }

type appPickModel struct {
	list     list.Model
	selected string
	quitting bool
}

func newAppPickModel(apps []config.AppConfig, title string, activeID string) appPickModel {
	items := make([]list.Item, len(apps))
	initialIdx := 0
	for i, app := range apps {
		var marker string
		if app.ID == activeID {
			marker = "*"
			initialIdx = i
		}
		items[i] = appPickItem{app: app, marker: marker}
	}

	t := theme.Current()
	delegate := list.NewDefaultDelegate()
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.
		Foreground(lipgloss.Color(t.TextPrimary.Fg))
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.
		Foreground(lipgloss.Color(t.TextSecondary.Fg))
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color(t.GetAccent())).
		Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color(t.TextMuted.Fg))
	delegate.Styles.DimmedTitle = delegate.Styles.DimmedTitle.
		Foreground(lipgloss.Color(t.TextMuted.Fg))
	delegate.Styles.DimmedDesc = delegate.Styles.DimmedDesc.
		Foreground(lipgloss.Color(t.TextMuted.Fg))

	l := list.New(items, delegate, 40, min(len(items)*3+6, 20))
	l.SetShowTitle(true)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(false)
	l.Select(initialIdx)

	return appPickModel{list: l}
}

func (m appPickModel) Init() tea.Cmd { return nil }

func (m appPickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if item, ok := m.list.SelectedItem().(appPickItem); ok {
				m.selected = item.app.ID
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m appPickModel) View() tea.View {
	if m.quitting && m.selected == "" {
		return tea.NewView("")
	}
	return tea.NewView(m.list.View())
}

func runAppPicker(apps []config.AppConfig, title string, activeID string) (string, error) {
	model := newAppPickModel(apps, title, activeID)
	p := tea.NewProgram(model)

	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	m := finalModel.(appPickModel)
	return m.selected, nil
}

func pickApp(apps []config.AppConfig, title string) (string, error) {
	if !isTTY() {
		return "", fmt.Errorf("interactive picker requires a terminal; pass app ID as argument")
	}
	return runAppPicker(apps, title, "")
}

func pickTerminal(apps []config.AppConfig, currentTerminal string) (string, error) {
	if !isTTY() {
		return "", fmt.Errorf("interactive picker requires a terminal; pass terminal ID as argument")
	}
	activeID := currentTerminal
	if activeID == "" {
		activeID = "terminal"
	}
	return runAppPicker(apps, "Select default terminal", activeID)
}
