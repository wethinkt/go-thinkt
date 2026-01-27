// Package analytics provides DuckDB-powered session analysis.
package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
)

// Engine provides analytical queries over session data.
type Engine struct {
	db      *sql.DB
	baseDir string
}

// NewEngine creates a new analytics engine.
// Uses an in-memory DuckDB instance that only reads external JSONL files
// via read_json_auto(). No data is written - all queries are SELECT-only.
func NewEngine(baseDir string) (*Engine, error) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}
	return &Engine{db: db, baseDir: baseDir}, nil
}

// Close closes the database connection.
func (e *Engine) Close() error {
	return e.db.Close()
}

// projectsPattern returns the glob pattern for all session files.
func (e *Engine) projectsPattern() string {
	return filepath.Join(e.baseDir, "projects", "*", "*.jsonl")
}

// projectPattern returns the glob pattern for a specific project.
func (e *Engine) projectPattern(projectPath string) string {
	// Convert project path to encoded directory name
	encoded := encodePathToDirName(projectPath)
	return filepath.Join(e.baseDir, "projects", encoded, "*.jsonl")
}

func encodePathToDirName(path string) string {
	if path == "" {
		return "-"
	}
	encoded := strings.ReplaceAll(path, "/", "-")
	if !strings.HasPrefix(encoded, "-") {
		encoded = "-" + encoded
	}
	return encoded
}

// SearchResult represents a search match.
type SearchResult struct {
	SessionPath string
	EntryType   string
	Content     string
	Timestamp   time.Time
}

// Search performs full-text search across sessions.
func (e *Engine) Search(ctx context.Context, query string, projectPath string, limit int) ([]SearchResult, error) {
	pattern := e.projectsPattern()
	if projectPath != "" {
		pattern = e.projectPattern(projectPath)
	}

	if limit <= 0 {
		limit = 50
	}

	// Query to search in user messages and assistant text blocks
	// JSONL structure: each line has type, message (object or string), timestamp, etc.
	sqlQuery := `
		SELECT
			filename,
			type as entry_type,
			COALESCE(
				json_extract_string(message, '$.content'),
				CAST(message AS VARCHAR)
			) as content,
			timestamp as ts
		FROM read_json_auto($1, ignore_errors=true, format='newline_delimited')
		WHERE type IN ('user', 'assistant')
		  AND (
			json_extract_string(message, '$.content') ILIKE '%' || $2 || '%'
			OR CAST(message AS VARCHAR) ILIKE '%' || $2 || '%'
		  )
		ORDER BY ts DESC
		LIMIT $3
	`

	rows, err := e.db.QueryContext(ctx, sqlQuery, pattern, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var ts sql.NullString
		if err := rows.Scan(&r.SessionPath, &r.EntryType, &r.Content, &ts); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		if ts.Valid {
			// Parse ISO timestamp
			if t, err := time.Parse(time.RFC3339, ts.String); err == nil {
				r.Timestamp = t
			}
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// TokenStats represents token usage statistics.
type TokenStats struct {
	SessionID    string
	SessionPath  string
	InputTokens  int64
	OutputTokens int64
	CacheTokens  int64
	TotalTokens  int64
}

// GetTokenStats returns token usage by session.
func (e *Engine) GetTokenStats(ctx context.Context, projectPath string, limit int) ([]TokenStats, error) {
	pattern := e.projectsPattern()
	if projectPath != "" {
		pattern = e.projectPattern(projectPath)
	}

	if limit <= 0 {
		limit = 20
	}

	// Token usage is in message.usage for assistant entries
	sqlQuery := `
		SELECT
			regexp_extract(filename, '([^/]+)\.jsonl$', 1) as session_id,
			filename as session_path,
			COALESCE(SUM(CAST(json_extract(message, '$.usage.input_tokens') AS BIGINT)), 0) as input_tokens,
			COALESCE(SUM(CAST(json_extract(message, '$.usage.output_tokens') AS BIGINT)), 0) as output_tokens,
			COALESCE(SUM(CAST(json_extract(message, '$.usage.cache_read_input_tokens') AS BIGINT)), 0) as cache_tokens
		FROM read_json_auto($1, ignore_errors=true, format='newline_delimited')
		WHERE type = 'assistant'
		  AND json_extract(message, '$.usage') IS NOT NULL
		GROUP BY session_id, session_path
		ORDER BY input_tokens + output_tokens DESC
		LIMIT $2
	`

	rows, err := e.db.QueryContext(ctx, sqlQuery, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("token stats query: %w", err)
	}
	defer rows.Close()

	var results []TokenStats
	for rows.Next() {
		var s TokenStats
		if err := rows.Scan(&s.SessionID, &s.SessionPath, &s.InputTokens, &s.OutputTokens, &s.CacheTokens); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		s.TotalTokens = s.InputTokens + s.OutputTokens
		results = append(results, s)
	}

	return results, rows.Err()
}

// ToolUsage represents tool usage frequency.
type ToolUsage struct {
	ToolName   string
	UsageCount int64
}

// GetToolStats returns tool usage frequency.
func (e *Engine) GetToolStats(ctx context.Context, projectPath string, limit int) ([]ToolUsage, error) {
	pattern := e.projectsPattern()
	if projectPath != "" {
		pattern = e.projectPattern(projectPath)
	}

	if limit <= 0 {
		limit = 20
	}

	// Tool usage is in message.content[] array for assistant entries
	// Only process when content is an array (not a string)
	sqlQuery := `
		WITH tool_calls AS (
			SELECT
				json_extract_string(content_item, '$.name') as tool_name
			FROM read_json_auto($1, ignore_errors=true, format='newline_delimited'),
				 LATERAL unnest(
					CASE
						WHEN json_type(json_extract(message, '$.content')) = 'ARRAY'
						THEN CAST(json_extract(message, '$.content') AS JSON[])
						ELSE ARRAY[]::JSON[]
					END
				 ) AS t(content_item)
			WHERE type = 'assistant'
			  AND json_extract_string(content_item, '$.type') = 'tool_use'
		)
		SELECT tool_name, COUNT(*) as usage_count
		FROM tool_calls
		WHERE tool_name IS NOT NULL
		GROUP BY tool_name
		ORDER BY usage_count DESC
		LIMIT $2
	`

	rows, err := e.db.QueryContext(ctx, sqlQuery, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("tool stats query: %w", err)
	}
	defer rows.Close()

	var results []ToolUsage
	for rows.Next() {
		var t ToolUsage
		if err := rows.Scan(&t.ToolName, &t.UsageCount); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, t)
	}

	return results, rows.Err()
}

// WordFrequency represents word frequency data.
type WordFrequency struct {
	Word  string
	Count int64
}

// Common English stopwords to exclude
var stopwords = map[string]bool{
	"the": true, "and": true, "that": true, "this": true, "with": true,
	"from": true, "have": true, "will": true, "been": true, "were": true,
	"they": true, "their": true, "what": true, "about": true, "would": true,
	"there": true, "could": true, "other": true, "into": true, "more": true,
	"some": true, "very": true, "just": true, "also": true, "only": true,
	"over": true, "such": true, "than": true, "then": true, "these": true,
	"most": true, "your": true, "when": true, "which": true, "like": true,
	"make": true, "want": true, "does": true, "need": true, "should": true,
	"file": true, "code": true, "please": true, "using": true, "here": true,
	"can": true, "for": true, "are": true, "but": true, "not": true,
	"you": true, "all": true, "was": true, "her": true, "she": true,
	"his": true, "had": true, "how": true, "its": true, "may": true,
	"now": true, "our": true, "out": true, "own": true, "see": true,
	"way": true, "who": true, "get": true, "has": true, "him": true,
}

// GetWordFrequency returns word frequency from user prompts.
func (e *Engine) GetWordFrequency(ctx context.Context, projectPath string, limit int) ([]WordFrequency, error) {
	pattern := e.projectsPattern()
	if projectPath != "" {
		pattern = e.projectPattern(projectPath)
	}

	if limit <= 0 {
		limit = 50
	}

	// Build stopwords list for SQL
	stopwordsList := make([]string, 0, len(stopwords))
	for w := range stopwords {
		stopwordsList = append(stopwordsList, "'"+w+"'")
	}
	stopwordsSQL := strings.Join(stopwordsList, ",")

	// User messages have type='user' and message.content
	sqlQuery := fmt.Sprintf(`
		WITH user_content AS (
			SELECT
				COALESCE(
					json_extract_string(message, '$.content'),
					CAST(message AS VARCHAR)
				) as content
			FROM read_json_auto($1, ignore_errors=true, format='newline_delimited')
			WHERE type = 'user'
		),
		words AS (
			SELECT unnest(string_split(
				lower(regexp_replace(content, '[^a-zA-Z ]', ' ', 'g')),
				' '
			)) as word
			FROM user_content
			WHERE content IS NOT NULL
		)
		SELECT word, COUNT(*) as freq
		FROM words
		WHERE length(word) > 3
		  AND word NOT IN (%s)
		GROUP BY word
		ORDER BY freq DESC
		LIMIT $2
	`, stopwordsSQL)

	rows, err := e.db.QueryContext(ctx, sqlQuery, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("word frequency query: %w", err)
	}
	defer rows.Close()

	var results []WordFrequency
	for rows.Next() {
		var w WordFrequency
		if err := rows.Scan(&w.Word, &w.Count); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, w)
	}

	return results, rows.Err()
}

// ActivityDay represents daily activity.
type ActivityDay struct {
	Date     time.Time
	Sessions int64
	Messages int64
}

// GetActivity returns daily activity statistics.
func (e *Engine) GetActivity(ctx context.Context, projectPath string, days int) ([]ActivityDay, error) {
	pattern := e.projectsPattern()
	if projectPath != "" {
		pattern = e.projectPattern(projectPath)
	}

	if days <= 0 {
		days = 30
	}

	// Timestamp is an ISO string, parse it
	sqlQuery := `
		SELECT
			DATE_TRUNC('day', CAST(timestamp AS TIMESTAMP)) as day,
			COUNT(DISTINCT regexp_extract(filename, '([^/]+)\.jsonl$', 1)) as sessions,
			COUNT(*) as messages
		FROM read_json_auto($1, ignore_errors=true, format='newline_delimited')
		WHERE timestamp IS NOT NULL
		  AND type IN ('user', 'assistant')
		GROUP BY day
		ORDER BY day DESC
		LIMIT $2
	`

	rows, err := e.db.QueryContext(ctx, sqlQuery, pattern, days)
	if err != nil {
		return nil, fmt.Errorf("activity query: %w", err)
	}
	defer rows.Close()

	var results []ActivityDay
	for rows.Next() {
		var a ActivityDay
		if err := rows.Scan(&a.Date, &a.Sessions, &a.Messages); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, a)
	}

	return results, rows.Err()
}

// ModelUsage represents model usage statistics.
type ModelUsage struct {
	Model           string
	Responses       int64
	AvgOutputTokens float64
}

// GetModelStats returns model usage statistics.
func (e *Engine) GetModelStats(ctx context.Context, projectPath string) ([]ModelUsage, error) {
	pattern := e.projectsPattern()
	if projectPath != "" {
		pattern = e.projectPattern(projectPath)
	}

	// Model is in message.model for assistant entries
	sqlQuery := `
		SELECT
			json_extract_string(message, '$.model') as model,
			COUNT(*) as responses,
			AVG(CAST(json_extract(message, '$.usage.output_tokens') AS DOUBLE)) as avg_output
		FROM read_json_auto($1, ignore_errors=true, format='newline_delimited')
		WHERE type = 'assistant'
		  AND json_extract_string(message, '$.model') IS NOT NULL
		GROUP BY model
		ORDER BY responses DESC
	`

	rows, err := e.db.QueryContext(ctx, sqlQuery, pattern)
	if err != nil {
		return nil, fmt.Errorf("model stats query: %w", err)
	}
	defer rows.Close()

	var results []ModelUsage
	for rows.Next() {
		var m ModelUsage
		var avgOutput sql.NullFloat64
		if err := rows.Scan(&m.Model, &m.Responses, &avgOutput); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		if avgOutput.Valid {
			m.AvgOutputTokens = avgOutput.Float64
		}
		results = append(results, m)
	}

	return results, rows.Err()
}

// ErrorInfo represents a tool error.
type ErrorInfo struct {
	ToolName    string
	SessionPath string
	Content     string
	Timestamp   time.Time
}

// GetErrors returns tool errors.
func (e *Engine) GetErrors(ctx context.Context, projectPath string, limit int) ([]ErrorInfo, error) {
	pattern := e.projectsPattern()
	if projectPath != "" {
		pattern = e.projectPattern(projectPath)
	}

	if limit <= 0 {
		limit = 20
	}

	// Tool results are in message.content[] for user entries
	// Only process when content is an array
	sqlQuery := `
		WITH tool_errors AS (
			SELECT
				json_extract_string(content_item, '$.tool_use_id') as tool_id,
				filename as session_path,
				COALESCE(json_extract_string(content_item, '$.content'), '') as content,
				timestamp as ts
			FROM read_json_auto($1, ignore_errors=true, format='newline_delimited'),
				 LATERAL unnest(
					CASE
						WHEN json_type(json_extract(message, '$.content')) = 'ARRAY'
						THEN CAST(json_extract(message, '$.content') AS JSON[])
						ELSE ARRAY[]::JSON[]
					END
				 ) AS t(content_item)
			WHERE type = 'user'
			  AND json_extract_string(content_item, '$.type') = 'tool_result'
			  AND json_extract(content_item, '$.is_error') = true
		)
		SELECT
			COALESCE(tool_id, 'unknown') as tool_name,
			session_path,
			content,
			ts
		FROM tool_errors
		ORDER BY ts DESC
		LIMIT $2
	`

	rows, err := e.db.QueryContext(ctx, sqlQuery, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("errors query: %w", err)
	}
	defer rows.Close()

	var results []ErrorInfo
	for rows.Next() {
		var ei ErrorInfo
		var ts sql.NullString
		if err := rows.Scan(&ei.ToolName, &ei.SessionPath, &ei.Content, &ts); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		if ts.Valid {
			if t, err := time.Parse(time.RFC3339, ts.String); err == nil {
				ei.Timestamp = t
			}
		}
		results = append(results, ei)
	}

	return results, rows.Err()
}

// Query executes a raw SQL query and returns results as maps.
func (e *Engine) Query(ctx context.Context, sqlQuery string) ([]map[string]interface{}, error) {
	rows, err := e.db.QueryContext(ctx, sqlQuery)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		row := make(map[string]interface{})
		for i, col := range cols {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return results, rows.Err()
}
