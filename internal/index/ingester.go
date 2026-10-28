package index

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wethinkt/go-thinkt/internal/index/db"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Ingester reads session files and writes metadata to the SQLite index.
// No embedding or summarization — metadata only.
type Ingester struct {
	db       *db.DB
	registry *thinkt.StoreRegistry

	// OnProgress is called with indexing progress updates.
	OnProgress func(pIdx, pTotal, sIdx, sTotal int, message string)

	// Verbose enables additional warning output.
	Verbose bool
}

// NewIngester creates a new Ingester.
func NewIngester(database *db.DB, registry *thinkt.StoreRegistry) *Ingester {
	return &Ingester{db: database, registry: registry}
}

func (i *Ingester) reportProgress(pIdx, pTotal, sIdx, sTotal int, message string) {
	if i.OnProgress != nil {
		i.OnProgress(pIdx, pTotal, sIdx, sTotal, message)
	}
}

// parsedSession holds the result of reading and parsing a session file.
type parsedSession struct {
	meta      thinkt.SessionMeta
	projectID string
	entries   []thinkt.Entry
	err       error
}

// SyncProject upserts a project row in the index.
func (i *Ingester) SyncProject(ctx context.Context, p thinkt.Project) error {
	projectID := db.ScopedProjectID(p.Source, p.ID)
	_, err := i.db.ExecContext(ctx, `
		INSERT INTO projects (id, path, name, source, workspace_id)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			path = excluded.path,
			name = excluded.name,
			source = excluded.source,
			workspace_id = excluded.workspace_id`,
		projectID, p.Path, p.Name, string(p.Source), p.WorkspaceID)
	return err
}

// IngestProject indexes all sessions within a project.
// File reads are parallelized; DB writes are serialized.
func (i *Ingester) IngestProject(ctx context.Context, project thinkt.Project, pIdx, pTotal int) error {
	if err := i.SyncProject(ctx, project); err != nil {
		return fmt.Errorf("sync project %s: %w", project.ID, err)
	}

	store, ok := i.registry.Get(project.Source)
	if !ok {
		return fmt.Errorf("no store for source %s", project.Source)
	}

	sessions, err := store.ListSessions(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("list sessions for %s: %w", project.ID, err)
	}

	projectID := db.ScopedProjectID(project.Source, project.ID)

	// Rows for files the store no longer lists (deleted rollouts, or
	// sessions a parser change now classifies as subagents) would otherwise
	// stay in the index forever at their old parser version.
	if err := i.pruneUnlisted(ctx, projectID, sessions); err != nil {
		fmt.Fprintf(os.Stderr, "\nError pruning stale sessions for %s: %v\n", project.Name, err)
	}

	if len(sessions) == 0 {
		i.reportProgress(pIdx, pTotal, 0, 0, fmt.Sprintf("Project %s (no sessions)", project.Name))
		return nil
	}

	// Filter to sessions needing sync.
	var toSync []thinkt.SessionMeta
	for _, s := range sessions {
		shouldSync, err := i.ShouldSyncSession(s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError checking session %s: %v\n", s.ID, err)
			continue
		}
		if shouldSync {
			toSync = append(toSync, s)
		}
	}

	if len(toSync) == 0 {
		i.reportProgress(pIdx, pTotal, len(sessions), len(sessions),
			fmt.Sprintf("Project %s (up to date)", project.Name))
		return nil
	}

	// Phase 1: Parse in parallel.
	workers := min(runtime.NumCPU(), len(toSync))
	if workers < 1 {
		workers = 1
	}

	parsed := make([]parsedSession, len(toSync))
	var wg sync.WaitGroup
	work := make(chan int, len(toSync))

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range work {
				if ctx.Err() != nil {
					parsed[idx] = parsedSession{err: ctx.Err()}
					continue
				}
				parsed[idx] = i.parseSession(ctx, store, toSync[idx], projectID)
			}
		}()
	}
	for idx := range toSync {
		work <- idx
	}
	close(work)
	wg.Wait()

	// Phase 2: Write to DB serially.
	written := 0
	for _, p := range parsed {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if p.err != nil {
			fmt.Fprintf(os.Stderr, "\nError reading session %s: %v\n", p.meta.ID, p.err)
			continue
		}

		written++
		i.reportProgress(pIdx, pTotal, written, len(toSync),
			fmt.Sprintf("Indexing %s: %s", project.Name, p.meta.ID))

		if err := i.writeSession(ctx, p); err != nil {
			fmt.Fprintf(os.Stderr, "\nError ingesting session %s: %v\n", p.meta.ID, err)
		}
	}

	return nil
}

// IngestSession indexes a single session if it has changed since last sync.
func (i *Ingester) IngestSession(ctx context.Context, projectID string, meta thinkt.SessionMeta) error {
	shouldSync, err := i.ShouldSyncSession(meta)
	if err != nil {
		return err
	}
	if !shouldSync {
		return nil
	}

	store, ok := i.registry.Get(meta.Source)
	if !ok {
		return fmt.Errorf("no store for source %s", meta.Source)
	}

	p := i.parseSession(ctx, store, meta, projectID)
	if p.err != nil {
		return p.err
	}

	return i.writeSession(ctx, p)
}

func (i *Ingester) parseSession(ctx context.Context, store thinkt.Store, meta thinkt.SessionMeta, projectID string) parsedSession {
	reader, err := store.OpenSession(ctx, meta.ID)
	if err != nil {
		return parsedSession{meta: meta, projectID: projectID, err: err}
	}
	defer reader.Close()

	var entries []thinkt.Entry
	for {
		entry, err := reader.ReadNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return parsedSession{meta: meta, projectID: projectID, err: fmt.Errorf("read entry: %w", err)}
		}
		if entry.UUID == "" {
			continue
		}
		entries = append(entries, *entry)
	}

	return parsedSession{meta: meta, projectID: projectID, entries: entries}
}

// writeSession writes a parsed session to the database in a single transaction.
func (i *Ingester) writeSession(ctx context.Context, p parsedSession) error {
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint: errcheck

	// Delete existing entries for this session before re-ingesting.
	if _, err := tx.ExecContext(ctx, "DELETE FROM entries WHERE session_id = ?", p.meta.ID); err != nil {
		return fmt.Errorf("clear old entries: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO entries (
			session_id, uuid, timestamp, role,
			input_tokens, output_tokens, tool_name, is_error, word_count, thinking_len,
			line_number
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (session_id, uuid) DO UPDATE SET
			timestamp = excluded.timestamp,
			role = excluded.role,
			input_tokens = excluded.input_tokens,
			output_tokens = excluded.output_tokens,
			tool_name = excluded.tool_name,
			is_error = excluded.is_error,
			word_count = excluded.word_count,
			thinking_len = excluded.thinking_len,
			line_number = excluded.line_number`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for idx, entry := range p.entries {
		var inputTokens, outputTokens int
		if entry.Usage != nil {
			inputTokens = entry.Usage.InputTokens
			outputTokens = entry.Usage.OutputTokens
		}

		var toolName string
		var isError bool
		var thinkingLen int
		for _, b := range entry.ContentBlocks {
			switch b.Type {
			case "tool_use":
				toolName = b.ToolName
			case "tool_result":
				isError = b.IsError
			case "thinking":
				thinkingLen += len(b.Thinking)
			}
		}

		wordCount := len(strings.Fields(entry.Text))

		if _, err := stmt.ExecContext(ctx,
			p.meta.ID, entry.UUID, entry.Timestamp, string(entry.Role),
			inputTokens, outputTokens, toolName, isError, wordCount, thinkingLen,
			idx+1,
		); err != nil {
			return fmt.Errorf("insert entry %s: %w", entry.UUID, err)
		}
	}

	// Attachment entries, for quick access without full-text search. Rows are
	// replaced per session so re-ingestion never duplicates them.
	if _, err := tx.ExecContext(ctx, "DELETE FROM entry_attachments WHERE session_id = ?", p.meta.ID); err != nil {
		return fmt.Errorf("clear old attachments: %w", err)
	}
	for idx, entry := range p.entries {
		if entry.Kind != thinkt.KindAttachment {
			continue
		}
		label, _, _ := strings.Cut(entry.Text, "\n")
		if len(label) > 200 {
			label = label[:200]
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO entry_attachments (session_id, entry_index, kind, label, timestamp)
			VALUES (?, ?, ?, ?, ?)`,
			p.meta.ID, idx, entry.Subtype, label, entry.Timestamp.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("insert attachment %s: %w", entry.UUID, err)
		}
	}

	// Session metadata.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (id, project_id, path, model, first_prompt, entry_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			project_id = excluded.project_id,
			path = excluded.path,
			model = excluded.model,
			first_prompt = excluded.first_prompt,
			entry_count = excluded.entry_count,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at`,
		p.meta.ID, p.projectID, p.meta.FullPath, p.meta.Model, p.meta.FirstPrompt,
		len(p.entries), p.meta.CreatedAt, p.meta.ModifiedAt,
	); err != nil {
		return fmt.Errorf("upsert session meta: %w", err)
	}

	// Sync state — last_mod_time is Unix epoch (INTEGER).
	if _, err := tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO sync_state (file_path, last_mod_time, file_size, lines_read, parser_version, last_synced)
		VALUES (?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`,
		p.meta.FullPath, p.meta.ModifiedAt.Unix(), p.meta.FileSize, len(p.entries), thinkt.ParserVersion,
	); err != nil {
		return fmt.Errorf("update sync state: %w", err)
	}

	return tx.Commit()
}

// pruneUnlisted deletes the index rows of sessions recorded under projectID
// whose file path the store no longer lists.
func (i *Ingester) pruneUnlisted(ctx context.Context, projectID string, listed []thinkt.SessionMeta) error {
	keep := make(map[string]bool, len(listed))
	for _, s := range listed {
		keep[s.FullPath] = true
	}
	rows, err := i.db.QueryContext(ctx, `SELECT id, path FROM sessions WHERE project_id = ?`, projectID)
	if err != nil {
		return err
	}
	type stale struct{ id, path string }
	var stales []stale
	for rows.Next() {
		var s stale
		if err := rows.Scan(&s.id, &s.path); err != nil {
			rows.Close()
			return err
		}
		if !keep[s.path] {
			stales = append(stales, s)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(stales) == 0 {
		return err
	}

	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, s := range stales {
		for _, q := range []struct {
			sql string
			arg string
		}{
			{"DELETE FROM entries WHERE session_id = ?", s.id},
			{"DELETE FROM entry_attachments WHERE session_id = ?", s.id},
			{"DELETE FROM sessions WHERE id = ?", s.id},
			{"DELETE FROM sync_state WHERE file_path = ?", s.path},
		} {
			if _, err := tx.ExecContext(ctx, q.sql, q.arg); err != nil {
				return fmt.Errorf("prune %s: %w", s.id, err)
			}
		}
	}
	return tx.Commit()
}

// PruneUnlistedProjects deletes the index rows of projects that a registered
// source no longer lists: the project row and its sessions, entries,
// attachments, and sync state. Each source is listed on its own, and a
// source whose listing fails is skipped, so a transient error never empties
// its index. Sources the registry does not hold (disabled ones) are left
// untouched, and a nil registry prunes nothing.
func (i *Ingester) PruneUnlistedProjects(ctx context.Context) error {
	if i.registry == nil {
		return nil
	}
	for _, store := range i.registry.All() {
		listed, err := store.ListProjects(ctx)
		if err != nil {
			continue
		}
		keep := make(map[string]bool, len(listed))
		for _, p := range listed {
			keep[db.ScopedProjectID(store.Source(), p.ID)] = true
		}
		source := string(store.Source())
		rows, err := i.db.QueryContext(ctx, `SELECT id FROM projects WHERE source = ?`, source)
		if err != nil {
			return err
		}
		var stale []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			if !keep[id] {
				stale = append(stale, id)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range stale {
			if err := i.pruneUnlisted(ctx, id, nil); err != nil {
				return err
			}
			if _, err := i.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id); err != nil {
				return fmt.Errorf("prune project %s: %w", id, err)
			}
		}
	}
	return nil
}

// ShouldSyncSession checks whether a session needs re-indexing.
func (i *Ingester) ShouldSyncSession(meta thinkt.SessionMeta) (bool, error) {
	var lastModUnix int64
	var lastSize int64
	var parserVersion int

	err := i.db.QueryRow(`SELECT last_mod_time, file_size, parser_version FROM sync_state WHERE file_path = ?`,
		meta.FullPath).Scan(&lastModUnix, &lastSize, &parserVersion)
	if err == db.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	// A parser change makes every earlier ingestion stale; re-ingest lazily.
	return meta.ModifiedAt.Unix() > lastModUnix || meta.FileSize != lastSize || parserVersion != thinkt.ParserVersion, nil
}
