package search

import (
	"fmt"
)

// SemanticSearchOptions contains options for semantic search.
type SemanticSearchOptions struct {
	QueryEmbedding []float32
	Model          string
	FilterProject  string
	FilterSource   string
	Limit          int
	MaxDistance     float64 // 0 means no threshold
}

// SemanticResult represents a single semantic search hit.
type SemanticResult struct {
	SessionID   string  `json:"session_id"`
	EntryUUID   string  `json:"entry_uuid"`
	ChunkIndex  int     `json:"chunk_index"`
	Distance    float64 `json:"distance"`
	ProjectName string  `json:"project_name,omitempty"`
	Source      string  `json:"source,omitempty"`
	Path        string  `json:"path,omitempty"`
}

// SemanticSearch queries the embeddings table for vectors similar to the query.
func (s *Service) SemanticSearch(opts SemanticSearchOptions) ([]SemanticResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	q := `
		SELECT e.session_id, e.entry_uuid, e.chunk_index,
		       array_cosine_distance(e.embedding, ?::FLOAT[512]) AS distance,
		       COALESCE(p.name, '') AS project_name,
		       COALESCE(p.source, '') AS source,
		       COALESCE(s.path, '') AS path
		FROM embeddings e
		LEFT JOIN sessions s ON e.session_id = s.id
		LEFT JOIN projects p ON s.project_id = p.id
		WHERE e.model = ?`

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
		q += fmt.Sprintf(" AND array_cosine_distance(e.embedding, ?::FLOAT[512]) < %f", opts.MaxDistance)
		args = append(args, opts.QueryEmbedding)
	}

	q += " ORDER BY distance ASC LIMIT ?"
	args = append(args, opts.Limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("semantic search query: %w", err)
	}
	defer rows.Close()

	var results []SemanticResult
	for rows.Next() {
		var r SemanticResult
		if err := rows.Scan(&r.SessionID, &r.EntryUUID, &r.ChunkIndex,
			&r.Distance, &r.ProjectName, &r.Source, &r.Path); err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}
		results = append(results, r)
	}

	return results, nil
}
