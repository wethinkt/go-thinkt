package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/wethinkt/go-thinkt/internal/cli"
	"github.com/wethinkt/go-thinkt/internal/tui"
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

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "View and manage sessions across all sources",
	Long: `View and manage sessions from Kimi, Claude, and other sources.

Running without a subcommand launches the interactive session viewer.

Project selection:
  - In a project directory: automatically uses that project
  - Otherwise: shows interactive project picker
  - -p/--project <path>: use specified project
  - --pick: force picker even if in a project directory

Use --source to filter by source (kimi, claude).

Examples:
  thinkt sessions                   # Interactive viewer (same as view)
  thinkt sessions view              # Interactive picker
  thinkt sessions list              # Auto-detect or picker
  thinkt sessions list --pick       # Force project picker
  thinkt sessions list -p ./myproject
  thinkt sessions summary -p ./myproject --source kimi
  thinkt sessions delete -p ./myproject <session-id>
  thinkt sessions copy -p ./myproject <session-id> ./backup`,
	RunE: runSessionsView,
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

// logSelectedProject prints the resolved project to stderr when -v is set.
func logSelectedProject() {
	if verbose && sessionProject != "" {
		fmt.Fprintf(os.Stderr, "project: %s\n", sessionProject)
	}
}

func runSessionsList(cmd *cobra.Command, args []string) error {
	registry := CreateSourceRegistry()
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
		projects, err := GetProjectsFromSources(registry, sessionSources)
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

	logSelectedProject()

	// Get sessions for the selected project
	sessions, err := GetSessionsForProject(registry, sessionProject, sessionSources)
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
	registry := CreateSourceRegistry()
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
		projects, err := GetProjectsFromSources(registry, sessionSources)
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

	logSelectedProject()

	// Get sessions for the selected project
	sessions, err := GetSessionsForProject(registry, sessionProject, sessionSources)
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
	registry := CreateSourceRegistry()
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

	logSelectedProject()

	deleter := cli.NewSessionDeleter(registry, cli.SessionDeleteOptions{
		Force:   sessionForceDelete,
		Project: sessionProject,
	})
	return deleter.Delete(args[0])
}

func runSessionsCopy(cmd *cobra.Command, args []string) error {
	registry := CreateSourceRegistry()
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

	logSelectedProject()

	copier := cli.NewSessionCopier(registry, cli.SessionCopyOptions{
		Project: sessionProject,
	})
	return copier.Copy(args[0], args[1])
}

func runSessionsView(cmd *cobra.Command, args []string) error {
	registry := CreateSourceRegistry()
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
		projects, err := GetProjectsFromSources(registry, sessionSources)
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

	logSelectedProject()

	// Get all sessions in the project
	sessions, err := GetSessionsForProject(registry, sessionProject, sessionSources)
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
