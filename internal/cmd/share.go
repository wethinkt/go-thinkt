package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/share"
	shareTUI "github.com/wethinkt/go-thinkt/internal/tui"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
	"golang.org/x/term"
)

var (
	sharePushPublic  bool
	shareExploreTag  string
	shareExploreSort string
	shareDeleteForce bool
	shareLoginGoogle bool
	shareLoginGitHub bool
	shareListJSON    bool
)

var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "Share traces on share.wethinkt.com",
	Long:  "Upload, browse, and manage reasoning traces on the wethinkt sharing platform.",
	Args:  cobra.NoArgs,
	RunE:  runShareList,
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
	Long:         "Authenticate with share.wethinkt.com using GitHub or Google to enable sharing traces.",
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
	s := "\nLog in with:\n\n"
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
		s += fmt.Sprintf("%s%s\n", cursor, label)
	}
	s += "\n↑/↓ to move, enter to select, esc to cancel\n"
	return tea.NewView(s)
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

	out.printKV("Traces:", fmt.Sprintf("%d total (%d public, %d private)",
		profile.Stats.TotalTraces, profile.Stats.PublicTraces, profile.Stats.PrivateTraces))
	return nil
}

// --- push ---

var sharePushCmd = &cobra.Command{
	Use:          "push <path>",
	Short:        "Upload a trace to share.wethinkt.com",
	Long:         "Upload a Thinkt reasoning trace for private storage or public sharing.",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runSharePush,
}

func runSharePush(cmd *cobra.Command, args []string) error {
	out := newShareOutputStyles()
	creds, err := requireShareAuth()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("read trace: %w", err)
	}

	visibility := "private"
	if sharePushPublic {
		visibility = "public"
	}

	title := filepath.Base(args[0])
	client := share.NewUploadClient(creds)
	fmt.Printf("%s %s (%s)\n",
		out.render(out.muted, "Uploading to share.wethinkt.com"),
		out.render(out.label, "..."),
		out.render(out.accent, visibility))

	resp, err := client.Upload(data, visibility, title)
	if err != nil {
		return err
	}

	fmt.Printf("\n%s\n", out.render(out.url, resp.URL))
	if visibility == "private" {
		fmt.Println(out.render(out.muted, "(private - only you can view)"))
	}
	return nil
}

// --- list ---

var shareListCmd = &cobra.Command{
	Use:          "list",
	Short:        "List your shared traces",
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
	traces, err := client.ListTraces()
	if err != nil {
		return fmt.Errorf("list traces: %w", err)
	}

	if len(traces) == 0 {
		if shareListJSON {
			fmt.Println("[]")
		} else {
			fmt.Printf("%s %s\n", out.render(out.muted, "No traces."), out.render(out.accent, "Push one with: thinkt share push <path>"))
		}
		return nil
	}

	if shareListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(traces)
	}

	if term.IsTerminal(int(os.Stdout.Fd())) {
		return runShareBrowser(traces, shareTUI.ShareBrowserMine)
	}

	printTraceTable(traces)
	return nil
}

// --- explore ---

var shareExploreCmd = &cobra.Command{
	Use:          "explore",
	Short:        "Browse public traces",
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

	if len(resp.Traces) == 0 {
		fmt.Println(out.render(out.muted, "No public traces found."))
		return nil
	}

	if term.IsTerminal(int(os.Stdout.Fd())) {
		return runShareBrowser(resp.Traces, shareTUI.ShareBrowserExplore)
	}

	printTraceTable(resp.Traces)
	return nil
}

func runShareBrowser(traces []share.Trace, mode shareTUI.ShareBrowserMode) error {
	m := shareTUI.NewShareBrowser(traces, mode)
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
	Short:        "Open a trace in the web browser",
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
	Short:        "Delete a shared trace",
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
		fmt.Printf("%s %s? %s ", out.render(out.warning, "Delete trace"), out.render(out.value, fmt.Sprintf("%q", slug)), out.render(out.label, "[y/N]"))
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(answer) != "y" {
			fmt.Println(out.render(out.muted, "Cancelled."))
			return nil
		}
	}

	client := share.NewClientFromCreds(creds)
	if err := client.DeleteTrace(slug); err != nil {
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
	out.printKV("Traces:", fmt.Sprintf("%d total (%d public, %d private)",
		profile.Stats.TotalTraces, profile.Stats.PublicTraces, profile.Stats.PrivateTraces))
	out.printKV("Storage:", shareFormatBytes(profile.Stats.TotalBytes))

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

func requireShareAuth() (*share.Credentials, error) {
	creds, err := share.LoadCredentials(share.DefaultCredentialsPath())
	if err != nil {
		return nil, fmt.Errorf("not logged in — run: thinkt share login")
	}
	return creds, nil
}

func printTraceTable(traces []share.Trace) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tTITLE\tVISIBILITY\tSIZE\tLIKES\tCREATED")
	for _, t := range traces {
		created := t.CreatedAt
		if len(created) >= 10 {
			created = created[:10]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
			t.Slug, shareTruncate(t.Title, 40), t.Visibility,
			shareFormatBytes(t.SizeBytes), t.LikesCount, created)
	}
	w.Flush()
}

func shareTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "..."
}

func shareFormatBytes(b int) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
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
