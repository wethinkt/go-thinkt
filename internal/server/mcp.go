package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// MCPServer wraps an MCP server for thinkt.
type MCPServer struct {
	server        *mcp.Server
	registry      *thinkt.StoreRegistry
	authenticator *BearerAuthenticator
	allowTools    map[string]bool
	denyTools     map[string]bool
}

// NewMCPServer creates a new MCP server with thinkt tools.
func NewMCPServer(registry *thinkt.StoreRegistry) *MCPServer {
	return NewMCPServerWithAuth(registry, DefaultMCPAuthConfig())
}

// NewMCPServerWithAuth creates a new MCP server with authentication.
func NewMCPServerWithAuth(registry *thinkt.StoreRegistry, authConfig AuthConfig) *MCPServer {
	tuilog.Log.Info("NewMCPServer: creating MCP server", "auth_mode", authConfig.Mode)
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "thinkt",
		Version: "1.0.0",
	}, nil)

	ms := &MCPServer{
		server:        server,
		registry:      registry,
		authenticator: NewBearerAuthenticator(authConfig),
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
			Description: "List all projects across all sources. Supports pagination (limit/offset), source filtering, and include_deleted to include path_exists=false projects. Returns projects sorted by last modified time (newest first) by default.",
		}, ms.handleListProjects)
	}

	// list_sessions
	if ms.isToolAllowed("list_sessions") {
		mcp.AddTool(ms.server, &mcp.Tool{
			Name:        "list_sessions",
			Description: "List sessions for a specific project and source. Supports pagination (limit/offset) and sorting (newest first).",
		}, ms.handleListSessions)
	}

	// get_session_metadata
	if ms.isToolAllowed("get_session_metadata") {
		mcp.AddTool(ms.server, &mcp.Tool{
			Name:        "get_session_metadata",
			Description: "Get session metadata and entry summaries without loading full content. Default limits to 50 entries and excludes checkpoints. Use offset/limit for pagination, or summary_only for lightweight user-message previews.",
		}, ms.handleGetSessionMetadata)
	}

	// get_session_entries
	if ms.isToolAllowed("get_session_entries") {
		mcp.AddTool(ms.server, &mcp.Tool{
			Name:        "get_session_entries",
			Description: "Get session entry content with pagination and filtering. Defaults: limit=5, max_content_length=500, include_thinking=false. Use entry_indices to fetch specific entries by index, or roles to filter by role type.",
		}, ms.handleGetSessionEntries)
	}

	// Register indexer tools if binary is available
	if indexerPath := config.FindIndexerBinary(); indexerPath != "" {
		tuilog.Log.Info("NewMCPServer: thinkt-indexer found, checking tool permissions", "path", indexerPath)

		// search_sessions
		if ms.isToolAllowed("search_sessions") {
			mcp.AddTool(ms.server, &mcp.Tool{
				Name:        "search_sessions",
				Description: "Search for text across all indexed sessions. Results are limited to 50 total and 2 per session by default. Supports case_sensitive matching and regex patterns.",
			}, ms.handleSearchSessions)
		}

		// get_usage_stats
		if ms.isToolAllowed("get_usage_stats") {
			mcp.AddTool(ms.server, &mcp.Tool{
				Name:        "get_usage_stats",
				Description: "Get aggregate usage statistics including total tokens and most used tools.",
			}, ms.handleGetUsageStats)
		}

		// semantic_search
		if ms.isToolAllowed("semantic_search") {
			mcp.AddTool(ms.server, &mcp.Tool{
				Name:        "semantic_search",
				Description: "Search for sessions by meaning using on-device embeddings. Returns sessions ranked by semantic similarity to the query. Requires the indexer server with a synced embedding index.\n\nParameters:\n- query (required): Natural language search query\n- project: Filter by project name (partial match)\n- source: Filter by source (kimi|claude|gemini|copilot|codex|qwen)\n- max_distance: Cosine distance threshold (0-2 range, lower is more similar)\n- diversity: When true, applies diversity scoring to return results from different sessions rather than many results from the same session\n- limit: Maximum number of results (default: 20)",
			}, ms.handleSemanticSearch)
		}
	}
}

// Tool input/output types

type listSourcesInput struct{}

type listSourcesOutput struct {
	Sources []SourceInfo `json:"sources"`
}

type listProjectsInput struct {
	Source         string `json:"source,omitempty"`          // Filter by source
	IncludeDeleted bool   `json:"include_deleted,omitempty"` // Include projects where path_exists=false (default: false)
	Limit          int    `json:"limit,omitempty"`           // Max projects to return (default: 20)
	Offset         int    `json:"offset,omitempty"`          // Number of projects to skip
}

type listProjectsOutput struct {
	Projects []projectInfo `json:"projects"`
	Total    int           `json:"total"`
	Returned int           `json:"returned"`
}

type projectInfo struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	SessionCount int       `json:"session_count"`
	Source       string    `json:"source"`
	LastModified time.Time `json:"last_modified"`
	PathExists   bool      `json:"path_exists"`
}

type listSessionsInput struct {
	ProjectID string `json:"project_id"`
	Source    string `json:"source"`
	Limit     int    `json:"limit,omitempty"`  // Max sessions to return (default: 20)
	Offset    int    `json:"offset,omitempty"` // Number of sessions to skip
}

type listSessionsOutput struct {
	Sessions []sessionInfo    `json:"sessions"`
	Total    int              `json:"total"`
	Returned int              `json:"returned"`
	Error    *toolErrorDetail `json:"error,omitempty"`
}

type sessionInfo struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	CreatedAt  string `json:"created_at,omitempty"`
	ModifiedAt string `json:"modified_at,omitempty"`
	EntryCount int    `json:"entry_count"`
	FileSize   int64  `json:"file_size"`
	Source     string `json:"source"`
	Model      string `json:"model,omitempty"`
}

type getSessionMetadataInput struct {
	Path         string   `json:"path"`                    // Full path to the session file (required)
	Limit        int      `json:"limit,omitempty"`         // Max summaries to return (default: 50)
	Offset       int      `json:"offset,omitempty"`        // Number of summaries to skip (default: 0)
	ExcludeRoles []string `json:"exclude_roles,omitempty"` // Filter out specific roles (default: ["checkpoint"])
	SummaryOnly  bool     `json:"summary_only,omitempty"`  // If true, return lightweight user previews in entry_summary (default: first 5)
	SortBy       string   `json:"sort_by,omitempty"`       // "index" (default) or "length" (largest first)
}

type getSessionMetadataOutput struct {
	Meta         sessionMetaInfo  `json:"meta"`
	Description  string           `json:"description,omitempty"`
	RoleCounts   map[string]int   `json:"role_counts"`
	EntrySummary []entrySummary   `json:"entry_summary"`
	TotalEntries int              `json:"total_entries"`
	TotalBytes   int              `json:"total_content_bytes"`
	Returned     int              `json:"returned_summaries"`
	Error        *toolErrorDetail `json:"error,omitempty"`
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
	Path             string   `json:"path"`                         // Full path to the session file (required)
	Limit            int      `json:"limit,omitempty"`              // Max entries to return (default: 5)
	Offset           int      `json:"offset,omitempty"`             // Number of entries to skip (default: 0)
	EntryIndices     []int    `json:"entry_indices,omitempty"`      // Specific entry indices to fetch
	Roles            []string `json:"roles,omitempty"`              // Filter to specific roles
	MaxContentLength int      `json:"max_content_length,omitempty"` // Truncate text content (default: 500)
	IncludeThinking  bool     `json:"include_thinking,omitempty"`   // Include thinking blocks
}

type getSessionEntriesOutput struct {
	Entries  []entryContent   `json:"entries"`
	HasMore  bool             `json:"has_more"`
	Total    int              `json:"total"`
	Returned int              `json:"returned"`
	Error    *toolErrorDetail `json:"error,omitempty"`
}

type entryContent struct {
	Index         int              `json:"index"`
	UUID          string           `json:"uuid"`
	Role          string           `json:"role"`
	Timestamp     string           `json:"timestamp,omitempty"`
	Text          string           `json:"text,omitempty"`
	TextTruncated bool             `json:"text_truncated,omitempty"`
	Thinking      string           `json:"thinking,omitempty"`
	ToolUses      []toolUseInfo    `json:"tool_uses,omitempty"`
	ToolResults   []toolResultInfo `json:"tool_results,omitempty"`
	Model         string           `json:"model,omitempty"`
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
	CaseSensitive   bool   `json:"case_sensitive,omitempty"`
	Regex           bool   `json:"regex,omitempty"`
}

type semanticSearchInput struct {
	Query       string  `json:"query"`
	Project     string  `json:"project,omitempty"`
	Source      string  `json:"source,omitempty"`
	Limit       int     `json:"limit,omitempty"`
	MaxDistance float64 `json:"max_distance,omitempty"`
	Diversity   bool    `json:"diversity,omitempty"`
}

// semanticResult represents a single semantic search hit.
type semanticResult struct {
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

// semanticSearchOutput wraps results for MCP structured content compatibility.
type semanticSearchOutput struct {
	Results []semanticResult `json:"results"`
}

type toolErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

type toolErrorOutput struct {
	Error toolErrorDetail `json:"error"`
}

type getUsageStatsInput struct{}

// Tool handlers

func (ms *MCPServer) handleListSources(ctx context.Context, req *mcp.CallToolRequest, _ listSourcesInput) (*mcp.CallToolResult, listSourcesOutput, error) {
	status := ms.registry.SourceStatus(ctx)
	sources := make([]SourceInfo, 0, len(status))
	for _, info := range status {
		sources = append(sources, SourceInfo{
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
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	var filtered []thinkt.Project
	if input.Source != "" {
		for _, p := range projects {
			if !input.IncludeDeleted && p.Path != "" && !p.PathExists {
				continue
			}
			if string(p.Source) == input.Source {
				filtered = append(filtered, p)
			}
		}
	} else {
		for _, p := range projects {
			if !input.IncludeDeleted && p.Path != "" && !p.PathExists {
				continue
			}
			filtered = append(filtered, p)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].LastModified.After(filtered[j].LastModified) })
	total := len(filtered)
	pInfos := []projectInfo{}
	start := input.Offset
	if start < 0 {
		start = 0
	}
	if start < total {
		end := start + limit
		if end > total {
			end = total
		}
		for _, p := range filtered[start:end] {
			pInfos = append(pInfos, projectInfo{ID: p.ID, Name: p.Name, Path: p.Path, SessionCount: p.SessionCount, Source: string(p.Source), LastModified: p.LastModified, PathExists: p.PathExists})
		}
	}
	output := listProjectsOutput{Projects: pInfos, Total: total, Returned: len(pInfos)}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}}}, output, nil
}

func (ms *MCPServer) handleListSessions(ctx context.Context, req *mcp.CallToolRequest, input listSessionsInput) (*mcp.CallToolResult, listSessionsOutput, error) {
	if strings.TrimSpace(input.ProjectID) == "" {
		r, _, err := toolErrorResult("missing_project_id", "project_id is required", nil)
		return r, errorListSessionsOutput("missing_project_id", "project_id is required", nil), err
	}
	source := thinkt.Source(strings.ToLower(strings.TrimSpace(input.Source)))
	if source == "" {
		r, _, err := toolErrorResult("missing_source", "source is required", nil)
		return r, errorListSessionsOutput("missing_source", "source is required", nil), err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	start := input.Offset
	if start < 0 {
		start = 0
	}

	var sInfos []sessionInfo
	var total int

	// 1. Try indexer first for accurate data
	if path := config.FindIndexerBinary(); path != "" {
		cmd := exec.Command(path, "sessions", "--json", "--source", string(source), input.ProjectID)
		if output, err := cmd.Output(); err == nil {
			var indexed []sessionInfo
			if err := json.Unmarshal(output, &indexed); err == nil {
				// If indexer has data, use it. If not, fall back to direct source reads
				// to avoid stale/empty index responses hiding real sessions.
				if len(indexed) == 0 {
					goto fallback
				}
				total = len(indexed)
				if start < total {
					end := start + limit
					if end > total {
						end = total
					}
					sInfos = indexed[start:end]
				}

				output := listSessionsOutput{Sessions: sInfos, Total: total, Returned: len(sInfos)}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}}}, output, nil
			}
		}
	}

	// 2. Fallback to direct source reading
fallback:
	store, ok := ms.registry.Get(source)
	if !ok {
		r, _, err := toolErrorResult("unknown_source", fmt.Sprintf("unknown source: %s", source), nil)
		return r, errorListSessionsOutput("unknown_source", fmt.Sprintf("unknown source: %s", source), nil), err
	}
	allSessions, err := store.ListSessions(ctx, input.ProjectID)
	if err != nil {
		r, _, retErr := toolErrorResult("list_sessions_failed", "failed to list sessions", err)
		return r, errorListSessionsOutput("list_sessions_failed", "failed to list sessions", err), retErr
	}

	sort.Slice(allSessions, func(i, j int) bool { return allSessions[i].ModifiedAt.After(allSessions[j].ModifiedAt) })

	total = len(allSessions)
	sInfos = []sessionInfo{}
	if start < total {
		end := start + limit
		if end > total {
			end = total
		}
		for _, s := range allSessions[start:end] {
			si := sessionInfo{ID: s.ID, Path: s.FullPath, EntryCount: s.EntryCount, FileSize: s.FileSize, Source: string(s.Source), Model: s.Model}
			if !s.CreatedAt.IsZero() {
				si.CreatedAt = s.CreatedAt.Format(time.RFC3339)
			}
			if !s.ModifiedAt.IsZero() {
				si.ModifiedAt = s.ModifiedAt.Format(time.RFC3339)
			}
			sInfos = append(sInfos, si)
		}
	}
	output := listSessionsOutput{Sessions: sInfos, Total: total, Returned: len(sInfos)}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}}}, output, nil
}

func (ms *MCPServer) handleGetSessionMetadata(ctx context.Context, req *mcp.CallToolRequest, input getSessionMetadataInput) (*mcp.CallToolResult, getSessionMetadataOutput, error) {
	if strings.TrimSpace(input.Path) == "" {
		r, _, err := toolErrorResult("missing_path", "path is required", nil)
		return r, errorGetSessionMetadataOutput("missing_path", "path is required", nil), err
	}
	output, err := collectSessionMetadata(ctx, ms.registry, input.Path, input)
	if err != nil {
		r, _, retErr := toolErrorResult("session_metadata_failed", "failed to load session metadata", err)
		return r, errorGetSessionMetadataOutput("session_metadata_failed", "failed to load session metadata", err), retErr
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}}}, output, nil
}

func collectSessionMetadata(ctx context.Context, registry *thinkt.StoreRegistry, path string, input getSessionMetadataInput) (getSessionMetadataOutput, error) {
	ls, err := registry.OpenLazySessionByPath(ctx, path)
	if err != nil {
		return getSessionMetadataOutput{}, err
	}
	defer ls.Close()
	if err := ls.LoadAll(); err != nil {
		return getSessionMetadataOutput{}, err
	}
	entries := ls.Entries()
	meta := ls.Metadata()
	excludeSet := make(map[string]bool)
	if len(input.ExcludeRoles) > 0 {
		for _, r := range input.ExcludeRoles {
			excludeSet[r] = true
		}
	} else {
		excludeSet["checkpoint"] = true
	}
	roleCounts := make(map[string]int)
	allSummaries := []entrySummary{}
	totalBytes, description := 0, ""
	for i, entry := range entries {
		roleCounts[string(entry.Role)]++
		contentLen := len(entry.Text)
		hasThinking, hasToolUse, hasToolResult := false, false, false
		for _, block := range entry.ContentBlocks {
			switch block.Type {
			case "thinking":
				hasThinking = true
				contentLen += len(block.Thinking)
			case "tool_use":
				hasToolUse = true
			case "tool_result":
				hasToolResult = true
				contentLen += len(block.ToolResult)
			}
		}
		totalBytes += contentLen
		if description == "" && entry.Role == thinkt.RoleUser {
			description = truncateString(entry.Text, 200)
		}
		if excludeSet[string(entry.Role)] {
			continue
		}
		allSummaries = append(allSummaries, entrySummary{Index: i, Role: string(entry.Role), ContentLength: contentLen, HasThinking: hasThinking, HasToolUse: hasToolUse, HasToolResult: hasToolResult, Preview: truncateString(entry.Text, 100)})
	}
	output := getSessionMetadataOutput{
		Meta:       sessionMetaInfo{ID: meta.ID, Path: meta.FullPath, Model: meta.Model, GitBranch: meta.GitBranch, Source: string(meta.Source)},
		RoleCounts: roleCounts, TotalEntries: len(entries), TotalBytes: totalBytes, Description: description,
		EntrySummary: []entrySummary{},
	}
	if input.SummaryOnly {
		// summary_only returns lightweight previews focused on user intent by default.
		userSummaries := make([]entrySummary, 0, len(allSummaries))
		for _, s := range allSummaries {
			if s.Role == string(thinkt.RoleUser) {
				userSummaries = append(userSummaries, s)
			}
		}

		limit := input.Limit
		if limit == 0 {
			limit = 5
		}
		start := input.Offset
		if start < 0 {
			start = 0
		}
		if start < len(userSummaries) {
			end := start + limit
			if end > len(userSummaries) {
				end = len(userSummaries)
			}
			output.EntrySummary = userSummaries[start:end]
			output.Returned = len(output.EntrySummary)
		}
	} else {
		if input.SortBy == "length" {
			sort.Slice(allSummaries, func(i, j int) bool { return allSummaries[i].ContentLength > allSummaries[j].ContentLength })
		}
		limit := input.Limit
		if limit == 0 {
			limit = 50
		}
		start := input.Offset
		if start < 0 {
			start = 0
		}
		if start < len(allSummaries) {
			end := start + limit
			if end > len(allSummaries) {
				end = len(allSummaries)
			}
			output.EntrySummary = allSummaries[start:end]
			output.Returned = len(output.EntrySummary)
		}
	}
	return output, nil
}

func (ms *MCPServer) handleGetSessionEntries(ctx context.Context, req *mcp.CallToolRequest, input getSessionEntriesInput) (*mcp.CallToolResult, getSessionEntriesOutput, error) {
	if input.Path == "" {
		r, _, err := toolErrorResult("missing_path", "path is required", nil)
		return r, errorGetSessionEntriesOutput("missing_path", "path is required", nil), err
	}
	ls, err := ms.registry.OpenLazySessionByPath(ctx, input.Path)
	if err != nil {
		r, _, retErr := toolErrorResult("open_session_failed", "failed to open session", err)
		return r, errorGetSessionEntriesOutput("open_session_failed", "failed to open session", err), retErr
	}
	defer ls.Close()
	if err := ls.LoadAll(); err != nil {
		r, _, retErr := toolErrorResult("read_session_failed", "failed to read session entries", err)
		return r, errorGetSessionEntriesOutput("read_session_failed", "failed to read session entries", err), retErr
	}
	allEntries := ls.Entries()

	roleFilter := make(map[string]bool)
	for _, r := range input.Roles {
		roleFilter[r] = true
	}
	indexFilter := make(map[int]bool)
	for _, idx := range input.EntryIndices {
		indexFilter[idx] = true
	}

	limit := input.Limit
	if limit == 0 {
		limit = 5
	}
	maxLen := input.MaxContentLength
	if maxLen == 0 {
		maxLen = 500
	}

	filtered := []entryContent{}
	for i, entry := range allEntries {
		if len(roleFilter) > 0 && !roleFilter[string(entry.Role)] {
			continue
		}
		if len(indexFilter) > 0 && !indexFilter[i] {
			continue
		}

		ec := entryContent{Index: i, UUID: entry.UUID, Role: string(entry.Role), Model: entry.Model, ToolUses: []toolUseInfo{}, ToolResults: []toolResultInfo{}}
		if !entry.Timestamp.IsZero() {
			ec.Timestamp = entry.Timestamp.Format(time.RFC3339)
		}
		ec.Text = truncateString(entry.Text, maxLen)
		if len(entry.Text) > maxLen {
			ec.TextTruncated = true
		}

		for _, b := range entry.ContentBlocks {
			switch b.Type {
			case "thinking":
				if input.IncludeThinking {
					ec.Thinking = truncateString(b.Thinking, maxLen)
				}
			case "tool_use":
				ec.ToolUses = append(ec.ToolUses, toolUseInfo{ID: b.ToolUseID, Name: b.ToolName, Input: truncateString(fmt.Sprintf("%v", b.ToolInput), maxLen)})
			case "tool_result":
				ec.ToolResults = append(ec.ToolResults, toolResultInfo{ToolUseID: b.ToolUseID, IsError: b.IsError, Result: truncateString(b.ToolResult, maxLen)})
			}
		}
		filtered = append(filtered, ec)
	}

	totalFiltered := len(filtered)
	start := input.Offset
	if start < 0 {
		start = 0
	}

	result := []entryContent{}
	hasMore := false

	if len(indexFilter) > 0 {
		result = filtered
	} else if start < totalFiltered {
		end := start + limit
		if end < totalFiltered {
			hasMore = true
		} else {
			end = totalFiltered
		}
		result = filtered[start:end]
	}

	output := getSessionEntriesOutput{
		Entries: result, Total: len(allEntries), Returned: len(result), HasMore: hasMore,
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}}}, output, nil
}

func (ms *MCPServer) handleSearchSessions(ctx context.Context, req *mcp.CallToolRequest, input searchSessionsInput) (*mcp.CallToolResult, any, error) {
	path := config.FindIndexerBinary()
	if path == "" {
		return toolErrorResult("indexer_not_found", "thinkt-indexer binary not found", nil)
	}
	args := buildIndexerSearchArgs(input)
	cmd := exec.Command(path, args...)
	out, err := cmd.Output()
	if err != nil {
		return toolErrorResult("search_failed", "indexer search failed", combineCmdError(err, nil))
	}
	var res any
	if err := json.Unmarshal(out, &res); err != nil {
		return toolErrorResult("invalid_response", "indexer returned invalid JSON", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out))))
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(out)}}}, res, nil
}

func buildIndexerSearchArgs(input searchSessionsInput) []string {
	args := []string{"search", "--json", input.Query}
	if input.Project != "" {
		args = append(args, "--project", input.Project)
	}
	if source := strings.TrimSpace(strings.ToLower(input.Source)); source != "" {
		args = append(args, "--source", source)
	}
	if input.Limit > 0 {
		args = append(args, "--limit", fmt.Sprintf("%d", input.Limit))
	}
	if input.LimitPerSession > 0 {
		args = append(args, "--limit-per-session", fmt.Sprintf("%d", input.LimitPerSession))
	}
	if input.CaseSensitive {
		args = append(args, "--case-sensitive")
	}
	if input.Regex {
		args = append(args, "--regex")
	}
	return args
}

func (ms *MCPServer) handleSemanticSearch(ctx context.Context, req *mcp.CallToolRequest, input semanticSearchInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Query) == "" {
		return toolErrorResult("missing_query", "query is required", nil)
	}
	path := config.FindIndexerBinary()
	if path == "" {
		return toolErrorResult("indexer_not_found", "thinkt-indexer binary not found", nil)
	}
	args := []string{"semantic", "search", "--json", input.Query}
	if input.Project != "" {
		args = append(args, "--project", input.Project)
	}
	if source := strings.TrimSpace(strings.ToLower(input.Source)); source != "" {
		args = append(args, "--source", source)
	}
	if input.Limit > 0 {
		args = append(args, "--limit", fmt.Sprintf("%d", input.Limit))
	}
	if input.MaxDistance > 0 {
		args = append(args, "--max-distance", fmt.Sprintf("%f", input.MaxDistance))
	}
	if input.Diversity {
		args = append(args, "--diversity")
	}
	cmd := exec.Command(path, args...)
	out, err := cmd.Output()
	if err != nil {
		return toolErrorResult("semantic_search_failed", "semantic search failed", combineCmdError(err, nil))
	}
	output, err := decodeSemanticSearchOutput(out)
	if err != nil {
		return toolErrorResult("invalid_response", "indexer returned invalid JSON", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out))))
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}}}, output, nil
}

func (ms *MCPServer) handleGetUsageStats(ctx context.Context, req *mcp.CallToolRequest, _ getUsageStatsInput) (*mcp.CallToolResult, any, error) {
	path := config.FindIndexerBinary()
	if path == "" {
		return toolErrorResult("indexer_not_found", "thinkt-indexer binary not found", nil)
	}
	cmd := exec.Command(path, "stats", "--json")
	out, err := cmd.Output()
	if err != nil {
		return toolErrorResult("stats_failed", "failed to load usage stats", combineCmdError(err, nil))
	}
	var res any
	if err := json.Unmarshal(out, &res); err != nil {
		return toolErrorResult("invalid_response", "indexer returned invalid JSON", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out))))
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(out)}}}, res, nil
}

func toolErrorResult(code, message string, err error) (*mcp.CallToolResult, any, error) {
	detail := makeToolErrorDetail(code, message, err)
	output := toolErrorOutput{Error: detail}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}},
	}, output, nil
}

func makeToolErrorDetail(code, message string, err error) toolErrorDetail {
	detail := toolErrorDetail{
		Code:    code,
		Message: message,
	}
	if err != nil {
		detail.Details = err.Error()
	}
	return detail
}

func emptyListSessionsOutput() listSessionsOutput {
	return listSessionsOutput{
		Sessions: []sessionInfo{},
	}
}

func errorListSessionsOutput(code, message string, err error) listSessionsOutput {
	detail := makeToolErrorDetail(code, message, err)
	out := emptyListSessionsOutput()
	out.Error = &detail
	return out
}

func emptyGetSessionMetadataOutput() getSessionMetadataOutput {
	return getSessionMetadataOutput{
		Meta:         sessionMetaInfo{},
		RoleCounts:   map[string]int{},
		EntrySummary: []entrySummary{},
	}
}

func errorGetSessionMetadataOutput(code, message string, err error) getSessionMetadataOutput {
	detail := makeToolErrorDetail(code, message, err)
	out := emptyGetSessionMetadataOutput()
	out.Error = &detail
	return out
}

func emptyGetSessionEntriesOutput() getSessionEntriesOutput {
	return getSessionEntriesOutput{
		Entries: []entryContent{},
	}
}

func errorGetSessionEntriesOutput(code, message string, err error) getSessionEntriesOutput {
	detail := makeToolErrorDetail(code, message, err)
	out := emptyGetSessionEntriesOutput()
	out.Error = &detail
	return out
}

func combineCmdError(err error, out []byte) error {
	if len(out) > 0 {
		if stderr := strings.TrimSpace(string(out)); stderr != "" {
			return fmt.Errorf("%w: %s", err, stderr)
		}
	}
	if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
	}
	return err
}

func normalizeSemanticResults(results []semanticResult) []semanticResult {
	if results == nil {
		return make([]semanticResult, 0)
	}
	return results
}

func decodeSemanticSearchOutput(out []byte) (semanticSearchOutput, error) {
	var results []semanticResult
	if err := json.Unmarshal(out, &results); err != nil {
		return semanticSearchOutput{}, err
	}
	return semanticSearchOutput{Results: normalizeSemanticResults(results)}, nil
}

func truncateString(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func (ms *MCPServer) RunStdio(ctx context.Context) error {
	return ms.server.Run(ctx, &mcp.LoggingTransport{Transport: &mcp.StdioTransport{}, Writer: os.Stderr})
}

func (ms *MCPServer) RunHTTP(ctx context.Context, host string, port int) error {
	// Check for port conflicts via instance discovery
	if existing := config.FindInstanceByPort(port); existing != nil {
		return fmt.Errorf("port %d is already in use by thinkt %s (PID %d, started %s)",
			port, existing.Type, existing.PID, existing.StartedAt.Format(time.RFC3339))
	}

	sseHandler := mcp.NewSSEHandler(func(req *http.Request) *mcp.Server { return ms.server }, nil)
	var handler http.Handler = sseHandler
	if ms.authenticator.config.Mode != AuthModeNone {
		handler = ms.authenticator.Middleware(sseHandler)
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	srv := &http.Server{Addr: addr, Handler: handler}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	// Register instance for discovery
	inst := config.Instance{
		Type:      config.InstanceServerMCP,
		PID:       os.Getpid(),
		Port:      port,
		Host:      host,
		StartedAt: time.Now(),
	}
	if err := config.RegisterInstance(inst); err != nil {
		tuilog.Log.Warn("Failed to register MCP instance", "error", err)
	}

	go func() {
		<-ctx.Done()
		_ = config.UnregisterInstance(os.Getpid()) // Ignore error, cleanup is best-effort
		_ = srv.Shutdown(context.Background())     // Ignore error, shutdown errors are logged by server
	}()
	return srv.Serve(ln)
}

func (ms *MCPServer) Server() *mcp.Server { return ms.server }

func formatJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
