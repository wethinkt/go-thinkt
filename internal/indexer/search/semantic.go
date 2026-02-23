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
}

// SemanticSearch queries the embeddings table for vectors similar to the query.
func (s *Service) SemanticSearch(opts SemanticSearchOptions) ([]SemanticResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

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
		       COALESCE(s.first_prompt, '') AS first_prompt
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
	args = append(args, opts.Limit)

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
			&r.ProjectName, &r.Source, &r.SessionPath, &r.FirstPrompt); err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}
		results = append(results, r)
	}

	return results, nil
}
