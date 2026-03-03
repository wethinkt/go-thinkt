package copilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

const allSessionsCacheKey = "\x00copilot_all_sessions"

// Store implements thinkt.Store for Copilot CLI sessions.
type Store struct {
	baseDir  string
	cacheDir string // directory for persistent metadata cache
	cache    thinkt.StoreCache
	mc       *thinkt.MetadataCache // lazily loaded
}

// NewStore creates a new Copilot store.
func NewStore(baseDir string) *Store {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".copilot")
	}
	cacheDir := ""
	if dir, err := config.Dir(); err == nil {
		cacheDir = filepath.Join(dir, "cache")
	}
	return &Store{baseDir: baseDir, cacheDir: cacheDir}
}

// NewStoreWithCacheDir creates a Copilot store with an explicit cache directory.
// This is primarily useful for testing.
func NewStoreWithCacheDir(baseDir, cacheDir string) *Store {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".copilot")
	}
	return &Store{baseDir: baseDir, cacheDir: cacheDir}
}

// metadataCache returns the lazily-loaded persistent metadata cache.
func (s *Store) metadataCache() *thinkt.MetadataCache {
	if s.mc != nil {
		return s.mc
	}
	if s.cacheDir == "" {
		s.mc = &thinkt.MetadataCache{
			Version:  1,
			Source:   thinkt.SourceCopilot,
			Sessions: make(map[string]thinkt.CachedSession),
		}
		return s.mc
	}
	mc, _ := thinkt.LoadMetadataCache(thinkt.SourceCopilot, s.cacheDir)
	s.mc = mc
	return s.mc
}

// SetCacheTTL sets the cache time-to-live for this store.
func (s *Store) SetCacheTTL(d time.Duration) {
	s.cache.SetName("copilot")
	s.cache.SetTTL(d)
}

// Source returns the store type.
func (s *Store) Source() thinkt.Source {
	return thinkt.SourceCopilot
}

// Workspace returns information about this store's workspace.
func (s *Store) Workspace() thinkt.Workspace {
	hostname, _ := os.Hostname()
	return thinkt.Workspace{
		ID:       "copilot-cli-" + hostname,
		Name:     "Copilot CLI",
		Hostname: hostname,
		Source:   thinkt.SourceCopilot,
		BasePath: s.baseDir,
	}
}

// ListProjects returns all Copilot projects (derived from sessions). Results
// are cached after the first call. Use ResetCache to force a rescan.
func (s *Store) ListProjects(ctx context.Context) ([]thinkt.Project, error) {
	return s.cache.LoadProjects(func() ([]thinkt.Project, error) {
		sessions, err := s.loadAllSessions()
		if err != nil {
			return nil, err
		}

		projectsMap := make(map[string]*thinkt.Project)
		ws := s.Workspace()

		for _, sess := range sessions {
			path := sess.ProjectPath
			if path == "" {
				path = "unknown"
			}

			if _, exists := projectsMap[path]; !exists {
				projectsMap[path] = &thinkt.Project{
					ID:             path, // Use path as ID
					Name:           filepath.Base(path),
					Path:           path,
					DisplayPath:    path,
					Source:         thinkt.SourceCopilot,
					WorkspaceID:    ws.ID,
					SourceBasePath: ws.BasePath,
					LastModified:   sess.ModifiedAt,
				}
			}

			p := projectsMap[path]
			p.SessionCount++
			if sess.ModifiedAt.After(p.LastModified) {
				p.LastModified = sess.ModifiedAt
			}
		}

		var projects []thinkt.Project
		for _, p := range projectsMap {
			projects = append(projects, *p)
		}

		// Sort by last modified desc
		sort.Slice(projects, func(i, j int) bool {
			return projects[i].LastModified.After(projects[j].LastModified)
		})

		return projects, nil
	})
}

// ResetCache clears all cached data, forcing the next calls to rescan.
func (s *Store) ResetCache() { s.cache.Clear() }

// GetProject returns a specific project.
func (s *Store) GetProject(ctx context.Context, id string) (*thinkt.Project, error) {
	projects, err := s.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, nil
}

// ListSessions returns sessions for a project.
func (s *Store) ListSessions(ctx context.Context, projectID string, opts ...thinkt.ListSessionsOption) ([]thinkt.SessionMeta, error) {
	return s.cache.LoadSessions(projectID, func() ([]thinkt.SessionMeta, error) {
		allSessions, err := s.loadAllSessions()
		if err != nil {
			return nil, err
		}

		var sessions []thinkt.SessionMeta
		for _, sess := range allSessions {
			// If projectID matches the session's project path
			if sess.ProjectPath == projectID {
				sessions = append(sessions, sess)
			}
		}

		// Sort by created desc
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
		})

		return sessions, nil
	})
}

func (s *Store) loadAllSessions() ([]thinkt.SessionMeta, error) {
	return s.cache.LoadSessions(allSessionsCacheKey, func() ([]thinkt.SessionMeta, error) {
		return s.scanSessions()
	})
}

// GetSessionMeta returns session metadata.
func (s *Store) GetSessionMeta(ctx context.Context, sessionID string) (*thinkt.SessionMeta, error) {
	// Support absolute path lookups for API/MCP/TUI path-based entry points.
	if filepath.IsAbs(sessionID) {
		// Validate path is under this store's base directory
		if err := thinkt.ValidateSessionPath(sessionID, s.baseDir); err != nil {
			return nil, nil
		}

		if _, err := os.Stat(sessionID); os.IsNotExist(err) {
			return nil, nil
		}
		id := filepath.Base(filepath.Dir(sessionID))
		return s.readSessionMeta(id, sessionID)
	}

	// sessionID is expected to be the UUID
	path := filepath.Join(s.baseDir, "session-state", sessionID, "events.jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}

	return s.readSessionMeta(sessionID, path)
}

// LoadSession loads a complete session.
func (s *Store) LoadSession(ctx context.Context, sessionID string) (*thinkt.Session, error) {
	meta, err := s.GetSessionMeta(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}

	f, err := os.Open(meta.FullPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	parser := NewParser(f)
	var entries []thinkt.Entry

	for {
		entry, err := parser.NextEntry()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// Fill in missing metadata derived from session
		entry.WorkspaceID = meta.WorkspaceID
		entries = append(entries, *entry)
	}

	return &thinkt.Session{
		Meta:    *meta,
		Entries: entries,
	}, nil
}

// OpenSession returns a streaming reader for a session.
func (s *Store) OpenSession(ctx context.Context, sessionID string) (thinkt.SessionReader, error) {
	meta, err := s.GetSessionMeta(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}

	f, err := os.Open(meta.FullPath)
	if err != nil {
		return nil, err
	}

	return &sessionReader{
		parser: NewParser(f),
		file:   f,
		meta:   *meta,
	}, nil
}

// -- Internal Helpers --

func (s *Store) scanSessions() ([]thinkt.SessionMeta, error) {
	sessionDir := filepath.Join(s.baseDir, "session-state")
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return nil, err // Return empty if dir doesn't exist?
	}

	mc := s.metadataCache()
	ws := s.Workspace()
	var sessions []thinkt.SessionMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		id := e.Name()
		fullPath := filepath.Join(sessionDir, id, "events.jsonl")

		// Skip if no events file
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		// Try cache first to avoid file parsing
		if cached, ok := mc.Lookup(fullPath, info.ModTime(), info.Size()); ok {
			meta := thinkt.SessionMeta{
				ID:          id,
				ProjectPath: "unknown",
				FullPath:    fullPath,
				FirstPrompt: cached.FirstPrompt,
				Model:       cached.Model,
				EntryCount:  cached.EntryCount,
				GitBranch:   cached.GitBranch,
				FileSize:    info.Size(),
				CreatedAt:   info.ModTime(), // approximation for cache hits
				ModifiedAt:  info.ModTime(),
				Source:      thinkt.SourceCopilot,
				WorkspaceID: ws.ID,
				ChunkCount:  1,
			}
			sessions = append(sessions, meta)
			continue
		}

		// Cache miss — parse the file
		meta, err := s.readSessionMetaFast(id, fullPath)
		if err != nil {
			// Skip malformed sessions but log error?
			continue
		}
		mc.Set(fullPath, thinkt.CachedSession{
			FirstPrompt: meta.FirstPrompt,
			Model:       meta.Model,
			EntryCount:  meta.EntryCount,
			GitBranch:   meta.GitBranch,
			ModifiedAt:  info.ModTime(),
			FileSize:    info.Size(),
		})
		sessions = append(sessions, *meta)
	}

	_ = mc.Save()
	return sessions, nil
}

func (s *Store) readSessionMetaFast(id, path string) (*thinkt.SessionMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	meta := &thinkt.SessionMeta{
		ID:          id,
		ProjectPath: "unknown",
		FullPath:    path,
		FileSize:    info.Size(),
		ModifiedAt:  info.ModTime(),
		Source:      thinkt.SourceCopilot,
		WorkspaceID: s.Workspace().ID,
		ChunkCount:  1,
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	const maxLines = 40
	scanner := thinkt.NewScannerWithMaxCapacity(f)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		if lineCount > maxLines {
			break
		}

		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		if meta.CreatedAt.IsZero() && !event.Timestamp.IsZero() {
			meta.CreatedAt = event.Timestamp
		}

		switch event.Type {
		case EventTypeSessionStart, EventTypeSessionInfo:
			if ctx, ok := event.Data["context"].(map[string]any); ok {
				if cwd, ok := ctx["cwd"].(string); ok && cwd != "" {
					meta.ProjectPath = cwd
				}
			}
		case EventTypeUserMessage:
			if meta.FirstPrompt == "" {
				if transformed, ok := event.Data["transformedContent"].(string); ok && transformed != "" {
					meta.FirstPrompt = thinkt.TruncateString(transformed, thinkt.DefaultTruncateLength)
				} else if content, ok := event.Data["content"].(string); ok && content != "" {
					meta.FirstPrompt = thinkt.TruncateString(content, thinkt.DefaultTruncateLength)
				}
			}
		case EventTypeAssistantMsg:
			if !thinkt.IsRealModel(meta.Model) {
				if model, ok := event.Data["model"].(string); ok && thinkt.IsRealModel(model) {
					meta.Model = model
				}
			}
		}

		if meta.ProjectPath != "unknown" && meta.FirstPrompt != "" {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = meta.ModifiedAt
	}

	return meta, nil
}

func (s *Store) readSessionMeta(id, path string) (*thinkt.SessionMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read first few lines to get session.start
	scanner := bufio.NewScanner(f)
	var projectPath string
	var createdAt time.Time

	// Also get file stats
	info, _ := f.Stat()

	var entryCount int
	for scanner.Scan() {
		entryCount++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if entryCount == 1 {
			var event Event
			if err := json.Unmarshal(line, &event); err == nil {
				createdAt = event.Timestamp
				if event.Type == EventTypeSessionStart {
					if ctx, ok := event.Data["context"].(map[string]any); ok {
						if cwd, ok := ctx["cwd"].(string); ok {
							projectPath = cwd
						}
					}
				}
			}
		}
	}

	return &thinkt.SessionMeta{
		ID:          id,
		ProjectPath: projectPath,
		FullPath:    path,
		FileSize:    info.Size(),
		CreatedAt:   createdAt,
		ModifiedAt:  info.ModTime(),
		EntryCount:  entryCount,
		Source:      thinkt.SourceCopilot,
		WorkspaceID: s.Workspace().ID,
	}, nil
}

type sessionReader struct {
	parser *Parser
	file   io.Closer
	meta   thinkt.SessionMeta
}

func (r *sessionReader) ReadNext() (*thinkt.Entry, error) {
	entry, err := r.parser.NextEntry()
	if err != nil {
		return nil, err
	}
	// Enrich
	entry.WorkspaceID = r.meta.WorkspaceID
	return entry, nil
}

func (r *sessionReader) Metadata() thinkt.SessionMeta {
	return r.meta
}

func (r *sessionReader) Close() error {
	return r.file.Close()
}

// WatchConfig returns the watch configuration for Copilot session files.
func (s *Store) WatchConfig() thinkt.WatchConfig {
	return thinkt.WatchConfig{
		IncludeDirs: []string{"session-state"},
		ExcludeDirs: []string{"rewind-snapshots", "backups"},
		MaxDepth:    3,
	}
}
