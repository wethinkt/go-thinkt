package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	indexdb "github.com/wethinkt/go-thinkt/internal/index/db"
	indexsearch "github.com/wethinkt/go-thinkt/internal/index/search"
	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
	"github.com/wethinkt/go-thinkt/internal/indexer/search"
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

// IndexerStatusData is the HTTP representation of indexer server state.
type IndexerStatusData = rpc.StatusData

// handleSearchSessions searches for text across indexed sessions.
// @Summary Search across indexed sessions
// @Description Search for text within the original session files using the DuckDB index
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

	params := rpc.SearchParams{
		Query:   query,
		Project: r.URL.Query().Get("project"),
		Source:  r.URL.Query().Get("source"),
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		params.Limit, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("limit_per_session"); v != "" {
		params.LimitPerSession, _ = strconv.Atoi(v)
	}
	params.CaseSensitive = r.URL.Query().Get("case_sensitive") == "true"
	params.Regex = r.URL.Query().Get("regex") == "true"

	// Try direct SQLite first.
	if s.indexDB != nil {
		svc := indexsearch.NewService(s.indexDB)
		opts := indexsearch.SearchOptions{
			Query:           params.Query,
			FilterProject:   params.Project,
			FilterSource:    params.Source,
			FilterSources:   s.enabledSourceNames(),
			Limit:           params.Limit,
			LimitPerSession: params.LimitPerSession,
			CaseSensitive:   params.CaseSensitive,
			UseRegex:        params.Regex,
		}
		results, totalMatches, err := svc.Search(opts)
		if err == nil {
			writeJSON(w, http.StatusOK, SearchResponse{Results: toSearchResponse(results), TotalMatches: totalMatches})
			return
		}
		// Fall through to RPC on error.
	}

	// RPC fallback.
	results, totalMatches, err := indexerSearch(params)
	if err != nil {
		if errors.Is(err, errIndexerUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "indexer_unavailable", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "search_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, SearchResponse{Results: fromIndexerSearchResults(results), TotalMatches: totalMatches})
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
	// Try direct SQLite first.
	if s.indexDB != nil {
		sourceClause, sourceArgs := indexdb.SourceFilter(s.enabledSourceNames(), "p.source")

		var stats StatsResponse
		if err := s.indexDB.QueryRow(
			"SELECT count(*) FROM projects p WHERE 1=1 "+sourceClause, sourceArgs...,
		).Scan(&stats.TotalProjects); err == nil {
			_ = s.indexDB.QueryRow(
				"SELECT count(*) FROM sessions s JOIN projects p ON s.project_id = p.id WHERE 1=1 "+sourceClause,
				sourceArgs...,
			).Scan(&stats.TotalSessions)
			_ = s.indexDB.QueryRow(
				"SELECT count(*) FROM entries e JOIN sessions s ON e.session_id = s.id JOIN projects p ON s.project_id = p.id WHERE 1=1 "+sourceClause,
				sourceArgs...,
			).Scan(&stats.TotalEntries)
			_ = s.indexDB.QueryRow(
				"SELECT COALESCE(sum(e.input_tokens + e.output_tokens), 0) FROM entries e JOIN sessions s ON e.session_id = s.id JOIN projects p ON s.project_id = p.id WHERE 1=1 "+sourceClause,
				sourceArgs...,
			).Scan(&stats.TotalTokens)

			toolSQL := "SELECT e.tool_name, count(*) AS cnt FROM entries e JOIN sessions s ON e.session_id = s.id JOIN projects p ON s.project_id = p.id WHERE e.tool_name != '' " + sourceClause + " GROUP BY e.tool_name ORDER BY cnt DESC LIMIT 25"
			rows, err := s.indexDB.Query(toolSQL, sourceArgs...)
			if err == nil {
				for rows.Next() {
					var tc StatsToolCount
					if rows.Scan(&tc.Name, &tc.Count) == nil {
						stats.TopTools = append(stats.TopTools, tc)
					}
				}
				rows.Close()
			}

			writeJSON(w, http.StatusOK, stats)
			return
		}
	}

	// RPC fallback.
	data, err := indexerStats()
	if err != nil {
		if errors.Is(err, errIndexerUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "indexer_unavailable", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "stats_failed", err.Error())
		return
	}

	var result StatsResponse
	if err := json.Unmarshal(data, &result); err != nil {
		writeError(w, http.StatusInternalServerError, "invalid_response", "Failed to parse stats response")
		return
	}

	writeJSON(w, http.StatusOK, result)
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
// @Router /semantic-search [get]
// @Security BearerAuth
func (s *HTTPServer) handleSemanticSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	params := rpc.SemanticSearchParams{
		Query:   query,
		Project: r.URL.Query().Get("project"),
		Source:  r.URL.Query().Get("source"),
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		params.Limit, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("max_distance"); v != "" {
		params.MaxDistance, _ = strconv.ParseFloat(v, 64)
	}
	params.Diversity = r.URL.Query().Get("diversity") == "true"

	results, err := indexerSemanticSearch(params)
	if err != nil {
		if errors.Is(err, errIndexerUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "indexer_unavailable", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "semantic_search_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, SemanticSearchResponse{Results: fromSemanticResults(results)})
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
		Available: s.indexDB != nil || rpc.ServerAvailable(),
	}

	if s.indexDB != nil {
		var projects, sessions int
		if s.indexDB.QueryRow("SELECT count(*) FROM projects").Scan(&projects) == nil {
			_ = s.indexDB.QueryRow("SELECT count(*) FROM sessions").Scan(&sessions)
			result.DatabaseAccessible = true
			result.IndexedProjects = projects
			result.IndexedSessions = sessions
		}
	} else if result.Available {
		data, err := indexerStats()
		if err == nil {
			var stats StatsResponse
			if err := json.Unmarshal(data, &stats); err == nil {
				result.DatabaseAccessible = true
				result.IndexedProjects = stats.TotalProjects
				result.IndexedSessions = stats.TotalSessions
			}
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// IndexerStatusResponse describes the current state of the indexer server.
type IndexerStatusResponse struct {
	Running bool `json:"running"`
	IndexerStatusData
}

// handleIndexerStatus returns the live status of the indexer server via RPC.
// @Summary Get indexer server status
// @Description Returns the current state of the indexer server including sync/embedding progress, model info, and uptime. Requires a running indexer server.
// @Tags indexer
// @Produce json
// @Success 200 {object} IndexerStatusResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Router /indexer/status [get]
// @Security BearerAuth
func (s *HTTPServer) handleIndexerStatus(w http.ResponseWriter, r *http.Request) {
	if !rpc.ServerAvailable() {
		writeJSON(w, http.StatusOK, IndexerStatusResponse{
			IndexerStatusData: IndexerStatusData{State: "stopped"},
		})
		return
	}

	resp, err := rpc.Call(rpc.MethodStatus, nil, nil)
	if err != nil {
		writeJSON(w, http.StatusOK, IndexerStatusResponse{
			IndexerStatusData: IndexerStatusData{State: "stopped"},
		})
		return
	}
	if !resp.OK {
		writeError(w, http.StatusInternalServerError, "status_failed", resp.Error)
		return
	}

	var status IndexerStatusData
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		writeError(w, http.StatusInternalServerError, "invalid_response", "Failed to parse status")
		return
	}

	writeJSON(w, http.StatusOK, IndexerStatusResponse{
		Running:           true,
		IndexerStatusData: status,
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

// fromIndexerSearchResults converts indexer/search results to API response types.
func fromIndexerSearchResults(results []search.SessionResult) []SearchSessionResult {
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

// fromSemanticResults converts indexer/search semantic results to API response types.
func fromSemanticResults(results []search.SemanticResult) []SemanticSearchResult {
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
