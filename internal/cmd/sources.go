package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// Source management commands
var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "Manage and view available session sources",
	Long: `View and manage available AI assistant session sources.

Sources are the AI coding assistants that store session data
on this machine (e.g., Claude Code, Kimi Code).

Examples:
  thinkt sources list      # List all available sources
  thinkt sources status    # Show detailed source status`,
}

var sourcesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available session sources",
	Long: `List all session sources (Kimi, Claude, etc.) and their availability.

Shows which sources have session data available on this machine.`,
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
		fmt.Println("No sources found.")
		fmt.Println("\nExpected sources:")
		fmt.Println("  - Kimi Code: ~/.kimi/")
		fmt.Println("  - Claude Code: ~/.claude/")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SOURCE\tSTATUS\tPROJECTS\tWORKSPACE")

	const workspaceColumnWidth = 40
	for _, s := range sources {
		status := "no data"
		if s.Available {
			status = "available"
		}
		projects := fmt.Sprintf("%d", s.ProjectCount)
		workspace := s.WorkspaceID
		if len(workspace) > workspaceColumnWidth {
			workspace = workspace[:(workspaceColumnWidth-3)] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, status, projects, workspace)
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
		fmt.Println("No sources found.")
		return nil
	}

	for i, s := range sources {
		if i > 0 {
			fmt.Println()
			fmt.Println("---")
			fmt.Println()
		}

		fmt.Printf("Source:      %s\n", s.Name)
		fmt.Printf("ID:          %s\n", s.Source)
		fmt.Printf("Description: %s\n", s.Description)
		fmt.Printf("Status:      %s\n", map[bool]string{true: "available", false: "no data"}[s.Available])

		if s.Available {
			fmt.Printf("Workspace:   %s\n", s.WorkspaceID)
			fmt.Printf("Base Path:   %s\n", s.BasePath)
			fmt.Printf("Projects:    %d\n", s.ProjectCount)
		}
	}

	return nil
}
