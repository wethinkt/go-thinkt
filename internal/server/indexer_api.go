package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
	"github.com/wethinkt/go-thinkt/internal/indexer/search"
)

// SearchResponse contains the results of a search query.
type SearchResponse struct {
	Sessions     []search.SessionResult `json:"sessions"`
	TotalMatches int                    `json:"total_matches"`
}

// StatsToolCount represents a tool and its usage count.
type StatsToolCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// StatsResponse contains usage statistics from the index.
type StatsResponse struct {
	TotalProjects int               `json:"total_projects"`
	TotalSessions int               `json:"total_sessions"`
	TotalEntries  int               `json:"total_entries"`
	TotalTokens   int               `json:"total_tokens"`
	TopTools      []StatsToolCount  `json:"top_tools"`
}

// handleSearchSessions searches for text across indexed sessions.
// @Summary Search across indexed sessions
// @Description Search for text within the original session files using the DuckDB index
// @Tags indexer
// @Produce json
// @Param q query string true "Search query text"
// @Param project query string false "Filter by project name (substring match)"
// @Param source query string false "Filter by source (claude, kimi)"
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
	query := r.URL.Query().Get("q")
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

	results, totalMatches, err := indexerSearch(params)
	if err != nil {
		if errors.Is(err, errIndexerUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "indexer_unavailable", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "search_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, SearchResponse{Sessions: results, TotalMatches: totalMatches})
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

// SemanticSearchResponse contains semantic search results.
type SemanticSearchResponse struct {
	Results []search.SemanticResult `json:"results"`
}

// handleSemanticSearch searches by meaning using on-device embeddings.
// @Summary Semantic search across indexed sessions
// @Description Search for sessions by meaning using on-device embeddings. Returns sessions ranked by semantic similarity. Requires the indexer with a synced embedding index.
// @Tags indexer
// @Produce json
// @Param q query string true "Natural language search query"
// @Param project query string false "Filter by project name (substring match)"
// @Param source query string false "Filter by source (claude, kimi, gemini, copilot, codex, qwen)"
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
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	params := rpc.SemanticSearchParams{
		Query:   query,
		Project: r.URL.Query().Get("project"),
		Source:  strings.TrimSpace(strings.ToLower(r.URL.Query().Get("source"))),
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

	writeJSON(w, http.StatusOK, SemanticSearchResponse{Results: results})
}

// handleIndexerHealth returns the health/status of the indexer.
// @Summary Get indexer health status
// @Description Returns whether the indexer binary is available and the database path
// @Tags indexer
// @Produce json
// @Success 200 {object} map[string]any
// @Router /indexer/health [get]
func (s *HTTPServer) handleIndexerHealth(w http.ResponseWriter, r *http.Request) {
	available := rpc.ServerAvailable()

	result := map[string]any{
		"available": available,
	}

	if available {
		data, err := indexerStats()
		if err == nil {
			var stats StatsResponse
			if err := json.Unmarshal(data, &stats); err == nil {
				result["database_accessible"] = true
				result["indexed_projects"] = stats.TotalProjects
				result["indexed_sessions"] = stats.TotalSessions
			} else {
				result["database_accessible"] = false
			}
		} else {
			result["database_accessible"] = false
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// IndexerStatusProgressInfo describes progress for a sync or embedding operation.
type IndexerStatusProgressInfo struct {
	Done         int    `json:"done"`
	Total        int    `json:"total"`
	SessionID    string `json:"session_id,omitempty"`
	Message      string `json:"message,omitempty"`
	Project      int    `json:"project,omitempty"`
	ProjectTotal int    `json:"project_total,omitempty"`
	ProjectName  string `json:"project_name,omitempty"`
	ChunksDone   int    `json:"chunks_done,omitempty"`
	ChunksTotal  int    `json:"chunks_total,omitempty"`
	Entries      int    `json:"entries,omitempty"`
}

// IndexerStatusResponse describes the current state of the indexer server.
type IndexerStatusResponse struct {
	Running       bool                       `json:"running"`
	State         string                     `json:"state"`
	UptimeSeconds int64                      `json:"uptime_seconds,omitempty"`
	Watching      bool                       `json:"watching,omitempty"`
	Model         string                     `json:"model,omitempty"`
	ModelDim      int                        `json:"model_dim,omitempty"`
	SyncProgress  *IndexerStatusProgressInfo `json:"sync_progress,omitempty"`
	EmbedProgress *IndexerStatusProgressInfo `json:"embed_progress,omitempty"`
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
			Running: false,
			State:   "stopped",
		})
		return
	}

	resp, err := rpc.Call("status", nil, nil)
	if err != nil {
		writeJSON(w, http.StatusOK, IndexerStatusResponse{
			Running: false,
			State:   "stopped",
		})
		return
	}
	if !resp.OK {
		writeError(w, http.StatusInternalServerError, "status_failed", resp.Error)
		return
	}

	var status rpc.StatusData
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		writeError(w, http.StatusInternalServerError, "invalid_response", "Failed to parse status")
		return
	}

	out := IndexerStatusResponse{
		Running:       true,
		State:         status.State,
		UptimeSeconds: status.UptimeSeconds,
		Watching:      status.Watching,
		Model:         status.Model,
		ModelDim:      status.ModelDim,
	}
	if status.SyncProgress != nil {
		out.SyncProgress = &IndexerStatusProgressInfo{
			Done:         status.SyncProgress.Done,
			Total:        status.SyncProgress.Total,
			SessionID:    status.SyncProgress.SessionID,
			Message:      status.SyncProgress.Message,
			Project:      status.SyncProgress.Project,
			ProjectTotal: status.SyncProgress.ProjectTotal,
			ProjectName:  status.SyncProgress.ProjectName,
			ChunksDone:   status.SyncProgress.ChunksDone,
			ChunksTotal:  status.SyncProgress.ChunksTotal,
			Entries:      status.SyncProgress.Entries,
		}
	}
	if status.EmbedProgress != nil {
		out.EmbedProgress = &IndexerStatusProgressInfo{
			Done:        status.EmbedProgress.Done,
			Total:       status.EmbedProgress.Total,
			SessionID:   status.EmbedProgress.SessionID,
			ChunksDone:  status.EmbedProgress.ChunksDone,
			ChunksTotal: status.EmbedProgress.ChunksTotal,
			Entries:     status.EmbedProgress.Entries,
		}
	}

	writeJSON(w, http.StatusOK, out)
}
