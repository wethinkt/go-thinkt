package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
)

// Source management commands
var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "Manage and view available session sources",
	Long: `View and manage available AI assistant session sources.

Sources are the AI coding assistants that store session data
on this machine (e.g., Claude Code, Kimi Code, Gemini CLI, Copilot CLI, Codex CLI).

Examples:
  thinkt sources list      # List all available sources
  thinkt sources status    # Show detailed source status`,
	RunE: runSourcesList,
}

var sourcesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available session sources",
	Long: `List all session sources and their availability.

Shows which sources have session data available on this machine.

Sources include:
  - Kimi Code (~/.kimi)
  - Claude Code (~/.claude)
  - Gemini CLI (~/.gemini)
  - GitHub Copilot (~/.copilot)
  - Codex CLI (~/.codex)
  - Qwen Code (~/.qwen)`,
	RunE: runSourcesList,
}

var sourcesStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show detailed source status",
	Long: `Show detailed information about each session source including
workspace ID, base path, and project count.`,
	RunE: runSourcesStatus,
}

// runSourcesList lists available sources.
func runSourcesList(cmd *cobra.Command, args []string) error {
	registry := CreateSourceRegistry()

	ctx := context.Background()
	sources := registry.SourceStatus(ctx)

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(sources)
	}

	if len(sources) == 0 {
		fmt.Println(thinktI18n.T("cmd.sources.noSources", "No sources found."))
		fmt.Println(thinktI18n.T("cmd.sources.expectedSources", "\nExpected sources:"))
		fmt.Println("  - Kimi Code: ~/.kimi/")
		fmt.Println("  - Claude Code: ~/.claude/")
		fmt.Println("  - Gemini CLI: ~/.gemini/")
		fmt.Println("  - Copilot CLI: ~/.copilot/")
		fmt.Println("  - Codex CLI: ~/.codex/")
		fmt.Println("  - Qwen Code: ~/.qwen/")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
		thinktI18n.T("cmd.sources.header.source", "SOURCE"),
		thinktI18n.T("cmd.sources.header.status", "STATUS"),
		thinktI18n.T("cmd.sources.header.projects", "PROJECTS"),
		thinktI18n.T("cmd.sources.header.basePath", "BASE PATH"),
		thinktI18n.T("cmd.sources.header.workspace", "WORKSPACE"))

	const workspaceColumnWidth = 40
	for _, s := range sources {
		status := thinktI18n.T("common.status.noData", "no data")
		if s.Available {
			status = thinktI18n.T("common.status.available", "available")
		}
		projects := fmt.Sprintf("%d", s.ProjectCount)
		workspace := s.WorkspaceID
		if len(workspace) > workspaceColumnWidth {
			workspace = workspace[:(workspaceColumnWidth-3)] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Name, status, projects, s.BasePath, workspace)
	}
	w.Flush()

	return nil
}

// runSourcesStatus shows detailed source status.
func runSourcesStatus(cmd *cobra.Command, args []string) error {
	registry := CreateSourceRegistry()

	ctx := context.Background()
	sources := registry.SourceStatus(ctx)

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(sources)
	}

	if len(sources) == 0 {
		fmt.Println(thinktI18n.T("cmd.sources.noSources", "No sources found."))
		return nil
	}

	for i, s := range sources {
		if i > 0 {
			fmt.Println()
			fmt.Println("---")
			fmt.Println()
		}

		fmt.Println(thinktI18n.Tf("cmd.sources.detail.source", "Source:      %s", s.Name))
		fmt.Println(thinktI18n.Tf("cmd.sources.detail.id", "ID:          %s", s.Source))
		fmt.Println(thinktI18n.Tf("cmd.sources.detail.description", "Description: %s", s.Description))
		statusStr := thinktI18n.T("common.status.noData", "no data")
		if s.Available {
			statusStr = thinktI18n.T("common.status.available", "available")
		}
		fmt.Println(thinktI18n.Tf("cmd.sources.detail.status", "Status:      %s", statusStr))

		if s.Available {
			fmt.Println(thinktI18n.Tf("cmd.sources.detail.workspace", "Workspace:   %s", s.WorkspaceID))
			fmt.Println(thinktI18n.Tf("cmd.sources.detail.basePath", "Base Path:   %s", s.BasePath))
			fmt.Println(thinktI18n.Tf("cmd.sources.detail.projects", "Projects:    %d", s.ProjectCount))
		}
	}

	return nil
}
