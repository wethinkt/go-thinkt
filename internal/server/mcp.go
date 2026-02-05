package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// MCPServer wraps an MCP server for thinkt.
type MCPServer struct {
	server        *mcp.Server
	registry      *thinkt.StoreRegistry
	authenticator *MCPAuthenticator
}

// NewMCPServer creates a new MCP server with thinkt tools.
func NewMCPServer(registry *thinkt.StoreRegistry) *MCPServer {
	return NewMCPServerWithAuth(registry, DefaultMCPAuthConfig())
}

// NewMCPServerWithAuth creates a new MCP server with authentication.
func NewMCPServerWithAuth(registry *thinkt.StoreRegistry, authConfig MCPAuthConfig) *MCPServer {
	tuilog.Log.Info("NewMCPServer: creating MCP server", "auth_mode", authConfig.Mode)
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "thinkt",
		Version: "1.0.0",
	}, nil)

	ms := &MCPServer{
		server:        server,
		registry:      registry,
		authenticator: NewMCPAuthenticator(authConfig),
	}

	// Register tools
	tuilog.Log.Info("NewMCPServer: registering tools")
	ms.registerTools()
	tuilog.Log.Info("NewMCPServer: server created successfully")

	return ms
}

// registerTools adds all thinkt tools to the MCP server.
func (ms *MCPServer) registerTools() {
	// list_sources - List available trace sources
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "list_sources",
		Description: "List available trace sources (e.g., kimi, claude)",
	}, ms.handleListSources)

	// list_projects - List all projects
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "list_projects",
		Description: "List all projects across all sources, optionally filtered by source",
	}, ms.handleListProjects)

	// list_sessions - List sessions for a project
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "list_sessions",
		Description: "List sessions for a specific project",
	}, ms.handleListSessions)

	// get_session_metadata - Get session metadata without full content
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "get_session_metadata",
		Description: "Get session metadata and entry summaries without loading full content. Returns description, entry count by role, and a lightweight summary of each entry (index, role, timestamp, content_length, has_thinking, has_tool_use). Use this first to understand a session before fetching specific entries.",
	}, ms.handleGetSessionMetadata)

	// get_session_entries - Get session entries with tight controls
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "get_session_entries",
		Description: "Get session entry content with pagination and filtering. Defaults: limit=5, max_content_length=500, include_thinking=false. Use entry_indices to fetch specific entries by index, or roles to filter by role type.",
	}, ms.handleGetSessionEntries)
}

// Tool input/output types

type listSourcesInput struct{}

type sourceInfo struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	BasePath  string `json:"base_path,omitempty"`
}

type listSourcesOutput struct {
	Sources []sourceInfo `json:"sources"`
}

type listProjectsInput struct {
	Source string `json:"source,omitempty"` // Filter by source (kimi or claude)
}

type listProjectsOutput struct {
	Projects []projectInfo `json:"projects"`
}

type projectInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	SessionCount int    `json:"session_count"`
	Source       string `json:"source"`
	PathExists   bool   `json:"path_exists"`
}

type listSessionsInput struct {
	ProjectID string `json:"project_id"` // The project ID to list sessions for (required)
}

type listSessionsOutput struct {
	Sessions []sessionInfo `json:"sessions"`
}

type sessionInfo struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	CreatedAt  string `json:"created_at,omitempty"`
	ModifiedAt string `json:"modified_at,omitempty"`
	EntryCount int    `json:"entry_count"`
	FileSize   int64  `json:"file_size"`
	Source     string `json:"source"`
}

// get_session_metadata types

type getSessionMetadataInput struct {
	Path string `json:"path"` // Full path to the session file (required)
}

type getSessionMetadataOutput struct {
	Meta         sessionMetaInfo  `json:"meta"`
	Description  string           `json:"description,omitempty"`  // First user prompt or extracted description
	RoleCounts   map[string]int   `json:"role_counts"`            // Count of entries by role
	EntrySummary []entrySummary   `json:"entry_summary"`          // Lightweight summary of each entry
	TotalEntries int              `json:"total_entries"`
	TotalBytes   int              `json:"total_content_bytes"`    // Approximate total content size
}

type sessionMetaInfo struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	CreatedAt  string `json:"created_at,omitempty"`
	ModifiedAt string `json:"modified_at,omitempty"`
	Model      string `json:"model,omitempty"`
	GitBranch  string `json:"git_branch,omitempty"`
	Source     string `json:"source,omitempty"`
}

type entrySummary struct {
	Index         int    `json:"index"`
	Role          string `json:"role"`
	Timestamp     string `json:"timestamp,omitempty"`
	ContentLength int    `json:"content_length"`     // Approximate size in bytes
	HasThinking   bool   `json:"has_thinking"`       // Contains thinking blocks
	HasToolUse    bool   `json:"has_tool_use"`       // Contains tool_use blocks
	HasToolResult bool   `json:"has_tool_result"`    // Contains tool_result blocks
	Preview       string `json:"preview,omitempty"`  // First 100 chars of text content
}

// get_session_entries types

type getSessionEntriesInput struct {
	Path             string   `json:"path"`                        // Full path to the session file (required)
	Limit            int      `json:"limit,omitempty"`             // Max entries to return (default: 5)
	Offset           int      `json:"offset,omitempty"`            // Number of entries to skip (default: 0)
	EntryIndices     []int    `json:"entry_indices,omitempty"`     // Specific entry indices to fetch (overrides limit/offset)
	Roles            []string `json:"roles,omitempty"`             // Filter to specific roles (e.g., ["user", "assistant"])
	MaxContentLength int      `json:"max_content_length,omitempty"` // Truncate text content (default: 500, 0 for no limit)
	IncludeThinking  bool     `json:"include_thinking,omitempty"`  // Include thinking blocks (default: false)
}

type getSessionEntriesOutput struct {
	Entries  []entryContent `json:"entries"`
	HasMore  bool           `json:"has_more"`
	Total    int            `json:"total"`           // Total entries in session
	Returned int            `json:"returned"`        // Number of entries returned
}

type entryContent struct {
	Index         int                  `json:"index"`
	UUID          string               `json:"uuid"`
	Role          string               `json:"role"`
	Timestamp     string               `json:"timestamp,omitempty"`
	Text          string               `json:"text,omitempty"`           // Main text content (possibly truncated)
	TextTruncated bool                 `json:"text_truncated,omitempty"` // True if text was truncated
	Thinking      string               `json:"thinking,omitempty"`       // Thinking content (if include_thinking=true)
	ToolUses      []toolUseInfo        `json:"tool_uses,omitempty"`      // Tool use blocks
	ToolResults   []toolResultInfo     `json:"tool_results,omitempty"`   // Tool result blocks
	Model         string               `json:"model,omitempty"`
}

type toolUseInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input,omitempty"` // JSON string of input (possibly truncated)
}

type toolResultInfo struct {
	ToolUseID string `json:"tool_use_id"`
	Result    string `json:"result,omitempty"` // Result text (possibly truncated)
	IsError   bool   `json:"is_error,omitempty"`
}

// Tool handlers

func (ms *MCPServer) handleListSources(ctx context.Context, req *mcp.CallToolRequest, _ listSourcesInput) (*mcp.CallToolResult, listSourcesOutput, error) {
	tuilog.Log.Info("handleListSources: called")
	status := ms.registry.SourceStatus(ctx)
	tuilog.Log.Info("handleListSources: got status", "count", len(status))

	sources := make([]sourceInfo, 0, len(status))
	for _, info := range status {
		sources = append(sources, sourceInfo{
			Name:      string(info.Source),
			Available: info.Available,
			BasePath:  info.BasePath,
		})
	}

	output := listSourcesOutput{Sources: sources}
	tuilog.Log.Info("handleListSources: returning", "sources", len(sources))
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: formatJSON(output)},
		},
	}, output, nil
}

func (ms *MCPServer) handleListProjects(ctx context.Context, req *mcp.CallToolRequest, input listProjectsInput) (*mcp.CallToolResult, listProjectsOutput, error) {
	tuilog.Log.Info("handleListProjects: called", "source_filter", input.Source)
	projects, err := ms.registry.ListAllProjects(ctx)
	if err != nil {
		tuilog.Log.Error("handleListProjects: failed", "error", err)
		return nil, listProjectsOutput{}, fmt.Errorf("list projects: %w", err)
	}
	tuilog.Log.Info("handleListProjects: got projects", "count", len(projects))

	// Filter by source if specified
	if input.Source != "" {
		filtered := make([]thinkt.Project, 0)
		for _, p := range projects {
			if string(p.Source) == input.Source {
				filtered = append(filtered, p)
			}
		}
		projects = filtered
		tuilog.Log.Info("handleListProjects: filtered", "count", len(projects))
	}

	infos := make([]projectInfo, len(projects))
	for i, p := range projects {
		infos[i] = projectInfo{
			ID:           p.ID,
			Name:         p.Name,
			Path:         p.Path,
			SessionCount: p.SessionCount,
			Source:       string(p.Source),
			PathExists:   p.PathExists,
		}
	}

	output := listProjectsOutput{Projects: infos}
	tuilog.Log.Info("handleListProjects: returning", "projects", len(infos))
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: formatJSON(output)},
		},
	}, output, nil
}

func (ms *MCPServer) handleListSessions(ctx context.Context, req *mcp.CallToolRequest, input listSessionsInput) (*mcp.CallToolResult, listSessionsOutput, error) {
	tuilog.Log.Info("handleListSessions: called", "project_id", input.ProjectID)
	if input.ProjectID == "" {
		tuilog.Log.Error("handleListSessions: project_id is required")
		return nil, listSessionsOutput{}, fmt.Errorf("project_id is required")
	}

	var allSessions []thinkt.SessionMeta
	for _, store := range ms.registry.All() {
		sessions, err := store.ListSessions(ctx, input.ProjectID)
		if err != nil {
			tuilog.Log.Debug("handleListSessions: store skipped", "error", err)
			continue // Skip stores that don't have this project
		}
		allSessions = append(allSessions, sessions...)
	}
	tuilog.Log.Info("handleListSessions: found sessions", "count", len(allSessions))

	infos := make([]sessionInfo, len(allSessions))
	for i, s := range allSessions {
		infos[i] = sessionInfo{
			ID:         s.ID,
			Path:       s.FullPath,
			EntryCount: s.EntryCount,
			FileSize:   s.FileSize,
			Source:     string(s.Source),
		}
		if !s.CreatedAt.IsZero() {
			infos[i].CreatedAt = s.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		if !s.ModifiedAt.IsZero() {
			infos[i].ModifiedAt = s.ModifiedAt.Format("2006-01-02T15:04:05Z07:00")
		}
	}

	output := listSessionsOutput{Sessions: infos}
	tuilog.Log.Info("handleListSessions: returning", "sessions", len(infos))
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: formatJSON(output)},
		},
	}, output, nil
}

func (ms *MCPServer) handleGetSessionMetadata(ctx context.Context, req *mcp.CallToolRequest, input getSessionMetadataInput) (*mcp.CallToolResult, getSessionMetadataOutput, error) {
	tuilog.Log.Info("handleGetSessionMetadata: called", "path", input.Path)
	if input.Path == "" {
		tuilog.Log.Error("handleGetSessionMetadata: path is required")
		return nil, getSessionMetadataOutput{}, fmt.Errorf("path is required")
	}

	// Open the session
	ls, err := tui.OpenLazySession(input.Path)
	if err != nil {
		tuilog.Log.Error("handleGetSessionMetadata: failed to open session", "error", err)
		return nil, getSessionMetadataOutput{}, fmt.Errorf("open session: %w", err)
	}
	defer ls.Close()

	// Load all entries for metadata scanning
	ls.LoadAll()
	entries := ls.Entries()
	meta := ls.Metadata()

	// Build role counts and entry summaries
	roleCounts := make(map[string]int)
	summaries := make([]entrySummary, len(entries))
	totalBytes := 0
	description := ""

	for i, entry := range entries {
		roleCounts[string(entry.Role)]++

		// Calculate content length and detect block types
		contentLen := len(entry.Text)
		hasThinking := false
		hasToolUse := false
		hasToolResult := false
		preview := ""

		for _, block := range entry.ContentBlocks {
			switch block.Type {
			case "thinking":
				hasThinking = true
				contentLen += len(block.Thinking)
			case "tool_use":
				hasToolUse = true
				if input, ok := block.ToolInput.(string); ok {
					contentLen += len(input)
				}
			case "tool_result":
				hasToolResult = true
				contentLen += len(block.ToolResult)
			case "text":
				contentLen += len(block.Text)
				if preview == "" && block.Text != "" {
					preview = truncateString(block.Text, 100)
				}
			}
		}

		// Use entry.Text for preview if no text blocks
		if preview == "" && entry.Text != "" {
			preview = truncateString(entry.Text, 100)
		}

		// Extract description from first user message
		if description == "" && entry.Role == thinkt.RoleUser {
			if entry.Text != "" {
				description = truncateString(entry.Text, 200)
			} else {
				for _, block := range entry.ContentBlocks {
					if block.Type == "text" && block.Text != "" {
						description = truncateString(block.Text, 200)
						break
					}
				}
			}
		}

		totalBytes += contentLen

		summaries[i] = entrySummary{
			Index:         i,
			Role:          string(entry.Role),
			ContentLength: contentLen,
			HasThinking:   hasThinking,
			HasToolUse:    hasToolUse,
			HasToolResult: hasToolResult,
			Preview:       preview,
		}
		if !entry.Timestamp.IsZero() {
			summaries[i].Timestamp = entry.Timestamp.Format("2006-01-02T15:04:05Z07:00")
		}
	}

	// Use FirstPrompt from meta if we didn't find a description
	if description == "" && meta.FirstPrompt != "" {
		description = truncateString(meta.FirstPrompt, 200)
	}

	output := getSessionMetadataOutput{
		Meta: sessionMetaInfo{
			ID:        meta.ID,
			Path:      meta.FullPath,
			Model:     meta.Model,
			GitBranch: meta.GitBranch,
			Source:    string(meta.Source),
		},
		Description:  description,
		RoleCounts:   roleCounts,
		EntrySummary: summaries,
		TotalEntries: len(entries),
		TotalBytes:   totalBytes,
	}
	if !meta.CreatedAt.IsZero() {
		output.Meta.CreatedAt = meta.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if !meta.ModifiedAt.IsZero() {
		output.Meta.ModifiedAt = meta.ModifiedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	tuilog.Log.Info("handleGetSessionMetadata: returning", "entries", len(entries), "total_bytes", totalBytes)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: formatJSON(output)},
		},
	}, output, nil
}

func (ms *MCPServer) handleGetSessionEntries(ctx context.Context, req *mcp.CallToolRequest, input getSessionEntriesInput) (*mcp.CallToolResult, getSessionEntriesOutput, error) {
	tuilog.Log.Info("handleGetSessionEntries: called", "path", input.Path, "limit", input.Limit, "offset", input.Offset,
		"entry_indices", input.EntryIndices, "roles", input.Roles, "max_content_length", input.MaxContentLength)

	if input.Path == "" {
		tuilog.Log.Error("handleGetSessionEntries: path is required")
		return nil, getSessionEntriesOutput{}, fmt.Errorf("path is required")
	}

	// Apply defaults
	limit := input.Limit
	if limit == 0 {
		limit = 5 // Default: 5 entries
	}
	maxContentLen := input.MaxContentLength
	if maxContentLen == 0 {
		maxContentLen = 500 // Default: 500 chars
	}

	// Open the session
	ls, err := tui.OpenLazySession(input.Path)
	if err != nil {
		tuilog.Log.Error("handleGetSessionEntries: failed to open session", "error", err)
		return nil, getSessionEntriesOutput{}, fmt.Errorf("open session: %w", err)
	}
	defer ls.Close()

	// Load all entries (we need indices)
	ls.LoadAll()
	allEntries := ls.Entries()
	total := len(allEntries)

	// Build role filter set
	roleFilter := make(map[string]bool)
	for _, r := range input.Roles {
		roleFilter[r] = true
	}

	// Build list of candidate indices (applying role filter first)
	var candidateIndices []int
	for i, entry := range allEntries {
		// Apply role filter if specified
		if len(roleFilter) > 0 && !roleFilter[string(entry.Role)] {
			continue
		}
		candidateIndices = append(candidateIndices, i)
	}
	filteredTotal := len(candidateIndices)

	// Determine which entries to return
	var indicesToFetch []int
	if len(input.EntryIndices) > 0 {
		// Specific indices requested - filter to valid candidates
		candidateSet := make(map[int]bool)
		for _, idx := range candidateIndices {
			candidateSet[idx] = true
		}
		for _, idx := range input.EntryIndices {
			if candidateSet[idx] {
				indicesToFetch = append(indicesToFetch, idx)
			}
		}
	} else {
		// Use offset/limit on filtered candidates
		start := input.Offset
		end := start + limit
		if start >= filteredTotal {
			indicesToFetch = nil
		} else {
			if end > filteredTotal {
				end = filteredTotal
			}
			indicesToFetch = candidateIndices[start:end]
		}
	}

	// Build output entries
	var resultEntries []entryContent
	for _, idx := range indicesToFetch {
		if idx < 0 || idx >= total {
			continue
		}
		entry := allEntries[idx]

		ec := entryContent{
			Index: idx,
			UUID:  entry.UUID,
			Role:  string(entry.Role),
			Model: entry.Model,
		}
		if !entry.Timestamp.IsZero() {
			ec.Timestamp = entry.Timestamp.Format("2006-01-02T15:04:05Z07:00")
		}

		// Extract text content
		textContent := entry.Text
		for _, block := range entry.ContentBlocks {
			switch block.Type {
			case "text":
				if textContent == "" {
					textContent = block.Text
				} else {
					textContent += "\n" + block.Text
				}
			case "thinking":
				if input.IncludeThinking && block.Thinking != "" {
					ec.Thinking = truncateString(block.Thinking, maxContentLen)
				}
			case "tool_use":
				inputStr := ""
				if s, ok := block.ToolInput.(string); ok {
					inputStr = truncateString(s, maxContentLen)
				} else if block.ToolInput != nil {
					b, _ := json.Marshal(block.ToolInput)
					inputStr = truncateString(string(b), maxContentLen)
				}
				ec.ToolUses = append(ec.ToolUses, toolUseInfo{
					ID:    block.ToolUseID,
					Name:  block.ToolName,
					Input: inputStr,
				})
			case "tool_result":
				ec.ToolResults = append(ec.ToolResults, toolResultInfo{
					ToolUseID: block.ToolUseID,
					Result:    truncateString(block.ToolResult, maxContentLen),
					IsError:   block.IsError,
				})
			}
		}

		// Truncate text content
		if len(textContent) > maxContentLen && maxContentLen > 0 {
			ec.Text = textContent[:maxContentLen] + "..."
			ec.TextTruncated = true
		} else {
			ec.Text = textContent
		}

		resultEntries = append(resultEntries, ec)
	}

	// Determine if there are more entries
	hasMore := false
	if len(input.EntryIndices) == 0 {
		// Using offset/limit - check if there are more in filtered set
		hasMore = input.Offset+limit < filteredTotal
	}

	// Report filtered total if roles filter is applied
	reportedTotal := total
	if len(roleFilter) > 0 {
		reportedTotal = filteredTotal
	}

	output := getSessionEntriesOutput{
		Entries:  resultEntries,
		HasMore:  hasMore,
		Total:    reportedTotal,
		Returned: len(resultEntries),
	}

	tuilog.Log.Info("handleGetSessionEntries: returning", "returned", len(resultEntries), "total", total, "has_more", hasMore)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: formatJSON(output)},
		},
	}, output, nil
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// RunStdio runs the MCP server on stdin/stdout.
func (ms *MCPServer) RunStdio(ctx context.Context) error {
	tuilog.Log.Info("RunStdio: creating buffered stdin reader")
	// Use IOTransport with a buffered reader that delays EOF,
	// allowing the server to process and respond before shutdown
	// reader := newBufferedStdinReader(ctx)
	// tuilog.Log.Info("RunStdio: stdin buffered", "bytes", reader.buf.Len())
	// transport := &mcp.IOTransport{
	// 	Reader: reader,
	// 	Writer: nopWriteCloser{os.Stdout},
	// }
	transport := &mcp.LoggingTransport{
		Transport: &mcp.StdioTransport{},
		Writer:    os.Stderr,
	}
	tuilog.Log.Info("RunStdio: starting server.Run")
	err := ms.server.Run(ctx, transport)
	tuilog.Log.Info("RunStdio: server.Run returned", "error", err)
	return err
}

// RunHTTP runs the MCP server over HTTP using SSE transport.
func (ms *MCPServer) RunHTTP(ctx context.Context, host string, port int) error {
	tuilog.Log.Info("RunHTTP: starting", "host", host, "port", port, "auth_mode", ms.authenticator.config.Mode)

	// Create SSE handler that returns our MCP server for each connection
	sseHandler := mcp.NewSSEHandler(func(req *http.Request) *mcp.Server {
		tuilog.Log.Info("RunHTTP: new SSE connection", "remote", req.RemoteAddr)
		return ms.server
	}, nil)

	// Wrap with authentication middleware
	var handler http.Handler = sseHandler
	if ms.authenticator.config.Mode != AuthModeNone {
		handler = ms.authenticator.Middleware(sseHandler)
		tuilog.Log.Info("RunHTTP: authentication enabled")
	} else {
		tuilog.Log.Warn("RunHTTP: running without authentication - use THINKT_MCP_TOKEN env var to secure")
	}

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", host, port)
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// Start listener
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		tuilog.Log.Error("RunHTTP: failed to listen", "error", err)
		return fmt.Errorf("listen: %w", err)
	}

	// Update port if auto-assigned
	actualPort := ln.Addr().(*net.TCPAddr).Port
	tuilog.Log.Info("RunHTTP: listening", "addr", ln.Addr().String())
	fmt.Fprintf(os.Stderr, "MCP server running at http://%s:%d\n", host, actualPort)

	// Graceful shutdown on context cancellation
	go func() {
		<-ctx.Done()
		tuilog.Log.Info("RunHTTP: context cancelled, shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	// Serve
	err = srv.Serve(ln)
	if err == http.ErrServerClosed {
		tuilog.Log.Info("RunHTTP: server closed")
		return nil
	}
	tuilog.Log.Error("RunHTTP: serve error", "error", err)
	return err
}

// Server returns the underlying mcp.Server for HTTP integration.
func (ms *MCPServer) Server() *mcp.Server {
	return ms.server
}

// formatJSON formats a value as indented JSON.
func formatJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
