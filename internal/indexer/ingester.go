package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/wethinkt/go-thinkt/internal/indexer/db"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Ingester handles the process of reading data from thinkt stores
// and writing it to the DuckDB index.
type Ingester struct {
	db         *db.DB
	registry   *thinkt.StoreRegistry
	OnProgress func(pIdx, pTotal, sIdx, sTotal int, message string)
}

// NewIngester creates a new Ingester instance.
func NewIngester(database *db.DB, registry *thinkt.StoreRegistry) *Ingester {
	return &Ingester{
		db:       database,
		registry: registry,
	}
}

func (i *Ingester) reportProgress(pIdx, pTotal, sIdx, sTotal int, message string) {
	if i.OnProgress != nil {
		i.OnProgress(pIdx, pTotal, sIdx, sTotal, message)
	}
}

// IngestProject indexes all sessions within a given project.
func (i *Ingester) IngestProject(ctx context.Context, project thinkt.Project, pIdx, pTotal int) error {
	// Ensure project exists in DB
	if err := i.syncProject(ctx, project); err != nil {
		return fmt.Errorf("failed to sync project %s: %w", project.ID, err)
	}

	store, ok := i.registry.Get(project.Source)
	if !ok {
		return fmt.Errorf("no store found for source %s", project.Source)
	}

	sessions, err := store.ListSessions(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("failed to list sessions for project %s: %w", project.ID, err)
	}

	totalSessions := len(sessions)
	if totalSessions == 0 {
		i.reportProgress(pIdx, pTotal, 0, 0, fmt.Sprintf("Project %s (no sessions)", project.Name))
		return nil
	}

	for idx, s := range sessions {
		i.reportProgress(pIdx, pTotal, idx+1, totalSessions, fmt.Sprintf("Indexing %s: %s", project.Name, s.ID))
		if err := i.IngestSession(ctx, project.ID, s); err != nil {
			// Log error but continue with other sessions
			fmt.Fprintf(os.Stderr, "\nError ingesting session %s: %v\n", s.ID, err)
		}
	}

	return nil
}

// IngestSession indexes a single session if it has changed since the last sync.
func (i *Ingester) IngestSession(ctx context.Context, projectID string, meta thinkt.SessionMeta) error {
	// 1. Check sync state
	shouldSync, _, err := i.shouldSyncSession(meta)
	if err != nil {
		return err
	}
	if !shouldSync {
		return nil
	}

	// 2. Load and parse the session
	store, ok := i.registry.Get(meta.Source)
	if !ok {
		return fmt.Errorf("no store found for source %s", meta.Source)
	}

	// Delete existing entries for this session before re-ingesting
	// This ensures we have a clean state and avoids FK constraint issues
	// during the metadata update if the DB implementation is restrictive.
	if _, err := i.db.ExecContext(ctx, "DELETE FROM entries WHERE session_id = ?", meta.ID); err != nil {
		return fmt.Errorf("failed to clear old entries for session %s: %w", meta.ID, err)
	}

	// Upsert session metadata
	if err := i.syncSessionMeta(ctx, projectID, meta); err != nil {
		return err
	}

	reader, err := store.OpenSession(ctx, meta.ID)
	if err != nil {
		return err
	}
	defer reader.Close()

	// 3. Ingest entries
	// Note: For now, we do a full re-ingest if the file changed.
	// Future optimization: use lines_read to only append new entries.
	count := 0
	for {
		entry, err := reader.ReadNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading entry: %w", err)
		}

		if err := i.upsertEntry(ctx, meta.ID, *entry); err != nil {
			return err
		}
		count++
	}

	// 4. Update sync state
	return i.updateSyncState(meta, count)
}

func (i *Ingester) syncProject(ctx context.Context, p thinkt.Project) error {
	query := `
		INSERT INTO projects (id, path, name, source, workspace_id)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			path = excluded.path,
			name = excluded.name,
			source = excluded.source,
			workspace_id = excluded.workspace_id`
	_, err := i.db.ExecContext(ctx, query, p.ID, p.Path, p.Name, string(p.Source), p.WorkspaceID)
	return err
}

func (i *Ingester) syncSessionMeta(ctx context.Context, projectID string, m thinkt.SessionMeta) error {
	query := `
		INSERT INTO sessions (id, project_id, path, model, first_prompt, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			project_id = excluded.project_id,
			path = excluded.path,
			model = excluded.model,
			first_prompt = excluded.first_prompt,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at`
	_, err := i.db.ExecContext(ctx, query, m.ID, projectID, m.FullPath, m.Model, m.FirstPrompt, m.CreatedAt, m.ModifiedAt)
	return err
}

func (i *Ingester) upsertEntry(ctx context.Context, sessionID string, entry thinkt.Entry) error {
	body, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO entries (uuid, session_id, timestamp, role, body)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (uuid) DO UPDATE SET
			session_id = excluded.session_id,
			timestamp = excluded.timestamp,
			role = excluded.role,
			body = excluded.body`
	_, err = i.db.ExecContext(ctx, query, entry.UUID, sessionID, entry.Timestamp, string(entry.Role), body)
	return err
}

func (i *Ingester) shouldSyncSession(meta thinkt.SessionMeta) (bool, time.Time, error) {
	var lastMod time.Time
	var lastSize int64

	err := i.db.QueryRow(`SELECT last_mod_time, file_size FROM sync_state WHERE file_path = ?`, meta.FullPath).Scan(&lastMod, &lastSize)
	if err == sqlErrNoRows() {
		return true, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, err
	}

	// Sync if modified time or size has changed
	return meta.ModifiedAt.After(lastMod) || meta.FileSize != lastSize, lastMod, nil
}

func (i *Ingester) updateSyncState(meta thinkt.SessionMeta, lines int) error {
	query := `
		INSERT OR REPLACE INTO sync_state (file_path, last_mod_time, file_size, lines_read, last_synced)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`
	_, err := i.db.Exec(query, meta.FullPath, meta.ModifiedAt, meta.FileSize, lines)
	return err
}

// Helper to handle sql.ErrNoRows without direct import if needed,
// though we already imported database/sql in db/db.go.
func sqlErrNoRows() error {
	return db.ErrNoRows
}
