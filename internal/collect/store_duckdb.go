package collect

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"

	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

const collectorSchema = `
CREATE TABLE IF NOT EXISTS collected_sessions (
    id VARCHAR PRIMARY KEY,
    project_path VARCHAR,
    source VARCHAR,
    instance_id VARCHAR,
    model VARCHAR,
    entry_count INTEGER DEFAULT 0,
    first_seen TIMESTAMP,
    last_updated TIMESTAMP
);

CREATE TABLE IF NOT EXISTS collected_entries (
    uuid VARCHAR PRIMARY KEY,
    session_id VARCHAR,
    role VARCHAR,
    timestamp TIMESTAMP,
    model VARCHAR,
    text VARCHAR,
    tool_name VARCHAR,
    is_error BOOLEAN DEFAULT FALSE,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    thinking_len INTEGER DEFAULT 0,
    ingested_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS collected_agents (
    instance_id VARCHAR PRIMARY KEY,
    platform VARCHAR,
    region VARCHAR,
    hostname VARCHAR,
    version VARCHAR,
    started_at TIMESTAMP,
    last_heartbeat TIMESTAMP,
    trace_count BIGINT DEFAULT 0,
    project VARCHAR,
    metadata VARCHAR
);
`

// DuckDBStore implements TraceStore backed by DuckDB.
type DuckDBStore struct {
	db            *sql.DB
	path          string
	startedAt     time.Time
	batchSize     int
	flushInterval time.Duration

	// Single-writer pattern: incoming requests go to a channel,
	// a single goroutine drains and writes them.
	ingestCh chan IngestRequest
	wg       sync.WaitGroup
	done     chan struct{}
}

// NewDuckDBStore opens (or creates) a DuckDB database for the collector
// and starts the background batch writer.
func NewDuckDBStore(dbPath string, batchSize int, flushInterval time.Duration) (*DuckDBStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}

	if _, err := db.Exec(collectorSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize collector schema: %w", err)
	}

	// Security hardening
	if _, err := db.Exec("SET enable_external_access=false"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set security settings: %w", err)
	}

	s := &DuckDBStore{
		db:            db,
		path:          dbPath,
		startedAt:     time.Now(),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		ingestCh:      make(chan IngestRequest, batchSize*2),
		done:          make(chan struct{}),
	}

	s.wg.Add(1)
	go s.batchWriter()

	return s, nil
}

// IngestBatch queues an ingest request for the batch writer.
func (s *DuckDBStore) IngestBatch(ctx context.Context, req IngestRequest) error {
	select {
	case s.ingestCh <- req:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// batchWriter is the single goroutine that drains the ingest channel
// and writes batches to DuckDB.
func (s *DuckDBStore) batchWriter() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	var batch []IngestRequest

	for {
		select {
		case req := <-s.ingestCh:
			batch = append(batch, req)
			if s.batchEntryCount(batch) >= s.batchSize {
				s.flushBatch(batch)
				batch = nil
			}

		case <-ticker.C:
			if len(batch) > 0 {
				s.flushBatch(batch)
				batch = nil
			}

		case <-s.done:
			// Drain remaining
			for {
				select {
				case req := <-s.ingestCh:
					batch = append(batch, req)
				default:
					if len(batch) > 0 {
						s.flushBatch(batch)
					}
					return
				}
			}
		}
	}
}

// batchEntryCount returns the total number of entries across all requests.
func (s *DuckDBStore) batchEntryCount(batch []IngestRequest) int {
	n := 0
	for _, req := range batch {
		n += len(req.Entries)
	}
	return n
}

// flushBatch writes a batch of ingest requests to DuckDB in a single transaction.
func (s *DuckDBStore) flushBatch(batch []IngestRequest) {
	if len(batch) == 0 {
		return
	}

	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		tuilog.Log.Error("Failed to begin batch transaction", "error", err)
		return
	}

	for _, req := range batch {
		if err := s.writeRequest(tx, req); err != nil {
			tuilog.Log.Error("Failed to write request in batch",
				"session_id", req.SessionID, "error", err)
			tx.Rollback()
			return
		}
	}

	if err := tx.Commit(); err != nil {
		tuilog.Log.Error("Failed to commit batch", "error", err)
		return
	}

	total := s.batchEntryCount(batch)
	tuilog.Log.Debug("Flushed batch",
		"requests", len(batch), "entries", total)
}

// writeRequest writes a single ingest request within a transaction.
func (s *DuckDBStore) writeRequest(tx *sql.Tx, req IngestRequest) error {
	now := time.Now()

	// Determine the model from the first entry that has one
	model := ""
	for _, e := range req.Entries {
		if e.Model != "" {
			model = e.Model
			break
		}
	}

	// Upsert session
	_, err := tx.Exec(`
		INSERT INTO collected_sessions (id, project_path, source, instance_id, model, entry_count, first_seen, last_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			entry_count = collected_sessions.entry_count + EXCLUDED.entry_count,
			last_updated = EXCLUDED.last_updated,
			model = CASE WHEN EXCLUDED.model != '' THEN EXCLUDED.model ELSE collected_sessions.model END
	`, req.SessionID, req.ProjectPath, req.Source, req.InstanceID, model, len(req.Entries), now, now)
	if err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}

	// Insert entries
	for _, e := range req.Entries {
		_, err := tx.Exec(`
			INSERT OR IGNORE INTO collected_entries
				(uuid, session_id, role, timestamp, model, text, tool_name, is_error, input_tokens, output_tokens, thinking_len, ingested_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, e.UUID, req.SessionID, e.Role, e.Timestamp, e.Model, e.Text, e.ToolName,
			e.IsError, e.InputTokens, e.OutputTokens, e.ThinkingLen, now)
		if err != nil {
			return fmt.Errorf("insert entry %s: %w", e.UUID, err)
		}
	}

	return nil
}

// QuerySessions returns session summaries matching the given filter.
func (s *DuckDBStore) QuerySessions(ctx context.Context, filter SessionFilter) ([]SessionSummary, error) {
	query := "SELECT id, project_path, source, instance_id, model, entry_count, first_seen, last_updated FROM collected_sessions"
	var conditions []string
	var args []any

	if filter.Source != "" {
		conditions = append(conditions, "source = ?")
		args = append(args, filter.Source)
	}
	if filter.ProjectPath != "" {
		conditions = append(conditions, "project_path = ?")
		args = append(args, filter.ProjectPath)
	}
	if filter.InstanceID != "" {
		conditions = append(conditions, "instance_id = ?")
		args = append(args, filter.InstanceID)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY last_updated DESC"

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []SessionSummary
	for rows.Next() {
		var ss SessionSummary
		if err := rows.Scan(&ss.ID, &ss.ProjectPath, &ss.Source, &ss.InstanceID,
			&ss.Model, &ss.EntryCount, &ss.FirstSeen, &ss.LastUpdated); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, ss)
	}
	return sessions, rows.Err()
}

// QueryEntries returns trace entries for a session with pagination.
func (s *DuckDBStore) QueryEntries(ctx context.Context, sessionID string, limit, offset int) ([]IngestEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT uuid, role, timestamp, model, text, tool_name, is_error, input_tokens, output_tokens, thinking_len
		FROM collected_entries
		WHERE session_id = ?
		ORDER BY timestamp ASC
		LIMIT ? OFFSET ?
	`, sessionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query entries: %w", err)
	}
	defer rows.Close()

	var entries []IngestEntry
	for rows.Next() {
		var e IngestEntry
		if err := rows.Scan(&e.UUID, &e.Role, &e.Timestamp, &e.Model, &e.Text,
			&e.ToolName, &e.IsError, &e.InputTokens, &e.OutputTokens, &e.ThinkingLen); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// SearchTraces searches collected traces by text query, matching against
// entry text, tool names, and session project paths.
func (s *DuckDBStore) SearchTraces(ctx context.Context, query string, limit int) ([]SessionSummary, error) {
	if limit <= 0 {
		limit = 50
	}

	pattern := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT cs.id, cs.project_path, cs.source, cs.instance_id, cs.model,
			cs.entry_count, cs.first_seen, cs.last_updated
		FROM collected_sessions cs
		JOIN collected_entries ce ON ce.session_id = cs.id
		WHERE ce.text LIKE ? OR ce.tool_name LIKE ? OR cs.project_path LIKE ?
		ORDER BY cs.last_updated DESC
		LIMIT ?
	`, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search traces: %w", err)
	}
	defer rows.Close()

	var sessions []SessionSummary
	for rows.Next() {
		var ss SessionSummary
		if err := rows.Scan(&ss.ID, &ss.ProjectPath, &ss.Source, &ss.InstanceID,
			&ss.Model, &ss.EntryCount, &ss.FirstSeen, &ss.LastUpdated); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		sessions = append(sessions, ss)
	}
	return sessions, rows.Err()
}

// GetUsageStats returns aggregate collector usage statistics.
func (s *DuckDBStore) GetUsageStats(ctx context.Context) (*CollectorStats, error) {
	stats := &CollectorStats{
		StartedAt:     s.startedAt,
		UptimeSeconds: time.Since(s.startedAt).Seconds(),
	}

	row := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM collected_entries")
	if err := row.Scan(&stats.TotalTraces); err != nil {
		return nil, fmt.Errorf("count entries: %w", err)
	}

	row = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM collected_sessions")
	if err := row.Scan(&stats.TotalSessions); err != nil {
		return nil, fmt.Errorf("count sessions: %w", err)
	}

	// Get DB file size
	if info, err := os.Stat(s.path); err == nil {
		stats.DBSizeBytes = info.Size()
	}

	return stats, nil
}

// Close stops the batch writer and closes the database.
func (s *DuckDBStore) Close() error {
	close(s.done)
	s.wg.Wait()
	return s.db.Close()
}
