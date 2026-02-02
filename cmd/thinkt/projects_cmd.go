package main

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/cli"
	"github.com/wethinkt/go-thinkt/internal/sources/claude"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Projects command flags
var (
	treeFormat      bool
	summaryTemplate string
	sortBy          string
	sortDesc        bool
	forceDelete     bool
	projectSources  []string // --source flag (can be specified multiple times)
	withSessions    bool     // --with-sessions flag for summary
	longFormat      bool     // --long flag for columnar output
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
