package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/cmd"
	"github.com/wethinkt/go-thinkt/internal/indexer"
	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize all local sessions into the index",
	RunE: func(cmdObj *cobra.Command, args []string) error {
		// Try RPC first
		if rpc.ServerAvailable() {
			progress := NewProgressReporter()
			var progressFn func(rpc.Progress)
			if progress.ShouldShowProgress(quiet, verbose) {
				progressFn = func(p rpc.Progress) {
					var data struct {
						Project      int    `json:"project"`
						ProjectTotal int    `json:"project_total"`
						Session      int    `json:"session"`
						SessionTotal int    `json:"session_total"`
						Message      string `json:"message"`
						Done         int    `json:"done"`
						Total        int    `json:"total"`
						Chunks       int    `json:"chunks"`
						SessionID    string `json:"session_id"`
					}
					if err := json.Unmarshal(p.Data, &data); err == nil {
						if data.ProjectTotal > 0 {
							progress.Print(fmt.Sprintf("Projects [%d/%d] | Sessions [%d/%d] %s",
								data.Project, data.ProjectTotal, data.Session, data.SessionTotal, data.Message))
						} else if data.Total > 0 {
							sid := data.SessionID
							if len(sid) > 12 {
								sid = sid[:12]
							}
							progress.Print(fmt.Sprintf("Embedding [%d/%d] %s — %d chunks",
								data.Done, data.Total, sid, data.Chunks))
						}
					}
				}
			}
			resp, err := rpc.Call("sync", rpc.SyncParams{}, progressFn)
			if progress.ShouldShowProgress(quiet, verbose) {
				progress.Finish()
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "RPC sync failed, falling back to inline: %v\n", err)
			} else if !resp.OK {
				return fmt.Errorf("sync: %s", resp.Error)
			} else {
				if !quiet {
					fmt.Println("Indexing complete (via server).")
				}
				return nil
			}
		}

		// Inline fallback
		database, err := getDB()
		if err != nil {
			return err
		}
		defer database.Close()

		registry := cmd.CreateSourceRegistry()

		// Load yzma embedder if model is available
		var embedder *embedding.Embedder
		if e, err := embedding.NewEmbedder(""); err == nil {
			embedder = e
			defer e.Close()
		}

		ingester := indexer.NewIngester(database, registry, embedder)

		// Drop old embeddings if model changed
		ctx := context.Background()
		if err := ingester.MigrateEmbeddings(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: migration check failed: %v\n", err)
		}

		// Initialize progress reporter with TTY detection
		progress := NewProgressReporter()

		if progress.ShouldShowProgress(quiet, verbose) {
			ingester.OnProgress = func(pIdx, pTotal, sIdx, sTotal int, message string) {
				statusMsg := fmt.Sprintf("Projects [%d/%d] | Sessions [%d/%d] %s", pIdx, pTotal, sIdx, sTotal, message)
				progress.Print(statusMsg)
			}
		}

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
			if verbose && !progress.IsTTY() {
				// Only print per-project info in non-TTY mode when verbose
				fmt.Printf("Indexing project: %s (%s)\n", p.Name, p.Path)
			}
			if err := ingester.IngestProject(ctx, p, idx+1, totalProjects); err != nil {
				if progress.IsTTY() {
					// Clear progress line before printing error
					progress.Finish()
				}
				fmt.Fprintf(os.Stderr, "Error indexing project %s: %v\n", p.Name, err)
			}
		}

		// Second pass: embed any sessions that need embeddings
		if ingester.HasEmbedder() {
			if progress.ShouldShowProgress(quiet, verbose) {
				ingester.OnEmbedProgress = func(done, total, chunks int, sessionID string, elapsed time.Duration) {
					sid := sessionID[:min(12, len(sessionID))]
					if elapsed == 0 {
						progress.Print(fmt.Sprintf("Embedding [%d/%d] %s...", done, total, sid))
					} else {
						progress.Print(fmt.Sprintf("Embedding [%d/%d] %s — %d chunks (%s)", done, total, sid, chunks, elapsed.Round(time.Millisecond)))
					}
				}
			}
			if err := ingester.EmbedAllSessions(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Embedding error: %v\n", err)
			}
		}

		if progress.ShouldShowProgress(quiet, verbose) {
			progress.Finish()
		}

		if !quiet {
			if embedder != nil {
				fmt.Println("Indexing complete (with embeddings).")
			} else {
				fmt.Println("Indexing complete (without embeddings — model not available).")
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
