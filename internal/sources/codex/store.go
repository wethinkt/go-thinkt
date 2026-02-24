package codex

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Store implements thinkt.Store for Codex CLI sessions.
type Store struct {
	baseDir string
	cache   thinkt.StoreCache
}

// NewStore creates a new Codex store.
func NewStore(baseDir string) *Store {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".codex")
	}
	return &Store{baseDir: baseDir}
}

// SetCacheTTL sets the cache time-to-live for this store.
func (s *Store) SetCacheTTL(d time.Duration) {
	s.cache.SetName("codex")
	s.cache.SetTTL(d)
}

// Source returns the store type.
func (s *Store) Source() thinkt.Source {
	return thinkt.SourceCodex
}

// Workspace returns information about this store's workspace.
func (s *Store) Workspace() thinkt.Workspace {
	hostname, _ := os.Hostname()
	return thinkt.Workspace{
		ID:       "codex-cli-" + hostname,
		Name:     "Codex CLI",
		Hostname: hostname,
		Source:   thinkt.SourceCodex,
		BasePath: s.baseDir,
	}
}

// ResetCache clears all cached data.
func (s *Store) ResetCache() {
	s.cache.Clear()
}

// ListProjects returns all Codex projects inferred from session metadata.
func (s *Store) ListProjects(ctx context.Context) ([]thinkt.Project, error) {
	return s.cache.LoadProjects(func() ([]thinkt.Project, error) {
		sessions, err := s.scanSessions()
		if err != nil {
			return nil, err
		}

		ws := s.Workspace()
		projectsMap := make(map[string]*thinkt.Project)
		for _, sess := range sessions {
			projectPath := sess.ProjectPath
			if projectPath == "" {
				projectPath = "unknown"
			}

			p := projectsMap[projectPath]
			if p == nil {
				name := filepath.Base(projectPath)
				if projectPath == "unknown" || name == "" || name == "." || name == "/" {
					name = "unknown"
				}

				pathExists := false
				if projectPath != "unknown" {
					if info, err := os.Stat(projectPath); err == nil && info.IsDir() {
						pathExists = true
					}
				}

				p = &thinkt.Project{
					ID:             projectPath,
					Name:           name,
					Path:           projectPath,
					DisplayPath:    projectPath,
					Source:         thinkt.SourceCodex,
					WorkspaceID:    ws.ID,
					SourceBasePath: ws.BasePath,
					PathExists:     pathExists,
				}
				projectsMap[projectPath] = p
			}

			p.SessionCount++
			if sess.ModifiedAt.After(p.LastModified) {
				p.LastModified = sess.ModifiedAt
			}
		}

		projects := make([]thinkt.Project, 0, len(projectsMap))
		for _, p := range projectsMap {
			projects = append(projects, *p)
		}
		sort.Slice(projects, func(i, j int) bool {
			return projects[i].LastModified.After(projects[j].LastModified)
		})

		return projects, nil
	})
}

// GetProject returns a specific project by ID/path.
func (s *Store) GetProject(ctx context.Context, id string) (*thinkt.Project, error) {
	projects, err := s.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if projects[i].ID == id || projects[i].Path == id {
			return &projects[i], nil
		}
	}
	return nil, nil
}

// ListSessions returns sessions for a project.
func (s *Store) ListSessions(ctx context.Context, projectID string) ([]thinkt.SessionMeta, error) {
	return s.cache.LoadSessions(projectID, func() ([]thinkt.SessionMeta, error) {
		all, err := s.scanSessions()
		if err != nil {
			return nil, err
		}

		filtered := make([]thinkt.SessionMeta, 0, len(all))
		for _, sess := range all {
			if sess.ProjectPath == projectID {
				filtered = append(filtered, sess)
			}
		}
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].ModifiedAt.After(filtered[j].ModifiedAt)
		})

		return filtered, nil
	})
}

// GetSessionMeta returns session metadata.
func (s *Store) GetSessionMeta(ctx context.Context, sessionID string) (*thinkt.SessionMeta, error) {
	ws := s.Workspace()

	// Fast path for absolute file path lookups.
	if filepath.IsAbs(sessionID) {
		// Validate path is under this store's base directory
		if err := thinkt.ValidateSessionPath(sessionID, s.baseDir); err != nil {
			return nil, nil
		}

		if _, err := os.Stat(sessionID); os.IsNotExist(err) {
			return nil, nil
		}
		return s.readSessionMeta(sessionID, ws.ID)
	}

	// Otherwise scan and match by ID.
	sessions, err := s.scanSessions()
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		if sessions[i].ID == sessionID {
			return &sessions[i], nil
		}
	}
	return nil, nil
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

	parser := NewParser(f, meta.ID)
	entries := make([]thinkt.Entry, 0, meta.EntryCount)
	for {
		entry, err := parser.NextEntry()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		entry.WorkspaceID = meta.WorkspaceID
		if entry.Source == "" {
			entry.Source = thinkt.SourceCodex
		}
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
		return nil, os.ErrNotExist
	}

	f, err := os.Open(meta.FullPath)
	if err != nil {
		return nil, err
	}

	return &sessionReader{
		parser: NewParser(f, meta.ID),
		file:   f,
		meta:   *meta,
	}, nil
}

func (s *Store) scanSessions() ([]thinkt.SessionMeta, error) {
	root := filepath.Join(s.baseDir, "sessions")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	ws := s.Workspace()
	sessions := make([]thinkt.SessionMeta, 0, 128)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}

		meta, err := s.readSessionMeta(path, ws.ID)
		if err != nil || meta == nil {
			return nil
		}
		sessions = append(sessions, *meta)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModifiedAt.After(sessions[j].ModifiedAt)
	})
	return sessions, nil
}

func (s *Store) readSessionMeta(path, workspaceID string) (*thinkt.SessionMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	meta := &thinkt.SessionMeta{
		ID:          strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		FullPath:    path,
		FileSize:    info.Size(),
		ModifiedAt:  info.ModTime(),
		Source:      thinkt.SourceCodex,
		WorkspaceID: workspaceID,
		ChunkCount:  1,
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := thinkt.NewScannerWithMaxCapacityCustom(f, 64*1024, thinkt.MaxScannerCapacity)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var l logLine
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			continue
		}

		if meta.CreatedAt.IsZero() {
			meta.CreatedAt = parseTimestamp(l.Timestamp)
		}

		switch l.Type {
		case "session_meta":
			var payload struct {
				ID            string `json:"id"`
				Timestamp     string `json:"timestamp"`
				CWD           string `json:"cwd"`
				Model         string `json:"model"`
				ModelProvider string `json:"model_provider"`
				Git           struct {
					Branch string `json:"branch"`
				} `json:"git"`
			}
			if err := json.Unmarshal(l.Payload, &payload); err != nil {
				continue
			}
			if payload.ID != "" {
				meta.ID = payload.ID
			}
			if payload.CWD != "" {
				meta.ProjectPath = payload.CWD
			}
			if payload.Model != "" {
				meta.Model = payload.Model
			} else if meta.Model == "" && payload.ModelProvider != "" {
				meta.Model = payload.ModelProvider
			}
			if payload.Git.Branch != "" {
				meta.GitBranch = payload.Git.Branch
			}
			if meta.CreatedAt.IsZero() {
				meta.CreatedAt = parseTimestamp(payload.Timestamp)
			}

		case "event_msg":
			var payload map[string]any
			if err := json.Unmarshal(l.Payload, &payload); err != nil {
				continue
			}
			payloadType := readString(payload, "type")
			switch payloadType {
			case "user_message":
				meta.EntryCount++
				if meta.FirstPrompt == "" {
					meta.FirstPrompt = readString(payload, "message")
				}
			case "agent_message", "agent_reasoning":
				meta.EntryCount++
			case "turn_context":
				if meta.ProjectPath == "" {
					meta.ProjectPath = readString(payload, "cwd")
				}
				if meta.Model == "" {
					meta.Model = readString(payload, "model")
				}
			}

		case "response_item":
			var payload map[string]any
			if err := json.Unmarshal(l.Payload, &payload); err != nil {
				continue
			}
			payloadType := readString(payload, "type")
			switch payloadType {
			case "message":
				role := readString(payload, "role")
				if role == "user" || role == "assistant" {
					meta.EntryCount++
				}
				if meta.FirstPrompt == "" && role == "user" {
					meta.FirstPrompt = extractMessageText(payload["content"])
				}
			case "reasoning", "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output":
				meta.EntryCount++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = meta.ModifiedAt
	}
	if meta.ProjectPath == "" {
		meta.ProjectPath = "unknown"
	}
	return meta, nil
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
	entry.WorkspaceID = r.meta.WorkspaceID
	if entry.Source == "" {
		entry.Source = thinkt.SourceCodex
	}
	return entry, nil
}

func (r *sessionReader) Metadata() thinkt.SessionMeta {
	return r.meta
}

func (r *sessionReader) Close() error {
	return r.file.Close()
}
