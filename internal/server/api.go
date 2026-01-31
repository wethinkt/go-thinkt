package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tui"
)

// API response types

// SourcesResponse lists available trace sources.
type SourcesResponse struct {
	Sources []APISourceInfo `json:"sources"`
}

// APISourceInfo describes a trace source for the API response.
type APISourceInfo struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	BasePath  string `json:"base_path,omitempty"`
}

// ProjectsResponse lists projects.
type ProjectsResponse struct {
	Projects []thinkt.Project `json:"projects"`
}

// SessionsResponse lists sessions.
type SessionsResponse struct {
	Sessions []thinkt.SessionMeta `json:"sessions"`
}

// SessionResponse contains session data.
type SessionResponse struct {
	Meta    thinkt.SessionMeta `json:"meta"`
	Entries []thinkt.Entry     `json:"entries"`
	HasMore bool               `json:"has_more"`
	Total   int                `json:"total,omitempty"`
}

// ErrorResponse represents an API error.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes an error response.
func writeError(w http.ResponseWriter, status int, err string, msg string) {
	writeJSON(w, status, ErrorResponse{Error: err, Message: msg})
}

// handleGetSources returns available trace sources.
func (s *HTTPServer) handleGetSources(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := s.registry.SourceStatus(ctx)

	sources := make([]APISourceInfo, 0, len(status))
	for _, info := range status {
		sources = append(sources, APISourceInfo{
			Name:      string(info.Source),
			Available: info.Available,
			BasePath:  info.BasePath,
		})
	}

	writeJSON(w, http.StatusOK, SourcesResponse{Sources: sources})
}

// handleGetProjects returns all projects, optionally filtered by source.
func (s *HTTPServer) handleGetProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sourceFilter := r.URL.Query().Get("source")

	projects, err := s.registry.ListAllProjects(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_projects_failed", err.Error())
		return
	}

	// Filter by source if specified
	if sourceFilter != "" {
		filtered := make([]thinkt.Project, 0)
		for _, p := range projects {
			if string(p.Source) == sourceFilter {
				filtered = append(filtered, p)
			}
		}
		projects = filtered
	}

	writeJSON(w, http.StatusOK, ProjectsResponse{Projects: projects})
}

// handleGetProjectSessions returns sessions for a project.
func (s *HTTPServer) handleGetProjectSessions(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "missing_project_id", "Project ID is required")
		return
	}

	// URL decode the project ID (it may contain path characters)
	decoded, err := url.PathUnescape(projectID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_project_id", err.Error())
		return
	}
	projectID = decoded

	ctx := r.Context()

	// Find sessions across all stores
	sessions, err := s.findSessionsForProject(ctx, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_sessions_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, SessionsResponse{Sessions: sessions})
}

// findSessionsForProject finds sessions for a project across all stores.
func (s *HTTPServer) findSessionsForProject(ctx context.Context, projectID string) ([]thinkt.SessionMeta, error) {
	var allSessions []thinkt.SessionMeta

	for _, store := range s.registry.All() {
		sessions, err := store.ListSessions(ctx, projectID)
		if err != nil {
			// Skip stores that don't have this project
			continue
		}
		allSessions = append(allSessions, sessions...)
	}

	return allSessions, nil
}

// handleGetSession returns session data.
// Path format: /api/v1/sessions/{encoded_path}
// Query params: limit, offset
func (s *HTTPServer) handleGetSession(w http.ResponseWriter, r *http.Request) {
	// Get the path after /sessions/
	path := chi.URLParam(r, "*")
	if path == "" {
		writeError(w, http.StatusBadRequest, "missing_path", "Session path is required")
		return
	}

	// URL decode
	decoded, err := url.PathUnescape(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_path", err.Error())
		return
	}
	path = decoded

	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Parse query params
	limit := 0
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		limit, _ = strconv.Atoi(l)
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		offset, _ = strconv.Atoi(o)
	}

	// Load the session
	ctx := r.Context()
	session, hasMore, err := s.loadSession(ctx, path, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load_session_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, session)
	_ = hasMore // used in response
}

// loadSession loads a session with optional pagination.
func (s *HTTPServer) loadSession(ctx context.Context, path string, limit, offset int) (*SessionResponse, bool, error) {
	// Try to open the session using the TUI session loader logic
	ls, err := openLazySession(path)
	if err != nil {
		return nil, false, err
	}
	defer ls.Close()

	// Load entries based on limit
	if limit > 0 {
		// Load only what we need
		targetBytes := (offset + limit) * 4096 // Rough estimate: 4KB per entry
		ls.LoadMore(targetBytes)
	} else {
		// Load all
		ls.LoadAll()
	}

	entries := ls.Entries()
	total := len(entries)
	hasMore := ls.HasMore()

	// Apply offset and limit
	if offset > 0 {
		if offset >= len(entries) {
			entries = nil
		} else {
			entries = entries[offset:]
		}
	}
	if limit > 0 && limit < len(entries) {
		entries = entries[:limit]
		hasMore = true
	}

	return &SessionResponse{
		Meta:    ls.Metadata(),
		Entries: entries,
		HasMore: hasMore,
		Total:   total,
	}, hasMore, nil
}

// openLazySession opens a session file, auto-detecting format.
func openLazySession(path string) (thinkt.LazySession, error) {
	// Use the TUI session loader which handles both Claude and Kimi formats
	return tui.OpenLazySession(path)
}
