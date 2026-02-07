package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	allowTools    map[string]bool
	denyTools     map[string]bool
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

	return ms
}

// SetToolFilters configures which tools are allowed or denied and then registers the tools.
func (ms *MCPServer) SetToolFilters(allow, deny []string) {
	if len(allow) > 0 {
		ms.allowTools = make(map[string]bool)
		for _, t := range allow {
			ms.allowTools[strings.TrimSpace(t)] = true
		}
	}
	if len(deny) > 0 {
		ms.denyTools = make(map[string]bool)
		for _, t := range deny {
			ms.denyTools[strings.TrimSpace(t)] = true
		}
	}

	ms.registerTools()
}

// isToolAllowed checks if a tool should be registered.
func (ms *MCPServer) isToolAllowed(name string) bool {
	if ms.denyTools != nil && ms.denyTools[name] {
		return false
	}
	if ms.allowTools != nil && !ms.allowTools[name] {
		return false
	}
	return true
}

// registerTools adds allowed thinkt tools to the MCP server.
func (ms *MCPServer) registerTools() {
	// list_sources
	if ms.isToolAllowed("list_sources") {
		mcp.AddTool(ms.server, &mcp.Tool{
			Name:        "list_sources",
			Description: "List available trace sources (e.g., kimi, claude)",
		}, ms.handleListSources)
	}

	// list_projects
	if ms.isToolAllowed("list_projects") {
		mcp.AddTool(ms.server, &mcp.Tool{
			Name:        "list_projects",
			Description: "List all projects across all sources, optionally filtered by source",
		}, ms.handleListProjects)
	}

	// list_sessions
	if ms.isToolAllowed("list_sessions") {
		mcp.AddTool(ms.server, &mcp.Tool{
			Name:        "list_sessions",
			Description: "List sessions for a specific project",
		}, ms.handleListSessions)
	}

	// get_session_metadata
	if ms.isToolAllowed("get_session_metadata") {
		mcp.AddTool(ms.server, &mcp.Tool{
			Name:        "get_session_metadata",
			Description: "Get session metadata and entry summaries without loading full content.",
		}, ms.handleGetSessionMetadata)
	}

	// get_session_entries
	if ms.isToolAllowed("get_session_entries") {
		mcp.AddTool(ms.server, &mcp.Tool{
			Name:        "get_session_entries",
			Description: "Get session entry content with pagination and filtering.",
		}, ms.handleGetSessionEntries)
	}

	// Register indexer tools if binary is available
	if indexerPath := findIndexerBinary(); indexerPath != "" {
		tuilog.Log.Info("NewMCPServer: thinkt-indexer found, checking tool permissions", "path", indexerPath)

		// search_sessions
		if ms.isToolAllowed("search_sessions") {
			mcp.AddTool(ms.server, &mcp.Tool{
				Name:        "search_sessions",
				Description: "Search for text across all indexed sessions. Results are limited to 50 total and 2 per session by default.",
			}, ms.handleSearchSessions)
		}

		// get_usage_stats
		if ms.isToolAllowed("get_usage_stats") {
			mcp.AddTool(ms.server, &mcp.Tool{
				Name:        "get_usage_stats",
				Description: "Get aggregate usage statistics including total tokens and most used tools.",
			}, ms.handleGetUsageStats)
		}
	}
}

// findIndexerBinary attempts to locate the thinkt-indexer binary.
func findIndexerBinary() string {
	if execPath, err := os.Executable(); err == nil {
		binDir := filepath.Dir(execPath)
		indexerPath := filepath.Join(binDir, "thinkt-indexer")
		if _, err := os.Stat(indexerPath); err == nil {
			return indexerPath
		}
	}
	if path, err := exec.LookPath("thinkt-indexer"); err == nil {
		return path
	}
	return ""
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
	Source string `json:"source,omitempty"` // Filter by source
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
	ProjectID string `json:"project_id"`
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

type getSessionMetadataInput struct {
	Path string `json:"path"`
}

type getSessionMetadataOutput struct {
	Meta         sessionMetaInfo  `json:"meta"`
	Description  string           `json:"description,omitempty"`
	RoleCounts   map[string]int   `json:"role_counts"`
	EntrySummary []entrySummary   `json:"entry_summary"`
	TotalEntries int              `json:"total_entries"`
	TotalBytes   int              `json:"total_content_bytes"`
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
	ContentLength int    `json:"content_length"`
	HasThinking   bool   `json:"has_thinking"`
	HasToolUse    bool   `json:"has_tool_use"`
	HasToolResult bool   `json:"has_tool_result"`
	Preview       string `json:"preview,omitempty"`
}

type getSessionEntriesInput struct {
	Path             string   `json:"path"`
	Limit            int      `json:"limit,omitempty"`
	Offset           int      `json:"offset,omitempty"`
	EntryIndices     []int    `json:"entry_indices,omitempty"`
	Roles            []string `json:"roles,omitempty"`
	MaxContentLength int      `json:"max_content_length,omitempty"`
	IncludeThinking  bool     `json:"include_thinking,omitempty"`
}

type getSessionEntriesOutput struct {
	Entries  []entryContent `json:"entries"`
	HasMore  bool           `json:"has_more"`
	Total    int            `json:"total"`
	Returned int            `json:"returned"`
}

type entryContent struct {
	Index         int                  `json:"index"`
	UUID          string               `json:"uuid"`
	Role          string               `json:"role"`
	Timestamp     string               `json:"timestamp,omitempty"`
	Text          string               `json:"text,omitempty"`
	TextTruncated bool                 `json:"text_truncated,omitempty"`
	Thinking      string               `json:"thinking,omitempty"`
	ToolUses      []toolUseInfo        `json:"tool_uses,omitempty"`
	ToolResults   []toolResultInfo     `json:"tool_results,omitempty"`
	Model         string               `json:"model,omitempty"`
}

type toolUseInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input,omitempty"`
}

type toolResultInfo struct {
	ToolUseID string `json:"tool_use_id"`
	Result    string `json:"result,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type searchSessionsInput struct {
	Query           string `json:"query"`
	Project         string `json:"project,omitempty"`
	Source          string `json:"source,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	LimitPerSession int    `json:"limit_per_session,omitempty"`
}

type getUsageStatsInput struct{}

// Tool handlers

func (ms *MCPServer) handleListSources(ctx context.Context, req *mcp.CallToolRequest, _ listSourcesInput) (*mcp.CallToolResult, listSourcesOutput, error) {
	status := ms.registry.SourceStatus(ctx)
	sources := make([]sourceInfo, 0, len(status))
	for _, info := range status {
		sources = append(sources, sourceInfo{
			Name:      string(info.Source),
			Available: info.Available,
			BasePath:  info.BasePath,
		})
	}
	output := listSourcesOutput{Sources: sources}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}},
	}, output, nil
}

func (ms *MCPServer) handleListProjects(ctx context.Context, req *mcp.CallToolRequest, input listProjectsInput) (*mcp.CallToolResult, listProjectsOutput, error) {
	projects, err := ms.registry.ListAllProjects(ctx)
	if err != nil {
		return nil, listProjectsOutput{}, err
	}
	if input.Source != "" {
		filtered := make([]thinkt.Project, 0)
		for _, p := range projects {
			if string(p.Source) == input.Source {
				filtered = append(filtered, p)
			}
		}
		projects = filtered
	}
	infos := make([]projectInfo, len(projects))
	for i, p := range projects {
		infos[i] = projectInfo{ID: p.ID, Name: p.Name, Path: p.Path, SessionCount: p.SessionCount, Source: string(p.Source), PathExists: p.PathExists}
	}
	output := listProjectsOutput{Projects: infos}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}},
	}, output, nil
}

func (ms *MCPServer) handleListSessions(ctx context.Context, req *mcp.CallToolRequest, input listSessionsInput) (*mcp.CallToolResult, listSessionsOutput, error) {
	var allSessions []thinkt.SessionMeta
	for _, store := range ms.registry.All() {
		sessions, err := store.ListSessions(ctx, input.ProjectID)
		if err == nil {
			allSessions = append(allSessions, sessions...)
		}
	}
	infos := make([]sessionInfo, len(allSessions))
	for i, s := range allSessions {
		infos[i] = sessionInfo{ID: s.ID, Path: s.FullPath, EntryCount: s.EntryCount, FileSize: s.FileSize, Source: string(s.Source)}
		if !s.CreatedAt.IsZero() { infos[i].CreatedAt = s.CreatedAt.Format(time.RFC3339) }
		if !s.ModifiedAt.IsZero() { infos[i].ModifiedAt = s.ModifiedAt.Format(time.RFC3339) }
	}
	output := listSessionsOutput{Sessions: infos}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}},
	}, output, nil
}

func (ms *MCPServer) handleGetSessionMetadata(ctx context.Context, req *mcp.CallToolRequest, input getSessionMetadataInput) (*mcp.CallToolResult, getSessionMetadataOutput, error) {
	ls, err := tui.OpenLazySession(input.Path)
	if err != nil { return nil, getSessionMetadataOutput{}, err }
	defer ls.Close()
	ls.LoadAll()
	entries := ls.Entries()
	meta := ls.Metadata()
	roleCounts := make(map[string]int)
	summaries := make([]entrySummary, len(entries))
	totalBytes := 0
	for i, entry := range entries {
		roleCounts[string(entry.Role)]++
		contentLen := len(entry.Text)
		hasThinking, hasToolUse, hasToolResult := false, false, false
		preview := truncateString(entry.Text, 100)
		for _, block := range entry.ContentBlocks {
			switch block.Type {
			case "thinking": hasThinking = true; contentLen += len(block.Thinking)
			case "tool_use": hasToolUse = true
			case "tool_result": hasToolResult = true; contentLen += len(block.ToolResult)
			}
		}
		totalBytes += contentLen
		summaries[i] = entrySummary{Index: i, Role: string(entry.Role), ContentLength: contentLen, HasThinking: hasThinking, HasToolUse: hasToolUse, HasToolResult: hasToolResult, Preview: preview}
	}
	output := getSessionMetadataOutput{
		Meta: sessionMetaInfo{ID: meta.ID, Path: meta.FullPath, Model: meta.Model, GitBranch: meta.GitBranch, Source: string(meta.Source)},
		RoleCounts: roleCounts, EntrySummary: summaries, TotalEntries: len(entries), TotalBytes: totalBytes,
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}},
	}, output, nil
}

func (ms *MCPServer) handleGetSessionEntries(ctx context.Context, req *mcp.CallToolRequest, input getSessionEntriesInput) (*mcp.CallToolResult, getSessionEntriesOutput, error) {
	ls, err := tui.OpenLazySession(input.Path)
	if err != nil { return nil, getSessionEntriesOutput{}, err }
	defer ls.Close()
	ls.LoadAll()
	allEntries := ls.Entries()
	limit := input.Limit
	if limit == 0 { limit = 5 }
	maxLen := input.MaxContentLength
	if maxLen == 0 { maxLen = 500 }
	var resultEntries []entryContent
	for i, entry := range allEntries {
		if len(resultEntries) >= limit { break }
		ec := entryContent{Index: i, UUID: entry.UUID, Role: string(entry.Role), Model: entry.Model, Text: truncateString(entry.Text, maxLen)}
		resultEntries = append(resultEntries, ec)
	}
	output := getSessionEntriesOutput{Entries: resultEntries, Total: len(allEntries), Returned: len(resultEntries)}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}},
	}, output, nil
}

func (ms *MCPServer) handleSearchSessions(ctx context.Context, req *mcp.CallToolRequest, input searchSessionsInput) (*mcp.CallToolResult, any, error) {
	path := findIndexerBinary()
	args := []string{"search", "--json", input.Query}
	if input.Project != "" { args = append(args, "--project", input.Project) }
	if input.Limit > 0 { args = append(args, "--limit", fmt.Sprintf("%d", input.Limit)) }
	if input.LimitPerSession > 0 { args = append(args, "--limit-per-session", fmt.Sprintf("%d", input.LimitPerSession)) }
	cmd := exec.Command(path, args...)
	out, err := cmd.Output()
	if err != nil { return nil, nil, err }
	var res any
	json.Unmarshal(out, &res)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(out)}}}, res, nil
}

func (ms *MCPServer) handleGetUsageStats(ctx context.Context, req *mcp.CallToolRequest, _ getUsageStatsInput) (*mcp.CallToolResult, any, error) {
	path := findIndexerBinary()
	cmd := exec.Command(path, "stats", "--json")
	out, err := cmd.Output()
	if err != nil { return nil, nil, err }
	var res any
	json.Unmarshal(out, &res)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(out)}}}, res, nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen { return s }
	return s[:maxLen-3] + "..."
}

func (ms *MCPServer) RunStdio(ctx context.Context) error {
	return ms.server.Run(ctx, &mcp.LoggingTransport{Transport: &mcp.StdioTransport{}, Writer: os.Stderr})
}

func (ms *MCPServer) RunHTTP(ctx context.Context, host string, port int) error {
	sseHandler := mcp.NewSSEHandler(func(req *http.Request) *mcp.Server { return ms.server }, nil)
	var handler http.Handler = sseHandler
	if ms.authenticator.config.Mode != AuthModeNone { handler = ms.authenticator.Middleware(sseHandler) }
	addr := fmt.Sprintf("%s:%d", host, port)
	srv := &http.Server{Addr: addr, Handler: handler}
	ln, _ := net.Listen("tcp", addr)
	go func() { <-ctx.Done(); srv.Shutdown(context.Background()) }()
	return srv.Serve(ln)
}

func (ms *MCPServer) Server() *mcp.Server { return ms.server }

func formatJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}