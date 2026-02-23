package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"os"
	"time"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/fingerprint"
	"github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
	"github.com/wethinkt/go-thinkt/internal/version"
)

// @title Thinkt API
// @version 1.0
// @description API for exploring AI conversation traces from Claude Code, Kimi Code, and other sources.
// @description
// @description ## Authentication
// @description
// @description The API supports Bearer token authentication. When `THINKT_API_TOKEN` environment variable
// @description is set or `--token` flag is provided, all requests must include the token in the
// @description Authorization header:
// @description
// @description     Authorization: Bearer <token>
// @description
// @description Generate a secure token with: `thinkt server token`
// @description
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Bearer token authentication. Format: "Bearer <token>"
// @host localhost:8784
// @BasePath /api/v1

// API response types

// SourcesResponse lists available trace sources.
type SourcesResponse struct {
	Sources []SourceInfo `json:"sources"`
}

// SourceInfo describes a trace source for the API response.
type SourceInfo struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	CanResume bool   `json:"can_resume"`
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

// SessionMetadataResponse contains metadata-only session details and previews.
type SessionMetadataResponse = getSessionMetadataOutput

// ErrorResponse represents an API error.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// SessionResolveResponse contains canonical project/session ownership for a session path.
// Clients can use this to immediately synchronize sidebar/header state for deep-links
// instead of scanning projects and sessions client-side.
type SessionResolveResponse struct {
	ProjectID     string       `json:"project_id"`
	ProjectName   string       `json:"project_name"`
	ProjectSource thinkt.Source `json:"project_source"`
	SessionID     string       `json:"session_id"`
	SessionPath   string       `json:"session_path"`
	WorkspaceID   string       `json:"workspace_id,omitempty"`
}

// ServerInfoResponse contains server identity and runtime metadata.
type ServerInfoResponse struct {
	Fingerprint   string    `json:"fingerprint"`
	Version       string    `json:"version"`
	Revision      string    `json:"revision,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	UptimeSeconds int64     `json:"uptime_seconds"`
	PID           int       `json:"pid"`
	Authenticated bool      `json:"authenticated"`
}

// handleGetInfo returns server identity and runtime metadata.
// @Summary Get server info
// @Description Returns server fingerprint, version, uptime, and authentication status
// @Tags server
// @Produce json
// @Success 200 {object} ServerInfoResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Router /info [get]
// @Security BearerAuth
func (s *HTTPServer) handleGetInfo(w http.ResponseWriter, r *http.Request) {
	fp, _ := fingerprint.GetFingerprint()
	v := version.GetInfo("thinkt")

	writeJSON(w, http.StatusOK, ServerInfoResponse{
		Fingerprint:   fp,
		Version:       v.Version,
		Revision:      v.Revision,
		StartedAt:     s.startedAt,
		UptimeSeconds: int64(time.Since(s.startedAt).Seconds()),
		PID:           os.Getpid(),
		Authenticated: s.authenticator.IsEnabled(),
	})
}

// handleResolveSession resolves a session file path to its canonical project/session ownership.
// @Summary Resolve session ownership
// @Description Resolves an absolute session file path to its canonical project and session metadata.
// @Description Use this for deep-link synchronization: given a session path, immediately know
// @Description which project and source it belongs to without scanning all projects/sessions.
// @Tags sessions
// @Produce json
// @Param path query string true "Absolute session file path"
// @Success 200 {object} SessionResolveResponse
// @Failure 400 {object} ErrorResponse "Missing or invalid path"
// @Failure 404 {object} ErrorResponse "Session not found or not in any registered source"
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 500 {object} ErrorResponse
// @Router /sessions/resolve [get]
// @Security BearerAuth
func (s *HTTPServer) handleResolveSession(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		writeError(w, http.StatusBadRequest, "missing_path", "Query parameter 'path' is required")
		return
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	ctx := r.Context()

	store, meta, err := s.registry.ResolveSessionByPath(ctx, path)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "session_not_found", "No session found at the given path")
			return
		}
		writeError(w, http.StatusInternalServerError, "resolve_failed", err.Error())
		return
	}
	if store == nil || meta == nil {
		writeError(w, http.StatusNotFound, "session_not_found", "No session found at the given path")
		return
	}

	resp := SessionResolveResponse{
		ProjectSource: meta.Source,
		SessionID:     meta.ID,
		SessionPath:   meta.FullPath,
		WorkspaceID:   meta.WorkspaceID,
	}

	// Resolve project details from the session's ProjectPath.
	if meta.ProjectPath != "" {
		project, err := store.GetProject(ctx, meta.ProjectPath)
		if err == nil && project != nil {
			resp.ProjectID = project.ID
			resp.ProjectName = project.Name
		} else {
			// Fallback: use the session's ProjectPath as the project ID.
			resp.ProjectID = meta.ProjectPath
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

type ambiguousProjectSessionsError struct {
	projectID string
	sources   []thinkt.Source
}

func (e *ambiguousProjectSessionsError) Error() string {
	names := make([]string, len(e.sources))
	for i, src := range e.sources {
		names[i] = string(src)
	}
	return fmt.Sprintf("project ID %q is ambiguous across sources: %s", e.projectID, strings.Join(names, ", "))
}

type unknownSourceSessionsError struct {
	source thinkt.Source
}

func (e *unknownSourceSessionsError) Error() string {
	return fmt.Sprintf("unknown source %q", e.source)
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // Ignore encoding error, client connection issues are transient
}

// writeError writes an error response.
func writeError(w http.ResponseWriter, status int, err string, msg string) {
	writeJSON(w, status, ErrorResponse{Error: err, Message: msg})
}

// ActiveSessionsResponse lists currently active sessions.
type ActiveSessionsResponse struct {
	Sessions []thinkt.ActiveSession `json:"sessions"`
}

// handleGetActiveSessions returns currently active sessions detected locally.
// @Summary List active sessions
// @Description Returns sessions detected as currently active via IDE lock files and file mtime heuristics
// @Tags sessions
// @Produce json
// @Success 200 {object} ActiveSessionsResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 500 {object} ErrorResponse
// @Router /sessions/active [get]
func (s *HTTPServer) handleGetActiveSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.activeDetector.Detect(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "detect_active_failed", err.Error())
		return
	}

	if sessions == nil {
		sessions = []thinkt.ActiveSession{}
	}

	writeJSON(w, http.StatusOK, ActiveSessionsResponse{Sessions: sessions})
}

// handleGetSources returns available trace sources.
// @Summary List available trace sources
// @Description Returns all configured trace sources (e.g., Claude Code, Kimi Code)
// @Tags sources
// @Produce json
// @Success 200 {object} SourcesResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Router /sources [get]
// @Security BearerAuth
func (s *HTTPServer) handleGetSources(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := s.registry.SourceStatus(ctx)

	sources := make([]SourceInfo, 0, len(status))
	for _, info := range status {
		sources = append(sources, SourceInfo{
			Name:      string(info.Source),
			Available: info.Available,
			CanResume: info.CanResume,
			BasePath:  info.BasePath,
		})
	}

	writeJSON(w, http.StatusOK, SourcesResponse{Sources: sources})
}

// handleGetProjects returns all projects, optionally filtered by source.
// @Summary List all projects
// @Description Returns all projects from all sources, optionally filtered by source
// @Tags projects
// @Produce json
// @Param source query string false "Filter by source (e.g., 'claude', 'kimi')"
// @Param include_deleted query bool false "Include projects with path_exists=false (default false)"
// @Success 200 {object} ProjectsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 500 {object} ErrorResponse
// @Router /projects [get]
// @Security BearerAuth
func (s *HTTPServer) handleGetProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sourceFilter := r.URL.Query().Get("source")
	includeDeleted := false
	if raw := strings.TrimSpace(r.URL.Query().Get("include_deleted")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_include_deleted", "include_deleted must be true or false")
			return
		}
		includeDeleted = parsed
	}

	projects, err := s.registry.ListAllProjects(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_projects_failed", err.Error())
		return
	}

	filtered := make([]thinkt.Project, 0, len(projects))
	for _, p := range projects {
		if !includeDeleted && p.Path != "" && !p.PathExists {
			continue
		}
		if sourceFilter != "" && string(p.Source) != sourceFilter {
			continue
		}
		filtered = append(filtered, p)
	}

	writeJSON(w, http.StatusOK, ProjectsResponse{Projects: filtered})
}

// handleGetProjectSessionsBySource returns sessions for a project in a specific source.
// @Summary List sessions for a project in a source
// @Description Returns all sessions belonging to a specific project within a specific source
// @Tags sessions
// @Produce json
// @Param source path string true "Source name (e.g., 'claude', 'kimi')"
// @Param projectID path string true "Project ID (URL-encoded path)"
// @Success 200 {object} SessionsResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{source}/{projectID}/sessions [get]
// @Security BearerAuth
func (s *HTTPServer) handleGetProjectSessionsBySource(w http.ResponseWriter, r *http.Request) {
	source := strings.TrimSpace(chi.URLParam(r, "source"))
	if source == "" {
		writeError(w, http.StatusBadRequest, "missing_source", "Source is required")
		return
	}

	projectID, err := decodeProjectIDParam(chi.URLParam(r, "projectID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_project_id", err.Error())
		return
	}

	sessions, err := s.findSessionsForSourceProject(r.Context(), source, projectID)
	if err != nil {
		var unknownSourceErr *unknownSourceSessionsError
		if errors.As(err, &unknownSourceErr) {
			writeError(w, http.StatusBadRequest, "unknown_source", unknownSourceErr.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "list_sessions_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, SessionsResponse{Sessions: sessions})
}

// handleGetProjectSessions returns sessions for a project.
// This endpoint is retained for backward compatibility and supports
// an optional `source` query parameter to disambiguate project IDs.
// @Summary List sessions for a project
// @Description Returns all sessions belonging to a specific project
// @Tags sessions
// @Produce json
// @Param projectID path string true "Project ID (URL-encoded path)"
// @Param source query string false "Source name (recommended to avoid ambiguous project IDs)"
// @Success 200 {object} SessionsResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{projectID}/sessions [get]
// @Security BearerAuth
func (s *HTTPServer) handleGetProjectSessions(w http.ResponseWriter, r *http.Request) {
	projectID, err := decodeProjectIDParam(chi.URLParam(r, "projectID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_project_id", err.Error())
		return
	}

	source := strings.TrimSpace(r.URL.Query().Get("source"))

	var sessions []thinkt.SessionMeta
	if source != "" {
		sessions, err = s.findSessionsForSourceProject(r.Context(), source, projectID)
	} else {
		sessions, err = s.findSessionsForProject(r.Context(), projectID)
	}
	if err != nil {
		var unknownSourceErr *unknownSourceSessionsError
		if errors.As(err, &unknownSourceErr) {
			writeError(w, http.StatusBadRequest, "unknown_source", unknownSourceErr.Error())
			return
		}
		var ambiguousErr *ambiguousProjectSessionsError
		if errors.As(err, &ambiguousErr) {
			writeError(w, http.StatusBadRequest, "ambiguous_project_id", ambiguousErr.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "list_sessions_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, SessionsResponse{Sessions: sessions})
}

func decodeProjectIDParam(rawProjectID string) (string, error) {
	if strings.TrimSpace(rawProjectID) == "" {
		return "", fmt.Errorf("Project ID is required")
	}
	projectID, err := url.PathUnescape(rawProjectID)
	if err != nil {
		return "", err
	}
	return projectID, nil
}

// findSessionsForSourceProject finds sessions for a project in a specific source.
func (s *HTTPServer) findSessionsForSourceProject(ctx context.Context, sourceName, projectID string) ([]thinkt.SessionMeta, error) {
	source := thinkt.Source(strings.ToLower(strings.TrimSpace(sourceName)))
	store, ok := s.registry.Get(source)
	if !ok {
		return nil, &unknownSourceSessionsError{source: source}
	}
	return store.ListSessions(ctx, projectID)
}

// findSessionsForProject finds sessions for a project across all stores.
// If multiple sources contain the same projectID, it returns an ambiguity error.
func (s *HTTPServer) findSessionsForProject(ctx context.Context, projectID string) ([]thinkt.SessionMeta, error) {
	var sources []thinkt.Source
	var matched []thinkt.SessionMeta

	for _, store := range s.registry.All() {
		sessions, err := store.ListSessions(ctx, projectID)
		if err != nil || len(sessions) == 0 {
			continue
		}
		sources = append(sources, store.Source())
		if len(matched) == 0 {
			matched = sessions
		}
	}

	switch len(sources) {
	case 0:
		return []thinkt.SessionMeta{}, nil
	case 1:
		return matched, nil
	default:
		return nil, &ambiguousProjectSessionsError{projectID: projectID, sources: sources}
	}
}

// handleGetSession returns session data.
// @Summary Get session content
// @Description Returns session metadata and entries with optional pagination
// @Tags sessions
// @Produce json
// @Param path path string true "Session file path (URL-encoded)"
// @Param limit query int false "Maximum number of entries to return (0 for all)"
// @Param offset query int false "Number of entries to skip"
// @Success 200 {object} SessionResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /sessions/{path} [get]
// @Security BearerAuth
func (s *HTTPServer) handleGetSession(w http.ResponseWriter, r *http.Request) {
	path, err := extractWildcardPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_path", "Session path is required")
		return
	}

	// Delegate /sessions/<path>/metadata to the metadata-only handler.
	// Chi's wildcard matches the entire suffix, so the metadata route
	// pattern (/sessions/*/metadata) is handled here.
	if sessionPath, ok := strings.CutSuffix(path, "/metadata"); ok {
		rewriteWildcardPath(r, sessionPath)
		s.handleGetSessionMetadata(w, r)
		return
	}

	// Delegate /sessions/<path>/resume to the resume handler.
	// Chi's wildcard matches the entire suffix, so the resume route
	// pattern (/sessions/resume/*) only works when "resume" comes first.
	if sessionPath, ok := strings.CutSuffix(path, "/resume"); ok {
		rewriteWildcardPath(r, sessionPath)
		s.handleResumeSession(w, r)
		return
	}

	// Parse query params
	limit := 0
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be a non-negative integer")
			return
		}
		limit = parsed
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		parsed, err := strconv.Atoi(o)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "invalid_offset", "offset must be a non-negative integer")
			return
		}
		offset = parsed
	}

	// Load the session
	ctx := r.Context()
	session, err := s.loadSession(ctx, path, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load_session_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, session)
}

// handlePostSession dispatches state-changing session sub-actions.
func (s *HTTPServer) handlePostSession(w http.ResponseWriter, r *http.Request) {
	path, err := extractWildcardPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_path", "Session path is required")
		return
	}

	if sessionPath, ok := strings.CutSuffix(path, "/resume"); ok {
		rewriteWildcardPath(r, sessionPath)
		s.handleResumeSessionExec(w, r)
		return
	}

	writeError(w, http.StatusNotFound, "unsupported_action", "Unsupported session action")
}

// handleGetSessionMetadata returns session metadata and lightweight previews
// without returning full entry content.
// @Summary Get session metadata
// @Description Returns session metadata, role counts, and entry previews. Set summary_only=true for quick user-message previews.
// @Tags sessions
// @Produce json
// @Param path path string true "Session file path (URL-encoded)"
// @Param limit query int false "Maximum summaries/previews to return (default 50; default 5 when summary_only=true)"
// @Param offset query int false "Number of summaries/previews to skip"
// @Param exclude_roles query []string false "Roles to exclude (repeat query param or comma-separated). Defaults to checkpoint."
// @Param summary_only query bool false "Return lightweight user-message previews only"
// @Param sort_by query string false "Summary ordering: index (default) or length"
// @Success 200 {object} SessionMetadataResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 500 {object} ErrorResponse
// @Router /sessions/{path}/metadata [get]
// @Security BearerAuth
func (s *HTTPServer) handleGetSessionMetadata(w http.ResponseWriter, r *http.Request) {
	path, err := extractWildcardPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_path", "Session path is required")
		return
	}

	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be an integer")
			return
		}
		if parsed < 0 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be non-negative")
			return
		}
		limit = parsed
	}

	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_offset", "offset must be an integer")
			return
		}
		if parsed < 0 {
			writeError(w, http.StatusBadRequest, "invalid_offset", "offset must be non-negative")
			return
		}
		offset = parsed
	}

	summaryOnly := false
	if raw := strings.TrimSpace(r.URL.Query().Get("summary_only")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_summary_only", "summary_only must be true or false")
			return
		}
		summaryOnly = parsed
	}

	excludeRoles := parseListQueryParam(r.URL.Query(), "exclude_roles")

	input := getSessionMetadataInput{
		Path:         path,
		Limit:        limit,
		Offset:       offset,
		ExcludeRoles: excludeRoles,
		SummaryOnly:  summaryOnly,
		SortBy:       strings.TrimSpace(r.URL.Query().Get("sort_by")),
	}

	output, err := collectSessionMetadata(r.Context(), s.registry, path, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "metadata_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, output)
}

// loadSession loads a session with optional pagination.
func (s *HTTPServer) loadSession(ctx context.Context, path string, limit, offset int) (*SessionResponse, error) {
	ls, err := s.registry.OpenLazySessionByPath(ctx, path)
	if err != nil {
		return nil, err
	}
	defer ls.Close()

	// Load entries based on limit
	if limit > 0 {
		// Load only what we need
		targetBytes := (offset + limit) * 4096 // Rough estimate: 4KB per entry
		if _, err := ls.LoadMore(targetBytes); err != nil {
			return nil, err
		}
	} else {
		// Load all
		if err := ls.LoadAll(); err != nil {
			return nil, err
		}
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
	}, nil
}

// ResumeResponse returns the command to resume a session.
type ResumeResponse struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Dir     string   `json:"dir,omitempty"`
}

// extractWildcardPath extracts and normalises a session path from a chi wildcard route param.
func extractWildcardPath(r *http.Request) (string, error) {
	path := chi.URLParam(r, "*")
	if path == "" {
		return "", fmt.Errorf("missing_path")
	}
	decoded, err := url.PathUnescape(path)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(decoded, "/") {
		decoded = "/" + decoded
	}
	return decoded, nil
}

func rewriteWildcardPath(r *http.Request, path string) {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil || len(rctx.URLParams.Values) == 0 {
		return
	}
	rctx.URLParams.Values[len(rctx.URLParams.Values)-1] = path
}

func parseListQueryParam(values url.Values, key string) []string {
	raw := values[key]
	if len(raw) == 0 {
		return nil
	}

	parsed := make([]string, 0, len(raw))
	for _, item := range raw {
		for _, piece := range strings.Split(item, ",") {
			value := strings.TrimSpace(piece)
			if value != "" {
				parsed = append(parsed, value)
			}
		}
	}
	if len(parsed) == 0 {
		return nil
	}
	return parsed
}

// resolveResumeInfo resolves a session path to its ResumeInfo.
func (s *HTTPServer) resolveResumeInfo(ctx context.Context, path string) (*thinkt.ResumeInfo, error) {
	_, meta, err := s.registry.ResolveSessionByPath(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("session_not_found: %w", err)
	}
	if meta == nil {
		return nil, fmt.Errorf("session_not_found")
	}

	resumer, ok := s.registry.GetResumer(meta.Source)
	if !ok {
		return nil, fmt.Errorf("resume_not_supported")
	}

	return resumer.ResumeCommand(*meta)
}

// handleResumeSession returns the command to resume a session.
// @Summary Get resume command for a session
// @Description Returns the command, arguments, and working directory needed to resume a session.
// @Tags sessions
// @Produce json
// @Param path path string true "Session file path (URL-encoded)"
// @Success 200 {object} ResumeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse "Source does not support resume"
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 405 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /sessions/{path}/resume [get]
// @Security BearerAuth
func (s *HTTPServer) handleResumeSession(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("action") == "exec" {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST /api/v1/sessions/{path}/resume to execute the resume command")
		return
	}

	path, err := extractWildcardPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_path", "Session path is required")
		return
	}

	info, err := s.resolveResumeInfo(r.Context(), path)
	if err != nil {
		writeResumeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, ResumeResponse{
		Command: info.Command,
		Args:    info.Args,
		Dir:     info.Dir,
	})
}

// handleResumeSessionExec executes the resume command in a terminal.
// @Summary Execute resume command for a session
// @Description Executes the resume command in a terminal for the target session.
// @Description This endpoint requires POST and rejects cross-origin browser requests.
// @Tags sessions
// @Produce json
// @Param path path string true "Session file path (URL-encoded)"
// @Success 200 {object} OpenInResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse "Source does not support resume"
// @Failure 500 {object} ErrorResponse
// @Router /sessions/{path}/resume [post]
// @Security BearerAuth
func (s *HTTPServer) handleResumeSessionExec(w http.ResponseWriter, r *http.Request) {
	path, err := extractWildcardPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_path", "Session path is required")
		return
	}

	if err := requireSameOriginForStateChange(r); err != nil {
		writeError(w, http.StatusForbidden, "forbidden", "Cross-origin requests are not allowed for this endpoint")
		return
	}

	info, err := s.resolveResumeInfo(r.Context(), path)
	if err != nil {
		writeResumeError(w, err)
		return
	}

	if err := spawnInTerminal(info); err != nil {
		writeError(w, http.StatusInternalServerError, "exec_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, OpenInResponse{
		Success: true,
		Message: "Resumed session in terminal",
	})
}

func requireSameOriginForStateChange(r *http.Request) error {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if origin == "" && referer == "" {
		// Allow non-browser and CLI clients that do not send origin metadata.
		return nil
	}

	expected := requestOrigin(r)
	if origin != "" {
		if sameOrigin(origin, expected) {
			return nil
		}
		return fmt.Errorf("origin mismatch")
	}

	refURL, err := url.Parse(referer)
	if err != nil || refURL.Scheme == "" || refURL.Host == "" {
		return fmt.Errorf("invalid referer")
	}
	if sameOrigin(refURL.Scheme+"://"+refURL.Host, expected) {
		return nil
	}
	return fmt.Errorf("referer mismatch")
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func sameOrigin(a, b string) bool {
	au, err := url.Parse(strings.TrimSpace(a))
	if err != nil || au.Scheme == "" || au.Host == "" {
		return false
	}
	bu, err := url.Parse(strings.TrimSpace(b))
	if err != nil || bu.Scheme == "" || bu.Host == "" {
		return false
	}
	return strings.EqualFold(au.Scheme, bu.Scheme) && strings.EqualFold(au.Host, bu.Host)
}

// writeResumeError maps resolveResumeInfo errors to appropriate HTTP responses.
func writeResumeError(w http.ResponseWriter, err error) {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "session_not_found"):
		writeError(w, http.StatusNotFound, "session_not_found", "No session found at path")
	case strings.HasPrefix(msg, "resume_not_supported"):
		writeError(w, http.StatusNotFound, "resume_not_supported", "Source does not support session resume")
	default:
		writeError(w, http.StatusInternalServerError, "resume_failed", msg)
	}
}

// spawnInTerminal opens the configured terminal app and runs the resume command.
func spawnInTerminal(info *thinkt.ResumeInfo) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	app := cfg.GetTerminalApp()
	if app == nil {
		return fmt.Errorf("no terminal app configured or available")
	}

	quotedArgs := make([]string, len(info.Args))
	for i, arg := range info.Args {
		quotedArgs[i] = quoteShell(arg)
	}
	shellCmd := strings.Join(quotedArgs, " ")
	if info.Dir != "" {
		shellCmd = fmt.Sprintf("cd %s && %s", quoteShell(info.Dir), shellCmd)
	}

	return app.LaunchCommand(shellCmd)
}

func quoteShell(s string) string {
	if runtime.GOOS == "windows" {
		// cmd.exe uses double quotes; escape embedded double quotes by doubling.
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// OpenInRequest is the request body for the open-in endpoint.
type OpenInRequest struct {
	App  string `json:"app"`  // App ID (e.g., "finder", "vscode", "cursor")
	Path string `json:"path"` // Path to open
}

// OpenInResponse is the response for the open-in endpoint.
type OpenInResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// AllowedAppsResponse lists enabled apps for the open-in feature.
type AllowedAppsResponse struct {
	Apps            []config.AppInfo `json:"apps"`
	DefaultTerminal string           `json:"default_terminal"`
}

// handleOpenIn opens a path in the specified application.
// @Summary Open a path in an application
// @Description Opens the specified path in an allowed application (e.g., Finder, VS Code)
// @Tags open-in
// @Accept json
// @Produce json
// @Param request body OpenInRequest true "Open-in request"
// @Success 200 {object} OpenInResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /open-in [post]
// @Security BearerAuth
func (s *HTTPServer) handleOpenIn(w http.ResponseWriter, r *http.Request) {
	var req OpenInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if req.App == "" {
		writeError(w, http.StatusBadRequest, "missing_app", "App ID is required")
		return
	}

	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "missing_path", "Path is required")
		return
	}

	// Load config and find the app
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}

	app := cfg.GetApp(req.App)
	if app == nil {
		writeError(w, http.StatusForbidden, "app_not_allowed",
			"App '"+req.App+"' is not enabled or not configured")
		return
	}

	// Validate the path for security
	validatedPath, err := s.pathValidator.ValidateOpenInPath(req.Path)
	if err != nil {
		writeError(w, http.StatusForbidden, "invalid_path", err.Error())
		return
	}

	// Launch the application
	if err := app.Launch(validatedPath); err != nil {
		writeError(w, http.StatusInternalServerError, "exec_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, OpenInResponse{
		Success: true,
		Message: "Opened in " + app.Name,
	})
}

// handleGetAllowedApps returns the list of enabled apps for open-in.
// @Summary List allowed apps for open-in
// @Description Returns the list of enabled applications that can be used with open-in
// @Tags open-in
// @Produce json
// @Success 200 {object} AllowedAppsResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Router /open-in/apps [get]
// @Security BearerAuth
func (s *HTTPServer) handleGetAllowedApps(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}

	defaultTerminal := "terminal"
	if t := cfg.GetTerminalApp(); t != nil {
		defaultTerminal = t.ID
	}
	writeJSON(w, http.StatusOK, AllowedAppsResponse{
		Apps:            cfg.GetEnabledApps(),
		DefaultTerminal: defaultTerminal,
	})
}

// ThemeStyle is the API representation of a theme style.
type ThemeStyle struct {
	Fg        string `json:"fg,omitempty"`
	Bg        string `json:"bg,omitempty"`
	Bold      bool   `json:"bold,omitempty"`
	Italic    bool   `json:"italic,omitempty"`
	Underline bool   `json:"underline,omitempty"`
}

// ThemeColors contains all the color/style definitions for a theme.
type ThemeColors struct {
	// UI chrome
	Accent         string `json:"accent,omitempty"`
	BorderActive   string `json:"border_active,omitempty"`
	BorderInactive string `json:"border_inactive,omitempty"`

	// Text styles
	TextPrimary   ThemeStyle `json:"text_primary,omitempty"`
	TextSecondary ThemeStyle `json:"text_secondary,omitempty"`
	TextMuted     ThemeStyle `json:"text_muted,omitempty"`

	// Conversation blocks
	UserBlock       ThemeStyle `json:"user_block,omitempty"`
	AssistantBlock  ThemeStyle `json:"assistant_block,omitempty"`
	ThinkingBlock   ThemeStyle `json:"thinking_block,omitempty"`
	ToolCallBlock   ThemeStyle `json:"tool_call_block,omitempty"`
	ToolResultBlock ThemeStyle `json:"tool_result_block,omitempty"`

	// Labels
	UserLabel      ThemeStyle `json:"user_label,omitempty"`
	AssistantLabel ThemeStyle `json:"assistant_label,omitempty"`
	ThinkingLabel  ThemeStyle `json:"thinking_label,omitempty"`
	ToolLabel      ThemeStyle `json:"tool_label,omitempty"`

	// Confirm dialog
	ConfirmPrompt     ThemeStyle `json:"confirm_prompt,omitempty"`
	ConfirmSelected   ThemeStyle `json:"confirm_selected,omitempty"`
	ConfirmUnselected ThemeStyle `json:"confirm_unselected,omitempty"`
}

// ThemeInfo is the API representation of a theme.
type ThemeInfo struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Embedded    bool        `json:"embedded"`
	Active      bool        `json:"active"`
	Colors      ThemeColors `json:"colors"`
}

// ThemesResponse lists available themes.
type ThemesResponse struct {
	Themes []ThemeInfo `json:"themes"`
	Active string      `json:"active"`
}

// LanguagesResponse lists available languages.
type LanguagesResponse struct {
	Languages []i18n.LangInfo `json:"languages"`
	Active    string          `json:"active"`
}

// handleGetThemes returns the list of available themes.
// @Summary List available themes
// @Description Returns all available themes (built-in and user themes) with their color definitions
// @Tags themes
// @Produce json
// @Success 200 {object} ThemesResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Router /themes [get]
// @Security BearerAuth
func (s *HTTPServer) handleGetThemes(w http.ResponseWriter, r *http.Request) {
	available, err := theme.ListAvailable()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "themes_error", err.Error())
		return
	}

	activeName := theme.ActiveName()

	themes := make([]ThemeInfo, len(available))
	for i, t := range available {
		// Load the full theme to get colors
		fullTheme, err := theme.LoadByName(t.Name)
		if err != nil {
			// Skip themes that fail to load
			continue
		}

		themes[i] = ThemeInfo{
			Name:        t.Name,
			Description: t.Description,
			Embedded:    t.Embedded,
			Active:      t.Name == activeName,
			Colors:      themeToColors(fullTheme),
		}
	}

	writeJSON(w, http.StatusOK, ThemesResponse{
		Themes: themes,
		Active: activeName,
	})
}

// handleGetLanguages returns the list of available languages.
// @Summary List available languages
// @Description Returns all supported languages and the currently active one
// @Tags languages
// @Produce json
// @Success 200 {object} LanguagesResponse
// @Router /languages [get]
func (s *HTTPServer) handleGetLanguages(w http.ResponseWriter, r *http.Request) {
	active := i18n.ActiveTag()
	writeJSON(w, http.StatusOK, LanguagesResponse{
		Languages: i18n.AvailableLanguages(active),
		Active:    active,
	})
}

// themeToColors converts a theme.Theme to ThemeColors for the API.
func themeToColors(t theme.Theme) ThemeColors {
	return ThemeColors{
		Accent:         t.Accent,
		BorderActive:   t.BorderActive,
		BorderInactive: t.BorderInactive,

		TextPrimary:   styleToAPI(t.TextPrimary),
		TextSecondary: styleToAPI(t.TextSecondary),
		TextMuted:     styleToAPI(t.TextMuted),

		UserBlock:       styleToAPI(t.UserBlock),
		AssistantBlock:  styleToAPI(t.AssistantBlock),
		ThinkingBlock:   styleToAPI(t.ThinkingBlock),
		ToolCallBlock:   styleToAPI(t.ToolCallBlock),
		ToolResultBlock: styleToAPI(t.ToolResultBlock),

		UserLabel:      styleToAPI(t.UserLabel),
		AssistantLabel: styleToAPI(t.AssistantLabel),
		ThinkingLabel:  styleToAPI(t.ThinkingLabel),
		ToolLabel:      styleToAPI(t.ToolLabel),

		ConfirmPrompt:     styleToAPI(t.ConfirmPrompt),
		ConfirmSelected:   styleToAPI(t.ConfirmSelected),
		ConfirmUnselected: styleToAPI(t.ConfirmUnselected),
	}
}

// styleToAPI converts a theme.Style to ThemeStyle for the API.
func styleToAPI(s theme.Style) ThemeStyle {
	return ThemeStyle{
		Fg:        s.Fg,
		Bg:        s.Bg,
		Bold:      s.Bold,
		Italic:    s.Italic,
		Underline: s.Underline,
	}
}
