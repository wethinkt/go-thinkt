// internal/cmd/search.go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	indexdb "github.com/wethinkt/go-thinkt/internal/index/db"
	"github.com/wethinkt/go-thinkt/internal/index/search"
	indexersearch "github.com/wethinkt/go-thinkt/internal/indexer/search"
	"github.com/wethinkt/go-thinkt/internal/tui"
	"golang.org/x/term"
)

var (
	searchFilterProject string
	searchFilterSource  string
	searchLimit         int
	searchLimitPerSess  int
	searchJSON          bool
	searchCaseSensitive bool
	searchRegex         bool
	searchList          bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for text across indexed sessions",
	Long: `Search for text within session files using the SQLite index.

The index is used to find relevant files, then scans them directly.
Your private content stays in local files, not the index.

By default opens an interactive TUI. Use --list for scripting output.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		queryText := args[0]

		dbPath, err := indexdb.DefaultPath()
		if err != nil {
			return fmt.Errorf("resolve db path: %w", err)
		}

		database, err := indexdb.OpenReadOnly(dbPath)
		if err != nil {
			return fmt.Errorf("open index db: %w (run 'thinkt sync' first)", err)
		}
		defer database.Close()

		svc := search.NewService(database)
		opts := search.SearchOptions{
			Query:           queryText,
			FilterProject:   searchFilterProject,
			FilterSource:    searchFilterSource,
			Limit:           searchLimit,
			LimitPerSession: searchLimitPerSess,
			CaseSensitive:   searchCaseSensitive,
			UseRegex:        searchRegex,
		}

		results, totalMatches, err := svc.Search(opts)
		if err != nil {
			return err
		}

		if searchJSON {
			output := struct {
				Sessions []search.SessionResult `json:"sessions"`
				Count    int                    `json:"total_matches"`
			}{Sessions: results, Count: totalMatches}
			return json.NewEncoder(os.Stdout).Encode(output)
		}

		if searchList || !term.IsTerminal(int(os.Stdin.Fd())) {
			for _, res := range results {
				fmt.Printf("\nSession: %s (Project: %s, Source: %s)\n", res.SessionID, res.ProjectName, res.Source)
				fmt.Printf("Path:    %s\n", search.ShortenPath(res.Path))
				for _, m := range res.Matches {
					fmt.Printf("  Line %d [%s]: %s\n", m.LineNum, m.Role, m.Preview)
				}
			}
			if totalMatches == 0 {
				fmt.Println("No matches found.")
			}
			return nil
		}

		if len(results) == 0 {
			fmt.Println("No matches found.")
			return nil
		}

		// Convert index/search results to indexer/search types for the TUI picker.
		tuiResults := toIndexerResults(results)
		for {
			selected, err := tui.PickSearchResult(tuiResults, queryText)
			if err != nil {
				return fmt.Errorf("TUI error: %w", err)
			}
			if selected == nil {
				return nil
			}
			vr, err := tui.RunViewer(selected.Path)
			if err != nil {
				return fmt.Errorf("viewer error: %w", err)
			}
			if !vr.Back {
				return nil
			}
		}
	},
}

// toIndexerResults converts index/search results to indexer/search types
// for the TUI picker, which currently imports indexer/search types.
func toIndexerResults(results []search.SessionResult) []indexersearch.SessionResult {
	out := make([]indexersearch.SessionResult, len(results))
	for i, r := range results {
		matches := make([]indexersearch.Match, len(r.Matches))
		for j, m := range r.Matches {
			matches[j] = indexersearch.Match{
				LineNum:    m.LineNum,
				Preview:    m.Preview,
				Role:       m.Role,
				MatchStart: m.MatchStart,
				MatchEnd:   m.MatchEnd,
			}
		}
		out[i] = indexersearch.SessionResult{
			SessionID:   r.SessionID,
			ProjectName: r.ProjectName,
			Source:      r.Source,
			Path:        r.Path,
			Matches:     matches,
		}
	}
	return out
}

func init() {
	searchCmd.Flags().StringVarP(&searchFilterProject, "project", "p", "", "Filter by project name")
	searchCmd.Flags().StringVarP(&searchFilterSource, "source", "s", "", "Filter by source (claude, kimi)")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 50, "Limit total matches")
	searchCmd.Flags().IntVar(&searchLimitPerSess, "limit-per-session", 2, "Limit hits per session (0 for no limit)")
	searchCmd.Flags().BoolVar(&searchList, "list", false, "Output as list instead of TUI")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "Output as JSON")
	searchCmd.Flags().BoolVarP(&searchCaseSensitive, "case-sensitive", "C", false, "Case-sensitive matching")
	searchCmd.Flags().BoolVarP(&searchRegex, "regex", "E", false, "Treat query as regex")
	// Registration happens in root.go
}
