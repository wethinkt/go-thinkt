// thinkt provides tools for exploring and extracting from Claude Code sessions.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/analytics"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/cli"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/prompt"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tui"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tuilog"
)

// Global flags
var (
	baseDir     string
	profilePath string
	logPath     string
	verbose     bool
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
	Short: "Tools for Claude Code session exploration and extraction",
	Long: `thinkt provides tools for exploring and extracting data from Claude Code sessions.

Running without a subcommand launches the interactive TUI.

Commands:
  tui       Launch interactive TUI explorer (default)
  prompts   Extract and manage prompts from trace files

Examples:
  thinkt                          # Launch TUI
  thinkt tui -d /custom/path      # TUI with custom directory
  thinkt prompts extract          # Extract prompts from latest session
  thinkt prompts list             # List available sessions`,
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
	sessionForceDelete bool
	sessionSortBy      string
	sessionSortDesc    bool
	sessionTemplate    string
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

var projectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List Claude Code projects",
	Long: `List all Claude Code projects found in the base directory.

By default, outputs project paths one per line.
Use --tree for a grouped tree view.

Examples:
  thinkt projects              # Paths, one per line
  thinkt projects --tree       # Tree view grouped by parent
  thinkt projects summary      # Detailed with sessions/modified`,
	RunE: runProjects,
}

var projectsSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show detailed project summary",
	Long: `Show detailed information about each project including
session count and last modified time.

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
	Short: "List and manage Claude Code sessions",
	Long: `List and manage sessions within a Claude Code project.

Requires -p/--project to specify which project to operate on.

Examples:
  thinkt sessions list -p /Users/evan/myproject
  thinkt sessions summary -p ./myproject
  thinkt sessions delete -p ./myproject <session-id>
  thinkt sessions copy -p ./myproject <session-id> ./backup`,
	RunE: runSessionsList,
}

var sessionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sessions in a project",
	Long: `List all sessions in a Claude Code project.

Outputs session paths one per line by default.

Examples:
  thinkt sessions list -p /Users/evan/myproject
  thinkt sessions list -p ./myproject`,
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

func main() {
	// Global flags on root
	rootCmd.PersistentFlags().StringVarP(&baseDir, "dir", "d", "", "base directory (default ~/.claude)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// TUI-specific flags
	tuiCmd.Flags().StringVar(&profilePath, "profile", "", "write CPU profile to file")
	tuiCmd.Flags().StringVar(&logPath, "log", "", "write debug log to file")
	// Also add to root since it can run TUI directly
	rootCmd.Flags().StringVar(&profilePath, "profile", "", "write CPU profile to file")
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
	projectsCmd.Flags().BoolVarP(&treeFormat, "tree", "t", false, "show tree view grouped by parent directory")
	projectsSummaryCmd.Flags().StringVar(&summaryTemplate, "template", "", "custom Go text/template for output")
	projectsSummaryCmd.Flags().StringVar(&sortBy, "sort", "time", "sort by: name, time")
	projectsSummaryCmd.Flags().BoolVar(&sortDesc, "desc", false, "sort descending (default for time)")
	projectsSummaryCmd.Flags().Bool("asc", false, "sort ascending (default for name)")
	projectsDeleteCmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "skip confirmation prompt")

	// Sessions command flags
	sessionsCmd.PersistentFlags().StringVarP(&sessionProject, "project", "p", "", "project path (required)")
	sessionsListCmd.Flags().StringVarP(&sessionProject, "project", "p", "", "project path (required)")
	sessionsSummaryCmd.Flags().StringVar(&sessionTemplate, "template", "", "custom Go text/template for output")
	sessionsSummaryCmd.Flags().StringVar(&sessionSortBy, "sort", "time", "sort by: name, time")
	sessionsSummaryCmd.Flags().BoolVar(&sessionSortDesc, "desc", false, "sort descending (default for time)")
	sessionsSummaryCmd.Flags().Bool("asc", false, "sort ascending (default for name)")
	sessionsDeleteCmd.Flags().BoolVarP(&sessionForceDelete, "force", "f", false, "skip confirmation prompt")

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
	rootCmd.AddCommand(promptsCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(projectsCmd)
	rootCmd.AddCommand(sessionsCmd)

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

	// Start CPU profiling if requested
	if profilePath != "" {
		f, err := os.Create(profilePath)
		if err != nil {
			return fmt.Errorf("create profile file: %w", err)
		}
		defer f.Close()

		if err := pprof.StartCPUProfile(f); err != nil {
			return fmt.Errorf("start CPU profile: %w", err)
		}
		defer pprof.StopCPUProfile()
	}

	tuilog.Log.Info("Starting TUI", "baseDir", baseDir)

	model := tui.NewModel(baseDir)
	p := tea.NewProgram(model)
	_, err := p.Run()

	tuilog.Log.Info("TUI exited", "error", err)
	return err
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
			latest, err := claude.FindLatestSession(baseDir)
			if err != nil {
				return fmt.Errorf("could not find latest trace: %w", err)
			}
			if latest == "" {
				dir := baseDir
				if dir == "" {
					dir = "~/.claude"
				}
				return fmt.Errorf("no traces found in %s/projects/", dir)
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
		sessions, err = claude.FindSessions(baseDir)
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
			latest, err := claude.FindLatestSession(baseDir)
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
	projects, err := claude.ListProjects(baseDir)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}

	if len(projects) == 0 {
		fmt.Println("No projects found")
		return nil
	}

	formatter := cli.NewProjectsFormatter(os.Stdout)
	if treeFormat {
		return formatter.FormatTree(projects)
	}
	return formatter.FormatLong(projects)
}

func runProjectsSummary(cmd *cobra.Command, args []string) error {
	projects, err := claude.ListProjects(baseDir)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}

	if len(projects) == 0 {
		fmt.Println("No projects found")
		return nil
	}

	// Determine sort order
	ascFlag, _ := cmd.Flags().GetBool("asc")
	descending := sortDesc || (!ascFlag && sortBy == "time") // time defaults to desc

	formatter := cli.NewProjectsFormatter(os.Stdout)
	return formatter.FormatSummary(projects, summaryTemplate, cli.SummaryOptions{
		SortBy:     sortBy,
		Descending: descending,
	})
}

func runProjectsDelete(cmd *cobra.Command, args []string) error {
	deleter := cli.NewProjectDeleter(baseDir, cli.DeleteOptions{
		Force: forceDelete,
	})
	return deleter.Delete(args[0])
}

func runProjectsCopy(cmd *cobra.Command, args []string) error {
	copier := cli.NewProjectCopier(baseDir, cli.CopyOptions{})
	return copier.Copy(args[0], args[1])
}

func runSessionsList(cmd *cobra.Command, args []string) error {
	if sessionProject == "" {
		return fmt.Errorf("--project/-p is required\n\nUse 'thinkt projects' to list available projects")
	}

	sessions, err := cli.ListSessionsForProject(baseDir, sessionProject)
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
	if sessionProject == "" {
		return fmt.Errorf("--project/-p is required\n\nUse 'thinkt projects' to list available projects")
	}

	sessions, err := cli.ListSessionsForProject(baseDir, sessionProject)
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
	deleter := cli.NewSessionDeleter(baseDir, cli.SessionDeleteOptions{
		Force:   sessionForceDelete,
		Project: sessionProject,
	})
	return deleter.Delete(args[0])
}

func runSessionsCopy(cmd *cobra.Command, args []string) error {
	copier := cli.NewSessionCopier(baseDir, cli.SessionCopyOptions{
		Project: sessionProject,
	})
	return copier.Copy(args[0], args[1])
}

// getAnalyticsBaseDir returns the base directory for analytics queries.
func getAnalyticsBaseDir() (string, error) {
	if baseDir != "" {
		return baseDir, nil
	}
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
