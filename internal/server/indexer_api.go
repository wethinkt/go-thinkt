package server

import (
	"encoding/json"
	"net/http"
	"os/exec"
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


