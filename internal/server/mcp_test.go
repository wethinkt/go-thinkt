package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// testStore implements thinkt.Store for MCP integration tests.
type testStore struct {
	source   thinkt.Source
	projects []thinkt.Project
	sessions map[string][]thinkt.SessionMeta // projectID -> sessions
}

func (s *testStore) Source() thinkt.Source { return s.source }
func (s *testStore) Workspace() thinkt.Workspace {
	return thinkt.Workspace{ID: "test-ws", Source: s.source, BasePath: "/tmp/test"}
}
func (s *testStore) ListProjects(ctx context.Context) ([]thinkt.Project, error) {
	return s.projects, nil
}
func (s *testStore) GetProject(ctx context.Context, id string) (*thinkt.Project, error) {
	for _, p := range s.projects {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, nil
}
func (s *testStore) ListSessions(ctx context.Context, projectID string) ([]thinkt.SessionMeta, error) {
	if s.sessions != nil {
		return s.sessions[projectID], nil
	}
	return nil, nil
}
func (s *testStore) GetSessionMeta(ctx context.Context, sessionID string) (*thinkt.SessionMeta, error) {
	return nil, nil
}
func (s *testStore) LoadSession(ctx context.Context, sessionID string) (*thinkt.Session, error) {
	return nil, nil
}
func (s *testStore) OpenSession(ctx context.Context, sessionID string) (thinkt.SessionReader, error) {
	return nil, nil
}

// newTestMCPServer creates an MCPServer with the given stores registered.
func newTestMCPServer(stores ...thinkt.Store) *MCPServer {
	registry := thinkt.NewRegistry()
	for _, s := range stores {
		registry.Register(s)
	}
	ms := NewMCPServerWithAuth(registry, AuthConfig{Mode: AuthModeNone})
	ms.SetToolFilters(nil, nil)
	return ms
}

// callTool is a test helper that invokes an MCP tool handler by name via the server.
func callTool(t *testing.T, ms *MCPServer, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx := context.Background()

	ct, st := mcp.NewInMemoryTransports()
	_, err := ms.server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}

	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", name, err)
	}
	return result
}

// callToolMayError is like callTool but does not fatal on errors.
// It returns nil if a transport-level error occurs (tool not found, etc.).
func callToolMayError(t *testing.T, ms *MCPServer, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx := context.Background()

	ct, st := mcp.NewInMemoryTransports()
	_, err := ms.server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}

	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil
	}
	return result
}

// parseToolResult extracts the JSON text from a CallToolResult and unmarshals it into v.
func parseToolResult(t *testing.T, result *mcp.CallToolResult, v any) {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("empty result content")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if err := json.Unmarshal([]byte(tc.Text), v); err != nil {
		t.Fatalf("unmarshal result: %v\nraw: %s", err, tc.Text)
	}
}

// --- list_sources ---

func TestMCP_ListSources_Empty(t *testing.T) {
	ms := newTestMCPServer()
	result := callTool(t, ms, "list_sources", nil)

	var out listSourcesOutput
	parseToolResult(t, result, &out)

	if len(out.Sources) != 0 {
		t.Errorf("expected 0 sources, got %d", len(out.Sources))
	}
}

func TestMCP_ListSources(t *testing.T) {
	ms := newTestMCPServer(
		&testStore{source: thinkt.SourceClaude, projects: []thinkt.Project{{ID: "p1"}}},
		&testStore{source: thinkt.SourceKimi},
	)
	result := callTool(t, ms, "list_sources", nil)

	var out listSourcesOutput
	parseToolResult(t, result, &out)

	if len(out.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(out.Sources))
	}

	sourceNames := map[string]bool{}
	for _, s := range out.Sources {
		sourceNames[s.Name] = true
	}
	if !sourceNames["claude"] || !sourceNames["kimi"] {
		t.Errorf("expected claude and kimi sources, got %v", sourceNames)
	}
}

// --- list_projects ---

func TestMCP_ListProjects_Empty(t *testing.T) {
	ms := newTestMCPServer(&testStore{source: thinkt.SourceClaude})
	result := callTool(t, ms, "list_projects", nil)

	var out listProjectsOutput
	parseToolResult(t, result, &out)

	if out.Total != 0 {
		t.Errorf("expected total 0, got %d", out.Total)
	}
	if len(out.Projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(out.Projects))
	}
}

func TestMCP_ListProjects(t *testing.T) {
	now := time.Now()
	pathOne := t.TempDir()
	pathTwo := t.TempDir()
	ms := newTestMCPServer(&testStore{
		source: thinkt.SourceClaude,
		projects: []thinkt.Project{
			{ID: "p1", Name: "project-one", Path: pathOne, SessionCount: 3, Source: thinkt.SourceClaude, LastModified: now.Add(-time.Hour), PathExists: true},
			{ID: "p2", Name: "project-two", Path: pathTwo, SessionCount: 1, Source: thinkt.SourceClaude, LastModified: now, PathExists: true},
		},
	})
	result := callTool(t, ms, "list_projects", nil)

	var out listProjectsOutput
	parseToolResult(t, result, &out)

	if out.Total != 2 {
		t.Errorf("expected total 2, got %d", out.Total)
	}
	if out.Returned != 2 {
		t.Errorf("expected returned 2, got %d", out.Returned)
	}
	// Should be sorted newest first
	if out.Projects[0].ID != "p2" {
		t.Errorf("expected newest project first (p2), got %s", out.Projects[0].ID)
	}
}

func TestMCP_ListProjects_SourceFilter(t *testing.T) {
	claudePath := t.TempDir()
	kimiPath := t.TempDir()
	ms := newTestMCPServer(
		&testStore{source: thinkt.SourceClaude, projects: []thinkt.Project{
			{ID: "c1", Name: "claude-proj", Path: claudePath, Source: thinkt.SourceClaude, PathExists: true},
		}},
		&testStore{source: thinkt.SourceKimi, projects: []thinkt.Project{
			{ID: "k1", Name: "kimi-proj", Path: kimiPath, Source: thinkt.SourceKimi, PathExists: true},
		}},
	)
	result := callTool(t, ms, "list_projects", map[string]any{"source": "kimi"})

	var out listProjectsOutput
	parseToolResult(t, result, &out)

	if out.Total != 1 {
		t.Errorf("expected total 1, got %d", out.Total)
	}
	if out.Projects[0].Source != "kimi" {
		t.Errorf("expected kimi source, got %s", out.Projects[0].Source)
	}
}

func TestMCP_ListProjects_Pagination(t *testing.T) {
	projects := make([]thinkt.Project, 5)
	baseDir := t.TempDir()
	for i := range projects {
		projects[i] = thinkt.Project{
			ID: string(rune('a' + i)), Name: string(rune('a' + i)),
			Path:   filepath.Join(baseDir, string(rune('a'+i))),
			Source: thinkt.SourceClaude, LastModified: time.Now().Add(time.Duration(i) * time.Minute), PathExists: true,
		}
		if err := os.MkdirAll(projects[i].Path, 0o755); err != nil {
			t.Fatalf("failed creating test project path: %v", err)
		}
	}
	ms := newTestMCPServer(&testStore{source: thinkt.SourceClaude, projects: projects})

	result := callTool(t, ms, "list_projects", map[string]any{"limit": 2, "offset": 1})

	var out listProjectsOutput
	parseToolResult(t, result, &out)

	if out.Total != 5 {
		t.Errorf("expected total 5, got %d", out.Total)
	}
	if out.Returned != 2 {
		t.Errorf("expected returned 2, got %d", out.Returned)
	}
}

func TestMCP_ListProjects_ExcludesDeletedByDefault(t *testing.T) {
	now := time.Now()
	activePath := t.TempDir()
	deletedPath := filepath.Join(t.TempDir(), "deleted")
	ms := newTestMCPServer(&testStore{
		source: thinkt.SourceClaude,
		projects: []thinkt.Project{
			{ID: "active", Name: "active", Path: activePath, Source: thinkt.SourceClaude, LastModified: now, PathExists: true},
			{ID: "deleted", Name: "deleted", Path: deletedPath, Source: thinkt.SourceClaude, LastModified: now.Add(-time.Minute), PathExists: false},
		},
	})
	result := callTool(t, ms, "list_projects", nil)

	var out listProjectsOutput
	parseToolResult(t, result, &out)

	if out.Total != 1 {
		t.Fatalf("expected total 1, got %d", out.Total)
	}
	if len(out.Projects) != 1 || out.Projects[0].ID != "active" {
		t.Fatalf("expected only active project, got %+v", out.Projects)
	}
}

func TestMCP_ListProjects_IncludeDeleted(t *testing.T) {
	now := time.Now()
	activePath := t.TempDir()
	deletedPath := filepath.Join(t.TempDir(), "deleted")
	ms := newTestMCPServer(&testStore{
		source: thinkt.SourceClaude,
		projects: []thinkt.Project{
			{ID: "active", Name: "active", Path: activePath, Source: thinkt.SourceClaude, LastModified: now, PathExists: true},
			{ID: "deleted", Name: "deleted", Path: deletedPath, Source: thinkt.SourceClaude, LastModified: now.Add(-time.Minute), PathExists: false},
		},
	})
	result := callTool(t, ms, "list_projects", map[string]any{"include_deleted": true})

	var out listProjectsOutput
	parseToolResult(t, result, &out)

	if out.Total != 2 {
		t.Fatalf("expected total 2, got %d", out.Total)
	}
	if out.Returned != 2 {
		t.Fatalf("expected returned 2, got %d", out.Returned)
	}
}

// --- list_sessions ---

func TestMCP_ListSessions(t *testing.T) {
	now := time.Now()
	ms := newTestMCPServer(&testStore{
		source: thinkt.SourceClaude,
		sessions: map[string][]thinkt.SessionMeta{
			"proj1": {
				{ID: "s1", FullPath: "/a/s1.jsonl", Source: thinkt.SourceClaude, ModifiedAt: now.Add(-time.Hour)},
				{ID: "s2", FullPath: "/a/s2.jsonl", Source: thinkt.SourceClaude, ModifiedAt: now},
			},
		},
	})
	result := callTool(t, ms, "list_sessions", map[string]any{"project_id": "proj1", "source": "claude"})

	var out listSessionsOutput
	parseToolResult(t, result, &out)

	if out.Total != 2 {
		t.Errorf("expected total 2, got %d", out.Total)
	}
	// Sorted newest first
	if out.Sessions[0].ID != "s2" {
		t.Errorf("expected newest session first (s2), got %s", out.Sessions[0].ID)
	}
}

func TestMCP_ListSessions_Pagination(t *testing.T) {
	sessions := make([]thinkt.SessionMeta, 5)
	for i := range sessions {
		sessions[i] = thinkt.SessionMeta{
			ID: string(rune('a' + i)), Source: thinkt.SourceClaude,
			ModifiedAt: time.Now().Add(time.Duration(i) * time.Minute),
		}
	}
	ms := newTestMCPServer(&testStore{
		source:   thinkt.SourceClaude,
		sessions: map[string][]thinkt.SessionMeta{"p": sessions},
	})

	result := callTool(t, ms, "list_sessions", map[string]any{"project_id": "p", "source": "claude", "limit": 2, "offset": 0})

	var out listSessionsOutput
	parseToolResult(t, result, &out)

	if out.Total != 5 {
		t.Errorf("expected total 5, got %d", out.Total)
	}
	if out.Returned != 2 {
		t.Errorf("expected returned 2, got %d", out.Returned)
	}
}

func TestMCP_ListSessions_Empty(t *testing.T) {
	ms := newTestMCPServer(&testStore{
		source:   thinkt.SourceClaude,
		sessions: map[string][]thinkt.SessionMeta{},
	})
	result := callTool(t, ms, "list_sessions", map[string]any{"project_id": "nonexistent", "source": "claude"})

	var out listSessionsOutput
	parseToolResult(t, result, &out)

	if out.Total != 0 {
		t.Errorf("expected total 0, got %d", out.Total)
	}
}

// --- Session file helpers ---

// createTestClaudeSession creates a Claude JSONL session file with realistic entries.
func createTestClaudeSession(t *testing.T, dir string) string {
	t.Helper()

	path := filepath.Join(dir, "test-session.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating session file: %v", err)
	}
	defer f.Close()

	entries := []map[string]any{
		{
			"type": "user", "uuid": "u1",
			"timestamp": "2024-06-01T10:00:00Z",
			"message":   map[string]any{"role": "user", "content": "Hello, Claude! Can you help me with Go?"},
		},
		{
			"type": "assistant", "uuid": "a1",
			"timestamp": "2024-06-01T10:00:05Z",
			"message": map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "text", "text": "Of course! I'd be happy to help with Go programming."},
				map[string]any{"type": "thinking", "thinking": "The user wants Go help, let me provide useful guidance."},
				map[string]any{"type": "tool_use", "id": "tool1", "name": "Read", "input": map[string]any{"path": "/main.go"}},
			}},
		},
		{
			"type": "tool", "uuid": "t1",
			"timestamp": "2024-06-01T10:00:06Z",
			"message": map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "tool1", "content": "package main\nfunc main() {}"},
			}},
		},
		{
			"type": "assistant", "uuid": "a2",
			"timestamp": "2024-06-01T10:00:10Z",
			"message": map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "text", "text": "I can see your main.go file. What would you like to change?"},
			}},
		},
		{
			"type": "user", "uuid": "u2",
			"timestamp": "2024-06-01T10:01:00Z",
			"message":   map[string]any{"role": "user", "content": "Add a hello world print"},
		},
	}

	enc := json.NewEncoder(f)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			t.Fatalf("encoding entry: %v", err)
		}
	}
	return path
}

// --- get_session_metadata ---

func TestMCP_GetSessionMetadata(t *testing.T) {
	dir := t.TempDir()
	path := createTestClaudeSession(t, dir)
	ms := newTestMCPServer()

	result := callTool(t, ms, "get_session_metadata", map[string]any{"path": path})

	var out getSessionMetadataOutput
	parseToolResult(t, result, &out)

	if out.TotalEntries == 0 {
		t.Error("expected entries, got 0")
	}
	if out.Meta.Path != path {
		t.Errorf("expected path %s, got %s", path, out.Meta.Path)
	}
	if out.RoleCounts["user"] != 2 {
		t.Errorf("expected 2 user entries, got %d", out.RoleCounts["user"])
	}
	if out.RoleCounts["assistant"] != 2 {
		t.Errorf("expected 2 assistant entries, got %d", out.RoleCounts["assistant"])
	}
	if out.Description == "" {
		t.Error("expected description from first user message")
	}
}

func TestMCP_GetSessionMetadata_SummaryOnly(t *testing.T) {
	dir := t.TempDir()
	path := createTestClaudeSession(t, dir)
	ms := newTestMCPServer()

	result := callTool(t, ms, "get_session_metadata", map[string]any{
		"path": path, "summary_only": true,
	})

	var out getSessionMetadataOutput
	parseToolResult(t, result, &out)

	if len(out.EntrySummary) == 0 {
		t.Fatalf("expected entry previews with summary_only, got 0")
	}
	if out.Returned != len(out.EntrySummary) {
		t.Fatalf("expected returned=%d, got %d", len(out.EntrySummary), out.Returned)
	}
	for _, summary := range out.EntrySummary {
		if summary.Role != "user" {
			t.Fatalf("expected only user previews in summary_only mode, got role=%s", summary.Role)
		}
	}
	// Metadata should still be present
	if out.TotalEntries == 0 {
		t.Error("expected total entries even with summary_only")
	}
}

func TestMCP_GetSessionMetadata_Pagination(t *testing.T) {
	dir := t.TempDir()
	path := createTestClaudeSession(t, dir)
	ms := newTestMCPServer()

	result := callTool(t, ms, "get_session_metadata", map[string]any{
		"path": path, "limit": 2, "offset": 0,
	})

	var out getSessionMetadataOutput
	parseToolResult(t, result, &out)

	if out.Returned > 2 {
		t.Errorf("expected at most 2 returned summaries, got %d", out.Returned)
	}
}

func TestMCP_GetSessionMetadata_InvalidPath(t *testing.T) {
	ms := newTestMCPServer()
	result := callToolMayError(t, ms, "get_session_metadata", map[string]any{"path": "/nonexistent/file.jsonl"})
	// Should return an error (either as MCP error or as IsError in result)
	if result != nil && !result.IsError {
		t.Error("expected error result for nonexistent path")
	}
}

// --- get_session_entries ---

func TestMCP_GetSessionEntries(t *testing.T) {
	dir := t.TempDir()
	path := createTestClaudeSession(t, dir)
	ms := newTestMCPServer()

	result := callTool(t, ms, "get_session_entries", map[string]any{
		"path": path,
	})

	var out getSessionEntriesOutput
	parseToolResult(t, result, &out)

	if out.Total == 0 {
		t.Fatal("expected entries, got 0 total")
	}
	// Default limit is 5, our test session has 5 entries
	if out.Returned == 0 {
		t.Error("expected some returned entries")
	}
	// First entry should be user
	if out.Entries[0].Role != "user" {
		t.Errorf("expected first entry to be user, got %s", out.Entries[0].Role)
	}
}

func TestMCP_GetSessionEntries_RoleFilter(t *testing.T) {
	dir := t.TempDir()
	path := createTestClaudeSession(t, dir)
	ms := newTestMCPServer()

	result := callTool(t, ms, "get_session_entries", map[string]any{
		"path": path, "roles": []string{"user"}, "limit": 20,
	})

	var out getSessionEntriesOutput
	parseToolResult(t, result, &out)

	for _, e := range out.Entries {
		if e.Role != "user" {
			t.Errorf("expected only user entries, got %s", e.Role)
		}
	}
	if out.Returned != 2 {
		t.Errorf("expected 2 user entries, got %d", out.Returned)
	}
}

func TestMCP_GetSessionEntries_ByIndex(t *testing.T) {
	dir := t.TempDir()
	path := createTestClaudeSession(t, dir)
	ms := newTestMCPServer()

	result := callTool(t, ms, "get_session_entries", map[string]any{
		"path": path, "entry_indices": []int{0, 3},
	})

	var out getSessionEntriesOutput
	parseToolResult(t, result, &out)

	if out.Returned != 2 {
		t.Fatalf("expected 2 entries, got %d", out.Returned)
	}
	if out.Entries[0].Index != 0 {
		t.Errorf("expected index 0, got %d", out.Entries[0].Index)
	}
	if out.Entries[1].Index != 3 {
		t.Errorf("expected index 3, got %d", out.Entries[1].Index)
	}
}

func TestMCP_GetSessionEntries_Truncation(t *testing.T) {
	dir := t.TempDir()
	path := createTestClaudeSession(t, dir)
	ms := newTestMCPServer()

	result := callTool(t, ms, "get_session_entries", map[string]any{
		"path": path, "max_content_length": 10,
	})

	var out getSessionEntriesOutput
	parseToolResult(t, result, &out)

	for _, e := range out.Entries {
		if e.Text != "" && len(e.Text) > 10 {
			t.Errorf("expected text truncated to 10 chars, got %d: %q", len(e.Text), e.Text)
		}
	}
}

func TestMCP_GetSessionEntries_IncludeThinking(t *testing.T) {
	dir := t.TempDir()
	path := createTestClaudeSession(t, dir)
	ms := newTestMCPServer()

	// Without include_thinking
	result := callTool(t, ms, "get_session_entries", map[string]any{
		"path": path, "limit": 20,
	})
	var out getSessionEntriesOutput
	parseToolResult(t, result, &out)

	hasThinking := false
	for _, e := range out.Entries {
		if e.Thinking != "" {
			hasThinking = true
		}
	}
	if hasThinking {
		t.Error("thinking should be excluded by default")
	}

	// With include_thinking
	result = callTool(t, ms, "get_session_entries", map[string]any{
		"path": path, "include_thinking": true, "limit": 20,
	})
	parseToolResult(t, result, &out)

	hasThinking = false
	for _, e := range out.Entries {
		if e.Thinking != "" {
			hasThinking = true
		}
	}
	if !hasThinking {
		t.Error("expected thinking blocks with include_thinking=true")
	}
}

func TestMCP_GetSessionEntries_Pagination(t *testing.T) {
	dir := t.TempDir()
	path := createTestClaudeSession(t, dir)
	ms := newTestMCPServer()

	result := callTool(t, ms, "get_session_entries", map[string]any{
		"path": path, "limit": 2, "offset": 0,
	})

	var out getSessionEntriesOutput
	parseToolResult(t, result, &out)

	if out.Returned != 2 {
		t.Errorf("expected 2 returned, got %d", out.Returned)
	}
	if !out.HasMore {
		t.Error("expected has_more=true")
	}
}

func TestMCP_GetSessionEntries_MissingPath(t *testing.T) {
	ms := newTestMCPServer()
	result := callToolMayError(t, ms, "get_session_entries", map[string]any{})
	// Should return an error for missing path
	if result != nil && !result.IsError {
		t.Error("expected error result for missing path")
	}
}

// --- Tool filtering ---

func TestMCP_ToolFilter_AllowList(t *testing.T) {
	registry := thinkt.NewRegistry()
	ms := NewMCPServerWithAuth(registry, AuthConfig{Mode: AuthModeNone})
	ms.SetToolFilters([]string{"list_sources"}, nil)

	// list_sources should work
	result := callToolMayError(t, ms, "list_sources", nil)
	if result == nil || result.IsError {
		t.Error("list_sources should be allowed")
	}

	// list_projects should fail (not in allow list)
	result = callToolMayError(t, ms, "list_projects", nil)
	if result != nil && !result.IsError {
		t.Error("list_projects should not be allowed when only list_sources is in allow list")
	}
}

func TestMCP_ToolFilter_DenyList(t *testing.T) {
	registry := thinkt.NewRegistry()
	ms := NewMCPServerWithAuth(registry, AuthConfig{Mode: AuthModeNone})
	ms.SetToolFilters(nil, []string{"list_projects"})

	// list_sources should work
	result := callToolMayError(t, ms, "list_sources", nil)
	if result == nil || result.IsError {
		t.Error("list_sources should be allowed")
	}

	// list_projects should fail
	result = callToolMayError(t, ms, "list_projects", nil)
	if result != nil && !result.IsError {
		t.Error("list_projects should be denied")
	}
}

// --- Helpers ---

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "..."},
	}
	for _, tt := range tests {
		got := truncateString(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestBuildIndexerSearchArgs_IncludesSourceAndOptions(t *testing.T) {
	got := buildIndexerSearchArgs(searchSessionsInput{
		Query:           "DuckDB",
		Project:         "go-thinkt",
		Source:          " KIMI ",
		Limit:           50,
		LimitPerSession: 2,
		CaseSensitive:   true,
		Regex:           true,
	})

	want := []string{
		"search", "--json", "DuckDB",
		"--project", "go-thinkt",
		"--source", "kimi",
		"--limit", "50",
		"--limit-per-session", "2",
		"--case-sensitive",
		"--regex",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args:\n got=%v\nwant=%v", got, want)
	}
}
