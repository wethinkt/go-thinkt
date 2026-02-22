package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
	"github.com/wethinkt/go-thinkt/internal/indexer/search"
)

var (
	semFilterProject string
	semFilterSource  string
	semLimit         int
	semMaxDistance    float64
	semJSON          bool
)

var semanticCmd = &cobra.Command{
	Use:   "semantic",
	Short: "Semantic search and index management",
	Long:  `Commands for semantic search using on-device embeddings.`,
}

var semanticSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search sessions by meaning using on-device embeddings",
	Long: `Search for sessions by meaning using Apple's on-device NLContextualEmbedding.

Requires thinkt-embed-apple to be installed and in PATH.
The query is embedded and compared against stored session embeddings
using cosine similarity.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		queryText := args[0]

		// Get embedding for query
		client, err := embedding.NewClient()
		if err != nil {
			return fmt.Errorf("semantic search unavailable: %w", err)
		}
		defer client.Close()

		responses, err := client.EmbedBatch(context.Background(), []embedding.EmbedRequest{
			{ID: "query", Text: queryText},
		})
		if err != nil {
			return fmt.Errorf("failed to embed query: %w", err)
		}
		if len(responses) == 0 {
			return fmt.Errorf("embedding returned no results for query")
		}

		// Search
		db, err := getReadOnlyDB()
		if err != nil {
			return err
		}
		defer db.Close()

		svc := search.NewService(db)
		results, err := svc.SemanticSearch(search.SemanticSearchOptions{
			QueryEmbedding: responses[0].Embedding,
			Model:          "apple-nlcontextual-v1",
			FilterProject:  semFilterProject,
			FilterSource:   semFilterSource,
			Limit:          semLimit,
			MaxDistance:     semMaxDistance,
		})
		if err != nil {
			return err
		}

		if semJSON {
			return json.NewEncoder(os.Stdout).Encode(results)
		}

		if len(results) == 0 {
			fmt.Println("No semantic matches found.")
			return nil
		}

		for _, r := range results {
			chunk := ""
			if r.TotalChunks > 1 {
				chunk = fmt.Sprintf(" [chunk %d/%d]", r.ChunkIndex+1, r.TotalChunks)
			}
			prompt := r.FirstPrompt
			if len(prompt) > 80 {
				prompt = prompt[:80] + "..."
			}
			fmt.Printf("%.4f  %-9s  %s  %s%s\n",
				r.Distance, r.Role, r.ProjectName, r.SessionID[:min(12, len(r.SessionID))], chunk)
			if r.ToolName != "" {
				fmt.Printf("         tool: %s\n", r.ToolName)
			}
			if prompt != "" {
				fmt.Printf("         session: %s\n", prompt)
			}
			fmt.Println()
		}
		return nil
	},
}

var semanticStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show statistics about the semantic search index",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := getReadOnlyDB()
		if err != nil {
			return err
		}
		defer db.Close()

		var totalEmbeddings int
		var totalSessions int
		var models string
		if err := db.QueryRow("SELECT count(*), count(DISTINCT session_id), COALESCE(string_agg(DISTINCT model, ', '), '') FROM embeddings").Scan(
			&totalEmbeddings, &totalSessions, &models,
		); err != nil {
			return fmt.Errorf("query embeddings stats: %w", err)
		}

		if totalEmbeddings == 0 {
			fmt.Println("No embeddings indexed yet.")
			fmt.Println("Run 'thinkt-indexer sync' with thinkt-embed-apple in PATH to generate embeddings.")
			return nil
		}

		fmt.Printf("Embeddings:  %d\n", totalEmbeddings)
		fmt.Printf("Sessions:    %d\n", totalSessions)
		fmt.Printf("Models:      %s\n", models)

		// Check if embedding binary is available
		if embedding.Available() {
			fmt.Printf("Backend:     thinkt-embed-apple (available)\n")
		} else {
			fmt.Printf("Backend:     thinkt-embed-apple (not found)\n")
		}

		return nil
	},
}

func init() {
	semanticSearchCmd.Flags().StringVarP(&semFilterProject, "project", "p", "", "Filter by project name")
	semanticSearchCmd.Flags().StringVarP(&semFilterSource, "source", "s", "", "Filter by source")
	semanticSearchCmd.Flags().IntVarP(&semLimit, "limit", "n", 20, "Max results (default 20)")
	semanticSearchCmd.Flags().Float64Var(&semMaxDistance, "max-distance", 0, "Max cosine distance (0 = no threshold)")
	semanticSearchCmd.Flags().BoolVar(&semJSON, "json", false, "Output as JSON")

	semanticCmd.AddCommand(semanticSearchCmd)
	semanticCmd.AddCommand(semanticStatsCmd)
	rootCmd.AddCommand(semanticCmd)
}
