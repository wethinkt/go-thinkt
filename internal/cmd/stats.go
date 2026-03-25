// internal/cmd/stats.go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	indexdb "github.com/wethinkt/go-thinkt/internal/index/db"
)

var statsJSON bool

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show usage statistics from the index",
	Long: `Display aggregate statistics from the SQLite session index.

Shows project, session, entry, and token counts along with top tool usage.
Run 'thinkt sync' first to populate the index.`,
	RunE: runStats,
}

type statsToolCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type statsOutput struct {
	TotalProjects int              `json:"total_projects"`
	TotalSessions int              `json:"total_sessions"`
	TotalEntries  int              `json:"total_entries"`
	TotalTokens   int              `json:"total_tokens"`
	TopTools      []statsToolCount `json:"top_tools"`
}

func runStats(cmd *cobra.Command, args []string) error {
	dbPath, err := indexdb.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolve db path: %w", err)
	}

	database, err := indexdb.OpenReadOnly(dbPath)
	if err != nil {
		return fmt.Errorf("open index db: %w (run 'thinkt sync' first)", err)
	}
	defer database.Close()

	var stats statsOutput

	if err := database.QueryRow("SELECT count(*) FROM projects").Scan(&stats.TotalProjects); err != nil {
		return fmt.Errorf("count projects: %w", err)
	}
	if err := database.QueryRow("SELECT count(*) FROM sessions").Scan(&stats.TotalSessions); err != nil {
		return fmt.Errorf("count sessions: %w", err)
	}
	if err := database.QueryRow("SELECT count(*) FROM entries").Scan(&stats.TotalEntries); err != nil {
		return fmt.Errorf("count entries: %w", err)
	}
	_ = database.QueryRow("SELECT COALESCE(sum(input_tokens + output_tokens), 0) FROM entries").Scan(&stats.TotalTokens)

	rows, err := database.Query("SELECT tool_name, count(*) AS cnt FROM entries WHERE tool_name != '' GROUP BY tool_name ORDER BY cnt DESC LIMIT 25")
	if err == nil {
		for rows.Next() {
			var tc statsToolCount
			if err := rows.Scan(&tc.Name, &tc.Count); err == nil {
				stats.TopTools = append(stats.TopTools, tc)
			}
		}
		rows.Close()
	}

	if statsJSON {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}

	fmt.Printf("Projects: %d\n", stats.TotalProjects)
	fmt.Printf("Sessions: %d\n", stats.TotalSessions)
	fmt.Printf("Entries:  %d\n", stats.TotalEntries)
	fmt.Printf("Tokens:   %d\n", stats.TotalTokens)
	if len(stats.TopTools) > 0 {
		fmt.Println("Top Tools:")
		for _, tc := range stats.TopTools {
			fmt.Printf("  %-30s %d\n", tc.Name, tc.Count)
		}
	}

	return nil
}

func init() {
	statsCmd.Flags().BoolVar(&statsJSON, "json", false, "Output as JSON")
	// Registration happens in root.go
}
