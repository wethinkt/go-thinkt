package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
	"github.com/wethinkt/go-thinkt/internal/indexer/search"
	"github.com/wethinkt/go-thinkt/internal/tui"
)

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
	Short: "Semantic search and index management",
	Long:  `Commands for semantic search using on-device embeddings.`,
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
		resp, err := rpc.Call("semantic_search", params, nil)
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
	embedder, err := embedding.NewEmbedder("")
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
		Model:          embedding.ModelID,
		FilterProject: semFilterProject,
		FilterSource:  semFilterSource,
		Limit:         semLimit,
		MaxDistance:   semMaxDistance,
		Diversity:     semDiversity,
	})
}

var semanticStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show statistics about the semantic search index",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		embDB, err := getReadOnlyEmbeddingsDB()
		if err != nil {
			fmt.Println("No embeddings indexed yet.")
			fmt.Println("Run 'thinkt-indexer sync' to generate embeddings.")
			return nil
		}
		defer embDB.Close()

		var totalEmbeddings int
		var totalSessions int
		var models string
		if err := embDB.QueryRow("SELECT count(*), count(DISTINCT session_id), COALESCE(string_agg(DISTINCT model, ', '), '') FROM embeddings").Scan(
			&totalEmbeddings, &totalSessions, &models,
		); err != nil {
			return fmt.Errorf("query embeddings stats: %w", err)
		}

		if totalEmbeddings == 0 {
			fmt.Println("No embeddings indexed yet.")
			fmt.Println("Run 'thinkt-indexer sync' to generate embeddings.")
			return nil
		}

		fmt.Printf("Embeddings:  %d\n", totalEmbeddings)
		fmt.Printf("Sessions:    %d\n", totalSessions)
		fmt.Printf("Models:      %s\n", models)

		// Check if model is available
		modelPath, _ := embedding.DefaultModelPath()
		if _, err := os.Stat(modelPath); err == nil {
			fmt.Printf("Embedder:    %s (available)\n", embedding.ModelID)
		} else {
			fmt.Printf("Embedder:    %s (model not downloaded)\n", embedding.ModelID)
		}

		return nil
	},
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
	semanticCmd.AddCommand(semanticStatsCmd)
	rootCmd.AddCommand(semanticCmd)
}
