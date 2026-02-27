package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// Store implements thinkt.Store for Gemini CLI sessions.
type Store struct {
	baseDir string
	cache   thinkt.StoreCache
}

// NewStore creates a new Gemini store.
func NewStore(baseDir string) *Store {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".gemini")
	}
	return &Store{baseDir: baseDir}
}

// SetCacheTTL sets the cache time-to-live for this store.
func (s *Store) SetCacheTTL(d time.Duration) {
	s.cache.SetName("gemini")
	s.cache.SetTTL(d)
}

// Source returns the store type.
func (s *Store) Source() thinkt.Source {
	return thinkt.SourceGemini
}

// Workspace returns information about this store's workspace.
func (s *Store) Workspace() thinkt.Workspace {
	hostname, _ := os.Hostname()
	workspaceID := hostname

	// Try to read installation_id
	installIDPath := filepath.Join(s.baseDir, "installation_id")
	if data, err := os.ReadFile(installIDPath); err == nil {
		workspaceID = strings.TrimSpace(string(data))
	}

	return thinkt.Workspace{
		ID:       workspaceID,
		Name:     hostname,
		Hostname: hostname,
		Source:   thinkt.SourceGemini,
		BasePath: s.baseDir,
	}
}

// ListProjects returns all Gemini projects.
func (s *Store) ListProjects(ctx context.Context) ([]thinkt.Project, error) {
	return s.cache.LoadProjects(func() ([]thinkt.Project, error) {
		tmpDir := filepath.Join(s.baseDir, "tmp")
		entries, err := os.ReadDir(tmpDir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}

		var projects []thinkt.Project
		ws := s.Workspace()

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			projectHash := entry.Name()
			projectPath := filepath.Join(tmpDir, projectHash)
			chatsDir := filepath.Join(projectPath, "chats")

			// Check if chats directory exists
			if _, err := os.Stat(chatsDir); os.IsNotExist(err) {
				continue
			}

			// Try to find a human-readable name from logs.json
			projectName := projectHash
			logsPath := filepath.Join(projectPath, "logs.json")
			if data, err := os.ReadFile(logsPath); err == nil {
				var logs []struct {
					Message string `json:"message"`
					Type    string `json:"type"`
				}
				if err := json.Unmarshal(data, &logs); err == nil {
					// Find first user message to use as a name hint
					for _, l := range logs {
						if l.Type == "user" && l.Message != "" {
							projectName = l.Message
							if len(projectName) > 40 {
								projectName = projectName[:37] + "..."
							}
							break
						}
					}
				}
			}

			// Count sessions and get last modified
			sessions, _ := os.ReadDir(chatsDir)
			sessionCount := 0
			var lastMod time.Time

			for _, sess := range sessions {
				if strings.HasSuffix(sess.Name(), ".json") {
					sessionCount++
					if info, err := sess.Info(); err == nil {
						if info.ModTime().After(lastMod) {
							lastMod = info.ModTime()
						}
					}
				}
			}

			if sessionCount > 0 {
				projects = append(projects, thinkt.Project{
					ID:             projectHash,
					Name:           projectName,
					Path:           projectPath,
					DisplayPath:    "gemini://" + projectHash[:8],
					SessionCount:   sessionCount,
					LastModified:   lastMod,
					Source:         thinkt.SourceGemini,
					WorkspaceID:    ws.ID,
					SourceBasePath: ws.BasePath,
					PathExists:     true,
				})
			}
		}

		return projects, nil
	})
}

// ResetCache clears all cached data.
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
func (s *Store) ListSessions(ctx context.Context, projectID string) ([]thinkt.SessionMeta, error) {
	return s.cache.LoadSessions(projectID, func() ([]thinkt.SessionMeta, error) {
		chatsDir := filepath.Join(s.baseDir, "tmp", projectID, "chats")
		entries, err := os.ReadDir(chatsDir)
		if err != nil {
			return nil, err
		}

		var sessions []thinkt.SessionMeta
		ws := s.Workspace()

		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
				fullPath := filepath.Join(chatsDir, entry.Name())
				info, err := entry.Info()
				if err != nil {
					continue
				}

				// Partial read to get metadata efficiently
				meta, err := s.readSessionMeta(fullPath, projectID, info.Size(), info.ModTime(), ws.ID)
				if err != nil {
					tuilog.Log.Error("failed to read gemini session meta", "path", fullPath, "error", err)
					continue
				}
				sessions = append(sessions, *meta)
			}
		}

		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].ModifiedAt.After(sessions[j].ModifiedAt)
		})

		return sessions, nil
	})
}

func (s *Store) readSessionMeta(path, projectID string, size int64, modTime time.Time, wsID string) (*thinkt.SessionMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// We need to parse enough to get the ID and first prompt
	var sess Session
	if err := json.NewDecoder(f).Decode(&sess); err != nil {
		return nil, err
	}

	meta := &thinkt.SessionMeta{
		ID:          sess.SessionID,
		ProjectPath: projectID,
		FullPath:    path,
		EntryCount:  len(sess.Messages),
		FileSize:    size,
		CreatedAt:   sess.StartTime,
		ModifiedAt:  sess.LastUpdated,
		Source:      thinkt.SourceGemini,
		WorkspaceID: wsID,
		ChunkCount:  1,
	}

	// Fallback for missing updated time
	if meta.ModifiedAt.IsZero() {
		meta.ModifiedAt = modTime
	}

	// Extract model and first prompt
	for _, msg := range sess.Messages {
		if msg.Type == "user" && meta.FirstPrompt == "" {
			meta.FirstPrompt = msg.Content
		}
		if msg.Type == "gemini" && thinkt.IsRealModel(msg.Model) {
			meta.Model = msg.Model
		}
	}

	return meta, nil
}

// GetSessionMeta returns session metadata.
func (s *Store) GetSessionMeta(ctx context.Context, sessionID string) (*thinkt.SessionMeta, error) {
	// Fast path: if sessionID is a path
	if filepath.IsAbs(sessionID) {
		// Validate path is under this store's base directory
		if err := thinkt.ValidateSessionPath(sessionID, s.baseDir); err != nil {
			return nil, nil
		}

		info, err := os.Stat(sessionID)
		if err != nil {
			return nil, err
		}

		// Extract project hash from path
		parts := strings.Split(sessionID, string(os.PathSeparator))
		projectID := ""
		for i, part := range parts {
			if part == "chats" && i > 0 {
				projectID = parts[i-1]
				break
			}
		}

		ws := s.Workspace()
		return s.readSessionMeta(sessionID, projectID, info.Size(), info.ModTime(), ws.ID)
	}

	// Search all projects
	projects, _ := s.ListProjects(ctx)
	for _, p := range projects {
		sessions, _ := s.ListSessions(ctx, p.ID)
		for _, sess := range sessions {
			if sess.ID == sessionID {
				return &sess, nil
			}
		}
	}

	return nil, nil
}

// LoadSession loads a complete session.
func (s *Store) LoadSession(ctx context.Context, sessionID string) (*thinkt.Session, error) {
	meta, err := s.GetSessionMeta(ctx, sessionID)
	if err != nil || meta == nil {
		return nil, err
	}

	f, err := os.Open(meta.FullPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var geminiSess Session
	if err := json.NewDecoder(f).Decode(&geminiSess); err != nil {
		return nil, err
	}

	return convertSession(&geminiSess, meta), nil
}

// OpenSession returns a streaming reader.
func (s *Store) OpenSession(ctx context.Context, sessionID string) (thinkt.SessionReader, error) {
	sess, err := s.LoadSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, os.ErrNotExist
	}

	return &memorySessionReader{
		session: sess,
		idx:     0,
	}, nil
}

type memorySessionReader struct {
	session *thinkt.Session
	idx     int
}

func (r *memorySessionReader) ReadNext() (*thinkt.Entry, error) {
	if r.idx >= len(r.session.Entries) {
		return nil, io.EOF
	}
	entry := r.session.Entries[r.idx]
	r.idx++
	return &entry, nil
}

func (r *memorySessionReader) Metadata() thinkt.SessionMeta {
	return r.session.Meta
}

func (r *memorySessionReader) Close() error {
	return nil
}

// convertSession converts Gemini specific structure to thinkt.
func convertSession(g *Session, meta *thinkt.SessionMeta) *thinkt.Session {
	var entries []thinkt.Entry

	for _, msg := range g.Messages {
		if msg.Type == "user" {
			entries = append(entries, thinkt.Entry{
				UUID:        msg.ID,
				Role:        thinkt.RoleUser,
				Timestamp:   msg.Timestamp,
				Source:      thinkt.SourceGemini,
				WorkspaceID: meta.WorkspaceID,
				Text:        msg.Content,
			})
		} else if msg.Type == "gemini" {
			asstEntry := thinkt.Entry{
				UUID:        msg.ID,
				Role:        thinkt.RoleAssistant,
				Timestamp:   msg.Timestamp,
				Source:      thinkt.SourceGemini,
				WorkspaceID: meta.WorkspaceID,
				Model:       msg.Model,
			}

			if msg.Tokens != nil {
				asstEntry.Usage = &thinkt.TokenUsage{
					InputTokens:  msg.Tokens.Input,
					OutputTokens: msg.Tokens.Output,
				}
			}

			if msg.Content != "" {
				asstEntry.ContentBlocks = append(asstEntry.ContentBlocks, thinkt.ContentBlock{
					Type: "text",
					Text: msg.Content,
				})
			}

			for _, t := range msg.Thoughts {
				asstEntry.ContentBlocks = append(asstEntry.ContentBlocks, thinkt.ContentBlock{
					Type:     "thinking",
					Thinking: fmt.Sprintf("[%s] %s", t.Subject, t.Description),
				})
			}

			for _, tc := range msg.ToolCalls {
				asstEntry.ContentBlocks = append(asstEntry.ContentBlocks, thinkt.ContentBlock{
					Type:      "tool_use",
					ToolUseID: tc.ID,
					ToolName:  tc.Name,
					ToolInput: tc.Args,
				})
			}

			entries = append(entries, asstEntry)

			// Separate entries for Tool Results
			for _, tc := range msg.ToolCalls {
				for _, res := range tc.Result {
					output := ""
					if o, ok := res.FunctionResponse.Response["output"]; ok {
						if s, ok := o.(string); ok {
							output = s
						} else {
							b, _ := json.Marshal(o)
							output = string(b)
						}
					}

					// Ensure unique UUID for tool results
					resID := res.FunctionResponse.ID
					if resID == "" {
						resID = fmt.Sprintf("%s-res", tc.ID)
					}

					entries = append(entries, thinkt.Entry{
						UUID:        resID,
						Role:        thinkt.RoleTool,
						Timestamp:   msg.Timestamp,
						Source:      thinkt.SourceGemini,
						WorkspaceID: meta.WorkspaceID,
						ContentBlocks: []thinkt.ContentBlock{
							{
								Type:       "tool_result",
								ToolUseID:  tc.ID,
								ToolResult: output,
							},
						},
					})
				}
			}
		}
	}

	meta.EntryCount = len(entries)
	return &thinkt.Session{
		Meta:    *meta,
		Entries: entries,
	}
}
