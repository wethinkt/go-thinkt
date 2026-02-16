package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
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
// @description Generate a secure token with: `thinkt serve token`
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
// @Success 200 {object} ProjectsResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 500 {object} ErrorResponse
// @Router /projects [get]
// @Security BearerAuth
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
// @Summary List sessions for a project
// @Description Returns all sessions belonging to a specific project
// @Tags sessions
// @Produce json
// @Param projectID path string true "Project ID (URL-encoded path)"
// @Success 200 {object} SessionsResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{projectID}/sessions [get]
// @Security BearerAuth
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

	// Delegate /sessions/<path>/resume to the resume handler.
	// Chi's wildcard matches the entire suffix, so the resume route
	// pattern (/sessions/resume/*) only works when "resume" comes first.
	if sessionPath, ok := strings.CutSuffix(path, "/resume"); ok {
		s.doResumeSession(w, r, sessionPath)
		return
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
	session, err := s.loadSession(ctx, path, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load_session_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, session)
}

// loadSession loads a session with optional pagination.
func (s *HTTPServer) loadSession(ctx context.Context, path string, limit, offset int) (*SessionResponse, error) {
	ls, err := s.registry.OpenLazySessionByPath(ctx, path)
	if err != nil {
		// Fallback for direct file paths not discovered via registry.
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		ls, err = tui.OpenLazySession(path)
		if err != nil {
			return nil, err
		}
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
	}, nil
}

// ResumeResponse returns the command to resume a session.
type ResumeResponse struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Dir     string   `json:"dir,omitempty"`
}

// handleResumeSession returns the command to resume a session in its original CLI tool.
// @Summary Get resume command for a session
// @Description Returns the command, arguments, and working directory needed to resume a session in its original CLI tool (e.g., claude --resume)
// @Tags sessions
// @Produce json
// @Param path path string true "Session file path (URL-encoded)"
// @Success 200 {object} ResumeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse "Source does not support resume"
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 500 {object} ErrorResponse
// @Router /sessions/{path}/resume [get]
// @Security BearerAuth
func (s *HTTPServer) handleResumeSession(w http.ResponseWriter, r *http.Request) {
	path := chi.URLParam(r, "*")
	if path == "" {
		writeError(w, http.StatusBadRequest, "missing_path", "Session path is required")
		return
	}

	decoded, err := url.PathUnescape(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_path", err.Error())
		return
	}
	path = decoded
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	s.doResumeSession(w, r, path)
}

// doResumeSession resolves a session by path and returns its resume command.
func (s *HTTPServer) doResumeSession(w http.ResponseWriter, r *http.Request, path string) {
	ctx := r.Context()
	_, meta, err := s.registry.ResolveSessionByPath(ctx, path)
	if err != nil || meta == nil {
		writeError(w, http.StatusNotFound, "session_not_found", "No session found at path")
		return
	}

	resumer, ok := s.registry.GetResumer(meta.Source)
	if !ok {
		writeError(w, http.StatusNotFound, "resume_not_supported", "Source does not support session resume")
		return
	}

	info, err := resumer.ResumeCommand(*meta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "resume_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ResumeResponse{
		Command: info.Command,
		Args:    info.Args,
		Dir:     info.Dir,
	})
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
	Apps []config.AppInfo `json:"apps"`
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

	writeJSON(w, http.StatusOK, AllowedAppsResponse{Apps: cfg.GetEnabledApps()})
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
