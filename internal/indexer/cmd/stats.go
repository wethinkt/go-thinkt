package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
			TotalProjects int            `json:"total_projects"`
			TotalSessions int            `json:"total_sessions"`
			TotalEntries  int            `json:"total_entries"`
			TotalTokens   int            `json:"total_tokens"`
			ToolUsage     map[string]int `json:"tool_usage"`
		}

		// 1. Basic counts
		db.QueryRow("SELECT count(*) FROM projects").Scan(&stats.TotalProjects)
		db.QueryRow("SELECT count(*) FROM sessions").Scan(&stats.TotalSessions)
		db.QueryRow("SELECT count(*) FROM entries").Scan(&stats.TotalEntries)
		db.QueryRow("SELECT sum(input_tokens + output_tokens) FROM entries").Scan(&stats.TotalTokens)

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

		return nil
	},
}

func init() {
	statsCmd.Flags().BoolVar(&statsJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(statsCmd)
}
