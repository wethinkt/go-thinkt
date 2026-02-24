// Package cmd provides the CLI commands for thinkt.
package cmd

import (
	"fmt"
	"os"
	"runtime/pprof"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/server"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
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

Supports: Claude Code, Kimi Code, Gemini CLI, GitHub Copilot CLI, Codex CLI

Running without a subcommand launches the interactive TUI.

Commands:
  sources   Manage and view available session sources
  tui       Launch interactive TUI explorer (default)
  prompts   Extract and manage prompts from trace files
  projects  List and manage projects
  sessions  List and manage sessions

Examples:
  thinkt                          # Launch TUI
  thinkt sources list             # List available sources (claude, kimi, gemini, copilot, codex)
  thinkt projects list            # List all projects from all sources`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Initialize debug logging once for all commands.
		if logPath == "" {
			if flag := cmd.Flags().Lookup("log"); flag != nil && flag.Value != nil {
				logPath = strings.TrimSpace(flag.Value.String())
			}
		}
		if logPath == "" {
			logPath = strings.TrimSpace(os.Getenv("THINKT_LOG_FILE"))
		}
		if logPath != "" {
			if err := tuilog.Init(logPath); err != nil {
				return fmt.Errorf("init debug log: %w", err)
			}
		}

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
		_ = tuilog.Log.Close()
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

	// Projects command flags are now handled in projects.go init()

	// Sessions command flags
	// Project selection:
	// - No flags: auto-detect from cwd, fallback to picker
	// - --pick: force picker even if in a project directory
	// - -p <path>: use specified path
	sessionsCmd.PersistentFlags().StringVarP(&sessionProject, "project", "p", "", "project path (auto-detects from cwd if not set)")
	sessionsCmd.PersistentFlags().BoolVar(&sessionForcePicker, "pick", false, "force project picker even if in a known project directory")
	sessionsCmd.PersistentFlags().StringArrayVarP(&sessionSources, "source", "s", nil, "filter by source (claude|kimi|gemini|copilot|codex|qwen, can be specified multiple times)")
	sessionsCmd.PersistentFlags().StringVar(&logPath, "log", "", "write debug log to file")
	sessionsSummaryCmd.Flags().StringVar(&sessionTemplate, "template", "", "custom Go text/template for output")
	sessionsSummaryCmd.Flags().StringVar(&sessionSortBy, "sort", "time", "sort by: name, time")
	sessionsSummaryCmd.Flags().BoolVar(&sessionSortDesc, "desc", false, "sort descending (default for time)")
	sessionsSummaryCmd.Flags().Bool("asc", false, "sort ascending (default for name)")
	sessionsDeleteCmd.Flags().BoolVarP(&sessionForceDelete, "force", "f", false, "skip confirmation prompt")
	sessionsCmd.Flags().BoolVarP(&sessionViewAll, "all", "a", false, "view all sessions in time order")
	sessionsCmd.Flags().BoolVar(&sessionViewRaw, "raw", false, "output raw text without decoration/rendering")
	sessionsViewCmd.Flags().BoolVarP(&sessionViewAll, "all", "a", false, "view all sessions in time order")
	sessionsViewCmd.Flags().BoolVar(&sessionViewRaw, "raw", false, "output raw text without decoration/rendering")

	// Build command tree
	sessionsCmd.AddCommand(sessionsListCmd)
	sessionsCmd.AddCommand(sessionsSummaryCmd)
	sessionsCmd.AddCommand(sessionsDeleteCmd)
	sessionsCmd.AddCommand(sessionsCopyCmd)
	sessionsCmd.AddCommand(sessionsViewCmd)
	sessionsCmd.AddCommand(sessionsResumeCmd)
	sessionsCmd.AddCommand(sessionsResolveCmd)
	sessionsListCmd.Flags().BoolVar(&sessionJSON, "json", false, "output sessions as JSON")
	sessionsResolveCmd.Flags().BoolVar(&sessionResolveJSON, "json", false, "output resolved session metadata as JSON")
	promptsCmd.AddCommand(extractCmd)
	promptsCmd.AddCommand(listCmd)
	promptsCmd.AddCommand(infoCmd)
	promptsCmd.AddCommand(templatesCmd)

	helpCmd.AddCommand(helpLlmsCmd)
	rootCmd.SetHelpCommand(helpCmd)

	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(promptsCmd)
	rootCmd.AddCommand(projectsCmd)
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(sourcesCmd)
	rootCmd.AddCommand(themeCmd)
	rootCmd.AddCommand(indexerCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(teamsCmd)
	teamsCmd.AddCommand(teamsListCmd)
	teamsCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "output as JSON")
	teamsCmd.PersistentFlags().BoolVar(&teamsFilterActive, "active", false, "show only active teams")
	teamsCmd.PersistentFlags().BoolVar(&teamsFilterInactive, "inactive", false, "show only inactive (historical) teams")

	// Theme subcommands
	themeCmd.AddCommand(themeShowCmd)
	themeCmd.AddCommand(themeListCmd)
	themeCmd.AddCommand(themeSetCmd)
	themeCmd.AddCommand(themeBrowseCmd)
	themeCmd.AddCommand(themeBuilderCmd)
	themeCmd.AddCommand(themeImportCmd)
	themeImportCmd.Flags().StringVar(&themeImportName, "name", "", "theme name (default: derived from filename)")

	// Theme data output flags
	themeShowCmd.Flags().BoolVar(&outputJSON, "json", false, "output theme as JSON")
	themeListCmd.Flags().BoolVar(&outputJSON, "json", false, "output as JSON")

	// Server command flags shared across subcommands
	serverCmd.PersistentFlags().StringVar(&serveCORSOrigin, "cors-origin", "", "CORS Access-Control-Allow-Origin (default \"*\" when unauthenticated, disabled when authenticated; env: THINKT_CORS_ORIGIN)")
	serverCmd.PersistentFlags().BoolVar(&serveNoIndexer, "no-indexer", false, "don't auto-start the background indexer")

	// Server run subcommand (foreground server)
	serverRunCmd.Flags().IntVarP(&servePort, "port", "p", server.DefaultPortServer, "server port")
	serverRunCmd.Flags().StringVar(&serveHost, "host", "localhost", "server host")
	serverRunCmd.Flags().BoolVar(&serveNoOpen, "no-open", false, "don't auto-open browser")
	serverRunCmd.Flags().StringVar(&logPath, "log", "", "write debug log to file")
	serverRunCmd.Flags().BoolVarP(&serveQuiet, "quiet", "q", false, "suppress HTTP request logging (errors still go to stderr)")
	serverRunCmd.Flags().StringVar(&serveHTTPLog, "http-log", "", "write HTTP access log to file (default: stdout, unless --quiet)")
	serverRunCmd.Flags().StringVar(&serveDev, "dev", "", "dev mode: proxy non-API routes to this URL (e.g. http://localhost:5173)")
	serverRunCmd.Flags().StringVar(&apiToken, "token", "", "bearer token for API authentication (default: use THINKT_API_TOKEN env var)")
	serverRunCmd.Flags().BoolVar(&serveNoAuth, "no-auth", false, "disable authentication (allow unauthenticated access)")

	serverStartCmd.Flags().BoolVar(&serveNoAuth, "no-auth", false, "disable authentication (allow unauthenticated access)")
	serverCmd.AddCommand(serverStartCmd)

	serverCmd.AddCommand(serverRunCmd)

	serverCmd.AddCommand(serverStopCmd)

	serverStatusCmd.Flags().BoolVar(&outputJSON, "json", false, "output as JSON")
	serverCmd.AddCommand(serverStatusCmd)

	serverLogsCmd.Flags().IntP("lines", "n", 50, "number of lines to show")
	serverLogsCmd.Flags().BoolP("follow", "f", false, "follow log output")
	serverCmd.AddCommand(serverLogsCmd)

	serverHTTPLogsCmd.Flags().IntP("lines", "n", 50, "number of lines to show")
	serverHTTPLogsCmd.Flags().BoolP("follow", "f", false, "follow log output")
	serverCmd.AddCommand(serverHTTPLogsCmd)

	// Server token subcommand
	serverCmd.AddCommand(serverTokenCmd)

	// Server fingerprint subcommand
	serverCmd.AddCommand(serverFingerprintCmd)
	serverFingerprintCmd.Flags().BoolVar(&fingerprintJSON, "json", false, "output as JSON")

	// Server MCP subcommand
	serverCmd.AddCommand(serverMcpCmd)
	serverMcpCmd.Flags().BoolVar(&mcpStdio, "stdio", false, "use stdio transport (default if no --port)")
	serverMcpCmd.Flags().IntVarP(&mcpPort, "port", "p", 0, "run MCP over HTTP on this port")
	serverMcpCmd.Flags().StringVar(&mcpHost, "host", "localhost", "host to bind MCP HTTP server")
	serverMcpCmd.Flags().StringVar(&mcpToken, "token", "", "bearer token for HTTP authentication (default: use THINKT_MCP_TOKEN env var)")
	serverMcpCmd.Flags().StringSliceVar(&mcpAllowTools, "allow-tools", nil, "explicitly allow only these tools (comma-separated, default: all)")
	serverMcpCmd.Flags().StringSliceVar(&mcpDenyTools, "deny-tools", nil, "explicitly deny these tools (comma-separated)")
	serverMcpCmd.Flags().StringVar(&logPath, "log", "", "write debug log to file")

	// Web command
	webCmd.AddCommand(webLiteCmd)
	webCmd.Flags().IntVarP(&servePort, "port", "p", server.DefaultPortServer, "server port")
	webCmd.Flags().StringVar(&serveHost, "host", "localhost", "server host")
	webCmd.Flags().BoolVar(&serveNoOpen, "no-open", false, "don't auto-open browser")
	webLiteCmd.Flags().IntVarP(&servePort, "port", "p", server.DefaultPortServer, "server port")
	webLiteCmd.Flags().StringVar(&serveHost, "host", "localhost", "server host")
	webLiteCmd.Flags().BoolVar(&serveNoOpen, "no-open", false, "don't auto-open browser")

	// Apps subcommands
	appsCmd.AddCommand(appsListCmd, appsEnableCmd, appsDisableCmd, appsGetTermCmd, appsSetTermCmd)
	appsCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(appsCmd)
	rootCmd.AddCommand(webCmd)

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
