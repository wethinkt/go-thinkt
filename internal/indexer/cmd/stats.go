package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
)

var (
	statsJSON bool
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show usage statistics from the index",
	RunE: func(cmd *cobra.Command, args []string) error {
		stats, err := getStats()
		if err != nil {
			return err
		}

		if statsJSON {
			return json.NewEncoder(os.Stdout).Encode(stats)
		}

		// Human readable output
		fmt.Printf("Projects: %d\n", stats.TotalProjects)
		fmt.Printf("Sessions: %d\n", stats.TotalSessions)
		fmt.Printf("Entries:  %d\n", stats.TotalEntries)
		fmt.Printf("Tokens:   %d\n", stats.TotalTokens)
		fmt.Println("Top Tools:")
		for name, count := range stats.ToolUsage {
			fmt.Printf("  %-20s %d\n", name, count)
		}

		// Indexer status
		fmt.Println("----")
		fmt.Printf("Database:    %s\n", dbPath)
		if stats.EmbedModel != "" {
			fmt.Printf("Embedder:    %s (available)\n", stats.EmbedModel)
		} else if stats.EmbedderAvail {
			fmt.Printf("Embedder:    %s (available)\n", embedding.DefaultModelID)
		} else {
			fmt.Printf("Embedder:    %s (model not downloaded)\n", embedding.DefaultModelID)
		}
		fmt.Printf("Embeddings:  %d\n", stats.TotalEmbeddings)

		return nil
	},
}

type statsData struct {
	TotalProjects   int            `json:"total_projects"`
	TotalSessions   int            `json:"total_sessions"`
	TotalEntries    int            `json:"total_entries"`
	TotalTokens     int            `json:"total_tokens"`
	TotalEmbeddings int            `json:"total_embeddings"`
	EmbedderAvail   bool           `json:"embedder_available"`
	EmbedModel      string         `json:"embed_model,omitempty"`
	ToolUsage       map[string]int `json:"tool_usage"`
}

func getStats() (*statsData, error) {
	// Try RPC first
	if rpc.ServerAvailable() {
		resp, err := rpc.Call("stats", nil, nil)
		if err == nil && resp.OK {
			var s statsData
			if err := json.Unmarshal(resp.Data, &s); err == nil {
				return &s, nil
			}
		}
		// Fall through to inline on RPC failure
	}

	// Inline fallback
	db, err := getReadOnlyDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var stats statsData

	if err := db.QueryRow("SELECT count(*) FROM projects").Scan(&stats.TotalProjects); err != nil {
		return nil, fmt.Errorf("failed to count projects: %w", err)
	}
	if err := db.QueryRow("SELECT count(*) FROM sessions").Scan(&stats.TotalSessions); err != nil {
		return nil, fmt.Errorf("failed to count sessions: %w", err)
	}
	if err := db.QueryRow("SELECT count(*) FROM entries").Scan(&stats.TotalEntries); err != nil {
		return nil, fmt.Errorf("failed to count entries: %w", err)
	}
	if err := db.QueryRow("SELECT sum(input_tokens + output_tokens) FROM entries").Scan(&stats.TotalTokens); err != nil {
		return nil, fmt.Errorf("failed to sum tokens: %w", err)
	}

	// Tool usage
	rows, err := db.Query("SELECT tool_name, count(*) FROM entries WHERE tool_name != '' GROUP BY tool_name ORDER BY count(*) DESC")
	if err == nil {
		stats.ToolUsage = make(map[string]int)
		for rows.Next() {
			var name string
			var count int
			if err := rows.Scan(&name, &count); err == nil {
				stats.ToolUsage[name] = count
			}
		}
		rows.Close()
	}

	// Embedding stats (from separate embeddings DB)
	if embDB, err := getReadOnlyEmbeddingsDB(); err == nil {
		_ = embDB.QueryRow("SELECT count(*) FROM embeddings").Scan(&stats.TotalEmbeddings)
		embDB.Close()
	}
	modelPath, _ := embedding.DefaultModelPath()
	if _, statErr := os.Stat(modelPath); statErr == nil {
		stats.EmbedderAvail = true
		stats.EmbedModel = embedding.DefaultModelID
	}

	return &stats, nil
}

func init() {
	statsCmd.Flags().BoolVar(&statsJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(statsCmd)
}
