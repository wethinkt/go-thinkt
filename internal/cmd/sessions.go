package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/wethinkt/go-thinkt/internal/cli"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
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
	sessionResolveJSON bool
	sessionJSON        bool // --json flag for JSON output
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "View and manage sessions across all sources",
	Long: `View and manage sessions from all discovered sources.

Running without a subcommand launches the interactive session viewer.

Project selection:
  - In a project directory: automatically uses that project
  - Otherwise: shows interactive project picker
  - -p/--project <path>: use specified project
  - --pick: force picker even if in a project directory

Use --source to filter by source (e.g. claude, kimi, gemini, copilot, codex, qwen).

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
  thinkt sessions list --source kimi
  thinkt sessions list --source qwen
  thinkt sessions list --json       # JSON output`,
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
	Long: `Delete a session file from a known source.

The session can be specified as:
  - Full path to a known session file
  - Session ID (requires -p/--project)
  - Filename (requires -p/--project)

Before deletion, shows session info and prompts for confirmation.
Use --force to skip the confirmation.

Examples:
  thinkt sessions delete /full/path/to/session
  thinkt sessions delete -p ./myproject abc123
  thinkt sessions delete -p ./myproject --force abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runSessionsDelete,
}

var sessionsCopyCmd = &cobra.Command{
	Use:   "copy <session> <target>",
	Short: "Copy a session to a target location",
	Long: `Copy a known session file to a target location.

The session can be specified as:
  - Full path to a known session file
  - Session ID (requires -p/--project)
  - Filename (requires -p/--project)

The target can be a file path or directory.

Examples:
  thinkt sessions copy /full/path/to/session ./backup/
  thinkt sessions copy -p ./myproject abc123 ./backup/
  thinkt sessions copy -p ./myproject abc123 ./backup/renamed-session`,
	Args: cobra.ExactArgs(2),
	RunE: runSessionsCopy,
}

var sessionsViewCmd = &cobra.Command{
	Use:   "view [session]",
	Short: "View a session in the terminal (interactive picker)",
	Long: `View a session in a full-terminal viewer.

If no session is specified, shows an interactive picker of all recent sessions.
The picker works across all discovered sources.

	The session can be specified as:
  - Full path to a known session file
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
  thinkt sessions view /full/path/to/session
  thinkt sessions view -p ./myproject abc123
  thinkt sessions view -p ./myproject --all        # view all
  thinkt sessions view /path/to/session --raw      # raw output to stdout`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSessionsView,
}

var sessionsResumeCmd = &cobra.Command{
	Use:   "resume [session]",
	Short: "Resume a session in its original CLI tool",
	Long: `Resume a session in its original CLI tool (e.g., claude --resume).

If no session is specified, shows an interactive picker.
Only sources that support resume (e.g., Claude Code, Kimi Code) are available.

The session can be specified as:
  - Full path to a known session file
  - Session ID (requires -p/--project)
  - Filename (requires -p/--project)

Examples:
  thinkt sessions resume                   # Interactive picker
  thinkt sessions resume -p ./myproject abc123
  thinkt sessions resume /full/path/to/session.jsonl`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSessionsResume,
}

var sessionsResolveCmd = &cobra.Command{
	Use:   "resolve <session>",
	Short: "Resolve a session query to its canonical path",
	Long: `Resolve a session query (ID, filename suffix, or absolute path)
to a known session from registered sources.

By default, outputs only the canonical full path.
Use --json for structured output.`,
	Args: cobra.ExactArgs(1),
	RunE: runSessionsResolve,
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
		// --json implies non-interactive mode; never show TUI
		if sessionJSON {
			return fmt.Errorf("no project detected\n\nUse -p <path> to specify a project, or run from within a project directory\nOr use 'thinkt sessions' (without --json) for interactive mode")
		}

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
	
	if sessionJSON {
		return formatter.FormatJSON(sessions)
	}
	
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
	if sessionProject == "" && !filepath.IsAbs(args[0]) && !sessionForcePicker {
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
	if sessionProject == "" && !filepath.IsAbs(args[0]) && !sessionForcePicker {
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
	if len(args) > 0 && filepath.IsAbs(args[0]) {
		if sessionViewRaw {
			return tui.ViewSessionRawWithRegistry(args[0], registry, os.Stdout)
		}
		_, err := tui.RunViewerWithRegistry(args[0], registry)
		return err
	}

	// Track a display-friendly project name for the TUI header
	var projectDisplayName string

	// If no project specified and not forcing picker, try auto-detection from cwd
	if sessionProject == "" && !sessionForcePicker {
		cwd, err := os.Getwd()
		if err == nil {
			if project := registry.FindProjectForPath(ctx, cwd); project != nil {
				sessionProject = project.ID
				projectDisplayName = project.Name
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
		projectDisplayName = selected.Name
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
		if filepath.IsAbs(sessionArg) {
			sessionPath = sessionArg
		} else {
			// Match by session ID or filename
			found := false
			for _, s := range sessions {
				if s.ID == sessionArg ||
					strings.HasSuffix(s.FullPath, sessionArg) ||
					strings.HasSuffix(s.FullPath, sessionArg+".jsonl") ||
					strings.HasSuffix(s.FullPath, sessionArg+".json") {
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
			return tui.ViewSessionRawWithRegistry(sessionPath, registry, os.Stdout)
		}
		_, err := tui.RunViewerWithRegistry(sessionPath, registry)
		return err
	}

	// No session specified - either show picker or view all
	if sessionViewAll {
		// View all sessions in time order (oldest first)
		paths := make([]string, len(sessions))
		for i, s := range sessions {
			paths[i] = s.FullPath
		}
		_, err := tui.RunMultiViewerWithRegistry(paths, registry)
		return err
	}

	// Show session picker (requires TTY)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("no session specified and no TTY available\n\nUsage: thinkt sessions view -p <project> <session>\n\nUse 'thinkt sessions list -p %s' to see available sessions", sessionProject)
	}

	// Run session browser: picker + viewer with back navigation via Shell
	return tui.RunSessionBrowserWithRegistry(sessions, registry, projectDisplayName)
}

func runSessionsResume(cmd *cobra.Command, args []string) error {
	registry := CreateSourceRegistry()
	ctx := context.Background()

	// Handle absolute path first
	if len(args) > 0 && filepath.IsAbs(args[0]) {
		_, meta, err := registry.ResolveSessionByPath(ctx, args[0])
		if err != nil || meta == nil {
			return fmt.Errorf("session not found: %s", args[0])
		}
		return execResume(registry, *meta)
	}

	var projectDisplayName string

	// If no project specified and not forcing picker, try auto-detection from cwd
	if sessionProject == "" && !sessionForcePicker {
		cwd, err := os.Getwd()
		if err == nil {
			if project := registry.FindProjectForPath(ctx, cwd); project != nil {
				sessionProject = project.ID
				projectDisplayName = project.Name
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
			fmt.Println("No projects found")
			return nil
		}
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("--project/-p is required when no TTY available")
		}
		selected, err := tui.PickProject(projects)
		if err != nil {
			return err
		}
		if selected == nil {
			return nil
		}
		sessionProject = selected.ID
		projectDisplayName = selected.Name
	}

	logSelectedProject()

	sessions, err := GetSessionsForProject(registry, sessionProject, sessionSources)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		return fmt.Errorf("no sessions found in project")
	}

	// If a session is specified, find and resume it
	if len(args) > 0 {
		sessionArg := args[0]
		for _, s := range sessions {
			if s.ID == sessionArg ||
				strings.HasSuffix(s.FullPath, sessionArg) ||
				strings.HasSuffix(s.FullPath, sessionArg+".jsonl") ||
				strings.HasSuffix(s.FullPath, sessionArg+".json") {
				return execResume(registry, s)
			}
		}
		return fmt.Errorf("session not found: %s\n\nUse 'thinkt sessions list -p %s' to see available sessions", sessionArg, sessionProject)
	}

	// No session specified — show picker
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("no session specified and no TTY available\n\nUsage: thinkt sessions resume -p <project> <session>")
	}

	title := "Resume session"
	if projectDisplayName != "" {
		title = projectDisplayName + " · " + title
	}
	selected, err := tui.PickSessionWithTitle(sessions, title)
	if err != nil {
		return err
	}
	if selected == nil {
		return nil
	}
	return execResume(registry, *selected)
}

// execResume looks up the SessionResumer for a session's source and execs the resume command.
func execResume(registry *thinkt.StoreRegistry, session thinkt.SessionMeta) error {
	resumer, ok := registry.GetResumer(session.Source)
	if !ok {
		return fmt.Errorf("source %q does not support session resume", session.Source)
	}
	info, err := resumer.ResumeCommand(session)
	if err != nil {
		return err
	}
	return thinkt.ExecResume(info)
}

func runSessionsResolve(cmd *cobra.Command, args []string) error {
	registry := CreateSourceRegistry()
	ctx := context.Background()

	// If no project specified and query is not absolute, try auto-detection from cwd.
	if sessionProject == "" && !filepath.IsAbs(args[0]) && !sessionForcePicker {
		cwd, err := os.Getwd()
		if err == nil {
			if project := registry.FindProjectForPath(ctx, cwd); project != nil {
				sessionProject = project.ID
			}
		}
	}

	meta, err := cli.ResolveSession(registry, sessionProject, args[0])
	if err != nil {
		return err
	}

	if sessionResolveJSON {
		out := struct {
			ID         string        `json:"id"`
			FullPath   string        `json:"full_path"`
			Project    string        `json:"project_path,omitempty"`
			Source     thinkt.Source `json:"source"`
			Workspace  string        `json:"workspace_id,omitempty"`
			EntryCount int           `json:"entry_count,omitempty"`
			ModifiedAt string        `json:"modified_at,omitempty"`
		}{
			ID:         meta.ID,
			FullPath:   meta.FullPath,
			Project:    meta.ProjectPath,
			Source:     meta.Source,
			Workspace:  meta.WorkspaceID,
			EntryCount: meta.EntryCount,
		}
		if !meta.ModifiedAt.IsZero() {
			out.ModifiedAt = meta.ModifiedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}

	fmt.Fprintln(os.Stdout, meta.FullPath)
	return nil
}
