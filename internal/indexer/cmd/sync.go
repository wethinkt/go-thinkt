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

		ctx := context.Background()
		projects, err := registry.ListAllProjects(ctx)
		if err != nil {
			return fmt.Errorf("failed to list projects: %w", err)
		}

		if len(projects) == 0 {
			fmt.Println("No projects found to index.")
			return nil
		}

		for _, p := range projects {
			if verbose {
				fmt.Printf("Indexing project: %s (%s)\n", p.Name, p.Path)
			}
			if err := ingester.IngestProject(ctx, p); err != nil {
				fmt.Fprintf(os.Stderr, "Error indexing project %s: %v\n", p.Name, err)
			}
		}

		fmt.Println("Indexing complete.")
		return nil
	},
}
