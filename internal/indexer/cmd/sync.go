package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/cmd"
	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/indexer"
	"github.com/wethinkt/go-thinkt/internal/indexer/db"
	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize all local sessions into the index",
	RunE: func(cmdObj *cobra.Command, args []string) error {
		// Try RPC first
		if rpc.ServerAvailable() {
			sp := NewSyncProgress()
			var progressFn func(rpc.Progress)
			if sp.ShouldShowProgress(quiet, verbose) {
				var lastSessionDone, lastSessionTotal int
				var sessionStart time.Time
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
						Entries      int    `json:"entries"`
						ChunksDone   int    `json:"chunks_done"`
						ChunksTotal  int    `json:"chunks_total"`
						TokensDone   int    `json:"tokens_done"`
						SessionID    string `json:"session_id"`
						SessionPath  string `json:"session_path"`
						ElapsedMs    int64  `json:"elapsed_ms"`
					}
					if err := json.Unmarshal(p.Data, &data); err == nil {
						if data.ProjectTotal > 0 {
							sp.RenderIndexing(data.Project, data.ProjectTotal, data.Session, data.SessionTotal, data.Message)
						} else if data.ChunksTotal > 0 {
							sid := data.SessionID
							if len(sid) > 8 {
								sid = sid[:8]
							}
							detail := fmt.Sprintf("%s · %d/%d chunks", sid, data.ChunksDone, data.ChunksTotal)
							if data.TokensDone > 0 && !sessionStart.IsZero() {
								if secs := time.Since(sessionStart).Seconds(); secs > 0 {
									detail += fmt.Sprintf("  %.0f tok/s", float64(data.TokensDone)/secs)
								}
							}
							sp.RenderEmbedding(lastSessionDone, lastSessionTotal, detail)
						} else if data.Total > 0 {
							lastSessionDone = data.Done
							lastSessionTotal = data.Total
							sid := data.SessionID
							if len(sid) > 8 {
								sid = sid[:8]
							}
							if data.ElapsedMs > 0 {
								elapsed := time.Duration(data.ElapsedMs) * time.Millisecond
								detail := fmt.Sprintf("%s · %d chunks (%s)", sid, data.Chunks, elapsed.Round(time.Millisecond))
								sp.RenderEmbedding(data.Done, data.Total, detail)
							} else {
								sessionStart = time.Now()
								detail := fmt.Sprintf("%s · %d entries", sid, data.Entries)
								sp.RenderEmbedding(data.Done, data.Total, detail)
							}
						}
					}
				}
			}
			resp, err := rpc.Call("sync", rpc.SyncParams{}, progressFn)
			if err != nil {
				if sp.ShouldShowProgress(quiet, verbose) {
					sp.Finish()
				}
				fmt.Fprintf(os.Stderr, "RPC sync failed, falling back to inline: %v\n", err)
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
					fmt.Println("Indexing complete (via server).")
				}
				return nil
			}
		}

		// Inline fallback
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

		// Load yzma embedder if enabled and model is available
		var embedder *embedding.Embedder
		var embDB *db.DB
		if cfg.Embedding.Enabled {
			if e, err := embedding.NewEmbedder(""); err == nil {
				embedder = e
				defer e.Close()
				if d, err := getEmbeddingsDB(); err == nil {
					embDB = d
					defer d.Close()
				}
			}
		}

		ingester := indexer.NewIngester(database, embDB, registry, embedder)

		// Drop old embeddings if model changed
		ctx := context.Background()
		if err := ingester.MigrateEmbeddings(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: migration check failed: %v\n", err)
		}

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
				fmt.Println("No projects found to index.")
			}
			return nil
		}

		totalProjects := len(projects)
		for idx, p := range projects {
			if verbose && !sp.IsTTY() {
				fmt.Printf("Indexing project: %s (%s)\n", p.Name, p.Path)
			}
			if err := ingester.IngestProject(ctx, p, idx+1, totalProjects); err != nil {
				if sp.IsTTY() {
					sp.Finish()
				}
				fmt.Fprintf(os.Stderr, "Error indexing project %s: %v\n", p.Name, err)
			}
		}

		// Second pass: embed any sessions that need embeddings
		if ingester.HasEmbedder() {
			if sp.ShouldShowProgress(quiet, verbose) {
				var inlineDone, inlineTotal int
				var inlineSessionStart time.Time
				ingester.OnEmbedProgress = func(done, total, chunks, entries int, sessionID, sessionPath string, elapsed time.Duration) {
					inlineDone = done
					inlineTotal = total
					sid := sessionID[:min(8, len(sessionID))]
					if elapsed == 0 {
						inlineSessionStart = time.Now()
						sp.RenderEmbedding(done, total, fmt.Sprintf("%s · %d entries", sid, entries))
					} else {
						sp.RenderEmbedding(done, total, fmt.Sprintf("%s · %d chunks (%s)", sid, chunks, elapsed.Round(time.Millisecond)))
					}
				}
				ingester.OnEmbedChunkProgress = func(chunksDone, chunksTotal, tokensDone int, sessionID string) {
					sid := sessionID[:min(8, len(sessionID))]
					detail := fmt.Sprintf("%s · %d/%d chunks", sid, chunksDone, chunksTotal)
					if tokensDone > 0 && !inlineSessionStart.IsZero() {
						if secs := time.Since(inlineSessionStart).Seconds(); secs > 0 {
							detail += fmt.Sprintf("  %.0f tok/s", float64(tokensDone)/secs)
						}
					}
					sp.RenderEmbedding(inlineDone, inlineTotal, detail)
				}
			}
			if err := ingester.EmbedAllSessions(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Embedding error: %v\n", err)
			}
		}

		if sp.ShouldShowProgress(quiet, verbose) {
			sp.Finish()
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
