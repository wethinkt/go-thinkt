package copilot

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Store implements thinkt.Store for Copilot CLI sessions.
type Store struct {
	baseDir string
	cache   thinkt.StoreCache
}

// NewStore creates a new Copilot store.
func NewStore(baseDir string) *Store {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".copilot")
	}
	return &Store{baseDir: baseDir}
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
	if cached, err, ok := s.cache.GetProjects(); ok {
		return cached, err
	}

	sessions, err := s.scanSessions()
	if err != nil {
		s.cache.SetProjects(nil, err)
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
				ID:           path, // Use path as ID
				Name:         filepath.Base(path),
				Path:         path,
				DisplayPath:  path,
				Source:       thinkt.SourceCopilot,
				WorkspaceID:  ws.ID,
				LastModified: sess.ModifiedAt,
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

	s.cache.SetProjects(projects, nil)
	return projects, nil
}

// ResetCache clears all cached data, forcing the next calls to rescan.
func (s *Store) ResetCache() { s.cache.Reset() }

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
func (s *Store) ListSessions(ctx context.Context, projectID string) ([]thinkt.SessionMeta, error) {
	if cached, err, ok := s.cache.GetSessions(projectID); ok {
		return cached, err
	}

	allSessions, err := s.scanSessions()
	if err != nil {
		s.cache.SetSessions(projectID, nil, err)
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

	s.cache.SetSessions(projectID, sessions, nil)
	return sessions, nil
}

// GetSessionMeta returns session metadata.
func (s *Store) GetSessionMeta(ctx context.Context, sessionID string) (*thinkt.SessionMeta, error) {
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

	var sessions []thinkt.SessionMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		
		id := e.Name()
		fullPath := filepath.Join(sessionDir, id, "events.jsonl")
		
		// Skip if no events file
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue
		}

		meta, err := s.readSessionMeta(id, fullPath)
		if err != nil {
			// Skip malformed sessions but log error?
			continue
		}
		sessions = append(sessions, *meta)
	}
	return sessions, nil
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
