package search

import (
	"fmt"
	"math"
	"strings"
)

// SemanticSearchOptions contains options for semantic search.
type SemanticSearchOptions struct {
	QueryEmbedding []float32
	Model          string
	FilterProject  string
	FilterSource   string
	Limit          int
	MaxDistance    float64 // 0 means no threshold
	Diversity      bool    // Enable diversity scoring to avoid similar results from same session
}

// SemanticResult represents a single semantic search hit.
type SemanticResult struct {
	SessionID    string  `json:"session_id"`
	EntryUUID    string  `json:"entry_uuid"`
	ChunkIndex   int     `json:"chunk_index"`
	TotalChunks  int     `json:"total_chunks"`
	Distance     float64 `json:"distance"`
	Role         string  `json:"role,omitempty"`
	Timestamp    string  `json:"timestamp,omitempty"`
	ToolName     string  `json:"tool_name,omitempty"`
	WordCount    int     `json:"word_count,omitempty"`
	ProjectName  string  `json:"project_name,omitempty"`
	Source       string  `json:"source,omitempty"`
	SessionPath  string  `json:"session_path,omitempty"`
	FirstPrompt  string  `json:"first_prompt,omitempty"`
	LineNumber   int     `json:"line_number,omitempty"`
	Score        float64 `json:"score,omitempty"` // Combined relevance + diversity score
}

// SemanticSearch queries the embeddings table for vectors similar to the query.
func (s *Service) SemanticSearch(opts SemanticSearchOptions) ([]SemanticResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	// Fetch more results than needed for diversity reranking
	fetchLimit := opts.Limit
	if opts.Diversity {
		fetchLimit = opts.Limit * 3 // Fetch extra for diversity scoring
		if fetchLimit < 30 {
			fetchLimit = 30
		}
	}

	results, err := s.fetchResults(opts, fetchLimit)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return results, nil
	}

	// Apply diversity scoring if enabled
	if opts.Diversity && len(results) > opts.Limit {
		results = applyDiversityScoring(results, opts.Limit)
	}

	// Trim to requested limit
	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results, nil
}

// fetchResults performs a two-phase search across the split databases:
// 1. Pre-filter session IDs from index DB (if project/source filters specified)
// 2. Query embeddings DB for nearest vectors (with optional session filter)
// 3. Batch-lookup metadata from index DB using result session/entry IDs
func (s *Service) fetchResults(opts SemanticSearchOptions, limit int) ([]SemanticResult, error) {
	if s.embDB == nil {
		return nil, fmt.Errorf("embeddings database not available")
	}

	// Phase 1: Pre-filter session IDs from index DB if filters are specified.
	var sessionFilter []string
	if opts.FilterProject != "" || opts.FilterSource != "" {
		q := `SELECT s.id FROM sessions s JOIN projects p ON s.project_id = p.id WHERE 1=1`
		var args []any
		if opts.FilterProject != "" {
			q += " AND p.name LIKE ?"
			args = append(args, "%"+opts.FilterProject+"%")
		}
		if opts.FilterSource != "" {
			q += " AND p.source = ?"
			args = append(args, opts.FilterSource)
		}
		rows, err := s.db.Query(q, args...)
		if err != nil {
			return nil, fmt.Errorf("pre-filter sessions: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, fmt.Errorf("scan session id: %w", err)
			}
			sessionFilter = append(sessionFilter, id)
		}
		if len(sessionFilter) == 0 {
			return nil, nil // No sessions match filter
		}
	}

	// Phase 2: Query embeddings DB for nearest vectors.
	embQ := `
		SELECT emb.session_id, emb.entry_uuid, emb.chunk_index,
		       (SELECT count(*) FROM embeddings c WHERE c.session_id = emb.session_id AND c.entry_uuid = emb.entry_uuid AND c.model = emb.model) AS total_chunks,
		       array_cosine_distance(emb.embedding, ?::FLOAT[1024]) AS distance
		FROM embeddings emb
		WHERE emb.model = ?`
	embArgs := []any{opts.QueryEmbedding, opts.Model}

	if len(sessionFilter) > 0 {
		placeholders := make([]string, len(sessionFilter))
		for i, sid := range sessionFilter {
			placeholders[i] = "?"
			embArgs = append(embArgs, sid)
		}
		embQ += " AND emb.session_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	if opts.MaxDistance > 0 {
		embQ += fmt.Sprintf(" AND array_cosine_distance(emb.embedding, ?::FLOAT[1024]) < %f", opts.MaxDistance)
		embArgs = append(embArgs, opts.QueryEmbedding)
	}

	embQ += " ORDER BY distance ASC LIMIT ?"
	embArgs = append(embArgs, limit)

	embRows, err := s.embDB.Query(embQ, embArgs...)
	if err != nil {
		return nil, fmt.Errorf("semantic search query: %w", err)
	}
	defer embRows.Close()

	type embHit struct {
		sessionID  string
		entryUUID  string
		chunkIndex int
		totalChunks int
		distance   float64
	}
	var hits []embHit
	for embRows.Next() {
		var h embHit
		if err := embRows.Scan(&h.sessionID, &h.entryUUID, &h.chunkIndex, &h.totalChunks, &h.distance); err != nil {
			return nil, fmt.Errorf("scan embedding hit: %w", err)
		}
		hits = append(hits, h)
	}

	if len(hits) == 0 {
		return nil, nil
	}

	// Phase 3: Batch-lookup metadata from index DB.
	// Collect unique (session_id, entry_uuid) pairs for the lookup.
	type entryKey struct{ sessionID, entryUUID string }
	uniqueEntries := make(map[entryKey]bool)
	uniqueSessions := make(map[string]bool)
	for _, h := range hits {
		uniqueEntries[entryKey{h.sessionID, h.entryUUID}] = true
		uniqueSessions[h.sessionID] = true
	}

	// Lookup entry metadata
	type entryMeta struct {
		role      string
		timestamp string
		toolName  string
		wordCount int
		lineNumber int
	}
	entryMetaMap := make(map[entryKey]entryMeta)
	for ek := range uniqueEntries {
		var m entryMeta
		err := s.db.QueryRow(`
			SELECT COALESCE(role, ''), COALESCE(CAST(timestamp AS VARCHAR), ''),
			       COALESCE(tool_name, ''), COALESCE(word_count, 0), COALESCE(line_number, 0)
			FROM entries WHERE session_id = ? AND uuid = ?`, ek.sessionID, ek.entryUUID).
			Scan(&m.role, &m.timestamp, &m.toolName, &m.wordCount, &m.lineNumber)
		if err == nil {
			entryMetaMap[ek] = m
		}
	}

	// Lookup session+project metadata
	type sessionMeta struct {
		projectName string
		source      string
		path        string
		firstPrompt string
	}
	sessionMetaMap := make(map[string]sessionMeta)
	for sid := range uniqueSessions {
		var m sessionMeta
		err := s.db.QueryRow(`
			SELECT COALESCE(p.name, ''), COALESCE(p.source, ''),
			       COALESCE(s.path, ''), COALESCE(SUBSTRING(s.first_prompt, 1, 200), '')
			FROM sessions s LEFT JOIN projects p ON s.project_id = p.id
			WHERE s.id = ?`, sid).
			Scan(&m.projectName, &m.source, &m.path, &m.firstPrompt)
		if err == nil {
			sessionMetaMap[sid] = m
		}
	}

	// Assemble results
	results := make([]SemanticResult, 0, len(hits))
	for _, h := range hits {
		r := SemanticResult{
			SessionID:   h.sessionID,
			EntryUUID:   h.entryUUID,
			ChunkIndex:  h.chunkIndex,
			TotalChunks: h.totalChunks,
			Distance:    h.distance,
		}
		if em, ok := entryMetaMap[entryKey{h.sessionID, h.entryUUID}]; ok {
			r.Role = em.role
			r.Timestamp = em.timestamp
			r.ToolName = em.toolName
			r.WordCount = em.wordCount
			r.LineNumber = em.lineNumber
		}
		if sm, ok := sessionMetaMap[h.sessionID]; ok {
			r.ProjectName = sm.projectName
			r.Source = sm.source
			r.SessionPath = sm.path
			r.FirstPrompt = sm.firstPrompt
		}
		results = append(results, r)
	}

	return results, nil
}

// applyDiversityScoring reranks results to promote diversity across sessions.
// Uses Maximal Marginal Relevance (MMR) style scoring.
// Since distance is 0-2 (lower is better), we convert to similarity (2-distance) for scoring.
func applyDiversityScoring(results []SemanticResult, limit int) []SemanticResult {
	lambda := 0.6 // Balance between relevance and diversity (0.6 favors relevance slightly)

	selected := make([]SemanticResult, 0, limit)
	remaining := make([]SemanticResult, len(results))
	copy(remaining, results)

	// First, pick the most relevant result (lowest distance)
	if len(remaining) > 0 {
		bestIdx := 0
		bestDist := remaining[0].Distance
		for i, r := range remaining {
			if r.Distance < bestDist {
				bestDist = r.Distance
				bestIdx = i
			}
		}
		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	// Then iteratively pick results with best MMR score
	for len(selected) < limit && len(remaining) > 0 {
		bestIdx := 0
		bestMMR := math.Inf(-1)

		for i, r := range remaining {
			// Relevance: convert distance to similarity (2.0 - distance, higher is better)
			relevance := 2.0 - r.Distance

			// Diversity: maximum similarity to already selected sessions
			maxSimToSelected := 0.0
			for _, s := range selected {
				sim := sessionSimilarity(r, s)
				if sim > maxSimToSelected {
					maxSimToSelected = sim
				}
			}

			// MMR = λ * Relevance - (1-λ) * maxSimToSelected
			mmr := lambda*relevance - (1.0-lambda)*maxSimToSelected
			r.Score = mmr

			if mmr > bestMMR {
				bestMMR = mmr
				bestIdx = i
			}
		}

		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	return selected
}

// sessionSimilarity returns a similarity score between two results from different sessions.
// Returns 1.0 if same session, 0.0 if completely different (different project and source).
func sessionSimilarity(a, b SemanticResult) float64 {
	if a.SessionID == b.SessionID {
		return 1.0 // Same session
	}
	if a.ProjectName == b.ProjectName && a.Source == b.Source {
		return 0.5 // Same project and source
	}
	if a.Source == b.Source {
		return 0.3 // Same source only
	}
	return 0.0 // Completely different
}
