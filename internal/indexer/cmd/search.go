package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

var (
	searchFilterProject string
	searchFilterSource  string
	searchLimit         int
	searchLimitPerSess  int
	searchJSON          bool
)

type candidate struct {
	Path        string
	Source      string
	SessionID   string
	ProjectName string
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for text across indexed sessions",
	Long: `Search for text within the original session files.
This uses the database to find relevant files and then scans them directly,
ensuring your private content remains in your local files, not the index.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		queryText := args[0]

		db, err := getDB()
		if err != nil {
			return err
		}
		defer db.Close()

		// 1. Find candidate sessions from the DB with metadata
		sql := `
			SELECT s.path, p.source, s.id, p.name 
			FROM sessions s 
			JOIN projects p ON s.project_id = p.id
			WHERE 1=1`

		var sqlArgs []interface{}
		if searchFilterProject != "" {
			sql += " AND p.name LIKE ?"
			sqlArgs = append(sqlArgs, "%"+searchFilterProject+"%")
		}
		if searchFilterSource != "" {
			sql += " AND p.source = ?"
			sqlArgs = append(sqlArgs, searchFilterSource)
		}

		rows, err := db.Query(sql, sqlArgs...)
		if err != nil {
			return fmt.Errorf("database error: %w", err)
		}
		defer rows.Close()

		var candidates []candidate
		for rows.Next() {
			var c candidate
			if err := rows.Scan(&c.Path, &c.Source, &c.SessionID, &c.ProjectName); err == nil {
				candidates = append(candidates, c)
			}
		}

		if verbose && !searchJSON {
			fmt.Printf("Scanning %d sessions for %q...\n", len(candidates), queryText)
		}

		// 2. Parallel Scan
		rawHits := make(chan rawMatch)
		var wg sync.WaitGroup
		sem := make(chan struct{}, 20)

		for _, c := range candidates {
			wg.Add(1)
			go func(cand candidate) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				scanFile(cand, queryText, rawHits)
			}(c)
		}

		// Closer
		go func() {
			wg.Wait()
			close(rawHits)
		}()

		// 3. Aggregate hits by session
		sessionGroups := make(map[string]*sessionResult)
		var sessionOrder []string // Maintain some order

		totalMatches := 0
		for hit := range rawHits {
			group, exists := sessionGroups[hit.SessionID]
			if !exists {
				group = &sessionResult{
					SessionID:   hit.SessionID,
					ProjectName: hit.ProjectName,
					Source:      hit.Source,
					Path:        hit.Path,
					Matches:     []match{},
				}
				sessionGroups[hit.SessionID] = group
				sessionOrder = append(sessionOrder, hit.SessionID)
			}

			// Apply per-session limit
			if searchLimitPerSess > 0 && len(group.Matches) >= searchLimitPerSess {
				continue
			}

			group.Matches = append(group.Matches, match{
				LineNum: hit.LineNum,
				Preview: hit.Preview,
				Role:    hit.Role,
			})
			totalMatches++

			// Apply global limit (rough, might go over slightly due to grouping)
			if searchLimit > 0 && totalMatches >= searchLimit {
				break
			}
		}

		// 4. Output Results
		finalResults := []sessionResult{}
		for _, id := range sessionOrder {
			res := sessionGroups[id]
			if len(res.Matches) > 0 {
				finalResults = append(finalResults, *res)
			}
		}

		if searchJSON {
			output := struct {
				Sessions []sessionResult `json:"sessions"`
				Count    int             `json:"total_matches"`
			}{
				Sessions: finalResults,
				Count:    totalMatches,
			}
			return json.NewEncoder(os.Stdout).Encode(output)
		}

		for _, res := range finalResults {
			fmt.Printf("\nSession: %s (Project: %s, Source: %s)\n", res.SessionID, res.ProjectName, res.Source)
			fmt.Printf("Path:    %s\n", shortenPath(res.Path))
			for _, m := range res.Matches {
				fmt.Printf("  Line %d [%s]: %s\n", m.LineNum, m.Role, m.Preview)
			}
		}

		if totalMatches == 0 && !searchJSON {
			fmt.Println("No matches found.")
		}

		return nil
	},
}

type sessionResult struct {
	SessionID   string  `json:"session_id"`
	ProjectName string  `json:"project_name"`
	Source      string  `json:"source"`
	Path        string  `json:"path"`
	Matches     []match `json:"matches"`
}

type match struct {
	LineNum int    `json:"line_num"`
	Preview string `json:"preview"`
	Role    string `json:"role"`
}

type rawMatch struct {
	candidate
	LineNum int
	Preview string
	Role    string
}

func scanFile(c candidate, query string, out chan<- rawMatch) {
	f, err := os.Open(c.Path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		text := scanner.Text()

		if strings.Contains(strings.ToLower(text), strings.ToLower(query)) {
			role := "unknown"
			var entry struct {
				Role string `json:"role"` // Kimi style
				Type string `json:"type"` // Claude style
			}
			if err := json.Unmarshal([]byte(text), &entry); err == nil {
				if entry.Role != "" {
					role = entry.Role
				} else if entry.Type != "" {
					role = entry.Type
				}
			}

			out <- rawMatch{
				candidate: c,
				LineNum:   lineNum,
				Preview:   extractPreview(text, query),
				Role:      role,
			}
		}
	}
}

func extractPreview(line string, query string) string {
	lowerLine := strings.ToLower(line)
	lowerQuery := strings.ToLower(query)
	idx := strings.Index(lowerLine, lowerQuery)
	if idx == -1 {
		return ""
	}

	const window = 100
	
	start := idx - window
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + window
	if end > len(line) {
		end = len(line)
	}

	preview := line[start:end]
	
	if start > 0 {
		preview = "..." + preview
	}
	if end < len(line) {
		preview = preview + "..."
	}

	return preview
}

func shortenPath(path string) string {
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func init() {
	searchCmd.Flags().StringVarP(&searchFilterProject, "project", "p", "", "Filter by project name")
	searchCmd.Flags().StringVarP(&searchFilterSource, "source", "s", "", "Filter by source (claude, kimi)")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 50, "Limit total number of matches (default 50)")
	searchCmd.Flags().IntVar(&searchLimitPerSess, "limit-per-session", 2, "Limit hits per session to reduce noise (default 2, 0 for no limit)")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "Output as JSON")

	rootCmd.AddCommand(searchCmd)
}
