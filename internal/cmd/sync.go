// internal/cmd/sync.go
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/index"
	indexdb "github.com/wethinkt/go-thinkt/internal/index/db"
)

var syncQuiet bool

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize all local sessions into the SQLite index",
	Long: `Index all local AI assistant sessions into the SQLite search database.

This scans all registered sources and indexes session metadata (no private
content is stored). The index enables fast search and stats queries.

Run this once after install, then the background watcher keeps it up to date.`,
	RunE: runSync,
}

func runSync(cmd *cobra.Command, args []string) error {
	dbPath, err := indexdb.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolve db path: %w", err)
	}

	database, err := indexdb.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open index db: %w", err)
	}
	defer database.Close()

	registry := CreateSourceRegistry()
	ingester := index.NewIngester(database, registry)

	ctx := context.Background()

	projects, err := registry.ListAllProjects(ctx)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}

	if len(projects) == 0 {
		if !syncQuiet {
			fmt.Println("No projects found to index.")
		}
		return nil
	}

	sp := NewSyncProgress()
	showProgress := sp.ShouldShowProgress(syncQuiet, verbose)

	if showProgress {
		ingester.OnProgress = func(pIdx, pTotal, sIdx, sTotal int, message string) {
			sp.RenderIndexing(pIdx, pTotal, sIdx, sTotal, message)
		}
	}

	totalProjects := len(projects)
	for idx, p := range projects {
		if err := ingester.IngestProject(ctx, p, idx+1, totalProjects); err != nil {
			if showProgress {
				sp.Finish()
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Error indexing project %s: %v\n", p.Name, err)
		}
	}

	if showProgress {
		sp.Finish()
	}
	if !syncQuiet {
		fmt.Println("Indexing complete.")
	}
	return nil
}

func init() {
	syncCmd.Flags().BoolVarP(&syncQuiet, "quiet", "q", false, "suppress progress output")
}
