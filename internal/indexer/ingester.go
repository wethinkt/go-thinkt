package indexer

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/indexer/db"
	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Ingester handles the process of reading data from thinkt stores
// and writing it to the DuckDB index.
type Ingester struct {
	db       *db.DB
	registry *thinkt.StoreRegistry
	embedder *embedding.Embedder // nil if embedding unavailable
	OnProgress  func(pIdx, pTotal, sIdx, sTotal int, message string)

	// OnEmbedProgress is called during EmbedAllSessions with progress updates.
	// Called before embedding (elapsed=0, chunks=0, entries=entry count) and
	// after embedding (elapsed>0, chunks=chunks stored, entries=0).
	OnEmbedProgress func(done, total, chunks, entries int, sessionID, sessionPath string, elapsed time.Duration)

	// OnEmbedChunkProgress is called after each sub-batch of chunks is embedded,
	// providing within-session progress visibility.
	OnEmbedChunkProgress func(chunksDone, chunksTotal, tokensDone int, sessionID string)
}

// NewIngester creates a new Ingester instance.
// The embedder may be nil if embedding is unavailable.
func NewIngester(database *db.DB, registry *thinkt.StoreRegistry, embedder *embedding.Embedder) *Ingester {
	return &Ingester{
		db:       database,
		registry: registry,
		embedder: embedder,
	}
}

// HasEmbedder returns true if an embedding backend is available.
func (i *Ingester) HasEmbedder() bool {
	return i.embedder != nil
}

// Close releases resources held by the ingester.
// Note: the embedder lifecycle is owned by the caller, not the ingester.
func (i *Ingester) Close() {}

// MigrateEmbeddings drops embeddings if the stored model doesn't match the current one.
// This ensures a clean re-embed when the model changes.
func (i *Ingester) MigrateEmbeddings(ctx context.Context) error {
	if i.embedder == nil {
		return nil
	}

	var count int
	err := i.db.QueryRowContext(ctx, `SELECT count(*) FROM embeddings WHERE model != ?`, i.embedder.EmbedModelID()).Scan(&count)
	if err != nil || count == 0 {
		return nil
	}

	fmt.Fprintf(os.Stderr, "Dropping %d embeddings from old model (will re-embed with %s)\n", count, i.embedder.EmbedModelID())
	_, err = i.db.ExecContext(ctx, `DELETE FROM embeddings WHERE model != ?`, i.embedder.EmbedModelID())
	return err
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
		if ctx.Err() != nil {
			return ctx.Err()
		}
		i.reportProgress(pIdx, pTotal, idx+1, totalSessions, fmt.Sprintf("Indexing %s: %s", project.Name, s.ID))
		if err := i.IngestSession(ctx, ScopedProjectID(project.Source, project.ID), s); err != nil {
			// Log error but continue with other sessions
			fmt.Fprintf(os.Stderr, "\nError ingesting session %s: %v\n", s.ID, err)
		}
	}

	return nil
}

// IngestSession indexes a single session if it has changed since the last sync.
// This only indexes metadata — call EmbedAllSessions separately for embeddings.
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
	if _, err := i.db.ExecContext(ctx, "DELETE FROM entries WHERE session_id = ?", meta.ID); err != nil {
		return fmt.Errorf("failed to clear old entries for session %s: %w", meta.ID, err)
	}

	reader, err := store.OpenSession(ctx, meta.ID)
	if err != nil {
		return err
	}
	defer reader.Close()

	// 3. Ingest entries
	count := 0
	for {
		entry, err := reader.ReadNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading entry: %w", err)
		}

		if entry.UUID == "" {
			continue // skip entries without UUIDs
		}
		count++
		if err := i.upsertEntry(ctx, meta.ID, *entry, count); err != nil {
			return err
		}
	}

	// 4. Upsert session metadata with final count
	if err := i.syncSessionMeta(ctx, projectID, meta, count); err != nil {
		return err
	}

	// 5. Update sync state
	return i.updateSyncState(meta, count)
}

// IngestAndEmbedSession indexes a single session and immediately embeds it.
// Used by the watcher for real-time updates.
func (i *Ingester) IngestAndEmbedSession(ctx context.Context, projectID string, meta thinkt.SessionMeta) error {
	if err := i.IngestSession(ctx, projectID, meta); err != nil {
		return err
	}
	if i.embedder == nil {
		return nil
	}
	_, err := i.embedSessionFromDB(ctx, meta.ID)
	return err
}

// EmbedAllSessions finds sessions with missing embeddings and generates them.
// This is designed to run as a second pass after indexing.
func (i *Ingester) EmbedAllSessions(ctx context.Context) error {
	if i.embedder == nil {
		return nil
	}

	// Find sessions that have entries but no embeddings, with entry counts
	rows, err := i.db.QueryContext(ctx, `
		SELECT s.id, s.path, count(e.uuid) as entry_count
		FROM sessions s
		JOIN entries e ON e.session_id = s.id
		WHERE NOT EXISTS (
			SELECT 1 FROM embeddings emb WHERE emb.session_id = s.id
		)
		GROUP BY s.id, s.path
		ORDER BY s.id`)
	if err != nil {
		return fmt.Errorf("query sessions needing embeddings: %w", err)
	}
	defer rows.Close()

	type sessionInfo struct {
		id         string
		path       string
		entryCount int
	}
	var sessions []sessionInfo
	for rows.Next() {
		var s sessionInfo
		if err := rows.Scan(&s.id, &s.path, &s.entryCount); err != nil {
			return fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, s)
	}

	if len(sessions) == 0 {
		return nil
	}

	total := len(sessions)
	for idx, s := range sessions {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if i.OnEmbedProgress != nil {
			i.OnEmbedProgress(idx, total, 0, s.entryCount, s.id, s.path, 0)
		}
		start := time.Now()
		chunks, err := i.embedSessionFromDB(ctx, s.id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nWarning: embedding failed for session %s: %v\n", s.id, err)
		}
		if i.OnEmbedProgress != nil {
			i.OnEmbedProgress(idx+1, total, chunks, 0, s.id, s.path, time.Since(start))
		}
	}

	return nil
}

// embedSessionFromDB reads a session's entries from the source file and embeds them.
// Returns the number of chunks embedded.
func (i *Ingester) embedSessionFromDB(ctx context.Context, sessionID string) (int, error) {
	// Look up session path and source
	var path, projectID string
	err := i.db.QueryRowContext(ctx, `
		SELECT s.path, s.project_id FROM sessions s WHERE s.id = ?`, sessionID).Scan(&path, &projectID)
	if err != nil {
		return 0, fmt.Errorf("lookup session %s: %w", sessionID, err)
	}

	// Determine source from project
	var source string
	err = i.db.QueryRowContext(ctx, `SELECT source FROM projects WHERE id = ?`, projectID).Scan(&source)
	if err != nil {
		return 0, fmt.Errorf("lookup project %s: %w", projectID, err)
	}

	store, ok := i.registry.Get(thinkt.Source(source))
	if !ok {
		return 0, fmt.Errorf("no store for source %s", source)
	}

	reader, err := store.OpenSession(ctx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("open session %s: %w", sessionID, err)
	}
	defer reader.Close()

	var entries []thinkt.Entry
	for {
		entry, err := reader.ReadNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("read entry: %w", err)
		}
		entries = append(entries, *entry)
	}

	return i.embedSession(ctx, sessionID, entries)
}

func (i *Ingester) syncProject(ctx context.Context, p thinkt.Project) error {
	projectID := ScopedProjectID(p.Source, p.ID)
	query := `
		INSERT INTO projects (id, path, name, source, workspace_id)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			path = excluded.path,
			name = excluded.name,
			source = excluded.source,
			workspace_id = excluded.workspace_id`
	_, err := i.db.ExecContext(ctx, query, projectID, p.Path, p.Name, string(p.Source), p.WorkspaceID)
	return err
}

func (i *Ingester) syncSessionMeta(ctx context.Context, projectID string, m thinkt.SessionMeta, count int) error {
	query := `
		INSERT INTO sessions (id, project_id, path, model, first_prompt, entry_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			project_id = excluded.project_id,
			path = excluded.path,
			model = excluded.model,
			first_prompt = excluded.first_prompt,
			entry_count = excluded.entry_count,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at`
	_, err := i.db.ExecContext(ctx, query, m.ID, projectID, m.FullPath, m.Model, m.FirstPrompt, count, m.CreatedAt, m.ModifiedAt)
	return err
}

func (i *Ingester) upsertEntry(ctx context.Context, sessionID string, entry thinkt.Entry, lineNum int) error {
	// Extract metrics
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

	query := `
		INSERT INTO entries (
			uuid, session_id, timestamp, role,
			input_tokens, output_tokens, tool_name, is_error, word_count, thinking_len,
			line_number
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (uuid) DO UPDATE SET
			session_id = excluded.session_id,
			timestamp = excluded.timestamp,
			role = excluded.role,
			input_tokens = excluded.input_tokens,
			output_tokens = excluded.output_tokens,
			tool_name = excluded.tool_name,
			is_error = excluded.is_error,
			word_count = excluded.word_count,
			thinking_len = excluded.thinking_len,
			line_number = excluded.line_number`
	_, err := i.db.ExecContext(ctx, query,
		entry.UUID, sessionID, entry.Timestamp, string(entry.Role),
		inputTokens, outputTokens, toolName, isError, wordCount, thinkingLen,
		lineNum,
	)
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

func sqlErrNoRows() error {
	return db.ErrNoRows
}

// embedBatchSize controls how many chunks are embedded per sub-batch.
// Smaller values give more granular progress updates.
const embedBatchSize = 16

func (i *Ingester) embedSession(ctx context.Context, sessionID string, entries []thinkt.Entry) (int, error) {
	if i.embedder == nil {
		return 0, nil
	}

	// Extract text from entries
	var entryTexts []embedding.EntryText
	for _, e := range entries {
		text := embedding.ExtractText(e)
		if text == "" {
			continue
		}
		entryTexts = append(entryTexts, embedding.EntryText{
			UUID:      e.UUID,
			SessionID: sessionID,
			Text:      text,
		})
	}
	if len(entryTexts) == 0 {
		return 0, nil
	}

	// Prepare chunks
	requests, mapping := embedding.PrepareEntries(entryTexts, 2000, 200)
	totalChunks := len(requests)

	// Embed and store in sub-batches for progress visibility and cancellation.
	stored := 0
	totalTokens := 0
	for batchStart := 0; batchStart < totalChunks; batchStart += embedBatchSize {
		if ctx.Err() != nil {
			return stored, ctx.Err()
		}

		batchEnd := min(batchStart+embedBatchSize, totalChunks)

		// Extract text strings for this sub-batch
		batchTexts := make([]string, batchEnd-batchStart)
		for j := batchStart; j < batchEnd; j++ {
			batchTexts[j-batchStart] = requests[j].Text
		}

		result, err := i.embedder.Embed(ctx, batchTexts)
		if err != nil {
			return stored, fmt.Errorf("embedding failed: %w", err)
		}
		totalTokens += result.TotalTokens

		// Store embeddings
		for j, vec := range result.Vectors {
			idx := batchStart + j
			id := requests[idx].ID
			m := mapping[idx]
			_, err := i.db.ExecContext(ctx, `
				INSERT INTO embeddings (id, session_id, entry_uuid, chunk_index, model, dim, embedding, text_hash)
				VALUES (?, ?, ?, ?, ?, ?, ?::FLOAT[1024], ?)
				ON CONFLICT (id) DO UPDATE SET
					embedding = excluded.embedding,
					text_hash = excluded.text_hash`,
				id, m.SessionID, m.EntryUUID, m.ChunkIndex,
				i.embedder.EmbedModelID(), i.embedder.Dim(), vec, m.TextHash,
			)
			if err != nil {
				return stored, fmt.Errorf("store embedding %s: %w", id, err)
			}
			stored++
		}

		// Report chunk-level progress
		if i.OnEmbedChunkProgress != nil {
			i.OnEmbedChunkProgress(stored, totalChunks, totalTokens, sessionID)
		}
	}

	return stored, nil
}
