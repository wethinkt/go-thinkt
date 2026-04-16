package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	indexdb "github.com/wethinkt/go-thinkt/internal/index/db"
	indexsearch "github.com/wethinkt/go-thinkt/internal/index/search"
)

// SearchMatch represents a single match within a session.
type SearchMatch struct {
	LineNum    int    `json:"line_num"`
	Preview    string `json:"preview"`
	Role       string `json:"role"`
	MatchStart int    `json:"match_start"`
	MatchEnd   int    `json:"match_end"`
}

// SearchSessionResult represents all matches found in a single session.
type SearchSessionResult struct {
	SessionID   string        `json:"session_id"`
	ProjectName string        `json:"project_name"`
	Source      string        `json:"source"`
	Path        string        `json:"path"`
	Matches     []SearchMatch `json:"matches"`
}

// SearchResponse is the HTTP response for text search.
type SearchResponse struct {
	Results      []SearchSessionResult `json:"results"`
	TotalMatches int                   `json:"total_matches"`
}

// StatsToolCount is a tool name and its usage count.
type StatsToolCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// StatsResponse is the HTTP response for usage statistics.
type StatsResponse struct {
	TotalProjects   int              `json:"total_projects"`
	TotalSessions   int              `json:"total_sessions"`
	TotalEntries    int              `json:"total_entries"`
	TotalTokens     int              `json:"total_tokens"`
	TotalEmbeddings int              `json:"total_embeddings"`
	EmbedModel      string           `json:"embed_model"`
	TopTools        []StatsToolCount `json:"top_tools"`
}

// SemanticSearchResult represents a single semantic search result.
type SemanticSearchResult struct {
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
}

// SemanticSearchResponse is the HTTP response for semantic search.
type SemanticSearchResponse struct {
	Results []SemanticSearchResult `json:"results"`
}

// handleSearchSessions searches for text across indexed sessions.
// @Summary Search across indexed sessions
// @Description Search for text within the original session files using the SQLite index
// @Tags indexer
// @Produce json
// @Param q query string true "Search query text"
// @Param project query string false "Filter by project name (substring match)"
// @Param source query string false "Filter by source (claude, kimi), case-insensitive"
// @Param limit query int false "Maximum total matches (default 50)"
// @Param limit_per_session query int false "Maximum matches per session (default 2, 0 for no limit)"
// @Param case_sensitive query bool false "Enable case-sensitive matching (default false)"
// @Param regex query bool false "Treat query as a regular expression (default false)"
// @Success 200 {object} SearchResponse
// @Failure 400 {object} ErrorResponse "Bad Request - missing query"
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 503 {object} ErrorResponse "Service Unavailable - indexer not found"
// @Router /search [get]
// @Security BearerAuth
func (s *HTTPServer) handleSearchSessions(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	if s.indexDB == nil {
		writeError(w, http.StatusServiceUnavailable, "indexer_unavailable", "index database is not available")
		return
	}

	var limit, limitPerSession int
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("limit_per_session"); v != "" {
		limitPerSession, _ = strconv.Atoi(v)
	}

	svc := indexsearch.NewService(s.indexDB)
	opts := indexsearch.SearchOptions{
		Query:           query,
		FilterProject:   r.URL.Query().Get("project"),
		FilterSource:    r.URL.Query().Get("source"),
		FilterSources:   s.enabledSourceNames(),
		Limit:           limit,
		LimitPerSession: limitPerSession,
		CaseSensitive:   r.URL.Query().Get("case_sensitive") == "true",
		UseRegex:        r.URL.Query().Get("regex") == "true",
	}
	results, totalMatches, err := svc.Search(opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, SearchResponse{Results: toSearchResponse(results), TotalMatches: totalMatches})
}

// handleGetStats returns usage statistics from the index.
// @Summary Get index usage statistics
// @Description Returns aggregate usage statistics including total tokens and most used tools
// @Tags indexer
// @Produce json
// @Success 200 {object} StatsResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 503 {object} ErrorResponse "Service Unavailable - indexer not found"
// @Router /stats [get]
// @Security BearerAuth
func (s *HTTPServer) handleGetStats(w http.ResponseWriter, r *http.Request) {
	if s.indexDB == nil {
		writeError(w, http.StatusServiceUnavailable, "indexer_unavailable", "index database is not available")
		return
	}

	stats := queryStats(s.indexDB, s.enabledSourceNames())
	writeJSON(w, http.StatusOK, stats)
}

// queryStats runs the stats queries against the index database.
func queryStats(db *indexdb.DB, enabledSources []string) StatsResponse {
	sourceClause, sourceArgs := indexdb.SourceFilter(enabledSources, "p.source")
	var stats StatsResponse

	_ = db.QueryRow(
		"SELECT count(*) FROM projects p WHERE 1=1 "+sourceClause, sourceArgs...,
	).Scan(&stats.TotalProjects)
	_ = db.QueryRow(
		"SELECT count(*) FROM sessions s JOIN projects p ON s.project_id = p.id WHERE 1=1 "+sourceClause,
		sourceArgs...,
	).Scan(&stats.TotalSessions)
	_ = db.QueryRow(
		"SELECT count(*) FROM entries e JOIN sessions s ON e.session_id = s.id JOIN projects p ON s.project_id = p.id WHERE 1=1 "+sourceClause,
		sourceArgs...,
	).Scan(&stats.TotalEntries)
	_ = db.QueryRow(
		"SELECT COALESCE(sum(e.input_tokens + e.output_tokens), 0) FROM entries e JOIN sessions s ON e.session_id = s.id JOIN projects p ON s.project_id = p.id WHERE 1=1 "+sourceClause,
		sourceArgs...,
	).Scan(&stats.TotalTokens)

	toolSQL := "SELECT e.tool_name, count(*) AS cnt FROM entries e JOIN sessions s ON e.session_id = s.id JOIN projects p ON s.project_id = p.id WHERE e.tool_name != '' " + sourceClause + " GROUP BY e.tool_name ORDER BY cnt DESC LIMIT 25"
	rows, err := db.Query(toolSQL, sourceArgs...)
	if err == nil {
		for rows.Next() {
			var tc StatsToolCount
			if rows.Scan(&tc.Name, &tc.Count) == nil {
				stats.TopTools = append(stats.TopTools, tc)
			}
		}
		rows.Close()
	}

	return stats
}

// handleSemanticSearch searches by meaning using on-device embeddings.
// @Summary Semantic search across indexed sessions
// @Description Search for sessions by meaning using on-device embeddings. Returns sessions ranked by semantic similarity. Requires the indexer with a synced embedding index.
// @Tags indexer
// @Produce json
// @Param q query string true "Natural language search query"
// @Param project query string false "Filter by project name (substring match)"
// @Param source query string false "Filter by source (claude, kimi, gemini, copilot, codex, qwen), case-insensitive"
// @Param limit query int false "Maximum number of results (default 20)"
// @Param max_distance query number false "Cosine distance threshold (0-2, lower is more similar)"
// @Param diversity query bool false "Apply diversity scoring to return results from different sessions"
// @Success 200 {object} SemanticSearchResponse
// @Failure 400 {object} ErrorResponse "Bad Request - missing query"
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 503 {object} ErrorResponse "Service Unavailable - indexer not found"
// DISABLED @Router /semantic-search [get]
// DISABLED @Security BearerAuth
//
//nolint:unused // retained for re-enable; route registration commented in server.go
func (s *HTTPServer) handleSemanticSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	if s.embedder == nil || s.embDB == nil || s.indexDB == nil {
		writeError(w, http.StatusServiceUnavailable, "indexer_unavailable", "semantic search is not available (embedder or database not configured)")
		return
	}

	// Embed the query text.
	vectors, err := s.embedder.EmbedTexts(r.Context(), []string{query})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "embed_failed", err.Error())
		return
	}
	if len(vectors) == 0 {
		writeError(w, http.StatusInternalServerError, "embed_failed", "no embedding vector produced")
		return
	}

	var limit int
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	var maxDistance float64
	if v := r.URL.Query().Get("max_distance"); v != "" {
		maxDistance, _ = strconv.ParseFloat(v, 64)
	}

	svc := indexsearch.NewService(s.indexDB)
	opts := indexsearch.SemanticSearchOptions{
		QueryEmbedding: vectors[0],
		Model:          s.embedder.EmbedModelID(),
		Dim:            s.embedder.Dim(),
		FilterProject:  r.URL.Query().Get("project"),
		FilterSource:   r.URL.Query().Get("source"),
		FilterSources:  s.enabledSourceNames(),
		Limit:          limit,
		MaxDistance:     maxDistance,
		Diversity:      r.URL.Query().Get("diversity") == "true",
	}
	results, err := svc.SemanticSearch(s.embDB, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "semantic_search_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, SemanticSearchResponse{Results: fromNewSemanticResults(results)})
}

// IndexerHealthResponse describes the health of the indexer server.
type IndexerHealthResponse struct {
	Available          bool `json:"available"`
	DatabaseAccessible bool `json:"database_accessible,omitempty"`
	IndexedProjects    int  `json:"indexed_projects,omitempty"`
	IndexedSessions    int  `json:"indexed_sessions,omitempty"`
}

// handleIndexerHealth returns the health/status of the indexer.
// @Summary Get indexer health status
// @Description Returns whether the indexer server is reachable and the database is accessible
// @Tags indexer
// @Produce json
// @Success 200 {object} IndexerHealthResponse
// @Router /indexer/health [get]
func (s *HTTPServer) handleIndexerHealth(w http.ResponseWriter, r *http.Request) {
	result := IndexerHealthResponse{
		Available: s.indexDB != nil,
	}

	if s.indexDB != nil {
		var projects, sessions int
		if s.indexDB.QueryRow("SELECT count(*) FROM projects").Scan(&projects) == nil {
			_ = s.indexDB.QueryRow("SELECT count(*) FROM sessions").Scan(&sessions)
			result.DatabaseAccessible = true
			result.IndexedProjects = projects
			result.IndexedSessions = sessions
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// handleIndexerStatus returns the live status of the index worker.
// @Summary Get indexer server status
// @Description Returns the current state of the indexer server including sync/embedding progress, model info, and uptime. Requires a running indexer server.
// @Tags indexer
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Router /indexer/status [get]
// @Security BearerAuth
func (s *HTTPServer) handleIndexerStatus(w http.ResponseWriter, r *http.Request) {
	if s.status == nil {
		writeJSON(w, http.StatusOK, map[string]any{"running": false, "state": "stopped"})
		return
	}
	snap := s.status.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"running":        true,
		"state":          snap.State,
		"syncing":        snap.Syncing,
		"embedding":      snap.Embedding,
		"summarizing":    snap.Summarizing,
		"model":          snap.Model,
		"model_dim":      snap.ModelDim,
		"uptime_seconds": snap.UptimeSeconds,
	})
}

// toSearchResponse converts index/search results to API response types.
func toSearchResponse(results []indexsearch.SessionResult) []SearchSessionResult {
	out := make([]SearchSessionResult, len(results))
	for i, r := range results {
		matches := make([]SearchMatch, len(r.Matches))
		for j, m := range r.Matches {
			matches[j] = SearchMatch{
				LineNum: m.LineNum, Preview: m.Preview, Role: m.Role,
				MatchStart: m.MatchStart, MatchEnd: m.MatchEnd,
			}
		}
		out[i] = SearchSessionResult{
			SessionID: r.SessionID, ProjectName: r.ProjectName,
			Source: r.Source, Path: r.Path, Matches: matches,
		}
	}
	return out
}

// fromNewSemanticResults converts index/search semantic results to API response types.
//
//nolint:unused // retained for re-enable alongside handleSemanticSearch
func fromNewSemanticResults(results []indexsearch.SemanticResult) []SemanticSearchResult {
	out := make([]SemanticSearchResult, len(results))
	for i, r := range results {
		out[i] = SemanticSearchResult{
			SessionID: r.SessionID, EntryUUID: r.EntryUUID,
			ChunkIndex: r.ChunkIndex, TotalChunks: r.TotalChunks,
			Distance: r.Distance, Tier: r.Tier, Role: r.Role,
			Timestamp: r.Timestamp, ToolName: r.ToolName,
			WordCount: r.WordCount, ProjectName: r.ProjectName,
			Source: r.Source, SessionPath: r.SessionPath,
			FirstPrompt: r.FirstPrompt, LineNumber: r.LineNumber,
		}
	}
	return out
}

// EmbedderInterface is the interface for embedding text, used by semantic search.
// The concrete implementation wraps index/embedding.Embedder to avoid importing
// CGO/llama dependencies into the server package.
type EmbedderInterface interface {
	// EmbedTexts returns embedding vectors for the given texts.
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
	// EmbedModelID returns the model identifier string.
	EmbedModelID() string
	// Dim returns the embedding dimension.
	Dim() int
}
