package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
	"github.com/wethinkt/go-thinkt/internal/indexer/search"
	"github.com/wethinkt/go-thinkt/internal/tui"
)

var (
	searchFilterProject string
	searchFilterSource  string
	searchLimit         int
	searchLimitPerSess  int
	searchJSON          bool
	searchCaseSensitive bool
	searchRegex         bool
	searchList          bool // Output as list (old behavior) instead of TUI
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for text across indexed sessions",
	Long: `Search for text within the original session files.

This uses the database to find relevant files and then scans them directly,
ensuring your private content remains in your local files, not the index.

By default, this command opens an interactive TUI to browse search results.
Use --list to output results directly to the terminal (useful for scripting).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		queryText := args[0]

		results, totalMatches, err := doSearch(queryText)
		if err != nil {
			return err
		}

		// Output as JSON
		if searchJSON {
			output := struct {
				Sessions []search.SessionResult `json:"sessions"`
				Count    int                    `json:"total_matches"`
			}{
				Sessions: results,
				Count:    totalMatches,
			}
			return json.NewEncoder(os.Stdout).Encode(output)
		}

		// Output as list (old behavior)
		if searchList {
			for _, res := range results {
				fmt.Printf("\nSession: %s (Project: %s, Source: %s)\n", res.SessionID, res.ProjectName, res.Source)
				fmt.Printf("Path:    %s\n", search.ShortenPath(res.Path))
				for _, m := range res.Matches {
					fmt.Printf("  Line %d [%s]: %s\n", m.LineNum, m.Role, m.Preview)
				}
			}

			if totalMatches == 0 {
				fmt.Println(thinktI18n.T("indexer.search.noMatches", "No matches found."))
			}
			return nil
		}

		// TUI mode (default) — fall back to list when not a TTY
		if !isTTY() {
			for _, res := range results {
				fmt.Printf("\nSession: %s (Project: %s, Source: %s)\n", res.SessionID, res.ProjectName, res.Source)
				fmt.Printf("Path:    %s\n", search.ShortenPath(res.Path))
				for _, m := range res.Matches {
					fmt.Printf("  Line %d [%s]: %s\n", m.LineNum, m.Role, m.Preview)
				}
			}
			if totalMatches == 0 {
				fmt.Println(thinktI18n.T("indexer.search.noMatches", "No matches found."))
			}
			return nil
		}

		if len(results) == 0 {
			fmt.Println(thinktI18n.T("indexer.search.noMatches", "No matches found."))
			return nil
		}

		for {
			selected, err := tui.PickSearchResult(results, queryText)
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
				return nil // q/ctrl+c — exit entirely
			}
			// esc — loop back to picker
		}
	},
}

func doSearch(queryText string) ([]search.SessionResult, int, error) {
	// Try RPC first
	if rpc.ServerAvailable() {
		params := rpc.SearchParams{
			Query:           queryText,
			Project:         searchFilterProject,
			Source:          searchFilterSource,
			Limit:           searchLimit,
			LimitPerSession: searchLimitPerSess,
			CaseSensitive:   searchCaseSensitive,
			Regex:           searchRegex,
		}
		resp, err := rpc.Call(rpc.MethodSearch, params, nil)
		if err == nil && resp.OK {
			var data struct {
				Results      []search.SessionResult `json:"results"`
				TotalMatches int                    `json:"total_matches"`
			}
			if err := json.Unmarshal(resp.Data, &data); err == nil {
				return data.Results, data.TotalMatches, nil
			}
		}
		// Fall through to inline on RPC failure
	}

	// Inline fallback
	db, err := getReadOnlyDB()
	if err != nil {
		return nil, 0, err
	}
	defer db.Close()

	opts := search.SearchOptions{
		Query:           queryText,
		FilterProject:   searchFilterProject,
		FilterSource:    searchFilterSource,
		Limit:           searchLimit,
		LimitPerSession: searchLimitPerSess,
		CaseSensitive:   searchCaseSensitive,
		UseRegex:        searchRegex,
	}

	svc := search.NewService(db, nil)

	if verbose && !searchJSON && !searchList {
		fmt.Print(thinktI18n.Tf("indexer.search.searching", "Searching for %q...\n", queryText))
	}

	return svc.Search(opts)
}

func init() {
	searchCmd.Flags().StringVarP(&searchFilterProject, "project", "p", "", "Filter by project name")
	searchCmd.Flags().StringVarP(&searchFilterSource, "source", "s", "", "Filter by source (claude, kimi)")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 50, "Limit total number of matches (default 50)")
	searchCmd.Flags().IntVar(&searchLimitPerSess, "limit-per-session", 2, "Limit hits per session to reduce noise (default 2, 0 for no limit)")
	searchCmd.Flags().BoolVar(&searchList, "list", false, "Output as list instead of TUI (useful for scripting)")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "Output as JSON")
	searchCmd.Flags().BoolVarP(&searchCaseSensitive, "case-sensitive", "C", false, "Case-sensitive matching")
	searchCmd.Flags().BoolVarP(&searchRegex, "regex", "E", false, "Treat query as a regular expression")

	rootCmd.AddCommand(searchCmd)
}
