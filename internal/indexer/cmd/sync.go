package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/cmd"
	"github.com/wethinkt/go-thinkt/internal/config"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/indexer"
	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
)

var syncEmbed bool

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize all local sessions into the index",
	Long: `Index all local AI assistant sessions into the search database.

By default this only indexes metadata and content (fast).
Use --embed to also run embedding sync afterwards.
Use 'thinkt-indexer embeddings sync' to run embedding independently.`,
	RunE: func(cmdObj *cobra.Command, args []string) error {
		if err := runIndexSync(); err != nil {
			return err
		}
		if syncEmbed {
			// Run embeddings sync via the embeddings sync command logic
			return embeddingsSyncCmd.RunE(cmdObj, args)
		}
		return nil
	},
}

func runIndexSync() error {
	// Try RPC first
	if rpc.ServerAvailable() {
		sp := NewSyncProgress()
		var progressFn func(rpc.Progress)
		if sp.ShouldShowProgress(quiet, verbose) {
			progressFn = func(p rpc.Progress) {
				var data struct {
					Project      int    `json:"project"`
					ProjectTotal int    `json:"project_total"`
					Session      int    `json:"session"`
					SessionTotal int    `json:"session_total"`
					Message      string `json:"message"`
				}
				if err := json.Unmarshal(p.Data, &data); err == nil {
					if data.ProjectTotal > 0 {
						sp.RenderIndexing(data.Project, data.ProjectTotal, data.Session, data.SessionTotal, data.Message)
					}
				}
			}
		}
		resp, err := rpc.Call(rpc.MethodIndexSync, rpc.SyncParams{}, progressFn)
		if err != nil {
			if sp.ShouldShowProgress(quiet, verbose) {
				sp.Finish()
			}
			fmt.Fprint(os.Stderr, thinktI18n.Tf("indexer.sync.rpcFallback", "RPC sync failed, falling back to inline: %v\n", err))
		} else if !resp.OK {
			if sp.ShouldShowProgress(quiet, verbose) {
				sp.Finish()
			}
			return fmt.Errorf("sync: %s", resp.Error)
		} else {
			if sp.ShouldShowProgress(quiet, verbose) {
				sp.Finish()
			}
			if !quiet {
				fmt.Println(thinktI18n.T("indexer.sync.completeViaServer", "Indexing complete (via server)."))
			}
			return nil
		}
	}

	// Inline fallback (index only, no embedding)
	cfg, err := config.Load()
	if err != nil {
		cfg = config.Default()
	}

	database, err := getDB()
	if err != nil {
		return err
	}
	defer database.Close()

	registry := cmd.CreateSourceRegistryFiltered(cfg.Indexer.Sources)
	ingester := indexer.NewIngester(database, nil, registry, nil)

	ctx := context.Background()

	sp := NewSyncProgress()
	if sp.ShouldShowProgress(quiet, verbose) {
		ingester.OnProgress = func(pIdx, pTotal, sIdx, sTotal int, message string) {
			sp.RenderIndexing(pIdx, pTotal, sIdx, sTotal, message)
		}
	}

	projects, err := registry.ListAllProjects(ctx)
	if err != nil {
		return fmt.Errorf("failed to list projects: %w", err)
	}

	if len(projects) == 0 {
		if !quiet {
			fmt.Println(thinktI18n.T("indexer.sync.noProjects", "No projects found to index."))
		}
		return nil
	}

	totalProjects := len(projects)
	for idx, p := range projects {
		if verbose && !sp.IsTTY() {
			fmt.Print(thinktI18n.Tf("indexer.sync.indexingProject", "Indexing project: %s (%s)\n", p.Name, p.Path))
		}
		if err := ingester.IngestProject(ctx, p, idx+1, totalProjects); err != nil {
			if sp.IsTTY() {
				sp.Finish()
			}
			fmt.Fprint(os.Stderr, thinktI18n.Tf("indexer.sync.projectError", "Error indexing project %s: %v\n", p.Name, err))
		}
	}

	if sp.ShouldShowProgress(quiet, verbose) {
		sp.Finish()
	}

	if !quiet {
		fmt.Println(thinktI18n.T("indexer.sync.complete", "Indexing complete."))
	}
	return nil
}

func init() {
	syncCmd.Flags().BoolVar(&syncEmbed, "embed", false, "also run embedding sync after indexing")
	rootCmd.AddCommand(syncCmd)
}
