package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
)

var (
	statsJSON bool
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show usage statistics from the index",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := getReadOnlyDB()
		if err != nil {
			return err
		}
		defer db.Close()

		var stats struct {
			TotalProjects   int            `json:"total_projects"`
			TotalSessions   int            `json:"total_sessions"`
			TotalEntries    int            `json:"total_entries"`
			TotalTokens     int            `json:"total_tokens"`
			TotalEmbeddings int            `json:"total_embeddings"`
			EmbedderAvail   bool           `json:"embedder_available"`
			ToolUsage       map[string]int `json:"tool_usage"`
		}

		// 1. Basic counts
		if err := db.QueryRow("SELECT count(*) FROM projects").Scan(&stats.TotalProjects); err != nil {
			return fmt.Errorf("failed to count projects: %w", err)
		}
		if err := db.QueryRow("SELECT count(*) FROM sessions").Scan(&stats.TotalSessions); err != nil {
			return fmt.Errorf("failed to count sessions: %w", err)
		}
		if err := db.QueryRow("SELECT count(*) FROM entries").Scan(&stats.TotalEntries); err != nil {
			return fmt.Errorf("failed to count entries: %w", err)
		}
		if err := db.QueryRow("SELECT sum(input_tokens + output_tokens) FROM entries").Scan(&stats.TotalTokens); err != nil {
			return fmt.Errorf("failed to sum tokens: %w", err)
		}

		// 2. Tool usage
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

		// Embedding stats
		db.QueryRow("SELECT count(*) FROM embeddings").Scan(&stats.TotalEmbeddings)
		stats.EmbedderAvail = embedding.Available()

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
		if stats.EmbedderAvail {
			fmt.Printf("Embedder:    thinkt-embed-apple (available)\n")
		} else {
			fmt.Printf("Embedder:    thinkt-embed-apple (not found)\n")
		}
		fmt.Printf("Embeddings:  %d\n", stats.TotalEmbeddings)

		return nil
	},
}

func init() {
	statsCmd.Flags().BoolVar(&statsJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(statsCmd)
}
