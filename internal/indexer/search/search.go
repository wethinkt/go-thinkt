// Package search provides types and functions for searching indexed sessions.
// This package is designed to be used both by the thinkt-indexer CLI and
// as a shareable TUI component in the main thinkt application.
package search

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/wethinkt/go-thinkt/internal/indexer/db"
)

// Matcher encapsulates the search matching strategy.
type Matcher struct {
	re *regexp.Regexp // non-nil for regex or case-insensitive substring
}

// NewMatcher creates a matcher from the query and flags. For plain substring
// search it compiles a regexp.QuoteMeta'd pattern; for regex it uses the
// query directly. Case-insensitivity is handled via the (?i) flag.
func NewMatcher(query string, caseSensitive, useRegex bool) (*Matcher, error) {
	pattern := query
	if !useRegex {
		pattern = regexp.QuoteMeta(query)
	}
	if !caseSensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern %q: %w", query, err)
	}
	return &Matcher{re: re}, nil
}

func (m *Matcher) Match(text string) bool {
	return m.re.MatchString(text)
}

// FindIndex returns the byte offsets [start, end) of the first match in text,
// or (-1, -1) if there is no match.
func (m *Matcher) FindIndex(text string) (int, int) {
	loc := m.re.FindStringIndex(text)
	if loc == nil {
		return -1, -1
	}
	return loc[0], loc[1]
}

// Candidate represents a session to be searched.
type Candidate struct {
	Path        string
	Source      string
	SessionID   string
	ProjectName string
}

// Match represents a single match within a session.
type Match struct {
	LineNum    int    `json:"line_num"`
	Preview    string `json:"preview"`
	Role       string `json:"role"`
	MatchStart int    `json:"match_start"` // Start offset of match within Preview
	MatchEnd   int    `json:"match_end"`   // End offset of match within Preview
}

// SessionResult represents all matches found in a single session.
type SessionResult struct {
	SessionID   string  `json:"session_id"`
	ProjectName string  `json:"project_name"`
	Source      string  `json:"source"`
	Path        string  `json:"path"`
	Matches     []Match `json:"matches"`
}

// RawMatch is an internal type used during parallel scanning.
type RawMatch struct {
	Candidate
	LineNum    int
	Preview    string
	Role       string
	MatchStart int // Start offset of match within Preview
	MatchEnd   int // End offset of match within Preview
}

// SearchOptions contains all options for a search operation.
type SearchOptions struct {
	Query           string
	FilterProject   string
	FilterSource    string
	Limit           int
	LimitPerSession int
	CaseSensitive   bool
	UseRegex        bool
}

// DefaultSearchOptions returns the default search options.
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		Limit:           50,
		LimitPerSession: 2,
	}
}

// Service provides search functionality against the indexed sessions.
type Service struct {
	db    *db.DB
	embDB *db.DB // separate embeddings database; nil for text-only search
}

// NewService creates a new search service.
// The embDB may be nil if only text search is needed.
func NewService(database *db.DB, embDB *db.DB) *Service {
	return &Service{db: database, embDB: embDB}
}

// FindCandidates finds candidate sessions from the database that match the filters.
func (s *Service) FindCandidates(opts SearchOptions) ([]Candidate, error) {
	sql := `
		SELECT s.path, p.source, s.id, p.name 
		FROM sessions s 
		JOIN projects p ON s.project_id = p.id
		WHERE 1=1`

	var sqlArgs []interface{}
	if opts.FilterProject != "" {
		sql += " AND p.name LIKE ?"
		sqlArgs = append(sqlArgs, "%"+opts.FilterProject+"%")
	}
	if opts.FilterSource != "" {
		sql += " AND p.source = ?"
		sqlArgs = append(sqlArgs, opts.FilterSource)
	}

	rows, err := s.db.Query(sql, sqlArgs...)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}
	defer rows.Close()

	var candidates []Candidate
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.Path, &c.Source, &c.SessionID, &c.ProjectName); err == nil {
			candidates = append(candidates, c)
		}
	}

	return candidates, nil
}

// Search performs a full search and returns the results.
func (s *Service) Search(opts SearchOptions) ([]SessionResult, int, error) {
	matcher, err := NewMatcher(opts.Query, opts.CaseSensitive, opts.UseRegex)
	if err != nil {
		return nil, 0, err
	}

	candidates, err := s.FindCandidates(opts)
	if err != nil {
		return nil, 0, err
	}

	// Parallel Scan
	rawHits := make(chan RawMatch)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 20)

	for _, c := range candidates {
		wg.Add(1)
		go func(cand Candidate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			scanFile(cand, matcher, rawHits)
		}(c)
	}

	// Closer
	go func() {
		wg.Wait()
		close(rawHits)
	}()

	// Aggregate hits by session
	sessionGroups := make(map[string]*SessionResult)
	var sessionOrder []string // Maintain some order

	totalMatches := 0
	for hit := range rawHits {
		group, exists := sessionGroups[hit.SessionID]
		if !exists {
			group = &SessionResult{
				SessionID:   hit.SessionID,
				ProjectName: hit.ProjectName,
				Source:      hit.Source,
				Path:        hit.Path,
				Matches:     []Match{},
			}
			sessionGroups[hit.SessionID] = group
			sessionOrder = append(sessionOrder, hit.SessionID)
		}

		// Apply per-session limit
		if opts.LimitPerSession > 0 && len(group.Matches) >= opts.LimitPerSession {
			continue
		}

		group.Matches = append(group.Matches, Match{
			LineNum:    hit.LineNum,
			Preview:    hit.Preview,
			Role:       hit.Role,
			MatchStart: hit.MatchStart,
			MatchEnd:   hit.MatchEnd,
		})
		totalMatches++

		// Apply global limit (rough, might go over slightly due to grouping)
		if opts.Limit > 0 && totalMatches >= opts.Limit {
			break
		}
	}

	// Build final results
	finalResults := []SessionResult{}
	for _, id := range sessionOrder {
		res := sessionGroups[id]
		if len(res.Matches) > 0 {
			finalResults = append(finalResults, *res)
		}
	}

	return finalResults, totalMatches, nil
}

func scanFile(c Candidate, m *Matcher, out chan<- RawMatch) {
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

		if m.Match(text) {
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

			preview, matchStart, matchEnd := extractPreview(text, m)
			out <- RawMatch{
				Candidate:  c,
				LineNum:    lineNum,
				Preview:    preview,
				Role:       role,
				MatchStart: matchStart,
				MatchEnd:   matchEnd,
			}
		}
	}
}

// extractPreview extracts a window of text around the match and returns the preview
// along with the start and end positions of the match within the preview.
func extractPreview(line string, m *Matcher) (preview string, matchStart, matchEnd int) {
	start, end := m.FindIndex(line)
	if start == -1 {
		return "", 0, 0
	}

	const window = 100

	pStart := start - window
	if pStart < 0 {
		pStart = 0
	}
	pEnd := end + window
	if pEnd > len(line) {
		pEnd = len(line)
	}

	preview = line[pStart:pEnd]
	matchStart = start - pStart
	matchEnd = matchStart + (end - start)

	if pStart > 0 {
		preview = "..." + preview
		matchStart += 3
		matchEnd += 3
	}
	if pEnd < len(line) {
		preview = preview + "..."
	}

	return preview, matchStart, matchEnd
}

// ShortenPath replaces the home directory with ~ in a path.
func ShortenPath(path string) string {
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}
