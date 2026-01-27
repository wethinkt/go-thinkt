// thinkt provides tools for exploring and extracting from Claude Code sessions.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

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
	treeFormat       bool
	summaryTemplate  string
	sortBy           string
	sortDesc         bool
	forceDelete      bool
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

	// Build command tree
	projectsCmd.AddCommand(projectsSummaryCmd)
	projectsCmd.AddCommand(projectsDeleteCmd)
	promptsCmd.AddCommand(extractCmd)
	promptsCmd.AddCommand(listCmd)
	promptsCmd.AddCommand(infoCmd)
	promptsCmd.AddCommand(templatesCmd)

	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(promptsCmd)
	rootCmd.AddCommand(projectsCmd)

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
