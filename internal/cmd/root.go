// Package cmd provides the CLI commands for thinkt.
package cmd

import (
	"fmt"
	"os"
	"runtime/pprof"
	"slices"
	"strings"
	"time"

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
	sessionsCmd.PersistentFlags().StringArrayVarP(&sessionSources, "source", "s", nil, "filter by source (claude|kimi|gemini|copilot|codex, can be specified multiple times)")
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
	sessionsCmd.AddCommand(sessionsResolveCmd)
	sessionsResolveCmd.Flags().BoolVar(&sessionResolveJSON, "json", false, "output resolved session metadata as JSON")
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
	rootCmd.AddCommand(indexerCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(teamsCmd)
	teamsCmd.AddCommand(teamsListCmd)
	teamsCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "output as JSON")
	teamsCmd.PersistentFlags().BoolVar(&teamsFilterActive, "active", false, "show only active teams")
	teamsCmd.PersistentFlags().BoolVar(&teamsFilterInactive, "inactive", false, "show only inactive (historical) teams")

	// Theme subcommands
	themeCmd.AddCommand(themeListCmd)
	themeCmd.AddCommand(themeSetCmd)
	themeCmd.AddCommand(themeBuilderCmd)

	// Theme command flags
	themeCmd.Flags().BoolVar(&outputJSON, "json", false, "output theme as JSON")

	// Serve command flags shared across subcommands
	serveCmd.PersistentFlags().StringVar(&serveCORSOrigin, "cors-origin", "", "CORS Access-Control-Allow-Origin (default \"*\", env: THINKT_CORS_ORIGIN)")

	// Serve command flags (non-persistent; only apply to 'serve' itself)
	// Subcommands that need these define their own (mcp, lite)
	serveCmd.Flags().IntVarP(&servePort, "port", "p", server.DefaultPortServe, "server port")
	serveCmd.Flags().StringVar(&serveHost, "host", "localhost", "server host")
	serveCmd.Flags().BoolVar(&serveNoOpen, "no-open", false, "don't auto-open browser")
	serveCmd.Flags().StringVar(&logPath, "log", "", "write debug log to file")
	serveCmd.Flags().BoolVarP(&serveQuiet, "quiet", "q", false, "suppress HTTP request logging (errors still go to stderr)")
	serveCmd.Flags().StringVar(&serveHTTPLog, "http-log", "", "write HTTP access log to file (default: stdout, unless --quiet)")

	// Serve token subcommand
	serveCmd.AddCommand(serveTokenCmd)

	// Serve fingerprint subcommand
	serveCmd.AddCommand(serveFingerprintCmd)
	serveFingerprintCmd.Flags().BoolVar(&fingerprintJSON, "json", false, "output as JSON")

	// Serve MCP subcommand
	serveCmd.AddCommand(serveMcpCmd)
	serveMcpCmd.Flags().BoolVar(&mcpStdio, "stdio", false, "use stdio transport (default if no --port)")
	serveMcpCmd.Flags().IntVarP(&mcpPort, "port", "p", 0, "run MCP over HTTP on this port")
	serveMcpCmd.Flags().StringVar(&mcpHost, "host", "localhost", "host to bind MCP HTTP server")
	serveMcpCmd.Flags().StringVar(&mcpToken, "token", "", "bearer token for HTTP authentication (default: use THINKT_MCP_TOKEN env var)")
	serveMcpCmd.Flags().BoolVar(&mcpNoIndexer, "no-indexer", false, "don't auto-start the background indexer")
	serveMcpCmd.Flags().StringSliceVar(&mcpAllowTools, "allow-tools", nil, "explicitly allow only these tools (comma-separated, default: all)")
	serveMcpCmd.Flags().StringSliceVar(&mcpDenyTools, "deny-tools", nil, "explicitly deny these tools (comma-separated)")
	serveMcpCmd.Flags().StringVar(&logPath, "log", "", "write debug log to file")

	// Serve API flags (only apply to main serve command)
	serveCmd.Flags().StringVar(&apiToken, "token", "", "bearer token for API authentication (default: use THINKT_API_TOKEN env var)")

	// Serve Lite subcommand (has its own port default)
	serveCmd.AddCommand(serveLiteCmd)
	serveLiteCmd.Flags().IntVarP(&serveLitePort, "port", "p", server.DefaultPortLite, "server port")
	serveLiteCmd.Flags().StringVar(&serveHost, "host", "localhost", "server host")
	serveLiteCmd.Flags().BoolVar(&serveNoOpen, "no-open", false, "don't auto-open browser")
	serveLiteCmd.Flags().StringVar(&logPath, "log", "", "write debug log to file")
	serveLiteCmd.Flags().DurationVar(&serveLiteTTL, "ttl", 60*time.Second, "cache TTL for refreshing source data (0 to cache forever)")

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
