// Package cmd provides the CLI commands for thinkt.
package cmd

import (
	"fmt"
	"os"
	"runtime/pprof"
	"slices"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/server"
)

// global flags
var (
	profileFile *os.File // held open for profiling
	logPath     string
	verbose     bool
	outputJSON  bool
)

// rootCmd is the root command for the CLI.
var rootCmd = &cobra.Command{
	Use:   "thinkt",
	Short: "Tools for AI assistant session exploration and extraction",
	Long: `thinkt provides tools for exploring and extracting data from AI coding assistant sessions.

Supports: Claude Code, Kimi Code, Gemini CLI

Running without a subcommand launches the interactive TUI.

Commands:
  sources   Manage and view available session sources
  tui       Launch interactive TUI explorer (default)
  prompts   Extract and manage prompts from trace files
  projects  List and manage projects
  sessions  List and manage sessions

Examples:
  thinkt                          # Launch TUI
  thinkt sources list             # List available sources (kimi, claude, gemini)
  thinkt projects list            # List all projects from all sources`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Start pprof profiling if THINKT_PROFILE is set
		if profilePath := os.Getenv("THINKT_PROFILE"); profilePath != "" {
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

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags on root
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// TUI-specific flags
	tuiCmd.Flags().StringVar(&logPath, "log", "", "write debug log to file")
	// Also add to root since it can run TUI directly
	rootCmd.Flags().StringVar(&logPath, "log", "", "write debug log to file")

	// Prompts subcommand flags
	promptsCmd.PersistentFlags().StringVarP(&traceType, "type", "t", traceTypeClaude, "trace type (claude)")

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

	// Build command tree
	projectsCmd.AddCommand(projectsSummaryCmd)
	projectsCmd.AddCommand(projectsDeleteCmd)
	projectsCmd.AddCommand(projectsCopyCmd)
	sessionsCmd.AddCommand(sessionsListCmd)
	sessionsCmd.AddCommand(sessionsSummaryCmd)
	sessionsCmd.AddCommand(sessionsDeleteCmd)
	sessionsCmd.AddCommand(sessionsCopyCmd)
	sessionsCmd.AddCommand(sessionsViewCmd)
	promptsCmd.AddCommand(extractCmd)
	promptsCmd.AddCommand(listCmd)
	promptsCmd.AddCommand(infoCmd)
	promptsCmd.AddCommand(templatesCmd)

	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(promptsCmd)
	rootCmd.AddCommand(projectsCmd)
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(sourcesCmd)
	rootCmd.AddCommand(themeCmd)

	// Theme subcommands
	themeCmd.AddCommand(themeListCmd)
	themeCmd.AddCommand(themeSetCmd)
	themeCmd.AddCommand(themeBuilderCmd)

	// Theme command flags
	themeCmd.Flags().BoolVar(&outputJSON, "json", false, "output theme as JSON")

	// Serve command flags (persistent so they're inherited by subcommands like 'lite')
	serveCmd.PersistentFlags().IntVarP(&servePort, "port", "p", server.DefaultPortServe, "server port")
	serveCmd.PersistentFlags().StringVar(&serveHost, "host", "localhost", "server host")
	serveCmd.PersistentFlags().BoolVar(&serveNoOpen, "no-open", false, "don't auto-open browser")
	serveCmd.PersistentFlags().StringVar(&logPath, "log", "", "write debug log to file")
	serveCmd.PersistentFlags().BoolVarP(&serveQuiet, "quiet", "q", false, "suppress HTTP request logging (errors still go to stderr)")
	serveCmd.PersistentFlags().StringVar(&serveHTTPLog, "http-log", "", "write HTTP access log to file (default: stdout, unless --quiet)")

	// Serve token subcommand
	serveCmd.AddCommand(serveTokenCmd)

	// Serve MCP subcommand
	serveCmd.AddCommand(serveMcpCmd)
	serveMcpCmd.Flags().BoolVar(&mcpStdio, "stdio", false, "use stdio transport (default if no --port)")
	serveMcpCmd.Flags().IntVarP(&mcpPort, "port", "p", 0, "run MCP over HTTP on this port")
	serveMcpCmd.Flags().StringVar(&mcpHost, "host", "localhost", "host to bind MCP HTTP server")
	serveMcpCmd.Flags().StringVar(&mcpToken, "token", "", "bearer token for HTTP authentication (default: use THINKT_MCP_TOKEN env var)")

	// Serve API flags (also apply to main serve command)
	serveCmd.PersistentFlags().StringVar(&apiToken, "token", "", "bearer token for API authentication (default: use THINKT_API_TOKEN env var)")

	// Serve Lite subcommand (has its own port default)
	serveCmd.AddCommand(serveLiteCmd)
	serveLiteCmd.Flags().IntVarP(&serveLitePort, "port", "p", server.DefaultPortLite, "server port")

	// Sources subcommands
	sourcesCmd.AddCommand(sourcesListCmd)
	sourcesCmd.AddCommand(sourcesStatusCmd)
	sourcesCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "output as JSON")

	// Docs command (hidden unless --verbose)
	docsCmd.PersistentFlags().StringVarP(&docsOutputDir, "output", "o", "./docs", "output directory for generated docs")
	docsCmd.PersistentFlags().BoolVar(&docsEnableAutoGenTag, "enableAutoGenTag", false, "include auto-generation tag (timestamp footer) for publishing")
	docsMarkdownCmd.Flags().BoolVar(&docsHugo, "hugo", false, "generate Hugo-compatible markdown with YAML front matter")
	docsCmd.AddCommand(docsMarkdownCmd)
	docsCmd.AddCommand(docsManCmd)
	rootCmd.AddCommand(docsCmd)
	if slices.Contains(os.Args, "-v") || slices.Contains(os.Args, "--verbose") {
		// Show hidden commands in help when --verbose is used
		docsCmd.Hidden = false
	}
}
