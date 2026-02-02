// Package claude provides Claude Code session storage implementation.
package claude

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
)

// Store implements thinkt.Store for Claude Code sessions.
type Store struct {
	baseDir string
}

// NewStore creates a new Claude store.
func NewStore(baseDir string) *Store {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".claude")
	}
	return &Store{baseDir: baseDir}
}

// Source returns the store type.
func (s *Store) Source() thinkt.Source {
	return thinkt.SourceClaude
}

// Workspace returns information about this store's workspace.
func (s *Store) Workspace() thinkt.Workspace {
	hostname, _ := os.Hostname()

	// Try to read stable_id from statsig
	workspaceID := hostname // fallback
	stableIDPath := filepath.Join(s.baseDir, "statsig", "statsig.stable_id.*")
	if matches, _ := filepath.Glob(stableIDPath); len(matches) > 0 {
		if data, err := os.ReadFile(matches[0]); err == nil {
			workspaceID = strings.TrimSpace(string(data))
		}
	}

	return thinkt.Workspace{
		ID:       workspaceID,
		Name:     hostname,
		Hostname: hostname,
		Source:   thinkt.SourceClaude,
		BasePath: s.baseDir,
	}
}

// ListProjects returns all Claude projects.
func (s *Store) ListProjects(ctx context.Context) ([]thinkt.Project, error) {
	projects, err := ListProjects(s.baseDir)
	if err != nil {
		return nil, err
	}

	ws := s.Workspace()
	result := make([]thinkt.Project, len(projects))
	for i, p := range projects {
		result[i] = thinkt.Project{
			ID:           p.DirPath,
			Name:         p.DisplayName,
			Path:         p.FullPath,
			DisplayPath:  p.FullPath,
			SessionCount: p.SessionCount,
			LastModified: p.LastModified,
			Source:       thinkt.SourceClaude,
			WorkspaceID:  ws.ID,
		}
	}
	return result, nil
}

// GetProject returns a specific project.
func (s *Store) GetProject(ctx context.Context, id string) (*thinkt.Project, error) {
	// id could be a path or we need to search
	projects, err := s.ListProjects(ctx)
	if err != nil {
		return nil, err
	}

	for _, p := range projects {
		if p.ID == id || p.Path == id {
			return &p, nil
		}
	}
	return nil, nil
}

// ListSessions returns sessions for a project.
func (s *Store) ListSessions(ctx context.Context, projectID string) ([]thinkt.SessionMeta, error) {
	sessions, err := ListProjectSessions(projectID)
	if err != nil {
		return nil, err
	}

	ws := s.Workspace()
	result := make([]thinkt.SessionMeta, len(sessions))
	for i, sess := range sessions {
		result[i] = thinkt.SessionMeta{
			ID:          sess.SessionID,
			ProjectPath: projectID,
			FullPath:    sess.FullPath,
			FirstPrompt: sess.FirstPrompt,
			Summary:     sess.Summary,
			EntryCount:  sess.MessageCount,
			FileSize:    sess.FileSize,
			CreatedAt:   sess.Created,
			ModifiedAt:  sess.Modified,
			GitBranch:   sess.GitBranch,
			Source:      thinkt.SourceClaude,
			WorkspaceID: ws.ID,
			ChunkCount:  1, // Claude sessions are not chunked
		}
	}
	return result, nil
}

// GetSessionMeta returns session metadata.
func (s *Store) GetSessionMeta(ctx context.Context, sessionID string) (*thinkt.SessionMeta, error) {
	// sessionID could be full path or just UUID
	// Try to find it by listing sessions in the project
	
	// If sessionID contains a path separator, extract the project dir
	var projectDir string
	if filepath.IsAbs(sessionID) {
		projectDir = filepath.Dir(sessionID)
	} else {
		// Need to search - try to find in all projects
		projects, _ := s.ListProjects(ctx)
		for _, p := range projects {
			// p.ID is the actual directory path, p.Path is the decoded full path
			sessions, _ := s.ListSessions(ctx, p.ID)
			for _, sess := range sessions {
				if sess.ID == sessionID {
					return &sess, nil
				}
			}
		}
		return nil, nil
	}

	// List sessions in the project directory
	sessions, err := ListProjectSessions(projectDir)
	if err != nil {
		return nil, err
	}

	ws := s.Workspace()
	for _, sess := range sessions {
		if sess.SessionID == sessionID || sess.FullPath == sessionID {
			return &thinkt.SessionMeta{
				ID:          sess.SessionID,
				ProjectPath: projectDir,
				FullPath:    sess.FullPath,
				FirstPrompt: sess.FirstPrompt,
				Summary:     sess.Summary,
				EntryCount:  sess.MessageCount,
				CreatedAt:   sess.Created,
				ModifiedAt:  sess.Modified,
				GitBranch:   sess.GitBranch,
				Source:      thinkt.SourceClaude,
				WorkspaceID: ws.ID,
			}, nil
		}
	}
	return nil, nil
}

// LoadSession loads a complete session.
func (s *Store) LoadSession(ctx context.Context, sessionID string) (*thinkt.Session, error) {
	// sessionID could be full path or we need to construct it
	path := sessionID
	if !filepath.IsAbs(path) {
		// Assume it's a UUID and we need to find it
		meta, err := s.GetSessionMeta(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if meta == nil {
			return nil, nil
		}
		path = meta.FullPath
	}

	session, err := LoadSession(path)
	if err != nil {
		return nil, err
	}

	ws := s.Workspace()
	return convertSession(session, ws.ID), nil
}

// OpenSession returns a streaming reader for a session.
func (s *Store) OpenSession(ctx context.Context, sessionID string) (thinkt.SessionReader, error) {
	path := sessionID
	if !filepath.IsAbs(path) {
		meta, err := s.GetSessionMeta(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if meta == nil {
			return nil, nil
		}
		path = meta.FullPath
	}

	ws := s.Workspace()

	// Try lazy session first for efficiency
	ls, err := OpenLazySession(path)
	if err == nil {
		return &lazySessionReader{
			ls:          ls,
			source:      thinkt.SourceClaude,
			workspaceID: ws.ID,
		}, nil
	}

	// Fall back to regular parser
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return &parserReader{
		parser: NewParser(f),
		file:   f,
		meta: thinkt.SessionMeta{
			ID:          sessionID,
			FullPath:    path,
			Source:      thinkt.SourceClaude,
			WorkspaceID: ws.ID,
		},
		source:      thinkt.SourceClaude,
		workspaceID: ws.ID,
	}, nil
}

// parserReader adapts Parser to SessionReader.
type parserReader struct {
	parser      *Parser
	file        io.Closer
	meta        thinkt.SessionMeta
	closed      bool
	source      thinkt.Source
	workspaceID string
}

func (r *parserReader) ReadNext() (*thinkt.Entry, error) {
	if r.closed {
		return nil, io.ErrClosedPipe
	}

	entry, err := r.parser.NextEntry()
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, io.EOF
	}

	return convertEntry(entry, r.source, r.workspaceID), nil
}

func (r *parserReader) Metadata() thinkt.SessionMeta {
	return r.meta
}

func (r *parserReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.file.Close()
}

// lazySessionReader adapts LazySession to SessionReader.
type lazySessionReader struct {
	ls          *LazySession
	idx         int
	closed      bool
	source      thinkt.Source
	workspaceID string
}

func (r *lazySessionReader) ReadNext() (*thinkt.Entry, error) {
	if r.closed {
		return nil, io.ErrClosedPipe
	}

	// Ensure we have entries loaded
	for r.idx >= r.ls.EntryCount() && r.ls.HasMore() {
		if _, err := r.ls.LoadMore(32 * 1024); err != nil {
			return nil, err
		}
	}

	entries := r.ls.ClaudeEntries()
	if r.idx >= len(entries) {
		return nil, io.EOF
	}

	entry := convertEntry(&entries[r.idx], r.source, r.workspaceID)
	r.idx++
	return entry, nil
}

func (r *lazySessionReader) Metadata() thinkt.SessionMeta {
	return thinkt.SessionMeta{
		ID:          r.ls.ID,
		FullPath:    r.ls.Path,
		GitBranch:   r.ls.Branch,
		Model:       r.ls.Model,
		Source:      thinkt.SourceClaude,
		WorkspaceID: r.workspaceID,
	}
}

func (r *lazySessionReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.ls.Close()
}

// convertEntry converts a claude.Entry to thinkt.Entry.
func convertEntry(e *Entry, source thinkt.Source, workspaceID string) *thinkt.Entry {
	if e == nil {
		return nil
	}

	entry := &thinkt.Entry{
		UUID:        e.UUID,
		Timestamp:   parseTimestamp(e.Timestamp),
		GitBranch:   e.GitBranch,
		CWD:         e.CWD,
		IsSidechain: e.IsSidechain,
		Source:      source,
		WorkspaceID: workspaceID,
		Metadata:    make(map[string]any),
	}

	// Set checkpoint flag for file history snapshot entries
	if e.Type == EntryTypeFileHistorySnapshot {
		entry.IsCheckpoint = true
	}

	// Set parent UUID if present
	if e.ParentUUID != nil {
		entry.ParentUUID = e.ParentUUID
	}

	// Convert role/type
	entry.Role = convertRole(e.Type)

	// Convert content based on type
	switch e.Type {
	case EntryTypeUser:
		if msg := e.GetUserMessage(); msg != nil {
			entry.Text = msg.Content.GetText()
			// Convert content blocks if present
			if len(msg.Content.Blocks) > 0 {
				entry.ContentBlocks = convertUserBlocks(msg.Content.Blocks)
			}
		}
	case EntryTypeAssistant:
		if msg := e.GetAssistantMessage(); msg != nil {
			entry.Model = msg.Model
			entry.ContentBlocks = convertAssistantBlocks(msg.Content)
			if msg.Usage != nil {
				entry.Usage = &thinkt.TokenUsage{
					InputTokens:              msg.Usage.InputTokens,
					OutputTokens:             msg.Usage.OutputTokens,
					CacheCreationInputTokens: msg.Usage.CacheCreationInputTokens,
					CacheReadInputTokens:     msg.Usage.CacheReadInputTokens,
				}
			}
		}
	}

	// Store extra metadata
	if e.SessionID != "" {
		entry.Metadata["session_id"] = e.SessionID
	}
	if e.Version != "" {
		entry.Metadata["version"] = e.Version
	}

	return entry
}

func convertRole(t EntryType) thinkt.Role {
	switch t {
	case EntryTypeUser:
		return thinkt.RoleUser
	case EntryTypeAssistant:
		return thinkt.RoleAssistant
	case EntryTypeSystem:
		return thinkt.RoleSystem
	case EntryTypeSummary:
		return thinkt.RoleSummary
	case EntryTypeProgress:
		return thinkt.RoleProgress
	case EntryTypeFileHistorySnapshot:
		return thinkt.RoleCheckpoint
	default:
		return thinkt.RoleSystem
	}
}

func convertUserBlocks(blocks []ContentBlock) []thinkt.ContentBlock {
	result := make([]thinkt.ContentBlock, 0, len(blocks))
	for _, b := range blocks {
		cb := thinkt.ContentBlock{Type: b.Type}
		switch b.Type {
		case "text":
			cb.Text = b.Text
		case "tool_result":
			cb.ToolUseID = b.ToolUseID
			if len(b.ToolContent) > 0 {
				cb.ToolResult = string(b.ToolContent)
			}
			cb.IsError = b.IsError
		case "image", "document":
			if b.Source != nil {
				cb.MediaType = b.Source.MediaType
				cb.MediaData = b.Source.Data
			}
		}
		result = append(result, cb)
	}
	return result
}

func convertAssistantBlocks(blocks []ContentBlock) []thinkt.ContentBlock {
	result := make([]thinkt.ContentBlock, 0, len(blocks))
	for _, b := range blocks {
		cb := thinkt.ContentBlock{Type: b.Type}
		switch b.Type {
		case "text":
			cb.Text = b.Text
		case "thinking":
			cb.Thinking = b.Thinking
			cb.Signature = b.Signature
		case "tool_use":
			cb.ToolUseID = b.ID
			cb.ToolName = b.Name
			cb.ToolInput = b.Input
		}
		result = append(result, cb)
	}
	return result
}

func convertSession(s *Session, workspaceID string) *thinkt.Session {
	if s == nil {
		return nil
	}

	meta := thinkt.SessionMeta{
		ID:          s.ID,
		ProjectPath: s.Path,
		FullPath:    s.Path,
		GitBranch:   s.Branch,
		Model:       s.Model,
		CreatedAt:   s.StartTime,
		ModifiedAt:  s.EndTime,
		Source:      thinkt.SourceClaude,
		WorkspaceID: workspaceID,
	}

	entries := make([]thinkt.Entry, 0, len(s.Entries))
	for _, e := range s.Entries {
		if entry := convertEntry(&e, thinkt.SourceClaude, workspaceID); entry != nil {
			entries = append(entries, *entry)
		}
	}
	meta.EntryCount = len(entries)

	return &thinkt.Session{
		Meta:    meta,
		Entries: entries,
	}
}

func parseTimestamp(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, ts)
	return t
}
