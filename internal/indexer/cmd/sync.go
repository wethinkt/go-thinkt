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
				var lastSessionDone, lastSessionTotal int // track session-level progress for chunk display
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
							progress.Print(fmt.Sprintf("[%d/%d] projects | [%d/%d] sessions %s",
								data.Project, data.ProjectTotal, data.Session, data.SessionTotal, data.Message))
						} else if data.ChunksTotal > 0 {
							sid := data.SessionID
							if len(sid) > 8 {
								sid = sid[:8]
							}
							msg := fmt.Sprintf("[%d/%d] %s chunks %d/%d",
								lastSessionDone, lastSessionTotal, sid, data.ChunksDone, data.ChunksTotal)
							if data.TokensDone > 0 && !sessionStart.IsZero() {
								if secs := time.Since(sessionStart).Seconds(); secs > 0 {
									msg += fmt.Sprintf(" (%.0f tok/s)", float64(data.TokensDone)/secs)
								}
							}
							progress.Print(msg)
						} else if data.Total > 0 {
							lastSessionDone = data.Done
							lastSessionTotal = data.Total
							sid := data.SessionID
							if len(sid) > 8 {
								sid = sid[:8]
							}
							if data.ElapsedMs > 0 {
								elapsed := time.Duration(data.ElapsedMs) * time.Millisecond
								progress.Print(fmt.Sprintf("[%d/%d] %s %d chunks (%s)",
									data.Done, data.Total, sid, data.Chunks, elapsed.Round(time.Millisecond)))
							} else {
								// "Before" event — session is about to start embedding
								sessionStart = time.Now()
								progress.Print(fmt.Sprintf("[%d/%d] %s %d entries",
									data.Done, data.Total, sid, data.Entries))
							}
						}
					}
				}
			}
			resp, err := rpc.Call("sync", rpc.SyncParams{}, progressFn)
			if err != nil {
				if progress.ShouldShowProgress(quiet, verbose) {
					progress.Finish()
				}
				fmt.Fprintf(os.Stderr, "RPC sync failed, falling back to inline: %v\n", err)
			} else if !resp.OK {
				if progress.ShouldShowProgress(quiet, verbose) {
					progress.Finish()
				}
				return fmt.Errorf("sync: %s", resp.Error)
			} else {
				if progress.ShouldShowProgress(quiet, verbose) {
					progress.Finish()
				}
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
				var inlineDone, inlineTotal int
				var inlineSessionStart time.Time
				ingester.OnEmbedProgress = func(done, total, chunks, entries int, sessionID, sessionPath string, elapsed time.Duration) {
					inlineDone = done
					inlineTotal = total
					sid := sessionID[:min(8, len(sessionID))]
					if elapsed == 0 {
						inlineSessionStart = time.Now()
						progress.Print(fmt.Sprintf("[%d/%d] %s %d entries", done, total, sid, entries))
					} else {
						progress.Print(fmt.Sprintf("[%d/%d] %s %d chunks (%s)", done, total, sid, chunks, elapsed.Round(time.Millisecond)))
					}
				}
				ingester.OnEmbedChunkProgress = func(chunksDone, chunksTotal, tokensDone int, sessionID string) {
					sid := sessionID[:min(8, len(sessionID))]
					msg := fmt.Sprintf("[%d/%d] %s chunks %d/%d", inlineDone, inlineTotal, sid, chunksDone, chunksTotal)
					if tokensDone > 0 && !inlineSessionStart.IsZero() {
						if secs := time.Since(inlineSessionStart).Seconds(); secs > 0 {
							msg += fmt.Sprintf(" (%.0f tok/s)", float64(tokensDone)/secs)
						}
					}
					progress.Print(msg)
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
