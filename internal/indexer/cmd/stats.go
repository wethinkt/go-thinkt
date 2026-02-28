package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/config"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
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
		fmt.Print(thinktI18n.Tf("indexer.stats.projects", "Projects: %d\n", stats.TotalProjects))
		fmt.Print(thinktI18n.Tf("indexer.stats.sessions", "Sessions: %d\n", stats.TotalSessions))
		fmt.Print(thinktI18n.Tf("indexer.stats.entries", "Entries:  %d\n", stats.TotalEntries))
		fmt.Print(thinktI18n.Tf("indexer.stats.tokens", "Tokens:   %d\n", stats.TotalTokens))
		if len(stats.TopTools) > 0 {
			fmt.Println(thinktI18n.T("indexer.stats.topTools", "Top Tools:"))
			for _, tc := range stats.TopTools {
				fmt.Printf("  %-30s %d\n", tc.Name, tc.Count)
			}
		}

		// Indexer status
		fmt.Println("----")
		fmt.Print(thinktI18n.Tf("indexer.stats.database", "Database:    %s\n", dbPath))
		if stats.EmbedModel != "" {
			fmt.Print(thinktI18n.Tf("indexer.stats.embedderAvailable", "Embedder:    %s (available)\n", stats.EmbedModel))
		} else if stats.EmbedderAvail {
			fmt.Print(thinktI18n.Tf("indexer.stats.embedderAvailable", "Embedder:    %s (available)\n", embedding.DefaultModelID))
		} else {
			fmt.Print(thinktI18n.Tf("indexer.stats.embedderNotDownloaded", "Embedder:    %s (model not downloaded)\n", embedding.DefaultModelID))
		}
		fmt.Print(thinktI18n.Tf("indexer.stats.embeddings", "Embeddings:  %d\n", stats.TotalEmbeddings))

		return nil
	},
}

type toolCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type statsData struct {
	TotalProjects   int         `json:"total_projects"`
	TotalSessions   int         `json:"total_sessions"`
	TotalEntries    int         `json:"total_entries"`
	TotalTokens     int         `json:"total_tokens"`
	TotalEmbeddings int         `json:"total_embeddings"`
	EmbedderAvail   bool        `json:"embedder_available"`
	EmbedModel      string      `json:"embed_model,omitempty"`
	TopTools        []toolCount `json:"top_tools"`
}

func getStats() (*statsData, error) {
	// Try RPC first
	if rpc.ServerAvailable() {
		resp, err := rpc.Call(rpc.MethodStats, nil, nil)
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

	// Tool usage (top 25)
	rows, err := db.Query("SELECT tool_name, count(*) AS cnt FROM entries WHERE tool_name != '' GROUP BY tool_name ORDER BY cnt DESC LIMIT 25")
	if err == nil {
		for rows.Next() {
			var tc toolCount
			if err := rows.Scan(&tc.Name, &tc.Count); err == nil {
				stats.TopTools = append(stats.TopTools, tc)
			}
		}
		rows.Close()
	}

	// Embedding stats (from separate embeddings DB)
	if statsCfg, cfgErr := config.Load(); cfgErr == nil {
		if embDB, err := getReadOnlyEmbeddingsDB(statsCfg.Embedding.Model); err == nil {
			_ = embDB.QueryRow("SELECT count(*) FROM embeddings").Scan(&stats.TotalEmbeddings)
			embDB.Close()
		}
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
