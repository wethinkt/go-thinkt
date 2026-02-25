package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
	"github.com/wethinkt/go-thinkt/internal/indexer/search"
	"github.com/wethinkt/go-thinkt/internal/tui"
)

// notifyServerConfigReload tells a running indexer server to reload its config.
func notifyServerConfigReload() {
	if rpc.ServerAvailable() {
		resp, err := rpc.Call(rpc.MethodConfigReload, nil, nil)
		if err != nil {
			fmt.Printf("Warning: failed to notify server: %v\n", err)
		} else if resp != nil && resp.OK {
			fmt.Println("Server notified.")
		}
	}
}

var (
	semFilterProject string
	semFilterSource  string
	semLimit         int
	semMaxDistance   float64
	semJSON          bool
	semList          bool
	semDiversity     bool
)

var semanticCmd = &cobra.Command{
	Use:   "semantic",
	Short: "Search sessions by meaning using on-device embeddings",
	Long: `Query indexed sessions using semantic similarity.

Requires embeddings to be generated first via 'embeddings sync'.
Use 'embeddings' to manage the embedding model and storage.`,
}

var semanticSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search sessions by meaning using on-device embeddings",
	Long: `Search for sessions by meaning using on-device embeddings.

The query is embedded using the Qwen3-Embedding model and compared
against stored session embeddings using cosine similarity.

By default, this command opens an interactive TUI to browse results.
Use --list to output results directly to the terminal (useful for scripting).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		queryText := args[0]

		results, err := doSemanticSearch(queryText)
		if err != nil {
			return err
		}

		// JSON output
		if semJSON {
			return json.NewEncoder(os.Stdout).Encode(results)
		}

		// List output
		if semList {
			if len(results) == 0 {
				fmt.Println("No semantic matches found.")
				return nil
			}
			for _, r := range results {
				fmt.Printf("\nSession: %s (Project: %s, Source: %s)\n",
					r.SessionID, r.ProjectName, r.Source)
				prompt := r.FirstPrompt
				if len(prompt) > 80 {
					prompt = prompt[:80] + "..."
				}
				chunk := ""
				if r.TotalChunks > 1 {
					chunk = fmt.Sprintf("  [chunk %d/%d]", r.ChunkIndex+1, r.TotalChunks)
				}
				fmt.Printf("  distance=%.4f  [%s]  %s%s\n", r.Distance, r.Role, prompt, chunk)
			}
			return nil
		}

		// TUI mode (default)
		if len(results) == 0 {
			fmt.Println("No semantic matches found.")
			return nil
		}

		for {
			selected, err := tui.PickSemanticResult(results, queryText)
			if err != nil {
				return fmt.Errorf("TUI error: %w", err)
			}
			if selected == nil {
				return nil
			}

			vr, err := tui.RunViewer(selected.SessionPath)
			if err != nil {
				return fmt.Errorf("viewer error: %w", err)
			}
			if !vr.Back {
				return nil // q/ctrl+c — exit entirely
			}
			// esc — loop back to picker
		}
	},
}

func doSemanticSearch(queryText string) ([]search.SemanticResult, error) {
	// Try RPC first
	if rpc.ServerAvailable() {
		params := rpc.SemanticSearchParams{
			Query:       queryText,
			Project:     semFilterProject,
			Source:      semFilterSource,
			Limit:       semLimit,
			MaxDistance:  semMaxDistance,
		}
		resp, err := rpc.Call(rpc.MethodSemanticSearch, params, nil)
		if err == nil && resp.OK {
			var data struct {
				Results []search.SemanticResult `json:"results"`
			}
			if err := json.Unmarshal(resp.Data, &data); err == nil {
				return data.Results, nil
			}
		}
		// Fall through to inline on RPC failure
	}

	// Inline fallback: load embedder locally
	cfg, err := config.Load()
	if err != nil {
		cfg = config.Default()
	}
	embedder, err := embedding.NewEmbedder(cfg.Embedding.Model, "")
	if err != nil {
		return nil, fmt.Errorf("semantic search unavailable: %w", err)
	}
	defer embedder.Close()

	result, err := embedder.Embed(context.Background(), []string{queryText})
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}
	if len(result.Vectors) == 0 {
		return nil, fmt.Errorf("embedding returned no results for query")
	}

	indexDB, err := getReadOnlyDB()
	if err != nil {
		return nil, err
	}
	defer indexDB.Close()

	embDB, err := getReadOnlyEmbeddingsDB()
	if err != nil {
		return nil, fmt.Errorf("embeddings database not available (run 'thinkt-indexer sync' first): %w", err)
	}
	defer embDB.Close()

	svc := search.NewService(indexDB, embDB)
	return svc.SemanticSearch(search.SemanticSearchOptions{
		QueryEmbedding: result.Vectors[0],
		Model:          embedder.EmbedModelID(),
		Dim:            embedder.Dim(),
		FilterProject: semFilterProject,
		FilterSource:  semFilterSource,
		Limit:         semLimit,
		MaxDistance:   semMaxDistance,
		Diversity:     semDiversity,
	})
}

func init() {
	semanticSearchCmd.Flags().StringVarP(&semFilterProject, "project", "p", "", "Filter by project name")
	semanticSearchCmd.Flags().StringVarP(&semFilterSource, "source", "s", "", "Filter by source")
	semanticSearchCmd.Flags().IntVarP(&semLimit, "limit", "n", 20, "Max results (default 20)")
	semanticSearchCmd.Flags().Float64Var(&semMaxDistance, "max-distance", 0, "Max cosine distance (0 = no threshold)")
	semanticSearchCmd.Flags().BoolVar(&semList, "list", false, "Output as list instead of TUI (useful for scripting)")
	semanticSearchCmd.Flags().BoolVar(&semJSON, "json", false, "Output as JSON")
	semanticSearchCmd.Flags().BoolVar(&semDiversity, "diversity", false, "Enable diversity scoring to get results from different sessions")

	semanticCmd.AddCommand(semanticSearchCmd)
	rootCmd.AddCommand(semanticCmd)
}
