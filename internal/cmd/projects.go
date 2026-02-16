package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/wethinkt/go-thinkt/internal/cli"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// Projects command flags
var (
	summaryTemplate string
	sortBy          string
	sortDesc        bool
	projectSources  []string // --source flag (can be specified multiple times)
	withSessions    bool     // --with-sessions flag for summary
	shortFormat     bool     // --short flag for path-only output
	jsonFormat      bool     // --json flag for JSON output
)

var projectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "Manage and view projects",
	Long: `Manage and view projects from available sources (Kimi, Claude, Gemini, etc.).

By default, this command launches the interactive project browser (TUI).
Use subcommands to list, summarize, or manage projects via CLI.

Examples:
  thinkt projects                      # Launch interactive browser (default)
  thinkt projects list                 # List detailed columns
  thinkt projects list --short         # List paths only
  thinkt projects summary              # Detailed summary with session names
  thinkt projects tree                 # Tree view`,
	RunE: runProjectsView, // Default to interactive view
}

var projectsViewCmd = &cobra.Command{
	Use:   "view",
	Short: "Interactive project browser",
	Long: `Launch the interactive TUI project browser.
This allows you to navigate projects and select sessions to view.`,
	RunE: runProjectsView,
}

var projectsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects from all sources",
	Long: `List all projects from available sources (Kimi, Claude, Gemini, etc.).

By default, shows detailed columns (path, source, sessions, modified time).
Use --short for a compact list of project paths only.
Use --json for JSON output.

Examples:
  thinkt projects list                 # Detailed columns
  thinkt projects list --short         # Paths only, one per line
  thinkt projects list --json          # JSON output
  thinkt projects list --source kimi   # Only Kimi projects`,
	RunE: runProjectsList,
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

var projectsTreeCmd = &cobra.Command{
	Use:   "tree",
	Short: "Show projects in a tree view",
	Long:  `Show projects grouped by parent directory in a tree layout.`,
	RunE:  runProjectsTree,
}

var projectsCopyCmd = &cobra.Command{
	Use:   "copy <project-path> <target-dir>",
	Short: "Copy project sessions to a target directory",
	Long: `Copy all session files from a project to a target directory.

The project-path can be:
  - Full project path (e.g., /Users/evan/myproject)
  - Path relative to current directory

The target directory will be created if it doesn't exist.
Session files and index files are copied.

Examples:
  thinkt projects copy /Users/evan/myproject ./backup
  thinkt projects copy /Users/evan/myproject /tmp/export`,
	Args: cobra.ExactArgs(2),
	RunE: runProjectsCopy,
}

func init() {
	// Root flags (persistent across all subcommands)
	projectsCmd.PersistentFlags().StringArrayVarP(&projectSources, "source", "s", nil, "source to include (kimi|claude|gemini|copilot|codex, can be specified multiple times, default: all)")

	// List command flags
	projectsListCmd.Flags().BoolVar(&shortFormat, "short", false, "show project paths only")
	projectsListCmd.Flags().BoolVar(&jsonFormat, "json", false, "output in JSON format")

	// Summary command flags
	projectsSummaryCmd.Flags().StringVar(&summaryTemplate, "template", "", "custom Go text/template for output")
	projectsSummaryCmd.Flags().StringVar(&sortBy, "sort", "time", "sort by: name, time")
	projectsSummaryCmd.Flags().BoolVar(&sortDesc, "desc", false, "sort descending (default for time)")
	projectsSummaryCmd.Flags().Bool("asc", false, "sort ascending (default for name)")
	projectsSummaryCmd.Flags().BoolVar(&withSessions, "with-sessions", false, "include session names in output")

	// Register subcommands
	projectsCmd.AddCommand(projectsListCmd)
	projectsCmd.AddCommand(projectsViewCmd)
	projectsCmd.AddCommand(projectsSummaryCmd)
	projectsCmd.AddCommand(projectsTreeCmd)
	projectsCmd.AddCommand(projectsCopyCmd)
}

func runProjectsView(cmd *cobra.Command, args []string) error {
	tuilog.Log.Info("Starting Projects View TUI")

	// Get initial terminal size
	var opts []tea.ProgramOption
	for _, fd := range []int{int(os.Stdout.Fd()), int(os.Stdin.Fd()), int(os.Stderr.Fd())} {
		if term.IsTerminal(fd) {
			w, h, err := term.GetSize(fd)
			if err == nil && w > 0 && h > 0 {
				opts = append(opts, tea.WithWindowSize(w, h))
				break
			}
		}
	}

	// Force projects picker
	shell := tui.NewShell(tui.InitialPageProjects)
	p := tea.NewProgram(shell, opts...)
	_, err := p.Run()

	tuilog.Log.Info("Projects View TUI exited", "error", err)
	return err
}

func runProjectsList(cmd *cobra.Command, args []string) error {
	registry := CreateSourceRegistry()

	projects, err := GetProjectsFromSources(registry, projectSources)
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

	if jsonFormat {
		return formatter.FormatJSON(projects)
	}

	if shortFormat {
		return formatter.FormatShort(projects)
	}

	return formatter.FormatVerbose(projects)
}

func runProjectsTree(cmd *cobra.Command, args []string) error {
	registry := CreateSourceRegistry()

	projects, err := GetProjectsFromSources(registry, projectSources)
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

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Path < projects[j].Path
	})

	formatter := cli.NewProjectsFormatter(os.Stdout)
	return formatter.FormatTree(projects)
}

func runProjectsSummary(cmd *cobra.Command, args []string) error {
	registry := CreateSourceRegistry()

	projects, err := GetProjectsFromSources(registry, projectSources)
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

func runProjectsCopy(cmd *cobra.Command, args []string) error {
	registry := CreateSourceRegistry()
	copier := cli.NewProjectCopier(registry, cli.CopyOptions{})
	return copier.Copy(args[0], args[1])
}
