package indexer

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wethinkt/go-thinkt/internal/indexer/db"
	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Ingester handles the process of reading data from thinkt stores
// and writing it to the DuckDB index.
type Ingester struct {
	db       *db.DB
	embDB    *db.DB // separate embeddings database (nil if embedding unavailable)
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

	// Verbose enables additional warning output (e.g. skipped sessions).
	Verbose bool
}

// NewIngester creates a new Ingester instance.
// The embDB may be nil if embedding is unavailable.
// The embedder may be nil if embedding is unavailable.
func NewIngester(database *db.DB, embDB *db.DB, registry *thinkt.StoreRegistry, embedder *embedding.Embedder) *Ingester {
	return &Ingester{
		db:       database,
		embDB:    embDB,
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

// MigrateEmbeddings handles model changes in the embeddings table.
//
// When the embedding dimension changes (e.g. switching between 768-dim nomic
// and 1024-dim qwen3), the table must be dropped and recreated because DuckDB
// uses fixed-size FLOAT[N] arrays. Old embeddings are lost in this case.
//
// When only the model name changes but dimension stays the same, old embeddings
// are preserved — queries already filter by model, so stale data is ignored.
// A warning is logged so the user knows stale embeddings exist.
func (i *Ingester) MigrateEmbeddings(ctx context.Context) error {
	if i.embedder == nil || i.embDB == nil {
		return nil
	}

	var count int
	err := i.embDB.QueryRowContext(ctx, `SELECT count(*) FROM embeddings WHERE model != ?`, i.embedder.EmbedModelID()).Scan(&count)
	if err != nil || count == 0 {
		return nil
	}

	// Check if the old embeddings have a different dimension.
	var oldDim int
	err = i.embDB.QueryRowContext(ctx, `SELECT dim FROM embeddings WHERE model != ? LIMIT 1`, i.embedder.EmbedModelID()).Scan(&oldDim)
	if err != nil {
		return nil // can't determine, leave as-is
	}

	if oldDim != i.embedder.Dim() {
		// Dimension changed — must drop and recreate the table.
		fmt.Fprintf(os.Stderr, "Dimension changed (%d → %d): dropping %d old embeddings and recreating table\n",
			oldDim, i.embedder.Dim(), count)
		if _, err := i.embDB.ExecContext(ctx, `DROP TABLE IF EXISTS embeddings`); err != nil {
			return fmt.Errorf("drop embeddings table: %w", err)
		}
		schema := db.EmbeddingsSchemaForDim(i.embedder.Dim())
		if _, err := i.embDB.ExecContext(ctx, schema); err != nil {
			return fmt.Errorf("recreate embeddings table: %w", err)
		}
		return nil
	}

	// Same dimension, different model — keep old embeddings, just warn.
	fmt.Fprintf(os.Stderr, "Note: %d embeddings from a previous model exist (ignored during search). Run 'thinkt-indexer embeddings purge' to remove them.\n", count)
	return nil
}

// PurgeStaleEmbeddings deletes embeddings that don't match the current model.
func (i *Ingester) PurgeStaleEmbeddings(ctx context.Context) (int, error) {
	if i.embedder == nil || i.embDB == nil {
		return 0, nil
	}
	return PurgeStaleEmbeddingsByModel(ctx, i.embDB, i.embedder.EmbedModelID())
}

// PurgeStaleEmbeddingsByModel deletes embeddings whose model doesn't match keepModelID.
// This variant doesn't require a loaded embedder — just the model ID and database.
func PurgeStaleEmbeddingsByModel(ctx context.Context, embDB *db.DB, keepModelID string) (int, error) {
	if embDB == nil {
		return 0, nil
	}

	var count int
	if err := embDB.QueryRowContext(ctx, `SELECT count(*) FROM embeddings WHERE model != ?`, keepModelID).Scan(&count); err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, nil
	}

	_, err := embDB.ExecContext(ctx, `DELETE FROM embeddings WHERE model != ?`, keepModelID)
	return count, err
}

func (i *Ingester) reportProgress(pIdx, pTotal, sIdx, sTotal int, message string) {
	if i.OnProgress != nil {
		i.OnProgress(pIdx, pTotal, sIdx, sTotal, message)
	}
}

// parsedSession holds the result of reading and parsing a session file.
// This is the CPU/IO-bound work that can be parallelized.
type parsedSession struct {
	meta      thinkt.SessionMeta
	projectID string
	entries   []thinkt.Entry
	err       error
}

// IngestProject indexes all sessions within a given project.
// File reads are parallelized across CPU cores; DB writes are serialized.
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

	projectID := ScopedProjectID(project.Source, project.ID)

	// Filter to sessions that need syncing.
	var toSync []thinkt.SessionMeta
	for _, s := range sessions {
		shouldSync, _, err := i.shouldSyncSession(s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError checking session %s: %v\n", s.ID, err)
			continue
		}
		if shouldSync {
			toSync = append(toSync, s)
		}
	}

	if len(toSync) == 0 {
		i.reportProgress(pIdx, pTotal, totalSessions, totalSessions,
			fmt.Sprintf("Project %s (up to date)", project.Name))
		return nil
	}

	// Phase 1: Parse session files in parallel (CPU/IO-bound).
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

	// Phase 2: Write to DB serially (DuckDB is single-writer).
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

// parseSession reads and parses a session file. Safe to call from multiple goroutines.
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

// writeSession writes a parsed session's entries to the database using a
// single transaction with a prepared statement for bulk insert performance.
func (i *Ingester) writeSession(ctx context.Context, p parsedSession) error {
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint: errcheck

	// Delete existing entries for this session before re-ingesting
	if _, err := tx.ExecContext(ctx, "DELETE FROM entries WHERE session_id = ?", p.meta.ID); err != nil {
		return fmt.Errorf("clear old entries: %w", err)
	}

	// Prepared statement for bulk insert
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

	// Session metadata and sync state within same transaction
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

	if _, err := tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO sync_state (file_path, last_mod_time, file_size, lines_read, last_synced)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		p.meta.FullPath, p.meta.ModifiedAt, p.meta.FileSize, len(p.entries),
	); err != nil {
		return fmt.Errorf("update sync state: %w", err)
	}

	return tx.Commit()
}

// IngestSession indexes a single session if it has changed since the last sync.
// This only indexes metadata — call EmbedAllSessions separately for embeddings.
func (i *Ingester) IngestSession(ctx context.Context, projectID string, meta thinkt.SessionMeta) error {
	shouldSync, _, err := i.shouldSyncSession(meta)
	if err != nil {
		return err
	}
	if !shouldSync {
		return nil
	}

	store, ok := i.registry.Get(meta.Source)
	if !ok {
		return fmt.Errorf("no store found for source %s", meta.Source)
	}

	p := i.parseSession(ctx, store, meta, projectID)
	if p.err != nil {
		return p.err
	}

	return i.writeSession(ctx, p)
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
	if i.embedder == nil || i.embDB == nil {
		return nil
	}

	// Two-phase: query sessions+entries from index DB, embedding counts from embDB, diff in Go.
	type sessionInfo struct {
		id         string
		path       string
		entryCount int
	}

	// Phase 1: Get all sessions with entry counts from index DB.
	rows, err := i.db.QueryContext(ctx, `
		SELECT s.id, s.path, count(DISTINCT e.uuid) as entry_count
		FROM sessions s
		JOIN entries e ON e.session_id = s.id
		GROUP BY s.id, s.path
		ORDER BY s.id`)
	if err != nil {
		return fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	var allSessions []sessionInfo
	for rows.Next() {
		var s sessionInfo
		if err := rows.Scan(&s.id, &s.path, &s.entryCount); err != nil {
			return fmt.Errorf("scan session: %w", err)
		}
		allSessions = append(allSessions, s)
	}

	if len(allSessions) == 0 {
		return nil
	}

	// Phase 2: Get embedded entry counts from embeddings DB.
	embRows, err := i.embDB.QueryContext(ctx, `
		SELECT session_id, count(DISTINCT entry_uuid) as embedded_entries
		FROM embeddings
		GROUP BY session_id`)
	if err != nil {
		return fmt.Errorf("query embedding counts: %w", err)
	}
	defer embRows.Close()

	embeddedCounts := make(map[string]int)
	for embRows.Next() {
		var sid string
		var count int
		if err := embRows.Scan(&sid, &count); err != nil {
			return fmt.Errorf("scan embedding count: %w", err)
		}
		embeddedCounts[sid] = count
	}

	// Phase 3: Diff — find sessions that need embedding.
	var sessions []sessionInfo
	for _, s := range allSessions {
		embedded := embeddedCounts[s.id]
		if embedded < s.entryCount {
			sessions = append(sessions, s)
		}
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
		if err != nil && i.Verbose {
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
	if reader == nil {
		return 0, fmt.Errorf("session %s: file not found (may have been deleted)", sessionID)
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

	return i.embedSession(ctx, sessionID, source, entries)
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

func sqlErrNoRows() error {
	return db.ErrNoRows
}

// embedBatchSize controls how many chunks are embedded per sub-batch.
// Smaller values give more granular progress updates.
const embedBatchSize = 16

func (i *Ingester) embedSession(ctx context.Context, sessionID string, source string, entries []thinkt.Entry) (int, error) {
	if i.embedder == nil || i.embDB == nil {
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
			Source:    source,
			Text:      text,
		})
	}
	if len(entryTexts) == 0 {
		return 0, nil
	}

	// Prepare chunks
	requests, mapping := embedding.PrepareEntries(entryTexts, 2000, 200)

	// Load existing text hashes for this session to skip unchanged chunks.
	existingHashes := make(map[string]string) // id -> text_hash
	rows, err := i.embDB.QueryContext(ctx, `SELECT id, text_hash FROM embeddings WHERE session_id = ?`, sessionID)
	if err == nil {
		for rows.Next() {
			var id, hash string
			if err := rows.Scan(&id, &hash); err == nil {
				existingHashes[id] = hash
			}
		}
		rows.Close()
	}

	// Filter to only chunks that are new or changed.
	var newRequests []embedding.EmbedRequest
	var newMapping []embedding.ChunkMapping
	newIDs := make(map[string]bool, len(requests))
	for idx, req := range requests {
		newIDs[req.ID] = true
		m := mapping[idx]
		if existingHash, ok := existingHashes[req.ID]; ok && existingHash == m.TextHash {
			continue // already embedded with same content
		}
		// Delete existing row if it exists (text changed) so INSERT below succeeds.
		if _, ok := existingHashes[req.ID]; ok {
			_, _ = i.embDB.ExecContext(ctx, `DELETE FROM embeddings WHERE id = ?`, req.ID)
		}
		newRequests = append(newRequests, req)
		newMapping = append(newMapping, m)
	}

	// Clean up stale embeddings (entries removed from session).
	for id := range existingHashes {
		if !newIDs[id] {
			_, _ = i.embDB.ExecContext(ctx, `DELETE FROM embeddings WHERE id = ?`, id)
		}
	}

	totalChunks := len(newRequests)
	if totalChunks == 0 {
		return 0, nil
	}

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
			batchTexts[j-batchStart] = newRequests[j].Text
		}

		result, err := i.embedder.Embed(ctx, batchTexts)
		if err != nil {
			return stored, fmt.Errorf("embedding failed: %w", err)
		}
		totalTokens += result.TotalTokens

		// Store embeddings
		for j, vec := range result.Vectors {
			idx := batchStart + j
			id := newRequests[idx].ID
			m := newMapping[idx]
			_, err := i.embDB.ExecContext(ctx, fmt.Sprintf(`
				INSERT INTO embeddings (id, session_id, entry_uuid, chunk_index, model, dim, embedding, text_hash)
				VALUES (?, ?, ?, ?, ?, ?, ?::FLOAT[%d], ?)`, i.embedder.Dim()),
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
