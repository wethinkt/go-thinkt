package collect

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tracewire"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// The archive keys a session by (machine_id, source, session_id) and an
// entry by (machine_id, source, session_id, uuid). entry_json holds the
// full thinkt.Entry; the other entry columns are derived from it for cheap
// queries. An entry a relay reports removed keeps its row with
// superseded_at set, unless the store runs in SupersededDelete mode.
const collectorSchema = `
CREATE TABLE IF NOT EXISTS sessions (
    machine_id VARCHAR NOT NULL,
    source VARCHAR NOT NULL,
    session_id VARCHAR NOT NULL,
    project_path VARCHAR,
    parent_session_id VARCHAR,
    title VARCHAR,
    model VARCHAR,
    created_at TIMESTAMP,
    modified_at TIMESTAMP,
    entry_count INTEGER DEFAULT 0,
    cost_json VARCHAR,
    instance_id VARCHAR,
    generation BIGINT DEFAULT 0,
    first_seen TIMESTAMP,
    last_seen TIMESTAMP,
    status VARCHAR DEFAULT 'unknown',
    last_activity TIMESTAMP,
    PRIMARY KEY (machine_id, source, session_id)
);

CREATE TABLE IF NOT EXISTS entries (
    machine_id VARCHAR NOT NULL,
    source VARCHAR NOT NULL,
    session_id VARCHAR NOT NULL,
    uuid VARCHAR NOT NULL,
    entry_json VARCHAR,
    role VARCHAR,
    kind VARCHAR,
    visibility VARCHAR,
    timestamp TIMESTAMP,
    model VARCHAR,
    text VARCHAR,
    tool_names VARCHAR,
    is_error BOOLEAN DEFAULT FALSE,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    cache_read_tokens INTEGER DEFAULT 0,
    thinking_tokens INTEGER DEFAULT 0,
    turn_id VARCHAR,
    prompt_id VARCHAR,
    content_hash VARCHAR,
    ingested_at TIMESTAMP,
    updated_at TIMESTAMP,
    superseded_at TIMESTAMP,
    PRIMARY KEY (machine_id, source, session_id, uuid)
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

CREATE TABLE IF NOT EXISTS session_activity (
    session_id VARCHAR NOT NULL,
    instance_id VARCHAR,
    source VARCHAR,
    project_path VARCHAR,
    event VARCHAR NOT NULL,
    timestamp TIMESTAMP NOT NULL
);
`

// DuckDBStore implements TraceStore backed by DuckDB.
type DuckDBStore struct {
	db            *sql.DB
	path          string
	startedAt     time.Time
	batchSize     int
	flushInterval time.Duration
	superseded    SupersededMode

	// Single-writer pattern: incoming payloads go to a channel, a single
	// goroutine drains and writes them.
	ingestCh chan tracewire.Payload
	flushCh  chan chan struct{}
	wg       sync.WaitGroup
	done     chan struct{}
}

// NewDuckDBStore opens (or creates) a DuckDB database for the collector
// and starts the background batch writer. Tables of the old, pre-archive
// layout are dropped; their data is not migrated.
func NewDuckDBStore(dbPath string, batchSize int, flushInterval time.Duration, superseded SupersededMode) (*DuckDBStore, error) {
	if superseded == "" {
		superseded = SupersededKeep
	}
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, config.DirPerms); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}

	for _, stmt := range []string{
		"DROP TABLE IF EXISTS collected_entries",
		"DROP TABLE IF EXISTS collected_sessions",
		collectorSchema,
		"ALTER TABLE session_activity ADD COLUMN IF NOT EXISTS machine_id VARCHAR",
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize collector schema: %w", err)
		}
	}

	// Security: external access stays enabled (required for COPY ... TO parquet
	// export) but we restrict file system access to only the database directory.
	// ExportParquet widens this to include the output directory before writing.
	dbDir, _ := filepath.Abs(filepath.Dir(dbPath))
	if _, err := db.Exec("SET allowed_directories = [$1]", dbDir); err != nil {
		db.Close()
		return nil, fmt.Errorf("set allowed_directories: %w", err)
	}

	s := &DuckDBStore{
		db:            db,
		path:          dbPath,
		startedAt:     time.Now(),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		superseded:    superseded,
		ingestCh:      make(chan tracewire.Payload, batchSize*2),
		flushCh:       make(chan chan struct{}),
		done:          make(chan struct{}),
	}

	s.wg.Add(1)
	go s.batchWriter()

	return s, nil
}

// IngestBatch queues one relay payload for the batch writer.
func (s *DuckDBStore) IngestBatch(ctx context.Context, p tracewire.Payload) error {
	select {
	case s.ingestCh <- p:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Flush blocks until every payload queued before the call is written.
func (s *DuckDBStore) Flush() {
	done := make(chan struct{})
	select {
	case s.flushCh <- done:
		<-done
	case <-s.done:
	}
}

// batchWriter is the single goroutine that drains the ingest channel
// and writes batches to DuckDB.
func (s *DuckDBStore) batchWriter() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	var batch []tracewire.Payload
	drain := func() {
		for {
			select {
			case p := <-s.ingestCh:
				batch = append(batch, p)
			default:
				return
			}
		}
	}

	for {
		select {
		case p := <-s.ingestCh:
			batch = append(batch, p)
			if batchEntryCount(batch) >= s.batchSize {
				s.flushBatch(batch)
				batch = nil
			}

		case done := <-s.flushCh:
			drain()
			s.flushBatch(batch)
			batch = nil
			close(done)

		case <-ticker.C:
			if len(batch) > 0 {
				s.flushBatch(batch)
				batch = nil
			}

		case <-s.done:
			drain()
			s.flushBatch(batch)
			return
		}
	}
}

// batchEntryCount returns the total number of entries across payloads.
func batchEntryCount(batch []tracewire.Payload) int {
	n := 0
	for _, p := range batch {
		n += len(p.Entries) + len(p.Removed)
	}
	return n
}

// flushBatch writes payloads to DuckDB, each in its own transaction, so one
// bad payload does not lose the others.
func (s *DuckDBStore) flushBatch(batch []tracewire.Payload) {
	if len(batch) == 0 {
		return
	}
	start := time.Now()
	for _, p := range batch {
		tx, err := s.db.BeginTx(context.Background(), nil)
		if err != nil {
			tuilog.Log.Error("Failed to begin transaction", "error", err)
			return
		}
		if err := s.writePayload(tx, p); err != nil {
			tuilog.Log.Error("Failed to write payload", "session_id", p.Session.ID, "source", p.Source, "error", err)
			_ = tx.Rollback()
			continue
		}
		if err := tx.Commit(); err != nil {
			tuilog.Log.Error("Failed to commit payload", "session_id", p.Session.ID, "error", err)
		}
	}
	batchFlushDurationSeconds.Observe(time.Since(start).Seconds())
	tuilog.Log.Debug("Flushed batch", "payloads", len(batch), "entries", batchEntryCount(batch))
}

// writePayload applies one payload. A payload whose generation is lower
// than the session's stored one is a stale delivery and is skipped; an
// equal generation (another chunk of the same sync) applies.
func (s *DuckDBStore) writePayload(tx *sql.Tx, p tracewire.Payload) error {
	now := time.Now().UTC()
	key := []any{p.MachineID, p.Source, p.Session.ID}

	var stored sql.NullInt64
	err := tx.QueryRow(`SELECT generation FROM sessions WHERE machine_id = ? AND source = ? AND session_id = ?`, key...).Scan(&stored)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read generation: %w", err)
	}
	if stored.Valid && p.Session.Generation < stored.Int64 {
		tuilog.Log.Debug("Skipping stale payload", "session_id", p.Session.ID, "generation", p.Session.Generation, "stored", stored.Int64)
		return nil
	}

	var costJSON any
	if p.Session.Cost != nil {
		b, _ := json.Marshal(p.Session.Cost)
		costJSON = string(b)
	}
	ss := p.Session
	if _, err := tx.Exec(`
		INSERT INTO sessions (machine_id, source, session_id, project_path, parent_session_id, title, model,
			created_at, modified_at, entry_count, cost_json, instance_id, generation, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (machine_id, source, session_id) DO UPDATE SET
			project_path = EXCLUDED.project_path,
			parent_session_id = EXCLUDED.parent_session_id,
			title = EXCLUDED.title,
			model = EXCLUDED.model,
			created_at = EXCLUDED.created_at,
			modified_at = EXCLUDED.modified_at,
			entry_count = EXCLUDED.entry_count,
			cost_json = EXCLUDED.cost_json,
			instance_id = EXCLUDED.instance_id,
			generation = EXCLUDED.generation,
			last_seen = EXCLUDED.last_seen
	`, p.MachineID, p.Source, ss.ID, ss.ProjectPath, ss.ParentSessionID, ss.Title, ss.Model,
		nullTime(ss.CreatedAt), nullTime(ss.ModifiedAt), ss.EntryCount, costJSON, p.InstanceID,
		ss.Generation, now, now); err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}

	for _, e := range p.Entries {
		raw, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("encode entry %s: %w", e.UUID, err)
		}
		d := derive(e)
		if _, err := tx.Exec(`
			INSERT INTO entries (machine_id, source, session_id, uuid, entry_json, role, kind, visibility,
				timestamp, model, text, tool_names, is_error, input_tokens, output_tokens, cache_read_tokens,
				thinking_tokens, turn_id, prompt_id, content_hash, ingested_at, updated_at, superseded_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
			ON CONFLICT (machine_id, source, session_id, uuid) DO UPDATE SET
				entry_json = EXCLUDED.entry_json, role = EXCLUDED.role, kind = EXCLUDED.kind,
				visibility = EXCLUDED.visibility, timestamp = EXCLUDED.timestamp, model = EXCLUDED.model,
				text = EXCLUDED.text, tool_names = EXCLUDED.tool_names, is_error = EXCLUDED.is_error,
				input_tokens = EXCLUDED.input_tokens, output_tokens = EXCLUDED.output_tokens,
				cache_read_tokens = EXCLUDED.cache_read_tokens, thinking_tokens = EXCLUDED.thinking_tokens,
				turn_id = EXCLUDED.turn_id, prompt_id = EXCLUDED.prompt_id,
				content_hash = EXCLUDED.content_hash, updated_at = EXCLUDED.updated_at, superseded_at = NULL
		`, p.MachineID, p.Source, ss.ID, e.UUID, string(raw), string(e.Role), string(e.Kind), string(e.Visibility),
			nullTime(e.Timestamp), e.Model, e.Text, d.toolNames, d.isError, d.input, d.output, d.cacheRead,
			d.thinking, e.TurnID, e.PromptID, tracewire.EntryHash(e), now, now); err != nil {
			return fmt.Errorf("upsert entry %s: %w", e.UUID, err)
		}
	}

	for _, uuid := range p.Removed {
		var err error
		if s.superseded == SupersededDelete {
			_, err = tx.Exec(`DELETE FROM entries WHERE machine_id = ? AND source = ? AND session_id = ? AND uuid = ?`,
				p.MachineID, p.Source, ss.ID, uuid)
		} else {
			_, err = tx.Exec(`UPDATE entries SET superseded_at = ? WHERE machine_id = ? AND source = ? AND session_id = ? AND uuid = ? AND superseded_at IS NULL`,
				now, p.MachineID, p.Source, ss.ID, uuid)
		}
		if err != nil {
			return fmt.Errorf("remove entry %s: %w", uuid, err)
		}
	}
	return nil
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}

// derived holds the entry columns computed from the full entry.
type derived struct {
	toolNames                          string
	isError                            bool
	input, output, cacheRead, thinking int
}

func derive(e thinkt.Entry) derived {
	var d derived
	var names []string
	d.isError = e.Error != ""
	for _, b := range e.ContentBlocks {
		if b.Type == "tool_use" && b.ToolName != "" {
			names = append(names, b.ToolName)
		}
		if b.IsError {
			d.isError = true
		}
	}
	d.toolNames = strings.Join(names, ",")
	if u := e.Usage; u != nil {
		d.input, d.output, d.cacheRead, d.thinking = u.InputTokens, u.OutputTokens, u.CacheReadInputTokens, u.ThinkingTokens
	}
	return d
}

// summarize maps a full entry onto the short form the session stream and
// entry queries serve.
func summarize(e thinkt.Entry) EntrySummary {
	s := EntrySummary{
		UUID: e.UUID, Role: string(e.Role), Timestamp: e.Timestamp, Model: e.Model, Text: e.Text,
		IsError: e.Error != "", ContentBlocks: e.ContentBlocks,
	}
	if e.Usage != nil {
		s.InputTokens, s.OutputTokens = e.Usage.InputTokens, e.Usage.OutputTokens
	}
	for _, b := range e.ContentBlocks {
		switch b.Type {
		case "tool_use":
			s.HasToolUse = true
			if s.ToolName == "" {
				s.ToolName = b.ToolName
			}
		case "thinking":
			s.HasThinking = true
			s.ThinkingLen += len(b.Thinking)
		}
		if b.IsError {
			s.IsError = true
		}
	}
	return s
}

// sessionColumns selects a SessionSummary from the sessions table aliased s.
const sessionColumns = `s.session_id, s.machine_id, COALESCE(s.title, ''), COALESCE(s.project_path, ''), s.source,
	COALESCE(s.instance_id, ''), COALESCE(s.model, ''), COALESCE(s.entry_count, 0), s.first_seen, s.last_seen`

func scanSession(rows *sql.Rows, extra ...any) (SessionSummary, error) {
	var ss SessionSummary
	dest := append([]any{&ss.ID, &ss.MachineID, &ss.Title, &ss.ProjectPath, &ss.Source,
		&ss.InstanceID, &ss.Model, &ss.EntryCount, &ss.FirstSeen, &ss.LastUpdated}, extra...)
	err := rows.Scan(dest...)
	return ss, err
}

// QuerySessions returns session summaries matching the given filter.
func (s *DuckDBStore) QuerySessions(ctx context.Context, filter SessionFilter) ([]SessionSummary, error) {
	query := "SELECT " + sessionColumns + " FROM sessions s"
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
	if filter.ActiveOnly {
		conditions = append(conditions, "status = 'active'")
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY last_seen DESC"
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
		ss, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, ss)
	}
	return sessions, rows.Err()
}

// QueryEntries returns a session's live entries in time order. It is keyed
// by session id alone; entries of two machines sharing an id are merged.
func (s *DuckDBStore) QueryEntries(ctx context.Context, sessionID string, limit, offset int) ([]EntrySummary, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT entry_json FROM entries
		WHERE session_id = ? AND superseded_at IS NULL
		ORDER BY timestamp ASC NULLS FIRST, uuid ASC
		LIMIT ? OFFSET ?
	`, sessionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query entries: %w", err)
	}
	defer rows.Close()
	var out []EntrySummary
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		var e thinkt.Entry
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			return nil, fmt.Errorf("decode entry: %w", err)
		}
		out = append(out, summarize(e))
	}
	return out, rows.Err()
}

// SearchTraces searches live entries' text and tool names, and session
// project paths.
func (s *DuckDBStore) SearchTraces(ctx context.Context, query string, limit int) ([]SessionSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	pattern := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+sessionColumns+` FROM sessions s
		WHERE s.project_path LIKE ? OR EXISTS (
			SELECT 1 FROM entries e
			WHERE e.machine_id = s.machine_id AND e.source = s.source AND e.session_id = s.session_id
			  AND e.superseded_at IS NULL AND (e.text LIKE ? OR e.tool_names LIKE ?))
		ORDER BY s.last_seen DESC
		LIMIT ?
	`, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search traces: %w", err)
	}
	defer rows.Close()
	var sessions []SessionSummary
	for rows.Next() {
		ss, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		sessions = append(sessions, ss)
	}
	return sessions, rows.Err()
}

// RecordSessionActivity records a session lifecycle event and updates the
// session's status, matched on the full session key.
func (s *DuckDBStore) RecordSessionActivity(ctx context.Context, event SessionActivityEvent) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session_activity (session_id, instance_id, source, project_path, event, timestamp, machine_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, event.SessionID, event.InstanceID, event.Source, event.ProjectPath, event.Event, event.Timestamp, event.MachineID)
	if err != nil {
		return fmt.Errorf("record session activity: %w", err)
	}

	status := "unknown"
	switch event.Event {
	case "session_start", "session_active":
		status = "active"
	case "session_end":
		status = "ended"
	}
	_, _ = s.db.ExecContext(ctx, `
		UPDATE sessions SET status = ?, last_activity = ?
		WHERE machine_id = ? AND source = ? AND session_id = ?
	`, status, event.Timestamp, event.MachineID, event.Source, event.SessionID)
	return nil
}

// QueryActiveSessions returns sessions whose latest activity event, per
// session key, is a start or an active report.
func (s *DuckDBStore) QueryActiveSessions(ctx context.Context) ([]SessionSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH latest_events AS (
			SELECT machine_id, source, session_id, event, timestamp,
				ROW_NUMBER() OVER (PARTITION BY machine_id, source, session_id ORDER BY timestamp DESC) AS rn
			FROM session_activity
		)
		SELECT `+sessionColumns+`, COALESCE(s.status, 'unknown'), COALESCE(s.last_activity, s.last_seen)
		FROM sessions s
		JOIN latest_events le ON le.machine_id = s.machine_id AND le.source = s.source
			AND le.session_id = s.session_id AND le.rn = 1
		WHERE le.event IN ('session_start', 'session_active')
		ORDER BY le.timestamp DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query active sessions: %w", err)
	}
	defer rows.Close()
	var sessions []SessionSummary
	for rows.Next() {
		var status string
		var last time.Time
		ss, err := scanSession(rows, &status, &last)
		if err != nil {
			return nil, fmt.Errorf("scan active session: %w", err)
		}
		ss.Status, ss.LastActivity = status, last
		sessions = append(sessions, ss)
	}
	return sessions, rows.Err()
}

// GetUsageStats returns aggregate collector usage statistics; TotalTraces
// counts live entries.
func (s *DuckDBStore) GetUsageStats(ctx context.Context) (*CollectorStats, error) {
	stats := &CollectorStats{
		StartedAt:     s.startedAt,
		UptimeSeconds: time.Since(s.startedAt).Seconds(),
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM entries WHERE superseded_at IS NULL").Scan(&stats.TotalTraces); err != nil {
		return nil, fmt.Errorf("count entries: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&stats.TotalSessions); err != nil {
		return nil, fmt.Errorf("count sessions: %w", err)
	}
	if info, err := os.Stat(s.path); err == nil {
		stats.DBSizeBytes = info.Size()
	}
	return stats, nil
}

// ExportParquet exports archived entries (entry_json included) to
// entries.parquet using DuckDB's COPY ... TO ... (FORMAT PARQUET).
func (s *DuckDBStore) ExportParquet(ctx context.Context, outDir string, opts ExportOptions) error {
	if err := os.MkdirAll(outDir, config.DirPerms); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Widen allowed_directories to include the output dir for this export,
	// then restore to just the database directory.
	absOut, _ := filepath.Abs(outDir)
	dbDir, _ := filepath.Abs(filepath.Dir(s.path))
	if _, err := s.db.ExecContext(ctx, "SET allowed_directories = [$1, $2]", dbDir, absOut); err != nil {
		return fmt.Errorf("widen allowed_directories: %w", err)
	}
	defer s.db.ExecContext(ctx, "SET allowed_directories = [$1]", dbDir) //nolint:errcheck

	var conditions []string
	var args []any
	if opts.Since != nil {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, *opts.Since)
	}
	if opts.Until != nil {
		conditions = append(conditions, "timestamp < ?")
		args = append(args, *opts.Until)
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	query := fmt.Sprintf(
		`COPY (SELECT * FROM entries%s) TO '%s' (FORMAT PARQUET)`,
		where, filepath.Join(outDir, "entries.parquet"),
	)
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("export parquet: %w", err)
	}
	return nil
}

// Close stops the batch writer and closes the database.
func (s *DuckDBStore) Close() error {
	close(s.done)
	s.wg.Wait()
	return s.db.Close()
}
