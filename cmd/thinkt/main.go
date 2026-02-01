// thinkt provides tools for exploring and extracting from Claude Code sessions.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"slices"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/analytics"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/cli"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/kimi"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/prompt"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/server"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tui"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tui/theme"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tuilog"
)

// Global flags
var (
	profilePath string
	profileFile *os.File // held open for profiling
	logPath     string
	verbose     bool
)

// Serve command flags
var (
	servePort int
	serveHost string
	serveOpen bool
)

// Serve MCP subcommand flags
var (
	mcpStdio bool
	mcpPort  int
	mcpHost  string
)

// Prompts subcommand flags
var (
	inputFile    string
	outputFile   string
	appendMode   bool
	formatType   string
	templateFile string
	traceType    string
)

// Supported trace types
const TraceTypeClaude = "claude"

var supportedTypes = []string{TraceTypeClaude}

var rootCmd = &cobra.Command{
	Use:   "thinkt",
	Short: "Tools for AI assistant session exploration and extraction",
	Long: `thinkt provides tools for exploring and extracting data from AI coding assistant sessions.

Supports: Claude Code, Kimi Code

Running without a subcommand launches the interactive TUI.

Commands:
  sources   Manage and view available session sources
  tui       Launch interactive TUI explorer (default)
  prompts   Extract and manage prompts from trace files
  projects  List and manage projects
  sessions  List and manage sessions

Examples:
  thinkt                          # Launch TUI
  thinkt sources list             # List available sources (kimi, claude)
  thinkt projects list            # List all projects from all sources
  thinkt prompts extract          # Extract prompts from latest session
  thinkt sessions list -p myproj  # List sessions for a project`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Start CPU profiling if requested
		if profilePath != "" {
			f, err := os.Create(profilePath)
			if err != nil {
				return fmt.Errorf("create profile file: %w", err)
			}
			profileFile = f

			if err := pprof.StartCPUProfile(f); err != nil {
				f.Close()
				profileFile = nil
				return fmt.Errorf("start CPU profile: %w", err)
			}
		}
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		// Stop CPU profiling
		if profileFile != nil {
			pprof.StopCPUProfile()
			profileFile.Close()
			profileFile = nil
		}
		return nil
	},
	RunE: runTUI,
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch interactive TUI explorer",
	Long: `Browse Claude Code conversation sessions in a three-column
terminal interface. Navigate projects, sessions, and conversation
content with keyboard controls.

Column 1: Project directories
Column 2: Sessions with timestamps
Column 3: Conversation content with colored blocks

Press T to open thinking-tracer for the selected session.`,
	RunE: runTUI,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start local HTTP server for trace exploration",
	Long: `Start a local HTTP server for exploring AI conversation traces.

The server provides:
  - REST API for accessing projects and sessions
  - Web interface for visual trace exploration

All data stays on your machine - nothing is uploaded to external servers.

Use 'thinkt serve mcp' for MCP (Model Context Protocol) server.

Examples:
  thinkt serve                    # Start HTTP server on default port 7433
  thinkt serve -p 8080            # Start on custom port
  thinkt serve --no-open          # Don't auto-open browser`,
	RunE: runServeHTTP,
}

var serveMcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server for AI tool integration",
	Long: `Start an MCP (Model Context Protocol) server for AI tool integration.

By default, runs on stdio for use with Claude Desktop and other MCP clients.
Use --port to run over HTTP instead.

Examples:
  thinkt serve mcp                # MCP server on stdio (default)
  thinkt serve mcp --stdio        # Explicitly use stdio transport
  thinkt serve mcp --port 8081    # MCP server over HTTP`,
	RunE: runServeMCP,
}

var promptsCmd = &cobra.Command{
	Use:   "prompts",
	Short: "Extract and manage prompts from trace files",
	Long: `Extract user prompts from LLM agent trace files
and generate output in various formats.

Supported trace types:
  claude    Claude Code JSONL traces (~/.claude/projects/)

Examples:
  thinkt prompts extract -i session.jsonl
  thinkt prompts extract            # uses latest session
  thinkt prompts list
  thinkt prompts info
  thinkt prompts templates`,
}

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract prompts from a trace file",
	RunE:  runExtract,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available trace files",
	RunE:  runList,
}

var infoCmd = &cobra.Command{
	Use:   "info [file]",
	Short: "Show session information",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runInfo,
}

var templatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "List available templates and show template variables",
	RunE:  runTemplates,
}

// Projects command flags
var (
	treeFormat      bool
	summaryTemplate string
	sortBy          string
	sortDesc        bool
	forceDelete     bool
)

// Sessions command flags
var (
	sessionProject     string
	sessionForcePicker bool     // --pick flag to force project picker
	sessionSources     []string // --source flag for sessions
	sessionForceDelete bool
	sessionSortBy      string
	sessionSortDesc    bool
	sessionTemplate    string
	sessionViewAll     bool
	sessionViewRaw     bool // --raw flag for undecorated output
)

// Search and stats command flags
var (
	searchProject string
	searchLimit   int
	statsProject  string
	statsLimit    int
	statsDays     int
	outputJSON    bool
)

// Projects command flags
var (
	projectSources []string // --source flag (can be specified multiple times)
	withSessions   bool     // --with-sessions flag for summary
	longFormat     bool     // --long flag for columnar output
)

var projectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List projects from all sources",
	Long: `List all projects from available sources (Kimi, Claude, etc.).

By default, outputs project paths one per line from ALL sources.
Use --source to limit to specific sources (can be specified multiple times).
Use --long for detailed columns (source, sessions, modified time).
Use --tree for a grouped tree view.

Examples:
  thinkt projects                      # All sources, paths one per line
  thinkt projects --long               # Detailed columns
  thinkt projects --source kimi        # Only Kimi projects
  thinkt projects --source claude      # Only Claude projects
  thinkt projects --source kimi --source claude  # Both sources
  thinkt projects --tree               # Tree view grouped by source/parent
  thinkt projects summary              # Detailed summary with session names`,
	RunE: runProjects,
}

var projectsSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show detailed project summary",
	Long: `Show detailed information about each project including
session count and last modified time.

By default, shows projects from ALL sources.
Use --source to limit to specific sources.

Sorting:
  --sort name|time    Sort by project name or modified time (default: time)
  --desc              Sort descending (default for time)
  --asc               Sort ascending (default for name)

Output can be customized with a Go text/template via --template.

` + cli.SummaryTemplateHelp,
	RunE: runProjectsSummary,
}

var projectsDeleteCmd = &cobra.Command{
	Use:   "delete <project-path>",
	Short: "Delete a project and all its sessions",
	Long: `Delete a Claude Code project directory and all session data within it.

The project-path can be:
  - Full project path (e.g., /Users/evan/myproject)
  - Path relative to current directory

Before deletion, shows the number of sessions and last modified time,
then prompts for confirmation. Use --force to skip the confirmation.

Examples:
  thinkt projects delete /Users/evan/myproject
  thinkt projects delete ./myproject
  thinkt projects delete --force /Users/evan/myproject`,
	Args: cobra.ExactArgs(1),
	RunE: runProjectsDelete,
}

var projectsCopyCmd = &cobra.Command{
	Use:   "copy <project-path> <target-dir>",
	Short: "Copy project sessions to a target directory",
	Long: `Copy all session files from a Claude Code project to a target directory.

The project-path can be:
  - Full project path (e.g., /Users/evan/myproject)
  - Path relative to current directory

The target directory will be created if it doesn't exist.
Session files (.jsonl) and index files are copied.

Examples:
  thinkt projects copy /Users/evan/myproject ./backup
  thinkt projects copy /Users/evan/myproject /tmp/export`,
	Args: cobra.ExactArgs(2),
	RunE: runProjectsCopy,
}

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List and manage sessions across all sources",
	Long: `List and manage sessions from Kimi, Claude, and other sources.

Project selection:
  - In a project directory: automatically uses that project
  - Otherwise: shows interactive project picker
  - -p/--project <path>: use specified project
  - --pick: force picker even if in a project directory

Use --source to filter by source (kimi, claude).

Examples:
  thinkt sessions list              # Auto-detect or picker
  thinkt sessions list --pick       # Force project picker
  thinkt sessions list -p ./myproject
  thinkt sessions summary -p ./myproject --source kimi
  thinkt sessions view              # Interactive picker
  thinkt sessions delete -p ./myproject <session-id>
  thinkt sessions copy -p ./myproject <session-id> ./backup`,
	RunE: runSessionsList,
}

var sessionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sessions (auto-detects project from cwd)",
	Long: `List all sessions in a project.

Project selection:
  - In a project directory: automatically uses that project
  - Otherwise: shows interactive project picker
  - -p/--project <path>: use specified project
  - --pick: force picker even if in a project directory

Examples:
  thinkt sessions list              # Auto-detect from cwd or picker
  thinkt sessions list --pick       # Force project picker
  thinkt sessions list -p ./myproject
  thinkt sessions list --source kimi`,
	RunE: runSessionsList,
}

var sessionsSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show detailed session summary",
	Long: `Show detailed information about each session in a project.

Sorting:
  --sort name|time    Sort by session name or modified time (default: time)
  --desc              Sort descending (default for time)
  --asc               Sort ascending (default for name)

Output can be customized with a Go text/template via --template.

` + cli.SessionSummaryTemplateHelp,
	RunE: runSessionsSummary,
}

var sessionsDeleteCmd = &cobra.Command{
	Use:   "delete <session>",
	Short: "Delete a session",
	Long: `Delete a Claude Code session file.

The session can be specified as:
  - Full path to the .jsonl file
  - Session ID (requires -p/--project)
  - Filename (requires -p/--project)

Before deletion, shows session info and prompts for confirmation.
Use --force to skip the confirmation.

Examples:
  thinkt sessions delete /full/path/to/session.jsonl
  thinkt sessions delete -p ./myproject abc123
  thinkt sessions delete -p ./myproject --force abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runSessionsDelete,
}

var sessionsCopyCmd = &cobra.Command{
	Use:   "copy <session> <target>",
	Short: "Copy a session to a target location",
	Long: `Copy a Claude Code session file to a target location.

The session can be specified as:
  - Full path to the .jsonl file
  - Session ID (requires -p/--project)
  - Filename (requires -p/--project)

The target can be a file path or directory.

Examples:
  thinkt sessions copy /full/path/to/session.jsonl ./backup/
  thinkt sessions copy -p ./myproject abc123 ./backup/
  thinkt sessions copy -p ./myproject abc123 ./backup/renamed.jsonl`,
	Args: cobra.ExactArgs(2),
	RunE: runSessionsCopy,
}

var sessionsViewCmd = &cobra.Command{
	Use:   "view [session]",
	Short: "View a session in the terminal (interactive picker)",
	Long: `View a session in a full-terminal viewer.

If no session is specified, shows an interactive picker of all recent sessions.
The picker works across all sources (kimi, claude).

The session can be specified as:
  - Full path to the .jsonl file
  - Session ID (requires -p/--project)
  - Filename (requires -p/--project)

Navigation:
  ↑/↓/j/k     Scroll up/down
  PgUp/PgDn   Page up/down
  g/G         Go to top/bottom
  q/Esc       Quit

Use --raw to output undecorated text to stdout (no TUI).

Examples:
  thinkt sessions view              # Interactive picker across all sources
  thinkt sessions view /full/path/to/session.jsonl
  thinkt sessions view -p ./myproject abc123
  thinkt sessions view -p ./myproject --all        # view all
  thinkt sessions view /path/to/session --raw      # raw output to stdout`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSessionsView,
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search across sessions using DuckDB",
	Long: `Full-text search across all Claude Code sessions.

Uses DuckDB to efficiently search through JSONL session files.
Searches in user messages and assistant responses.

Examples:
  thinkt search "authentication"
  thinkt search -p ./myproject "error handling"
  thinkt search --limit 100 "database"
  thinkt search --json "api" | jq .`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Analytics and statistics using DuckDB",
	Long: `Analyze Claude Code sessions using DuckDB.

Provides various analytics including token usage, tool frequency,
word frequency, activity timelines, and more.

Examples:
  thinkt stats tokens
  thinkt stats tools -p ./myproject
  thinkt stats words --limit 100
  thinkt stats activity --days 7`,
}

var statsTokensCmd = &cobra.Command{
	Use:   "tokens",
	Short: "Token usage by session",
	RunE:  runStatsTokens,
}

var statsToolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Tool usage frequency",
	RunE:  runStatsTools,
}

var statsWordsCmd = &cobra.Command{
	Use:   "words",
	Short: "Word frequency in user prompts",
	RunE:  runStatsWords,
}

var statsActivityCmd = &cobra.Command{
	Use:   "activity",
	Short: "Daily activity timeline",
	RunE:  runStatsActivity,
}

var statsModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Model usage statistics",
	RunE:  runStatsModels,
}

var statsErrorsCmd = &cobra.Command{
	Use:   "errors",
	Short: "Tool errors and failures",
	RunE:  runStatsErrors,
}

var queryCmd = &cobra.Command{
	Use:   "query <sql>",
	Short: "Run raw SQL query using DuckDB",
	Long: `Execute a raw SQL query against session data.

DuckDB can read JSONL files directly using read_json_auto().
The base directory pattern is available as a placeholder.

Examples:
  thinkt query "SELECT COUNT(*) FROM read_json_auto('~/.claude/projects/*/*.jsonl')"
  thinkt query "SELECT DISTINCT json_extract_string(entry, '$.model') FROM read_json_auto('~/.claude/projects/*/*.jsonl')"`,
	Args: cobra.ExactArgs(1),
	RunE: runQuery,
}

// Theme command
var themeCmd = &cobra.Command{
	Use:   "theme",
	Short: "Display and manage TUI theme settings",
	Long: `Display the current TUI theme with styled samples.

The theme controls colors for conversation blocks, labels, borders,
and other UI elements. Themes are stored in ~/.thinkt/themes/.

Built-in themes: dark, light
User themes can be added to ~/.thinkt/themes/

Examples:
  thinkt theme             # Show current theme with samples
  thinkt theme --json      # Output theme as JSON
  thinkt theme list        # List all available themes
  thinkt theme set light   # Switch to light theme
  thinkt theme builder     # Interactive theme builder (coming soon)`,
	RunE: runTheme,
}

var themeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available themes",
	Long:  `List all built-in and user themes. The active theme is marked with *.`,
	RunE:  runThemeList,
}

var themeSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Set the active theme",
	Long: `Set the active theme by name.

Available built-in themes: dark, light
User themes from ~/.thinkt/themes/ are also available.

Examples:
  thinkt theme set dark
  thinkt theme set light
  thinkt theme set my-custom-theme`,
	Args: cobra.ExactArgs(1),
	RunE: runThemeSet,
}

// Source management commands
var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "Manage and view available session sources",
	Long: `View and manage available AI assistant session sources.

Sources are the AI coding assistants that store session data
on this machine (e.g., Claude Code, Kimi Code).

Examples:
  thinkt sources list      # List all available sources
  thinkt sources status    # Show detailed source status`,
}

var sourcesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available session sources",
	Long: `List all session sources (Kimi, Claude, etc.) and their availability.

Shows which sources have session data available on this machine.`,
	RunE: runSourcesList,
}

var sourcesStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show detailed source status",
	Long: `Show detailed information about each session source including
workspace ID, base path, and project count.`,
	RunE: runSourcesStatus,
}

func main() {
	// Global flags on root
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVar(&profilePath, "profile", "", "write CPU profile to file")

	// TUI-specific flags
	tuiCmd.Flags().StringVar(&logPath, "log", "", "write debug log to file")
	// Also add to root since it can run TUI directly
	rootCmd.Flags().StringVar(&logPath, "log", "", "write debug log to file")

	// Prompts subcommand flags
	promptsCmd.PersistentFlags().StringVarP(&traceType, "type", "t", TraceTypeClaude, "trace type (claude)")

	// Extract flags
	extractCmd.Flags().StringVarP(&inputFile, "input", "i", "", "input trace file (use - for stdin)")
	extractCmd.Flags().StringVarP(&outputFile, "output", "o", "-", "output file (default stdout)")
	extractCmd.Flags().BoolVarP(&appendMode, "append", "a", false, "append to existing file")
	extractCmd.Flags().StringVarP(&formatType, "format", "f", "markdown", "output format (markdown|json|plain)")
	extractCmd.Flags().StringVar(&templateFile, "template", "", "custom template file (for markdown format)")

	// Projects command flags
	projectsCmd.PersistentFlags().StringArrayVarP(&projectSources, "source", "s", nil, "source to include (kimi|claude, can be specified multiple times, default: all)")
	projectsCmd.Flags().BoolVarP(&treeFormat, "tree", "t", false, "show tree view grouped by parent directory")
	projectsCmd.Flags().BoolVarP(&longFormat, "long", "l", false, "show detailed columns (path, source, sessions, modified)")
	projectsSummaryCmd.Flags().StringVar(&summaryTemplate, "template", "", "custom Go text/template for output")
	projectsSummaryCmd.Flags().StringVar(&sortBy, "sort", "time", "sort by: name, time")
	projectsSummaryCmd.Flags().BoolVar(&sortDesc, "desc", false, "sort descending (default for time)")
	projectsSummaryCmd.Flags().Bool("asc", false, "sort ascending (default for name)")
	projectsSummaryCmd.Flags().BoolVar(&withSessions, "with-sessions", false, "include session names in output")
	projectsDeleteCmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "skip confirmation prompt")

	// Sessions command flags
	// Project selection:
	// - No flags: auto-detect from cwd, fallback to picker
	// - --pick: force picker even if in a project directory
	// - -p <path>: use specified path
	sessionsCmd.PersistentFlags().StringVarP(&sessionProject, "project", "p", "", "project path (auto-detects from cwd if not set)")
	sessionsCmd.PersistentFlags().BoolVar(&sessionForcePicker, "pick", false, "force project picker even if in a known project directory")
	sessionsCmd.PersistentFlags().StringArrayVarP(&sessionSources, "source", "s", nil, "filter by source (kimi|claude, can be specified multiple times)")
	sessionsSummaryCmd.Flags().StringVar(&sessionTemplate, "template", "", "custom Go text/template for output")
	sessionsSummaryCmd.Flags().StringVar(&sessionSortBy, "sort", "time", "sort by: name, time")
	sessionsSummaryCmd.Flags().BoolVar(&sessionSortDesc, "desc", false, "sort descending (default for time)")
	sessionsSummaryCmd.Flags().Bool("asc", false, "sort ascending (default for name)")
	sessionsDeleteCmd.Flags().BoolVarP(&sessionForceDelete, "force", "f", false, "skip confirmation prompt")
	sessionsViewCmd.Flags().BoolVarP(&sessionViewAll, "all", "a", false, "view all sessions in time order")
	sessionsViewCmd.Flags().BoolVar(&sessionViewRaw, "raw", false, "output raw text without decoration/rendering")

	// Search command flags
	searchCmd.Flags().StringVarP(&searchProject, "project", "p", "", "limit search to a project")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 50, "maximum results")
	searchCmd.Flags().BoolVar(&outputJSON, "json", false, "output as JSON")

	// Stats command flags (common)
	statsCmd.PersistentFlags().StringVarP(&statsProject, "project", "p", "", "limit stats to a project")
	statsCmd.PersistentFlags().IntVarP(&statsLimit, "limit", "n", 20, "maximum results")
	statsCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "output as JSON")
	statsActivityCmd.Flags().IntVar(&statsDays, "days", 30, "number of days to show")

	// Query command flags
	queryCmd.Flags().BoolVar(&outputJSON, "json", false, "output as JSON")

	// Build command tree
	projectsCmd.AddCommand(projectsSummaryCmd)
	projectsCmd.AddCommand(projectsDeleteCmd)
	projectsCmd.AddCommand(projectsCopyCmd)
	sessionsCmd.AddCommand(sessionsListCmd)
	sessionsCmd.AddCommand(sessionsSummaryCmd)
	sessionsCmd.AddCommand(sessionsDeleteCmd)
	sessionsCmd.AddCommand(sessionsCopyCmd)
	sessionsCmd.AddCommand(sessionsViewCmd)
	statsCmd.AddCommand(statsTokensCmd)
	statsCmd.AddCommand(statsToolsCmd)
	statsCmd.AddCommand(statsWordsCmd)
	statsCmd.AddCommand(statsActivityCmd)
	statsCmd.AddCommand(statsModelsCmd)
	statsCmd.AddCommand(statsErrorsCmd)
	promptsCmd.AddCommand(extractCmd)
	promptsCmd.AddCommand(listCmd)
	promptsCmd.AddCommand(infoCmd)
	promptsCmd.AddCommand(templatesCmd)

	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(promptsCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(projectsCmd)
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(sourcesCmd)
	rootCmd.AddCommand(themeCmd)

	// Theme subcommands
	themeCmd.AddCommand(themeListCmd)
	themeCmd.AddCommand(themeSetCmd)

	// Theme command flags
	themeCmd.Flags().BoolVar(&outputJSON, "json", false, "output theme as JSON")

	// Serve command flags
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 7433, "server port")
	serveCmd.Flags().StringVar(&serveHost, "host", "localhost", "server host")
	serveCmd.Flags().BoolVar(&serveOpen, "open", true, "auto-open browser")
	serveCmd.PersistentFlags().StringVar(&logPath, "log", "", "write debug log to file")

	// Serve MCP subcommand
	serveCmd.AddCommand(serveMcpCmd)
	serveMcpCmd.Flags().BoolVar(&mcpStdio, "stdio", false, "use stdio transport (default if no --port)")
	serveMcpCmd.Flags().IntVarP(&mcpPort, "port", "p", 0, "run MCP over HTTP on this port")
	serveMcpCmd.Flags().StringVar(&mcpHost, "host", "localhost", "host to bind MCP HTTP server")

	// Sources subcommands
	sourcesCmd.AddCommand(sourcesListCmd)
	sourcesCmd.AddCommand(sourcesStatusCmd)
	sourcesCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "output as JSON")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	// Initialize logger if requested
	if logPath != "" {
		if err := tuilog.Init(logPath); err != nil {
			return fmt.Errorf("init logger: %w", err)
		}
		defer tuilog.Log.Close()
	}

	tuilog.Log.Info("Starting TUI")

	// Get initial terminal size - try stdout, stdin, stderr in order
	var opts []tea.ProgramOption
	for _, fd := range []int{int(os.Stdout.Fd()), int(os.Stdin.Fd()), int(os.Stderr.Fd())} {
		if term.IsTerminal(fd) {
			w, h, err := term.GetSize(fd)
			if err == nil && w > 0 && h > 0 {
				tuilog.Log.Info("Terminal size", "fd", fd, "width", w, "height", h)
				opts = append(opts, tea.WithWindowSize(w, h))
				break
			}
		}
	}

	// Use new Shell with NavStack for multi-source support
	shell := tui.NewShell()
	p := tea.NewProgram(shell, opts...)
	_, err := p.Run()

	tuilog.Log.Info("TUI exited", "error", err)
	return err
}

func runServeHTTP(cmd *cobra.Command, args []string) error {
	// Initialize logger if requested
	if logPath != "" {
		if err := tuilog.Init(logPath); err != nil {
			return fmt.Errorf("init logger: %w", err)
		}
		defer tuilog.Log.Close()
	}

	tuilog.Log.Info("Starting HTTP server", "port", servePort, "host", serveHost)

	// Create source registry
	registry := createSourceRegistry()
	tuilog.Log.Info("Source registry created", "stores", len(registry.All()))

	// Create context that cancels on interrupt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		tuilog.Log.Info("Received interrupt signal, shutting down")
		fmt.Fprintln(os.Stderr, "\nShutting down...")
		cancel()
	}()

	// HTTP mode: start HTTP server
	config := server.Config{
		Mode: server.ModeHTTPOnly,
		Port: servePort,
		Host: serveHost,
	}
	srv := server.NewHTTPServer(registry, config)

	// Print startup message
	fmt.Println("🚀 Thinkt server starting...")
	fmt.Println("📁 Serving traces from local sources")

	// Auto-open browser if requested (after small delay for server to start)
	if serveOpen {
		go func() {
			url := fmt.Sprintf("http://%s", srv.Addr())
			fmt.Printf("🌐 Opening %s in browser...\n", url)
			openBrowser(url)
		}()
	}

	// Start server
	return srv.ListenAndServe(ctx)
}

func runServeMCP(cmd *cobra.Command, args []string) error {
	// Initialize logger if requested
	if logPath != "" {
		if err := tuilog.Init(logPath); err != nil {
			return fmt.Errorf("init logger: %w", err)
		}
		defer tuilog.Log.Close()
	}

	// Determine transport mode: stdio (default) or HTTP
	useStdio := mcpStdio || mcpPort == 0

	tuilog.Log.Info("Starting MCP server", "stdio", useStdio, "port", mcpPort)

	// Create source registry
	registry := createSourceRegistry()
	tuilog.Log.Info("Source registry created", "stores", len(registry.All()))

	// Create context that cancels on interrupt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		tuilog.Log.Info("Received interrupt signal, shutting down")
		fmt.Fprintln(os.Stderr, "\nShutting down...")
		cancel()
	}()

	// Create MCP server
	tuilog.Log.Info("Creating MCP server")
	mcpServer := server.NewMCPServer(registry)

	if useStdio {
		// Stdio transport
		fmt.Fprintln(os.Stderr, "Starting MCP server on stdio...")
		tuilog.Log.Info("Running MCP server on stdio")
		err := mcpServer.RunStdio(ctx)
		tuilog.Log.Info("MCP server exited", "error", err)
		// EOF on stdin is normal termination (client disconnected), not an error
		if err != nil {
			if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "EOF") {
				tuilog.Log.Info("EOF received, treating as normal termination")
				return nil
			}
			return err
		}
		return nil
	}

	// HTTP transport (SSE)
	tuilog.Log.Info("Running MCP server on HTTP", "host", mcpHost, "port", mcpPort)
	return mcpServer.RunHTTP(ctx, mcpHost, mcpPort)
}

// openBrowser opens a URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		fmt.Printf("Please open %s in your browser\n", url)
		return
	}
	cmd.Start()
}

func validateTraceType() error {
	if slices.Contains(supportedTypes, traceType) {
		return nil
	}
	return fmt.Errorf("unsupported trace type: %s (supported: %v)", traceType, supportedTypes)
}

func runExtract(cmd *cobra.Command, args []string) error {
	if err := validateTraceType(); err != nil {
		return err
	}

	// Validate input
	if inputFile == "" {
		switch traceType {
		case TraceTypeClaude:
			claudeDir, dirErr := claude.DefaultDir()
			if dirErr != nil {
				return fmt.Errorf("could not find Claude directory: %w", dirErr)
			}
			latest, err := claude.FindLatestSession(claudeDir)
			if err != nil {
				return fmt.Errorf("could not find latest trace: %w", err)
			}
			if latest == "" {
				return fmt.Errorf("no traces found in %s/projects/", claudeDir)
			}
			inputFile = latest
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "Using latest trace: %s\n", inputFile)
		}
	}

	// Parse format
	format, err := prompt.ParseFormat(formatType)
	if err != nil {
		return err
	}

	// Open input
	var reader io.Reader
	if inputFile == "-" {
		reader = os.Stdin
	} else {
		f, err := os.Open(inputFile)
		if err != nil {
			return fmt.Errorf("open input: %w", err)
		}
		defer f.Close()
		reader = f
	}

	// Parse and extract based on trace type
	var prompts []prompt.Prompt
	var parseErrors []error

	switch traceType {
	case TraceTypeClaude:
		parser := claude.NewParser(reader)
		extractor := prompt.NewExtractor(parser)
		prompts, err = extractor.Extract()
		parseErrors = extractor.Errors()
	}

	if err != nil {
		return fmt.Errorf("extract prompts: %w", err)
	}

	// Report parse errors
	if verbose {
		for _, e := range parseErrors {
			fmt.Fprintf(os.Stderr, "warning: %v\n", e)
		}
		fmt.Fprintf(os.Stderr, "Extracted %d prompts\n", len(prompts))
	}

	// Open output
	var writer io.Writer
	if outputFile == "-" {
		writer = os.Stdout
	} else {
		flags := os.O_CREATE | os.O_WRONLY
		if appendMode {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		f, err := os.OpenFile(outputFile, flags, 0644)
		if err != nil {
			return fmt.Errorf("open output: %w", err)
		}
		defer f.Close()
		writer = f
	}

	// Build formatter options
	var opts []prompt.FormatterOption

	// Load custom template if specified
	if templateFile != "" && format == prompt.FormatMarkdown {
		tmpl, err := prompt.LoadTemplateFile(templateFile)
		if err != nil {
			return fmt.Errorf("load template: %w", err)
		}
		opts = append(opts, prompt.WithTemplate(tmpl))
		if verbose {
			fmt.Fprintf(os.Stderr, "Using template: %s\n", templateFile)
		}
	}

	// Format and write
	formatter := prompt.NewFormatter(writer, format, opts...)
	if err := formatter.Write(prompts); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	if err := validateTraceType(); err != nil {
		return err
	}

	var sessions []string
	var err error

	switch traceType {
	case TraceTypeClaude:
		claudeDir, dirErr := claude.DefaultDir()
		if dirErr != nil {
			return fmt.Errorf("could not find Claude directory: %w", dirErr)
		}
		sessions, err = claude.FindSessions(claudeDir)
	}

	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Printf("No %s traces found\n", traceType)
		return nil
	}

	for _, s := range sessions {
		fmt.Println(s)
	}
	return nil
}

func runInfo(cmd *cobra.Command, args []string) error {
	if err := validateTraceType(); err != nil {
		return err
	}

	var path string
	if len(args) > 0 {
		path = args[0]
	} else {
		switch traceType {
		case TraceTypeClaude:
			claudeDir, dirErr := claude.DefaultDir()
			if dirErr != nil {
				return fmt.Errorf("could not find Claude directory: %w", dirErr)
			}
			latest, err := claude.FindLatestSession(claudeDir)
			if err != nil {
				return err
			}
			if latest == "" {
				return fmt.Errorf("no %s traces found", traceType)
			}
			path = latest
		}
	}

	switch traceType {
	case TraceTypeClaude:
		return showClaudeInfo(path)
	}

	return nil
}

func showClaudeInfo(path string) error {
	session, err := claude.LoadSession(path)
	if err != nil {
		return err
	}

	fmt.Printf("Session: %s\n", session.ID)
	fmt.Printf("Path:    %s\n", session.Path)
	fmt.Printf("Model:   %s\n", session.Model)
	fmt.Printf("Version: %s\n", session.Version)
	fmt.Printf("Branch:  %s\n", session.Branch)
	fmt.Printf("CWD:     %s\n", session.CWD)
	fmt.Printf("Start:   %s\n", session.StartTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("End:     %s\n", session.EndTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("Duration: %s\n", session.Duration().Round(1e9))
	fmt.Printf("Turns:   %d\n", session.TurnCount())
	fmt.Printf("Entries: %d\n", len(session.Entries))

	return nil
}

func runTemplates(cmd *cobra.Command, args []string) error {
	fmt.Println("Available Templates")
	fmt.Println("===================")
	fmt.Println()

	templates, err := prompt.ListEmbeddedTemplates()
	if err != nil {
		return err
	}

	fmt.Println("Embedded templates:")
	for _, t := range templates {
		fmt.Printf("  - %s\n", t)
	}

	fmt.Println()
	fmt.Println(prompt.DefaultTemplateHelp)

	return nil
}

func runProjects(cmd *cobra.Command, args []string) error {
	// --long and --tree are mutually exclusive
	if treeFormat && longFormat {
		return fmt.Errorf("--long and --tree are mutually exclusive")
	}

	registry := createSourceRegistry()

	projects, err := getProjectsFromSources(registry, projectSources)
	if err != nil {
		return err
	}

	if len(projects) == 0 {
		if len(projectSources) > 0 {
			fmt.Printf("No projects found from sources: %v\n", projectSources)
		} else {
			fmt.Println("No projects found")
		}
		return nil
	}

	// Sort projects by path for consistent output
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Path < projects[j].Path
	})

	formatter := cli.NewProjectsFormatter(os.Stdout)

	if treeFormat {
		return formatter.FormatTree(projects)
	}

	if longFormat {
		return formatter.FormatVerbose(projects)
	}

	return formatter.FormatLong(projects)
}

func runProjectsSummary(cmd *cobra.Command, args []string) error {
	registry := createSourceRegistry()

	projects, err := getProjectsFromSources(registry, projectSources)
	if err != nil {
		return err
	}

	if len(projects) == 0 {
		if len(projectSources) > 0 {
			fmt.Printf("No projects found from sources: %v\n", projectSources)
		} else {
			fmt.Println("No projects found")
		}
		return nil
	}

	// Determine sort order
	ascFlag, _ := cmd.Flags().GetBool("asc")
	descending := sortDesc || (!ascFlag && sortBy == "time") // time defaults to desc

	// Optionally fetch sessions for each project
	var projectSessions map[string][]thinkt.SessionMeta
	if withSessions {
		projectSessions = make(map[string][]thinkt.SessionMeta)
		ctx := context.Background()
		for _, p := range projects {
			store, ok := registry.Get(p.Source)
			if !ok {
				continue
			}
			sessions, err := store.ListSessions(ctx, p.ID)
			if err != nil {
				continue
			}
			projectSessions[p.ID] = sessions
		}
	}

	formatter := cli.NewProjectsFormatter(os.Stdout)
	return formatter.FormatSummary(projects, projectSessions, summaryTemplate, cli.SummaryOptions{
		SortBy:     sortBy,
		Descending: descending,
	})
}

func runProjectsDelete(cmd *cobra.Command, args []string) error {
	// For multi-source delete, we need to find the project first
	registry := createSourceRegistry()

	// TODO: Update ProjectDeleter to use registry for multi-source support
	// For now, use Claude default for backward compatibility
	claudeDir, err := claude.DefaultDir()
	if err != nil {
		return fmt.Errorf("could not find Claude directory: %w", err)
	}
	_ = registry // Use registry when ProjectDeleter is updated

	deleter := cli.NewProjectDeleter(claudeDir, cli.DeleteOptions{
		Force: forceDelete,
	})
	return deleter.Delete(args[0])
}

func runProjectsCopy(cmd *cobra.Command, args []string) error {
	// For multi-source copy, we need to find the project first
	registry := createSourceRegistry()

	// TODO: Update ProjectCopier to use registry for multi-source support
	// For now, use Claude default for backward compatibility
	claudeDir, err := claude.DefaultDir()
	if err != nil {
		return fmt.Errorf("could not find Claude directory: %w", err)
	}
	_ = registry // Use registry when ProjectCopier is updated

	copier := cli.NewProjectCopier(claudeDir, cli.CopyOptions{})
	return copier.Copy(args[0], args[1])
}

func runSessionsList(cmd *cobra.Command, args []string) error {
	registry := createSourceRegistry()
	ctx := context.Background()

	// If no project specified and not forcing picker, try auto-detection from cwd
	if sessionProject == "" && !sessionForcePicker {
		cwd, err := os.Getwd()
		if err == nil {
			if project := registry.FindProjectForPath(ctx, cwd); project != nil {
				sessionProject = project.ID
			}
		}
	}

	// If still no project (no match or forcing picker), show project picker
	if sessionProject == "" {
		projects, err := getProjectsFromSources(registry, sessionSources)
		if err != nil {
			return err
		}

		if len(projects) == 0 {
			if len(sessionSources) > 0 {
				fmt.Printf("No projects found from sources: %v\n", sessionSources)
			} else {
				fmt.Println("No projects found")
			}
			return nil
		}

		// Check if TTY is available for picker
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("--project/-p is required when no TTY available\n\nUse 'thinkt projects list' to see available projects")
		}

		// Show project picker
		selected, err := tui.PickProject(projects)
		if err != nil {
			return err
		}
		if selected == nil {
			return nil // User cancelled
		}
		sessionProject = selected.ID
	}

	// Get sessions for the selected project
	sessions, err := getSessionsForProject(registry, sessionProject, sessionSources)
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found")
		return nil
	}

	formatter := cli.NewSessionsFormatter(os.Stdout)
	return formatter.FormatList(sessions)
}

func runSessionsSummary(cmd *cobra.Command, args []string) error {
	registry := createSourceRegistry()
	ctx := context.Background()

	// If no project specified and not forcing picker, try auto-detection from cwd
	if sessionProject == "" && !sessionForcePicker {
		cwd, err := os.Getwd()
		if err == nil {
			if project := registry.FindProjectForPath(ctx, cwd); project != nil {
				sessionProject = project.ID
			}
		}
	}

	// If still no project, show project picker
	if sessionProject == "" {
		projects, err := getProjectsFromSources(registry, sessionSources)
		if err != nil {
			return err
		}

		if len(projects) == 0 {
			if len(sessionSources) > 0 {
				fmt.Printf("No projects found from sources: %v\n", sessionSources)
			} else {
				fmt.Println("No projects found")
			}
			return nil
		}

		// Check if TTY is available for picker
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("--project/-p is required when no TTY available\n\nUse 'thinkt projects list' to see available projects")
		}

		// Show project picker
		selected, err := tui.PickProject(projects)
		if err != nil {
			return err
		}
		if selected == nil {
			return nil // User cancelled
		}
		sessionProject = selected.ID
	}

	// Get sessions for the selected project
	sessions, err := getSessionsForProject(registry, sessionProject, sessionSources)
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found")
		return nil
	}

	// Determine sort order
	ascFlag, _ := cmd.Flags().GetBool("asc")
	descending := sessionSortDesc || (!ascFlag && sessionSortBy == "time")

	formatter := cli.NewSessionsFormatter(os.Stdout)
	return formatter.FormatSummary(sessions, sessionTemplate, cli.SessionListOptions{
		SortBy:     sessionSortBy,
		Descending: descending,
	})
}

func runSessionsDelete(cmd *cobra.Command, args []string) error {
	registry := createSourceRegistry()
	ctx := context.Background()

	// If no project specified and not an absolute path, try auto-detection from cwd
	// (--pick is not useful for delete since we need a specific session, but we check for consistency)
	if sessionProject == "" && !strings.HasPrefix(args[0], "/") && !sessionForcePicker {
		cwd, err := os.Getwd()
		if err == nil {
			if project := registry.FindProjectForPath(ctx, cwd); project != nil {
				sessionProject = project.ID
			}
		}
		// If still no project, error out
		if sessionProject == "" {
			return fmt.Errorf("--project/-p is required when not using an absolute path\n\n(Not in a known project directory)")
		}
	}

	deleter := cli.NewSessionDeleter(registry, cli.SessionDeleteOptions{
		Force:   sessionForceDelete,
		Project: sessionProject,
	})
	return deleter.Delete(args[0])
}

func runSessionsCopy(cmd *cobra.Command, args []string) error {
	registry := createSourceRegistry()
	ctx := context.Background()

	// If no project specified and not an absolute path, try auto-detection from cwd
	// (--pick is not useful for copy since we need a specific session, but we check for consistency)
	if sessionProject == "" && !strings.HasPrefix(args[0], "/") && !sessionForcePicker {
		cwd, err := os.Getwd()
		if err == nil {
			if project := registry.FindProjectForPath(ctx, cwd); project != nil {
				sessionProject = project.ID
			}
		}
		// If still no project, error out
		if sessionProject == "" {
			return fmt.Errorf("--project/-p is required when not using an absolute path\n\n(Not in a known project directory)")
		}
	}

	copier := cli.NewSessionCopier(registry, cli.SessionCopyOptions{
		Project: sessionProject,
	})
	return copier.Copy(args[0], args[1])
}

func runSessionsView(cmd *cobra.Command, args []string) error {
	registry := createSourceRegistry()
	ctx := context.Background()

	// Handle absolute path first (doesn't need project)
	if len(args) > 0 && strings.HasPrefix(args[0], "/") {
		if sessionViewRaw {
			return tui.ViewSessionRaw(args[0], os.Stdout)
		}
		return tui.RunViewer(args[0])
	}

	// If no project specified and not forcing picker, try auto-detection from cwd
	if sessionProject == "" && !sessionForcePicker {
		cwd, err := os.Getwd()
		if err == nil {
			if project := registry.FindProjectForPath(ctx, cwd); project != nil {
				sessionProject = project.ID
			}
		}
	}

	// If still no project, show project picker
	if sessionProject == "" {
		projects, err := getProjectsFromSources(registry, sessionSources)
		if err != nil {
			return err
		}

		if len(projects) == 0 {
			if len(sessionSources) > 0 {
				fmt.Printf("No projects found from sources: %v\n", sessionSources)
			} else {
				fmt.Println("No projects found")
			}
			return nil
		}

		// Check if TTY is available for picker
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("--project/-p is required when no TTY available\n\nUse 'thinkt projects list' to see available projects")
		}

		// Show project picker
		selected, err := tui.PickProject(projects)
		if err != nil {
			return err
		}
		if selected == nil {
			return nil // User cancelled
		}
		sessionProject = selected.ID
	}

	// Get all sessions in the project
	sessions, err := getSessionsForProject(registry, sessionProject, sessionSources)
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		return fmt.Errorf("no sessions found in project\n\nUse 'thinkt sessions list -p %s' to verify", sessionProject)
	}

	// If a session is specified, find and view it
	if len(args) > 0 {
		sessionArg := args[0]
		var sessionPath string

		// Check if it's an absolute path
		if strings.HasPrefix(sessionArg, "/") {
			sessionPath = sessionArg
		} else {
			// Match by session ID or filename
			found := false
			for _, s := range sessions {
				if s.ID == sessionArg || strings.HasSuffix(s.FullPath, sessionArg) || strings.HasSuffix(s.FullPath, sessionArg+".jsonl") {
					sessionPath = s.FullPath
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("session not found: %s\n\nUse 'thinkt sessions list -p %s' to see available sessions", sessionArg, sessionProject)
			}
		}

		// Handle --raw flag for undecorated output
		if sessionViewRaw {
			return tui.ViewSessionRaw(sessionPath, os.Stdout)
		}
		return tui.RunViewer(sessionPath)
	}

	// No session specified - either show picker or view all
	if sessionViewAll {
		// View all sessions in time order (oldest first)
		paths := make([]string, len(sessions))
		for i, s := range sessions {
			paths[i] = s.FullPath
		}
		return tui.RunMultiViewer(paths)
	}

	// Show session picker (requires TTY)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("no session specified and no TTY available\n\nUsage: thinkt sessions view -p <project> <session>\n\nUse 'thinkt sessions list -p %s' to see available sessions", sessionProject)
	}

	selected, err := tui.PickSession(sessions)
	if err != nil {
		return err
	}
	if selected == nil {
		// User cancelled
		return nil
	}

	// Handle --raw flag for undecorated output
	if sessionViewRaw {
		return tui.ViewSessionRaw(selected.FullPath, os.Stdout)
	}
	return tui.RunViewer(selected.FullPath)
}

// getAnalyticsBaseDir returns the base directory for analytics queries.
// TODO: Update analytics to support multi-source
func getAnalyticsBaseDir() (string, error) {
	return claude.DefaultDir()
}

func runSearch(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	results, err := engine.Search(context.Background(), args[0], searchProject, searchLimit)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(results)
	}

	if len(results) == 0 {
		fmt.Println("No results found")
		return nil
	}

	for _, r := range results {
		// Truncate content for display
		content := r.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		content = strings.ReplaceAll(content, "\n", " ")

		fmt.Printf("[%s] %s\n", r.EntryType, r.SessionPath)
		fmt.Printf("  %s\n\n", content)
	}

	return nil
}

func runStatsTokens(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	stats, err := engine.GetTokenStats(context.Background(), statsProject, statsLimit)
	if err != nil {
		return fmt.Errorf("get token stats: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}

	if len(stats) == 0 {
		fmt.Println("No token data found")
		return nil
	}

	fmt.Printf("%-40s %10s %10s %10s %10s\n", "SESSION", "INPUT", "OUTPUT", "CACHE", "TOTAL")
	fmt.Println(strings.Repeat("-", 84))
	for _, s := range stats {
		sessionID := s.SessionID
		if len(sessionID) > 38 {
			sessionID = sessionID[:38] + ".."
		}
		fmt.Printf("%-40s %10d %10d %10d %10d\n",
			sessionID, s.InputTokens, s.OutputTokens, s.CacheTokens, s.TotalTokens)
	}

	return nil
}

func runStatsTools(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	stats, err := engine.GetToolStats(context.Background(), statsProject, statsLimit)
	if err != nil {
		return fmt.Errorf("get tool stats: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}

	if len(stats) == 0 {
		fmt.Println("No tool usage data found")
		return nil
	}

	fmt.Printf("%-30s %10s\n", "TOOL", "COUNT")
	fmt.Println(strings.Repeat("-", 42))
	for _, t := range stats {
		fmt.Printf("%-30s %10d\n", t.ToolName, t.UsageCount)
	}

	return nil
}

func runStatsWords(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	words, err := engine.GetWordFrequency(context.Background(), statsProject, statsLimit)
	if err != nil {
		return fmt.Errorf("get word frequency: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(words)
	}

	if len(words) == 0 {
		fmt.Println("No word data found")
		return nil
	}

	fmt.Printf("%-20s %10s\n", "WORD", "COUNT")
	fmt.Println(strings.Repeat("-", 32))
	for _, w := range words {
		fmt.Printf("%-20s %10d\n", w.Word, w.Count)
	}

	return nil
}

func runStatsActivity(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	activity, err := engine.GetActivity(context.Background(), statsProject, statsDays)
	if err != nil {
		return fmt.Errorf("get activity: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(activity)
	}

	if len(activity) == 0 {
		fmt.Println("No activity data found")
		return nil
	}

	fmt.Printf("%-12s %10s %10s\n", "DATE", "SESSIONS", "MESSAGES")
	fmt.Println(strings.Repeat("-", 34))
	for _, a := range activity {
		fmt.Printf("%-12s %10d %10d\n", a.Date.Format("2006-01-02"), a.Sessions, a.Messages)
	}

	return nil
}

func runStatsModels(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	models, err := engine.GetModelStats(context.Background(), statsProject)
	if err != nil {
		return fmt.Errorf("get model stats: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(models)
	}

	if len(models) == 0 {
		fmt.Println("No model data found")
		return nil
	}

	fmt.Printf("%-40s %10s %15s\n", "MODEL", "RESPONSES", "AVG OUTPUT")
	fmt.Println(strings.Repeat("-", 67))
	for _, m := range models {
		model := m.Model
		if len(model) > 38 {
			model = model[:38] + ".."
		}
		fmt.Printf("%-40s %10d %15.0f\n", model, m.Responses, m.AvgOutputTokens)
	}

	return nil
}

func runStatsErrors(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	errors, err := engine.GetErrors(context.Background(), statsProject, statsLimit)
	if err != nil {
		return fmt.Errorf("get errors: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(errors)
	}

	if len(errors) == 0 {
		fmt.Println("No errors found")
		return nil
	}

	for _, e := range errors {
		content := e.Content
		if len(content) > 100 {
			content = content[:100] + "..."
		}
		content = strings.ReplaceAll(content, "\n", " ")

		fmt.Printf("[%s] %s\n", e.ToolName, e.Timestamp.Format("2006-01-02 15:04"))
		fmt.Printf("  %s\n", e.SessionPath)
		fmt.Printf("  %s\n\n", content)
	}

	return nil
}

func runQuery(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	results, err := engine.Query(context.Background(), args[0])
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(results)
	}

	if len(results) == 0 {
		fmt.Println("No results")
		return nil
	}

	// Print as JSON by default for raw queries (easier to read)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

// createSourceRegistry creates a registry with all discovered sources.
func createSourceRegistry() *thinkt.StoreRegistry {
	// Create discovery with all source factories
	discovery := thinkt.NewDiscovery(
		kimi.Factory(),
		claude.Factory(),
	)

	ctx := context.Background()
	registry, err := discovery.Discover(ctx)
	if err != nil {
		// Return empty registry on error
		return thinkt.NewRegistry()
	}

	return registry
}

// getProjectsFromSources returns projects from the selected sources.
// If no sources specified, returns projects from all available sources.
func getProjectsFromSources(registry *thinkt.StoreRegistry, sources []string) ([]thinkt.Project, error) {
	ctx := context.Background()

	// If no sources specified, use all available sources
	if len(sources) == 0 {
		return registry.ListAllProjects(ctx)
	}

	// Validate and collect projects from specified sources
	var allProjects []thinkt.Project
	for _, sourceName := range sources {
		source := thinkt.Source(sourceName)
		store, ok := registry.Get(source)
		if !ok {
			return nil, fmt.Errorf("unknown source: %s (available: kimi, claude)", sourceName)
		}

		projects, err := store.ListProjects(ctx)
		if err != nil {
			return nil, fmt.Errorf("list projects from %s: %w", sourceName, err)
		}
		allProjects = append(allProjects, projects...)
	}

	return allProjects, nil
}

// getSessionsForProject returns sessions for a project from the selected sources.
// If no sources specified, searches all available sources.
func getSessionsForProject(registry *thinkt.StoreRegistry, projectID string, sources []string) ([]thinkt.SessionMeta, error) {
	ctx := context.Background()

	// If no sources specified, search all available sources
	if len(sources) == 0 {
		for _, store := range registry.All() {
			sessions, err := store.ListSessions(ctx, projectID)
			if err == nil && len(sessions) > 0 {
				return sessions, nil
			}
		}
		return []thinkt.SessionMeta{}, nil
	}

	// Validate and collect sessions from specified sources
	for _, sourceName := range sources {
		source := thinkt.Source(sourceName)
		store, ok := registry.Get(source)
		if !ok {
			return nil, fmt.Errorf("unknown source: %s (available: kimi, claude)", sourceName)
		}

		sessions, err := store.ListSessions(ctx, projectID)
		if err == nil && len(sessions) > 0 {
			return sessions, nil
		}
	}

	return []thinkt.SessionMeta{}, nil
}

// runSourcesList lists available sources.
func runSourcesList(cmd *cobra.Command, args []string) error {
	registry := createSourceRegistry()

	ctx := context.Background()
	sources := registry.SourceStatus(ctx)

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(sources)
	}

	if len(sources) == 0 {
		fmt.Println("No sources found.")
		fmt.Println("\nExpected sources:")
		fmt.Println("  - Kimi Code: ~/.kimi/")
		fmt.Println("  - Claude Code: ~/.claude/")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SOURCE\tSTATUS\tPROJECTS\tWORKSPACE")

	for _, s := range sources {
		status := "no data"
		if s.Available {
			status = "available"
		}
		projects := fmt.Sprintf("%d", s.ProjectCount)
		workspace := s.WorkspaceID
		if len(workspace) > 8 {
			workspace = workspace[:8] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, status, projects, workspace)
	}
	w.Flush()

	return nil
}

// runSourcesStatus shows detailed source status.
func runSourcesStatus(cmd *cobra.Command, args []string) error {
	registry := createSourceRegistry()

	ctx := context.Background()
	sources := registry.SourceStatus(ctx)

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(sources)
	}

	if len(sources) == 0 {
		fmt.Println("No sources found.")
		return nil
	}

	for i, s := range sources {
		if i > 0 {
			fmt.Println()
			fmt.Println("---")
			fmt.Println()
		}

		fmt.Printf("Source:      %s\n", s.Name)
		fmt.Printf("ID:          %s\n", s.Source)
		fmt.Printf("Description: %s\n", s.Description)
		fmt.Printf("Status:      %s\n", map[bool]string{true: "available", false: "no data"}[s.Available])

		if s.Available {
			fmt.Printf("Workspace:   %s\n", s.WorkspaceID)
			fmt.Printf("Base Path:   %s\n", s.BasePath)
			fmt.Printf("Projects:    %d\n", s.ProjectCount)
		}
	}

	return nil
}

// runTheme displays the current theme.
func runTheme(cmd *cobra.Command, args []string) error {
	t, err := theme.Load()
	if err != nil {
		// Fall back to defaults on error
		t = theme.DefaultTheme()
	}

	display := cli.NewThemeDisplay(os.Stdout, t)

	if outputJSON {
		return display.ShowJSON()
	}

	return display.Show()
}

// runThemeList lists all available themes.
func runThemeList(cmd *cobra.Command, args []string) error {
	return cli.ListThemes(os.Stdout)
}

// runThemeSet sets the active theme.
func runThemeSet(cmd *cobra.Command, args []string) error {
	name := args[0]

	if err := theme.SetActive(name); err != nil {
		return fmt.Errorf("failed to set theme: %w", err)
	}

	fmt.Printf("Theme set to: %s\n", name)
	return nil
}
