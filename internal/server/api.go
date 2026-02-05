package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os/exec"
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
// @host localhost:7433
// @BasePath /api/v1

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
// @Summary List available trace sources
// @Description Returns all configured trace sources (e.g., Claude Code, Kimi Code)
// @Tags sources
// @Produce json
// @Success 200 {object} SourcesResponse
// @Router /sources [get]
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
// @Summary List all projects
// @Description Returns all projects from all sources, optionally filtered by source
// @Tags projects
// @Produce json
// @Param source query string false "Filter by source (e.g., 'claude', 'kimi')"
// @Success 200 {object} ProjectsResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects [get]
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{projectID}/sessions [get]
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /sessions/{path} [get]
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
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /open-in [post]
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

	// Build the command using the app's Exec template with the validated path
	cmdName, args := app.BuildCommand(validatedPath)
	if cmdName == "" {
		writeError(w, http.StatusInternalServerError, "invalid_config", "App has no exec command")
		return
	}
	cmd := exec.Command(cmdName, args...)

	// Execute the command
	if err := cmd.Start(); err != nil {
		writeError(w, http.StatusInternalServerError, "exec_error", err.Error())
		return
	}

	// Don't wait for the command to finish - it's opening an external app
	go cmd.Wait()

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
// @Router /open-in/apps [get]
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
// @Router /themes [get]
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
