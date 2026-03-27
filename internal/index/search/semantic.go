package search

import (
	"fmt"
	"math"
	"strings"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	indexdb "github.com/wethinkt/go-thinkt/internal/index/db"
)

// SemanticSearchOptions contains options for semantic search.
type SemanticSearchOptions struct {
	QueryEmbedding []float32
	Model          string
	Dim            int
	FilterProject  string
	FilterSource   string
	FilterSources  []string
	FilterTier     string
	Limit          int
	MaxDistance    float64
	Diversity      bool
}

// SemanticResult represents a single semantic search hit.
type SemanticResult struct {
	SessionID   string  `json:"session_id"`
	EntryUUID   string  `json:"entry_uuid"`
	ChunkIndex  int     `json:"chunk_index"`
	TotalChunks int     `json:"total_chunks"`
	Distance    float64 `json:"distance"`
	Tier        string  `json:"tier,omitempty"`
	Role        string  `json:"role,omitempty"`
	Timestamp   string  `json:"timestamp,omitempty"`
	ToolName    string  `json:"tool_name,omitempty"`
	WordCount   int     `json:"word_count,omitempty"`
	ProjectName string  `json:"project_name,omitempty"`
	Source      string  `json:"source,omitempty"`
	SessionPath string  `json:"session_path,omitempty"`
	FirstPrompt string  `json:"first_prompt,omitempty"`
	LineNumber  int     `json:"line_number,omitempty"`
	Score       float64 `json:"score,omitempty"`
}

// SemanticSearch queries the embeddings table for vectors similar to the query.
func (s *Service) SemanticSearch(embDB *indexdb.DB, opts SemanticSearchOptions) ([]SemanticResult, error) {
	if embDB == nil {
		return nil, fmt.Errorf("embeddings database not available")
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	fetchLimit := opts.Limit
	if opts.Diversity {
		fetchLimit = opts.Limit * 3
		if fetchLimit < 30 {
			fetchLimit = 30
		}
	}

	results, err := s.fetchSemanticResults(embDB, opts, fetchLimit)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return results, nil
	}

	if opts.Diversity && len(results) > opts.Limit {
		results = applyDiversityScoring(results, opts.Limit)
	}
	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results, nil
}

func (s *Service) fetchSemanticResults(embDB *indexdb.DB, opts SemanticSearchOptions, limit int) ([]SemanticResult, error) {
	// Phase 1: Pre-filter session IDs from index DB if filters are specified.
	var sessionFilter []string
	if opts.FilterProject != "" || opts.FilterSource != "" || len(opts.FilterSources) > 0 {
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
		if len(opts.FilterSources) > 0 {
			placeholders := make([]string, len(opts.FilterSources))
			for i, src := range opts.FilterSources {
				placeholders[i] = "?"
				args = append(args, src)
			}
			q += " AND p.source IN (" + strings.Join(placeholders, ",") + ")"
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
			return nil, nil
		}
	}

	// Phase 2: KNN query against vec_embeddings using sqlite-vec.
	queryBlob, err := sqlite_vec.SerializeFloat32(opts.QueryEmbedding)
	if err != nil {
		return nil, fmt.Errorf("serialize query embedding: %w", err)
	}

	embQ := `SELECT embedding_id, distance FROM vec_embeddings WHERE embedding MATCH ? AND k = ?`
	embArgs := []any{queryBlob, limit}

	tier := opts.FilterTier
	if tier == "" {
		tier = "conversation"
	}
	if tier != "all" {
		embQ += " AND tier = ?"
		embArgs = append(embArgs, tier)
	}
	if opts.Model != "" {
		embQ += " AND model = ?"
		embArgs = append(embArgs, opts.Model)
	}
	if len(sessionFilter) > 0 {
		placeholders := make([]string, len(sessionFilter))
		for i, sid := range sessionFilter {
			placeholders[i] = "?"
			embArgs = append(embArgs, sid)
		}
		embQ += " AND session_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	embRows, err := embDB.Query(embQ, embArgs...)
	if err != nil {
		return nil, fmt.Errorf("semantic search query: %w", err)
	}
	defer embRows.Close()

	type embHit struct {
		embeddingID int64
		distance    float64
	}
	var hits []embHit
	for embRows.Next() {
		var h embHit
		if err := embRows.Scan(&h.embeddingID, &h.distance); err != nil {
			return nil, fmt.Errorf("scan embedding hit: %w", err)
		}
		if opts.MaxDistance > 0 && h.distance > opts.MaxDistance {
			continue
		}
		hits = append(hits, h)
	}
	if len(hits) == 0 {
		return nil, nil
	}

	// Phase 3: Look up embedding metadata.
	type hitMeta struct {
		embeddingID int64
		distance    float64
		sessionID   string
		tier        string
		entryUUID   string
		chunkIndex  int
	}
	var enrichedHits []hitMeta
	for _, h := range hits {
		var hm hitMeta
		hm.embeddingID = h.embeddingID
		hm.distance = h.distance
		err := embDB.QueryRow(
			`SELECT v.session_id, v.tier, COALESCE(m.entry_uuid, ''), COALESCE(m.chunk_index, 0)
			 FROM vec_embeddings v
			 LEFT JOIN embedding_meta m ON m.embedding_id = v.embedding_id
			 WHERE v.embedding_id = ?`, h.embeddingID,
		).Scan(&hm.sessionID, &hm.tier, &hm.entryUUID, &hm.chunkIndex)
		if err != nil {
			continue
		}
		enrichedHits = append(enrichedHits, hm)
	}

	// Phase 4: Batch-lookup metadata from index DB.
	type entryKey struct{ sessionID, entryUUID string }
	uniqueEntries := make(map[entryKey]bool)
	uniqueSessions := make(map[string]bool)
	for _, h := range enrichedHits {
		uniqueEntries[entryKey{h.sessionID, h.entryUUID}] = true
		uniqueSessions[h.sessionID] = true
	}

	type entryMeta struct {
		role, timestamp, toolName string
		wordCount, lineNumber     int
	}
	entryMetaMap := make(map[entryKey]entryMeta)
	for ek := range uniqueEntries {
		var m entryMeta
		err := s.db.QueryRow(`
			SELECT COALESCE(role, ''), COALESCE(timestamp, ''),
			       COALESCE(tool_name, ''), COALESCE(word_count, 0), COALESCE(line_number, 0)
			FROM entries WHERE session_id = ? AND uuid = ?`, ek.sessionID, ek.entryUUID).
			Scan(&m.role, &m.timestamp, &m.toolName, &m.wordCount, &m.lineNumber)
		if err == nil {
			entryMetaMap[ek] = m
		}
	}

	type sessionMeta struct {
		projectName, source, path, firstPrompt string
	}
	sessionMetaMap := make(map[string]sessionMeta)
	for sid := range uniqueSessions {
		var m sessionMeta
		err := s.db.QueryRow(`
			SELECT COALESCE(p.name, ''), COALESCE(p.source, ''),
			       COALESCE(s.path, ''), COALESCE(SUBSTR(s.first_prompt, 1, 200), '')
			FROM sessions s LEFT JOIN projects p ON s.project_id = p.id
			WHERE s.id = ?`, sid).
			Scan(&m.projectName, &m.source, &m.path, &m.firstPrompt)
		if err == nil {
			sessionMetaMap[sid] = m
		}
	}

	chunkCounts := make(map[entryKey]int)
	for _, h := range enrichedHits {
		chunkCounts[entryKey{h.sessionID, h.entryUUID}]++
	}

	results := make([]SemanticResult, 0, len(enrichedHits))
	for _, h := range enrichedHits {
		ek := entryKey{h.sessionID, h.entryUUID}
		r := SemanticResult{
			SessionID: h.sessionID, EntryUUID: h.entryUUID,
			ChunkIndex: h.chunkIndex, TotalChunks: chunkCounts[ek],
			Distance: h.distance, Tier: h.tier,
		}
		if em, ok := entryMetaMap[ek]; ok {
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

func applyDiversityScoring(results []SemanticResult, limit int) []SemanticResult {
	lambda := 0.6
	selected := make([]SemanticResult, 0, limit)
	remaining := make([]SemanticResult, len(results))
	copy(remaining, results)

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

	for len(selected) < limit && len(remaining) > 0 {
		bestIdx := 0
		bestMMR := math.Inf(-1)
		for i, r := range remaining {
			relevance := 2.0 - r.Distance
			maxSimToSelected := 0.0
			for _, sel := range selected {
				sim := sessionSimilarity(r, sel)
				if sim > maxSimToSelected {
					maxSimToSelected = sim
				}
			}
			mmr := lambda*relevance - (1.0-lambda)*maxSimToSelected
			if mmr > bestMMR {
				bestMMR = mmr
				bestIdx = i
			}
		}
		remaining[bestIdx].Score = bestMMR
		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}
	return selected
}

func sessionSimilarity(a, b SemanticResult) float64 {
	if a.SessionID == b.SessionID {
		return 1.0
	}
	if a.ProjectName == b.ProjectName && a.Source == b.Source {
		return 0.5
	}
	if a.Source == b.Source {
		return 0.3
	}
	return 0.0
}
