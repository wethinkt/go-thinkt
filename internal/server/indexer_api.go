package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
)

// SearchMatch represents a single search match within a session.
type SearchMatch struct {
	LineNum int    `json:"line_num"`
	Preview string `json:"preview"`
	Role    string `json:"role"`
}

// SearchSessionResult represents search results for a single session.
type SearchSessionResult struct {
	SessionID   string        `json:"session_id"`
	ProjectName string        `json:"project_name"`
	Source      string        `json:"source"`
	Path        string        `json:"path"`
	Matches     []SearchMatch `json:"matches"`
}

// SearchResponse contains the results of a search query.
type SearchResponse struct {
	Sessions     []SearchSessionResult `json:"sessions"`
	TotalMatches int                   `json:"total_matches"`
}

// StatsResponse contains usage statistics from the index.
type StatsResponse struct {
	TotalProjects int            `json:"total_projects"`
	TotalSessions int            `json:"total_sessions"`
	TotalEntries  int            `json:"total_entries"`
	TotalTokens   int            `json:"total_tokens"`
	ToolUsage     map[string]int `json:"tool_usage"`
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

	indexerPath := findIndexerBinary()
	if indexerPath == "" {
		writeError(w, http.StatusServiceUnavailable, "indexer_not_found", "thinkt-indexer binary not found")
		return
	}

	args := []string{"search", "--json", query}

	if project := r.URL.Query().Get("project"); project != "" {
		args = append(args, "--project", project)
	}
	if source := r.URL.Query().Get("source"); source != "" {
		args = append(args, "--source", source)
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		args = append(args, "--limit", limit)
	}
	if limitPerSess := r.URL.Query().Get("limit_per_session"); limitPerSess != "" {
		args = append(args, "--limit-per-session", limitPerSess)
	}
	if r.URL.Query().Get("case_sensitive") == "true" {
		args = append(args, "--case-sensitive")
	}
	if r.URL.Query().Get("regex") == "true" {
		args = append(args, "--regex")
	}

	cmd := exec.Command(indexerPath, args...)
	out, err := cmd.Output()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search_failed", err.Error())
		return
	}

	var result SearchResponse
	if err := json.Unmarshal(out, &result); err != nil {
		writeError(w, http.StatusInternalServerError, "invalid_response", "Failed to parse indexer output")
		return
	}

	writeJSON(w, http.StatusOK, result)
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
	indexerPath := findIndexerBinary()
	if indexerPath == "" {
		writeError(w, http.StatusServiceUnavailable, "indexer_not_found", "thinkt-indexer binary not found")
		return
	}

	cmd := exec.Command(indexerPath, "stats", "--json")
	out, err := cmd.Output()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stats_failed", err.Error())
		return
	}

	var result StatsResponse
	if err := json.Unmarshal(out, &result); err != nil {
		writeError(w, http.StatusInternalServerError, "invalid_response", "Failed to parse indexer output")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// SemanticSearchResult represents a single semantic search hit.
type SemanticSearchResult struct {
	SessionID   string  `json:"session_id"`
	EntryUUID   string  `json:"entry_uuid"`
	ChunkIndex  int     `json:"chunk_index"`
	TotalChunks int     `json:"total_chunks"`
	Distance    float64 `json:"distance"`
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

// SemanticSearchResponse contains semantic search results.
type SemanticSearchResponse struct {
	Results []SemanticSearchResult `json:"results"`
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

	indexerPath := findIndexerBinary()
	if indexerPath == "" {
		writeError(w, http.StatusServiceUnavailable, "indexer_not_found", "thinkt-indexer binary not found")
		return
	}

	args := []string{"semantic", "search", "--json", query}

	if project := r.URL.Query().Get("project"); project != "" {
		args = append(args, "--project", project)
	}
	if source := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("source"))); source != "" {
		args = append(args, "--source", source)
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		args = append(args, "--limit", limit)
	}
	if maxDist := r.URL.Query().Get("max_distance"); maxDist != "" {
		args = append(args, "--max-distance", maxDist)
	}
	if r.URL.Query().Get("diversity") == "true" {
		args = append(args, "--diversity")
	}

	cmd := exec.Command(indexerPath, args...)
	out, err := cmd.Output()
	if err != nil {
		errMsg := err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			errMsg = strings.TrimSpace(string(exitErr.Stderr))
		}
		writeError(w, http.StatusInternalServerError, "semantic_search_failed", errMsg)
		return
	}

	var results []SemanticSearchResult
	if err := json.Unmarshal(out, &results); err != nil {
		writeError(w, http.StatusInternalServerError, "invalid_response",
			fmt.Sprintf("Failed to parse indexer output: %s", strings.TrimSpace(string(out))))
		return
	}
	if results == nil {
		results = []SemanticSearchResult{}
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
	indexerPath := findIndexerBinary()

	result := map[string]any{
		"available": indexerPath != "",
		"path":      indexerPath,
	}

	if indexerPath != "" {
		// Try to get stats to verify DB is accessible
		cmd := exec.Command(indexerPath, "stats", "--json")
		out, err := cmd.Output()
		if err == nil {
			var stats StatsResponse
			if err := json.Unmarshal(out, &stats); err == nil {
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
