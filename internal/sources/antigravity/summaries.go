package antigravity

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // driver for conversation_summaries.db
)

// summaryRow is the part of a conversation_summaries row thinkt uses.
type summaryRow struct {
	ID, Title, Preview, ParentID, AgentName string
	Workspaces                              []string // folder paths, first = where agy started
	Modified                                time.Time
	StepCount                               int
}

// summaryTimeLayout is how last_modified_time is stored
// ("2026-06-10 14:02:22.02715+00:00").
const summaryTimeLayout = "2006-01-02 15:04:05.999999999-07:00"

// parseSummaryTime parses a stored time; the year-1 placeholder and
// unparseable values give the zero time.
func parseSummaryTime(s string) time.Time {
	t, err := time.Parse(summaryTimeLayout, strings.TrimSpace(s))
	if err != nil || t.Year() <= 1 {
		return time.Time{}
	}
	return t
}

// workspacePaths turns the workspace_uris JSON list of file:// URIs into
// folder paths.
func workspacePaths(raw string) []string {
	var uris []string
	if json.Unmarshal([]byte(raw), &uris) != nil {
		return nil
	}
	var out []string
	for _, u := range uris {
		parsed, err := url.Parse(u)
		if err != nil || parsed.Scheme != "file" || parsed.Path == "" {
			continue
		}
		out = append(out, filepath.Clean(parsed.Path))
	}
	return out
}

// loadSummaries reads conversation_summaries.db into conversation id → row.
// The db is opened immutable: it is journal_mode=delete and Antigravity
// writes it, so a read-only shared lock would stall its writer. An open or
// query error returns the rows read so far and the error; a row that fails
// to scan is skipped.
func loadSummaries(baseDir string) (map[string]summaryRow, error) {
	out := map[string]summaryRow{}
	path := filepath.Join(baseDir, "conversation_summaries.db")
	if _, err := os.Stat(path); err != nil {
		return out, err
	}
	dsn := (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro&immutable=1"}).String()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return out, err
	}
	defer func() { _ = db.Close() }()
	// CAST keeps the driver from converting the datetime column itself.
	rows, err := db.Query(`SELECT conversation_id, title, preview, step_count,
		CAST(last_modified_time AS TEXT), workspace_uris, agent_name, parent_conversation_id
		FROM conversation_summaries`)
	if err != nil {
		return out, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var r summaryRow
		var modified, workspaces sql.NullString
		if err := rows.Scan(&r.ID, &r.Title, &r.Preview, &r.StepCount, &modified, &workspaces, &r.AgentName, &r.ParentID); err != nil || r.ID == "" {
			continue
		}
		r.Modified = parseSummaryTime(modified.String)
		r.Workspaces = workspacePaths(workspaces.String)
		out[r.ID] = r
	}
	return out, rows.Err()
}

// loadLastConversations reads cache/last_conversations.json (workspace path
// → its last conversation id) into conversation id → workspace path. A
// missing or malformed file yields an empty map.
func loadLastConversations(baseDir string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(filepath.Join(baseDir, "cache", "last_conversations.json"))
	if err != nil {
		return out
	}
	var byWorkspace map[string]string
	if json.Unmarshal(data, &byWorkspace) != nil {
		return out
	}
	for ws, id := range byWorkspace {
		if id != "" && ws != "" {
			out[id] = ws
		}
	}
	return out
}
