package cmd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/indexer"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

var (
	sessionsJSON   bool
	sessionsSource string
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions <project_id>",
	Short: "List sessions for a project from the index",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectID := args[0]
		source := strings.ToLower(strings.TrimSpace(sessionsSource))

		db, err := getReadOnlyDB()
		if err != nil {
			return err
		}
		defer db.Close()

		var rows *sql.Rows
		if source != "" {
			candidates := indexer.ScopedProjectIDCandidates(thinkt.Source(source), projectID)
			rows, err = db.Query(`
				SELECT s.id, s.path, s.entry_count, s.created_at, s.updated_at, s.model, COALESCE(p.source, ?)
				FROM sessions s
				LEFT JOIN projects p ON s.project_id = p.id
				WHERE (s.project_id = ? OR s.project_id = ?)
				  AND (p.source = ? OR p.source IS NULL)
				ORDER BY s.updated_at DESC`, source, candidates[0], candidates[1], source)
		} else {
			rows, err = db.Query(`
				SELECT s.id, s.path, s.entry_count, s.created_at, s.updated_at, s.model, COALESCE(p.source, '')
				FROM sessions s
				LEFT JOIN projects p ON s.project_id = p.id
				WHERE s.project_id = ? OR s.project_id LIKE ?
				ORDER BY s.updated_at DESC`, projectID, "%"+indexer.ProjectIDScopeSeparator()+projectID)
		}
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
			Source     string    `json:"source,omitempty"`
		}

		var results []sessionInfo
		for rows.Next() {
			var s sessionInfo
			if err := rows.Scan(&s.ID, &s.Path, &s.EntryCount, &s.CreatedAt, &s.ModifiedAt, &s.Model, &s.Source); err == nil {
				results = append(results, s)
			}
		}

		if sessionsJSON {
			return json.NewEncoder(os.Stdout).Encode(results)
		}

		for _, s := range results {
			if s.Source != "" {
				fmt.Printf("%s [%d entries] (%s, %s) - %s\n", s.ID, s.EntryCount, s.Model, s.Source, s.ModifiedAt.Format(time.RFC3339))
				continue
			}
			fmt.Printf("%s [%d entries] (%s) - %s\n", s.ID, s.EntryCount, s.Model, s.ModifiedAt.Format(time.RFC3339))
		}

		return nil
	},
}

func init() {
	sessionsCmd.Flags().BoolVar(&sessionsJSON, "json", false, "Output as JSON")
	sessionsCmd.Flags().StringVar(&sessionsSource, "source", "", "Filter by source (e.g. claude, kimi, codex)")
	rootCmd.AddCommand(sessionsCmd)
}
