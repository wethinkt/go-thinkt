package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"text/tabwriter"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/share"
	"github.com/wethinkt/go-thinkt/internal/target"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	shareTUI "github.com/wethinkt/go-thinkt/internal/tui"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
	"golang.org/x/term"
)

var (
	sharePushPublic  bool
	sharePushTitle   string
	sharePushTags    string
	sharePushProject string
	sharePushSession string
	sharePushSources []string

	shareExploreTag  string
	shareExploreSort string
	shareDeleteForce bool
	shareLoginGoogle bool
	shareLoginGitHub bool
	shareListJSON    bool
)

var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "Share sessions on share.wethinkt.com",
	Long:  "Upload, browse, and manage reasoning sessions on the wethinkt sharing platform.",
	Args:  cobra.NoArgs,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Chain parent's PersistentPreRunE (root command setup).
		if parent := cmd.Parent(); parent != nil && parent.PersistentPreRunE != nil {
			if err := parent.PersistentPreRunE(cmd, args); err != nil {
				return err
			}
		}
		cfg, err := config.Load()
		if err != nil {
			return nil // no config is fine, sharing enabled by default
		}
		if !cfg.Share.Enabled {
			return fmt.Errorf("sharing is disabled in %s", configPathOrFallback())
		}
		return nil
	},
	RunE: runShareList,
}

type shareOutputStyles struct {
	themed   bool
	label    lipgloss.Style
	value    lipgloss.Style
	accent   lipgloss.Style
	muted    lipgloss.Style
	success  lipgloss.Style
	warning  lipgloss.Style
	url      lipgloss.Style
	emphasis lipgloss.Style
}

func newShareOutputStyles() shareOutputStyles {
	if !isTTY() {
		return shareOutputStyles{}
	}
	t := theme.Current()
	return shareOutputStyles{
		themed:   true,
		label:    lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)),
		value:    lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true),
		accent:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		muted:    lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		success:  lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		warning:  lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		url:      lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Underline(true),
		emphasis: lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true),
	}
}

func (s shareOutputStyles) render(style lipgloss.Style, value string) string {
	if !s.themed {
		return value
	}
	return style.Render(value)
}

func (s shareOutputStyles) printKV(label, value string) {
	fmt.Printf("%s %s\n", s.render(s.label, label), s.render(s.value, value))
}

// --- login ---

var shareLoginCmd = &cobra.Command{
	Use:          "login",
	Short:        "Log in to share.wethinkt.com",
	Long:         "Authenticate with share.wethinkt.com using GitHub or Google to enable sharing sessions.",
	SilenceUsage: true,
	RunE:         runShareLogin,
}

func resolveLoginProvider() (string, error) {
	if shareLoginGoogle {
		return "google", nil
	}
	if shareLoginGitHub {
		return "github", nil
	}

	// Check previous login provider.
	if creds, err := share.LoadCredentials(share.DefaultCredentialsPath()); err == nil && creds.Provider != "" {
		return creds.Provider, nil
	}

	// No previous — show picker.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("no previous login — specify --github or --google")
	}

	return pickLoginProvider()
}

func runShareLogin(cmd *cobra.Command, args []string) error {
	out := newShareOutputStyles()
	provider, err := resolveLoginProvider()
	if err != nil {
		return err
	}

	endpoint, err := share.Endpoint()
	if err != nil {
		return err
	}
	client := share.NewDeviceFlowClient(endpoint)

	var codeResp *share.DeviceCodeResponse

	switch provider {
	case "google":
		fmt.Println(out.render(out.muted, "Requesting Google login code..."))
		codeResp, err = client.RequestGoogleCode()
	default:
		fmt.Println(out.render(out.muted, "Requesting GitHub login code..."))
		codeResp, err = client.RequestCode()
	}
	if err != nil {
		return fmt.Errorf("failed to start login: %w", err)
	}

	fmt.Printf("\n%s %s\n", out.render(out.label, "Go to:"), out.render(out.url, codeResp.VerificationLink()))
	fmt.Printf("%s %s\n\n", out.render(out.label, "Enter code:"), out.render(out.accent, codeResp.UserCode))
	fmt.Println(out.render(out.muted, "Waiting for authorization..."))

	var tokenResp *share.TokenResponse
	switch provider {
	case "google":
		tokenResp, err = client.PollForGoogleToken(codeResp.DeviceCode, codeResp.Interval)
	default:
		tokenResp, err = client.PollForToken(codeResp.DeviceCode, codeResp.Interval)
	}
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	creds := &share.Credentials{
		Token:    tokenResp.Token,
		Username: tokenResp.User.Username,
		Endpoint: endpoint,
		Provider: provider,
	}

	path := share.DefaultCredentialsPath()
	if err := share.SaveCredentials(path, creds); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	fmt.Printf("%s %s\n", out.render(out.success, "Logged in as"), out.render(out.value, tokenResp.User.Username))
	return nil
}

// pickLoginProvider shows a mini-TUI to choose GitHub or Google.
func pickLoginProvider() (string, error) {
	m := newProviderPicker()
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	result := final.(providerPickerModel)
	if result.cancelled {
		return "", fmt.Errorf("login cancelled")
	}
	return result.providers[result.cursor], nil
}

type providerPickerModel struct {
	providers []string
	cursor    int
	cancelled bool
}

func newProviderPicker() providerPickerModel {
	return providerPickerModel{
		providers: []string{"github", "google"},
	}
}

func (m providerPickerModel) Init() tea.Cmd { return nil }

func (m providerPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.cursor < len(m.providers)-1 {
				m.cursor++
			}
		case "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m providerPickerModel) View() tea.View {
	var b strings.Builder
	b.WriteString("\nLog in with:\n\n")
	for i, p := range m.providers {
		cursor := "  "
		label := p
		switch p {
		case "github":
			label = "GitHub"
		case "google":
			label = "Google"
		}
		if i == m.cursor {
			cursor = "> "
			label = fmt.Sprintf("\033[1m%s\033[0m", label)
		}
		fmt.Fprintf(&b, "%s%s\n", cursor, label)
	}
	b.WriteString("\n↑/↓ to move, enter to select, esc to cancel\n")
	return tea.NewView(b.String())
}

// --- logout ---

var shareLogoutCmd = &cobra.Command{
	Use:          "logout",
	Short:        "Log out of share.wethinkt.com",
	SilenceUsage: true,
	RunE:         runShareLogout,
}

func runShareLogout(cmd *cobra.Command, args []string) error {
	out := newShareOutputStyles()
	path := share.DefaultCredentialsPath()
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			fmt.Println(out.render(out.muted, "Not logged in."))
			return nil
		}
		return fmt.Errorf("remove credentials: %w", err)
	}
	fmt.Println(out.render(out.success, "Logged out."))
	return nil
}

// --- status ---

var shareStatusCmd = &cobra.Command{
	Use:          "status",
	Short:        "Show login and account status",
	SilenceUsage: true,
	RunE:         runShareStatus,
}

func runShareStatus(cmd *cobra.Command, args []string) error {
	out := newShareOutputStyles()
	creds, err := share.LoadCredentials(share.DefaultCredentialsPath())
	if err != nil {
		fmt.Printf("%s %s\n", out.render(out.warning, "Not logged in."), out.render(out.accent, "Run: thinkt share login"))
		return nil
	}

	fmt.Printf("%s %s\n", out.render(out.success, "Logged in as"), out.render(out.value, creds.Username))
	out.printKV("Endpoint:", creds.Endpoint)

	client := share.NewClientFromCreds(creds)
	profile, err := client.GetProfile()
	if err != nil {
		fmt.Printf("%s %s\n", out.render(out.warning, "Session expired."), out.render(out.accent, "Run: thinkt share login"))
		return nil
	}

	out.printKV("Sessions:", fmt.Sprintf("%d total (%d public, %d private)",
		profile.Stats.TotalSessions, profile.Stats.PublicSessions, profile.Stats.PrivateSessions))
	return nil
}

// --- push ---

var sharePushCmd = &cobra.Command{
	Use:          "push [session]",
	Short:        "Upload a session to share.wethinkt.com",
	Long:         "Upload a session for private storage or public sharing.\n\nWithout arguments, opens a project and session picker.",
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runSharePush,
}

func runSharePush(cmd *cobra.Command, args []string) error {
	out := newShareOutputStyles()
	creds, err := requireShareAuth()
	if err != nil {
		return err
	}

	registry := CreateSourceRegistry()

	// Build target flags
	flags := target.Flags{
		Project: sharePushProject,
		Sources: sharePushSources,
	}
	if len(args) > 0 {
		flags.Session = args[0]
	} else if sharePushSession != "" {
		flags.Session = sharePushSession
	}

	result, err := target.ResolveSession(registry, flags)
	if err != nil {
		return err
	}

	// Content filtering
	filter := target.DefaultFilter()
	filterFlagsSet := cmd.Flags().Changed("no-thinking") ||
		cmd.Flags().Changed("no-tools") ||
		cmd.Flags().Changed("no-media") ||
		cmd.Flags().Changed("system")

	if filterFlagsSet {
		noThink, _ := cmd.Flags().GetBool("no-thinking")
		noTools, _ := cmd.Flags().GetBool("no-tools")
		noMedia, _ := cmd.Flags().GetBool("no-media")
		system, _ := cmd.Flags().GetBool("system")
		filter.IncludeThinking = !noThink
		filter.IncludeToolUse = !noTools
		filter.IncludeToolResults = !noTools
		filter.IncludeMedia = !noMedia
		filter.IncludeSystem = system
	} else if target.IsTTY() {
		filter, err = target.PickContentFilter(filter)
		if err != nil {
			return err
		}
	}

	entries := target.FilterEntries(result.Entries, filter)

	// Title — flag or TUI input
	title := sharePushTitle
	if title == "" {
		title = buildExportTitle(result.Meta)
		if target.IsTTY() && !cmd.Flags().Changed("title") {
			title, err = pickTitle(title)
			if err != nil {
				return err
			}
		}
	}

	// Visibility — flag or TUI picker
	visibility := "private"
	if sharePushPublic {
		visibility = "public"
	} else if !cmd.Flags().Changed("public") && target.IsTTY() {
		visibility, err = pickVisibility()
		if err != nil {
			return err
		}
	}

	// Tags — flag or TUI input
	var tags []string
	if cmd.Flags().Changed("tags") {
		for _, t := range strings.Split(sharePushTags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	} else if target.IsTTY() {
		tags, err = pickTags()
		if err != nil {
			return err
		}
	}

	// Serialize filtered entries
	data, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshal entries: %w", err)
	}

	client := share.NewUploadClient(creds)
	fmt.Printf("%s %s (%s)\n",
		out.render(out.muted, "Uploading to share.wethinkt.com"),
		out.render(out.label, "..."),
		out.render(out.accent, visibility))

	resp, err := client.Upload(data, visibility, title, tags)
	if err != nil {
		return err
	}

	fmt.Printf("\n%s\n", out.render(out.url, resp.URL))
	if visibility == "private" {
		fmt.Println(out.render(out.muted, "(private - only you can view)"))
	}
	return nil
}

// --- share push TUI components ---

type titleInputModel struct {
	value     string
	cursor    int
	cancelled bool

	promptStyle lipgloss.Style
	inputStyle  lipgloss.Style
	helpStyle   lipgloss.Style
}

func newTitleInput(initial string) titleInputModel {
	t := theme.Current()
	return titleInputModel{
		value:       initial,
		cursor:      len(initial),
		promptStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true),
		inputStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())),
		helpStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)).MarginTop(1),
	}
}

func (m titleInputModel) Init() tea.Cmd { return nil }

func (m titleInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m titleInputModel) View() tea.View {
	display := m.inputStyle.Render(m.value[:m.cursor])
	if m.cursor < len(m.value) {
		display += lipgloss.NewStyle().Reverse(true).Render(string(m.value[m.cursor]))
		display += m.inputStyle.Render(m.value[m.cursor+1:])
	} else {
		display += lipgloss.NewStyle().Reverse(true).Render(" ")
	}

	var b strings.Builder
	b.WriteString(m.promptStyle.Render("Title: "))
	b.WriteString(display)
	b.WriteString("\n")
	b.WriteString(m.helpStyle.Render("enter to confirm • esc to cancel"))
	b.WriteString("\n")

	inner := lipgloss.NewStyle().Padding(1, 2).Render(b.String())
	return tea.NewView(inner)
}

func pickTitle(initial string) (string, error) {
	m := newTitleInput(initial)
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	result := final.(titleInputModel)
	if result.cancelled {
		return "", fmt.Errorf("cancelled")
	}
	return result.value, nil
}

// --- visibility picker ---

type visPickerModel struct {
	options   []string
	cursor    int
	cancelled bool

	titleStyle    lipgloss.Style
	cursorStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	normalStyle   lipgloss.Style
	helpStyle     lipgloss.Style
}

func newVisPicker() visPickerModel {
	t := theme.Current()
	return visPickerModel{
		options:       []string{"private", "public"},
		titleStyle:    lipgloss.NewStyle().Bold(true).MarginBottom(1),
		cursorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true),
		selectedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true),
		normalStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		helpStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)).MarginTop(1),
	}
}

func (m visPickerModel) Init() tea.Cmd { return nil }

func (m visPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m visPickerModel) View() tea.View {
	var b strings.Builder

	b.WriteString(m.titleStyle.Render("Visibility:"))
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

func pickVisibility() (string, error) {
	m := newVisPicker()
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	result := final.(visPickerModel)
	if result.cancelled {
		return "", fmt.Errorf("cancelled")
	}
	return result.options[result.cursor], nil
}

// --- tag input ---

type tagInputModel struct {
	value     string
	cursor    int
	cancelled bool

	promptStyle lipgloss.Style
	inputStyle  lipgloss.Style
	helpStyle   lipgloss.Style
}

func newTagInput() tagInputModel {
	t := theme.Current()
	return tagInputModel{
		promptStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true),
		inputStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())),
		helpStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)).MarginTop(1),
	}
}

func (m tagInputModel) Init() tea.Cmd { return nil }

func (m tagInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m tagInputModel) View() tea.View {
	display := m.inputStyle.Render(m.value[:m.cursor])
	if m.cursor < len(m.value) {
		display += lipgloss.NewStyle().Reverse(true).Render(string(m.value[m.cursor]))
		display += m.inputStyle.Render(m.value[m.cursor+1:])
	} else {
		display += lipgloss.NewStyle().Reverse(true).Render(" ")
	}

	var b strings.Builder
	b.WriteString(m.promptStyle.Render("Tags (comma-separated): "))
	b.WriteString(display)
	b.WriteString("\n")
	b.WriteString(m.helpStyle.Render("enter to confirm • esc to cancel"))
	b.WriteString("\n")

	inner := lipgloss.NewStyle().Padding(1, 2).Render(b.String())
	return tea.NewView(inner)
}

func pickTags() ([]string, error) {
	m := newTagInput()
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	result := final.(tagInputModel)
	if result.cancelled {
		return nil, fmt.Errorf("cancelled")
	}
	var tags []string
	for _, t := range strings.Split(result.value, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags, nil
}

// --- list ---

var shareListCmd = &cobra.Command{
	Use:          "list",
	Short:        "List your shared sessions",
	Aliases:      []string{"ls"},
	SilenceUsage: true,
	RunE:         runShareList,
}

func runShareList(cmd *cobra.Command, args []string) error {
	out := newShareOutputStyles()
	creds, err := requireShareAuth()
	if err != nil {
		return err
	}

	client := share.NewClientFromCreds(creds)
	sessions, err := client.ListSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if len(sessions) == 0 {
		if shareListJSON {
			fmt.Println("[]")
		} else {
			fmt.Printf("%s %s\n", out.render(out.muted, "No sessions."), out.render(out.accent, "Push one with: thinkt share push <path>"))
		}
		return nil
	}

	if shareListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(sessions)
	}

	if term.IsTerminal(int(os.Stdout.Fd())) {
		return runShareBrowser(sessions, shareTUI.ShareBrowserMine)
	}

	printSessionTable(sessions)
	return nil
}

// --- explore ---

var shareExploreCmd = &cobra.Command{
	Use:          "explore",
	Short:        "Browse public sessions",
	SilenceUsage: true,
	RunE:         runShareExplore,
}

func runShareExplore(cmd *cobra.Command, args []string) error {
	out := newShareOutputStyles()
	token := ""
	if creds, err := share.LoadCredentials(share.DefaultCredentialsPath()); err == nil {
		token = creds.Token
	}

	endpoint, err := share.Endpoint()
	if err != nil {
		return err
	}
	client := share.NewClient(endpoint, token)
	resp, err := client.Explore(shareExploreSort, shareExploreTag, 1)
	if err != nil {
		return fmt.Errorf("explore: %w", err)
	}

	if len(resp.Sessions) == 0 {
		fmt.Println(out.render(out.muted, "No public sessions found."))
		return nil
	}

	if term.IsTerminal(int(os.Stdout.Fd())) {
		return runShareBrowser(resp.Sessions, shareTUI.ShareBrowserExplore)
	}

	printSessionTable(resp.Sessions)
	return nil
}

func runShareBrowser(sessions []share.Session, mode shareTUI.ShareBrowserMode) error {
	m := shareTUI.NewShareBrowser(sessions, mode)
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return err
	}

	if result := final.(shareTUI.ShareBrowserModel).Result(); result != nil && result.Action == "open" {
		endpoint, err := share.Endpoint()
		if err != nil {
			return err
		}
		u := endpoint + "/t/" + result.Slug
		fmt.Println(newShareOutputStyles().render(newShareOutputStyles().url, u))
		return openShareBrowser(u)
	}
	return nil
}

// --- open ---

var shareOpenCmd = &cobra.Command{
	Use:          "open <slug>",
	Short:        "Open a session in the web browser",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runShareOpen,
}

func runShareOpen(cmd *cobra.Command, args []string) error {
	out := newShareOutputStyles()
	endpoint, err := share.Endpoint()
	if err != nil {
		return err
	}
	u := endpoint + "/t/" + args[0]
	fmt.Println(out.render(out.url, u))
	return openShareBrowser(u)
}

var shareWebCmd = &cobra.Command{
	Use:          "web",
	Short:        "Open the share website in the web browser",
	SilenceUsage: true,
	Args:         cobra.NoArgs,
	RunE:         runShareWeb,
}

func runShareWeb(cmd *cobra.Command, args []string) error {
	out := newShareOutputStyles()
	u, err := share.Endpoint()
	if err != nil {
		return err
	}
	fmt.Println(out.render(out.url, u))
	return openShareBrowser(u)
}

// --- delete ---

var shareDeleteCmd = &cobra.Command{
	Use:          "delete <slug>",
	Short:        "Delete a shared session",
	Aliases:      []string{"rm"},
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runShareDelete,
}

func runShareDelete(cmd *cobra.Command, args []string) error {
	out := newShareOutputStyles()
	creds, err := requireShareAuth()
	if err != nil {
		return err
	}

	slug := args[0]

	if !shareDeleteForce {
		fmt.Printf("%s %s? %s ", out.render(out.warning, "Delete session"), out.render(out.value, fmt.Sprintf("%q", slug)), out.render(out.label, "[y/N]"))
		var answer string
		_, _ = fmt.Scanln(&answer)
		if strings.ToLower(answer) != "y" {
			fmt.Println(out.render(out.muted, "Cancelled."))
			return nil
		}
	}

	client := share.NewClientFromCreds(creds)
	if err := client.DeleteSession(slug); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	fmt.Printf("%s %s\n", out.render(out.success, "Deleted"), out.render(out.value, slug))
	return nil
}

// --- profile ---

var shareProfileCmd = &cobra.Command{
	Use:          "profile",
	Short:        "Show your profile and stats",
	SilenceUsage: true,
	RunE:         runShareProfile,
}

func runShareProfile(cmd *cobra.Command, args []string) error {
	out := newShareOutputStyles()
	creds, err := requireShareAuth()
	if err != nil {
		return err
	}

	client := share.NewClientFromCreds(creds)
	profile, err := client.GetProfile()
	if err != nil {
		return fmt.Errorf("get profile: %w", err)
	}

	out.printKV("Name:", profile.User.Name)
	out.printKV("Email:", profile.User.Email)
	out.printKV("Sessions:", fmt.Sprintf("%d total (%d public, %d private)",
		profile.Stats.TotalSessions, profile.Stats.PublicSessions, profile.Stats.PrivateSessions))
	out.printKV("Storage:", thinkt.FormatBytes(int64(profile.Stats.TotalBytes)))

	if len(profile.Tags) > 0 {
		tags := make([]string, 0, len(profile.Tags))
		for _, tc := range profile.Tags {
			tags = append(tags, tc.Tag)
		}
		out.printKV("Tags:", strings.Join(tags, ", "))
	}
	return nil
}

// --- helpers ---

func configPathOrFallback() string {
	if p, err := config.Path(); err == nil {
		return p
	}
	return "~/.thinkt/config.json"
}

func requireShareAuth() (*share.Credentials, error) {
	creds, err := share.LoadCredentials(share.DefaultCredentialsPath())
	if err != nil {
		return nil, fmt.Errorf("not logged in — run: thinkt share login")
	}
	return creds, nil
}

func printSessionTable(sessions []share.Session) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tTITLE\tVISIBILITY\tSIZE\tLIKES\tCREATED")
	for _, s := range sessions {
		created := s.CreatedAt
		if len(created) >= 10 {
			created = created[:10]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
			s.Slug, shareTruncate(s.Title, 40), s.Visibility,
			thinkt.FormatBytes(int64(s.SizeBytes)), s.LikesCount, created)
	}
	w.Flush()
}

func shareTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "..."
}

func openShareBrowser(rawURL string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", rawURL)
	case "linux":
		c = exec.Command("xdg-open", rawURL)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		return nil
	}
	return c.Start()
}
