package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	sessionsJSON bool
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions <project_id>",
	Short: "List sessions for a project from the index",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectID := args[0]
		db, err := getDB()
		if err != nil {
			return err
		}
		defer db.Close()

		rows, err := db.Query(`
			SELECT id, path, entry_count, created_at, updated_at, model 
			FROM sessions 
			WHERE project_id = ? 
			ORDER BY updated_at DESC`, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()

		type sessionInfo struct {
			ID         string    `json:"id"`
			Path       string    `json:"path"`
			EntryCount int       `json:"entry_count"`
			CreatedAt  time.Time `json:"created_at"`
			ModifiedAt time.Time `json:"modified_at"`
			Model      string    `json:"model"`
		}

		var results []sessionInfo
		for rows.Next() {
			var s sessionInfo
			if err := rows.Scan(&s.ID, &s.Path, &s.EntryCount, &s.CreatedAt, &s.ModifiedAt, &s.Model); err == nil {
				results = append(results, s)
			}
		}

		if sessionsJSON {
			return json.NewEncoder(os.Stdout).Encode(results)
		}

		for _, s := range results {
			fmt.Printf("%s [%d entries] (%s) - %s\n", s.ID, s.EntryCount, s.Model, s.ModifiedAt.Format(time.RFC3339))
		}

		return nil
	},
}

func init() {
	sessionsCmd.Flags().BoolVar(&sessionsJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(sessionsCmd)
}
