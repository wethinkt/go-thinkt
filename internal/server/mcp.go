package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tui"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tuilog"
)

// MCPServer wraps an MCP server for thinkt.
type MCPServer struct {
	server   *mcp.Server
	registry *thinkt.StoreRegistry
}

// NewMCPServer creates a new MCP server with thinkt tools.
func NewMCPServer(registry *thinkt.StoreRegistry) *MCPServer {
	tuilog.Log.Info("NewMCPServer: creating MCP server")
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "thinkt",
		Version: "1.0.0",
	}, nil)

	ms := &MCPServer{
		server:   server,
		registry: registry,
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

	// get_session - Get session content
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "get_session",
		Description: "Get the content of a session by path. Supports pagination with limit/offset.",
	}, ms.handleGetSession)
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

type getSessionInput struct {
	Path   string `json:"path"`             // Full path to the session file (required)
	Limit  int    `json:"limit,omitempty"`  // Maximum number of entries to return (0 for all)
	Offset int    `json:"offset,omitempty"` // Number of entries to skip
}

type getSessionOutput struct {
	Meta    sessionMeta    `json:"meta"`
	Entries []thinkt.Entry `json:"entries"`
	HasMore bool           `json:"has_more"`
	Total   int            `json:"total"`
}

type sessionMeta struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	CreatedAt string `json:"created_at,omitempty"`
	Model     string `json:"model,omitempty"`
	GitBranch string `json:"git_branch,omitempty"`
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

func (ms *MCPServer) handleGetSession(ctx context.Context, req *mcp.CallToolRequest, input getSessionInput) (*mcp.CallToolResult, getSessionOutput, error) {
	tuilog.Log.Info("handleGetSession: called", "path", input.Path, "limit", input.Limit, "offset", input.Offset)
	if input.Path == "" {
		tuilog.Log.Error("handleGetSession: path is required")
		return nil, getSessionOutput{}, fmt.Errorf("path is required")
	}

	// Open the session
	tuilog.Log.Info("handleGetSession: opening session")
	ls, err := tui.OpenLazySession(input.Path)
	if err != nil {
		tuilog.Log.Error("handleGetSession: failed to open session", "error", err)
		return nil, getSessionOutput{}, fmt.Errorf("open session: %w", err)
	}
	defer ls.Close()

	// Load entries based on limit
	if input.Limit > 0 {
		targetBytes := (input.Offset + input.Limit) * 4096
		tuilog.Log.Info("handleGetSession: loading partial", "target_bytes", targetBytes)
		ls.LoadMore(targetBytes)
	} else {
		tuilog.Log.Info("handleGetSession: loading all")
		ls.LoadAll()
	}

	entries := ls.Entries()
	total := len(entries)
	hasMore := ls.HasMore()
	tuilog.Log.Info("handleGetSession: loaded entries", "total", total, "has_more", hasMore)

	// Apply offset and limit
	if input.Offset > 0 {
		if input.Offset >= len(entries) {
			entries = nil
		} else {
			entries = entries[input.Offset:]
		}
	}
	if input.Limit > 0 && input.Limit < len(entries) {
		entries = entries[:input.Limit]
		hasMore = true
	}

	meta := ls.Metadata()
	output := getSessionOutput{
		Meta: sessionMeta{
			ID:        meta.ID,
			Path:      meta.FullPath,
			Model:     meta.Model,
			GitBranch: meta.GitBranch,
		},
		Entries: entries,
		HasMore: hasMore,
		Total:   total,
	}
	if !meta.CreatedAt.IsZero() {
		output.Meta.CreatedAt = meta.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	tuilog.Log.Info("handleGetSession: returning", "entries", len(entries), "total", total, "has_more", hasMore)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: formatJSON(output)},
		},
	}, output, nil
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
	tuilog.Log.Info("RunHTTP: starting", "host", host, "port", port)

	// Create SSE handler that returns our MCP server for each connection
	sseHandler := mcp.NewSSEHandler(func(req *http.Request) *mcp.Server {
		tuilog.Log.Info("RunHTTP: new SSE connection", "remote", req.RemoteAddr)
		return ms.server
	}, nil)

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", host, port)
	srv := &http.Server{
		Addr:    addr,
		Handler: sseHandler,
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

// nopWriteCloser wraps an io.Writer with a no-op Close.
type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

// bufferedStdinReader reads all of stdin into a buffer, then delays EOF
// to give the server time to process and respond.
type bufferedStdinReader struct {
	buf     *bytes.Reader
	ctx     context.Context
	eofSent bool
	mu      sync.Mutex
}

func newBufferedStdinReader(ctx context.Context) *bufferedStdinReader {
	tuilog.Log.Info("bufferedStdinReader: reading all stdin")
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		tuilog.Log.Error("bufferedStdinReader: failed to read stdin", "error", err)
	}
	tuilog.Log.Info("bufferedStdinReader: read complete", "bytes", len(input), "content", string(input))
	return &bufferedStdinReader{
		buf: bytes.NewReader(input),
		ctx: ctx,
	}
}

func (r *bufferedStdinReader) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Return buffered content first
	n, err = r.buf.Read(p)
	if n > 0 {
		tuilog.Log.Debug("bufferedStdinReader.Read: returning buffered data", "bytes", n)
		return n, err
	}
	if err != io.EOF {
		tuilog.Log.Debug("bufferedStdinReader.Read: non-EOF error", "error", err)
		return n, err
	}

	// Buffer exhausted - delay EOF to allow response processing
	if r.eofSent {
		tuilog.Log.Debug("bufferedStdinReader.Read: returning EOF (already sent)")
		return 0, io.EOF
	}

	tuilog.Log.Info("bufferedStdinReader.Read: buffer exhausted, waiting 100ms before EOF")
	// Wait briefly for server to process and write responses
	select {
	case <-time.After(100 * time.Millisecond):
		tuilog.Log.Info("bufferedStdinReader.Read: wait complete, sending EOF")
	case <-r.ctx.Done():
		tuilog.Log.Info("bufferedStdinReader.Read: context cancelled, sending EOF")
	}

	r.eofSent = true
	return 0, io.EOF
}

func (r *bufferedStdinReader) Close() error {
	return nil
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
