package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/cmd"
	"github.com/wethinkt/go-thinkt/internal/indexer"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize all local sessions into the index",
	RunE: func(cmdObj *cobra.Command, args []string) error {
		database, err := getDB()
		if err != nil {
			return err
		}
		defer database.Close()

		registry := cmd.CreateSourceRegistry()
		ingester := indexer.NewIngester(database, registry)

		if !quiet {
			ingester.OnProgress = func(pIdx, pTotal, sIdx, sTotal int, message string) {
				// \r moves cursor to start of line
				// \x1b[K clears from cursor to end of line
				fmt.Printf("\r\x1b[KProjects [%d/%d] | Sessions [%d/%d] %s", pIdx, pTotal, sIdx, sTotal, message)
			}
		}

		ctx := context.Background()
		projects, err := registry.ListAllProjects(ctx)
		if err != nil {
			return fmt.Errorf("failed to list projects: %w", err)
		}

		if len(projects) == 0 {
			if !quiet {
				fmt.Println("No projects found to index.")
			}
			return nil
		}

		totalProjects := len(projects)
		for idx, p := range projects {
			if verbose {
				fmt.Printf("\nIndexing project: %s (%s)\n", p.Name, p.Path)
			}
			if err := ingester.IngestProject(ctx, p, idx+1, totalProjects); err != nil {
				fmt.Fprintf(os.Stderr, "\nError indexing project %s: %v\n", p.Name, err)
			}
		}

		if !quiet {
			fmt.Println("\nIndexing complete.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
