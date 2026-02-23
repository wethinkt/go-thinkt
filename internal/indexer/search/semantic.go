package search

import (
	"fmt"
	"math"
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

// fetchResults executes the database query and returns raw results.
func (s *Service) fetchResults(opts SemanticSearchOptions, limit int) ([]SemanticResult, error) {
	q := `
		SELECT emb.session_id, emb.entry_uuid, emb.chunk_index,
		       (SELECT count(*) FROM embeddings c WHERE c.session_id = emb.session_id AND c.entry_uuid = emb.entry_uuid AND c.model = emb.model) AS total_chunks,
		       array_cosine_distance(emb.embedding, ?::FLOAT[1024]) AS distance,
		       COALESCE(ent.role, '') AS role,
		       COALESCE(CAST(ent.timestamp AS VARCHAR), '') AS timestamp,
		       COALESCE(ent.tool_name, '') AS tool_name,
		       COALESCE(ent.word_count, 0) AS word_count,
		       COALESCE(p.name, '') AS project_name,
		       COALESCE(p.source, '') AS source,
		       COALESCE(s.path, '') AS session_path,
		       COALESCE(SUBSTRING(s.first_prompt, 1, 200), '') AS first_prompt,
		       COALESCE(ent.line_number, 0) AS line_number
		FROM embeddings emb
		LEFT JOIN entries ent ON emb.session_id = ent.session_id AND emb.entry_uuid = ent.uuid
		LEFT JOIN sessions s ON emb.session_id = s.id
		LEFT JOIN projects p ON s.project_id = p.id
		WHERE emb.model = ?`

	args := []any{opts.QueryEmbedding, opts.Model}

	if opts.FilterProject != "" {
		q += " AND p.name LIKE ?"
		args = append(args, "%"+opts.FilterProject+"%")
	}
	if opts.FilterSource != "" {
		q += " AND p.source = ?"
		args = append(args, opts.FilterSource)
	}
	if opts.MaxDistance > 0 {
		q += fmt.Sprintf(" AND array_cosine_distance(emb.embedding, ?::FLOAT[1024]) < %f", opts.MaxDistance)
		args = append(args, opts.QueryEmbedding)
	}

	q += " ORDER BY distance ASC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("semantic search query: %w", err)
	}
	defer rows.Close()

	var results []SemanticResult
	for rows.Next() {
		var r SemanticResult
		if err := rows.Scan(&r.SessionID, &r.EntryUUID, &r.ChunkIndex, &r.TotalChunks,
			&r.Distance, &r.Role, &r.Timestamp, &r.ToolName, &r.WordCount,
			&r.ProjectName, &r.Source, &r.SessionPath, &r.FirstPrompt, &r.LineNumber); err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
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
