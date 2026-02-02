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

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
)

// Store implements thinkt.Store for Gemini CLI sessions.
type Store struct {
	baseDir string
}

// NewStore creates a new Gemini store.
func NewStore(baseDir string) *Store {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".gemini")
	}
	return &Store{baseDir: baseDir}
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

		// Count sessions
		sessions, _ := os.ReadDir(chatsDir)
		sessionCount := 0
		var lastMod time.Time

		for _, sess := range sessions {
			if strings.HasSuffix(sess.Name(), ".json") {
				sessionCount++
				info, err := sess.Info()
				if err == nil {
					if info.ModTime().After(lastMod) {
						lastMod = info.ModTime()
					}
				}
			}
		}

		if sessionCount > 0 {
			projects = append(projects, thinkt.Project{
				ID:           projectHash,
				Name:         projectHash, // We don't have the real name yet
				Path:         projectPath,
				DisplayPath:  projectHash,
				SessionCount: sessionCount,
				LastModified: lastMod,
				Source:       thinkt.SourceGemini,
				WorkspaceID:  ws.ID,
			})
		}
	}

	return projects, nil
}

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
	// projectID corresponds to the hash folder name
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
			
			// We need to read at least part of the file to get metadata
			// Or we can rely on file info for basic stats
			info, err := entry.Info()
			if err != nil {
				continue
			}

			// Read file to get ID and other meta
			// Optimization: could just read header, but for now read full
			f, err := os.Open(fullPath)
			if err != nil {
				continue
			}
			
			var sess Session
			if err := json.NewDecoder(f).Decode(&sess); err != nil {
				f.Close()
				continue
			}
			f.Close()

			meta := thinkt.SessionMeta{
				ID:          sess.SessionID,
				ProjectPath: projectID,
				FullPath:    fullPath,
				EntryCount:  len(sess.Messages), // Approx
				FileSize:    info.Size(),
				CreatedAt:   sess.StartTime,
				ModifiedAt:  sess.LastUpdated,
				Source:      thinkt.SourceGemini,
				WorkspaceID: ws.ID,
				ChunkCount:  1,
			}
			
			// Try to set model from last assistant message
			for i := len(sess.Messages) - 1; i >= 0; i-- {
				if sess.Messages[i].Type == "gemini" && sess.Messages[i].Model != "" {
					meta.Model = sess.Messages[i].Model
					break
				}
			}

			// Try to set first prompt
			for _, msg := range sess.Messages {
				if msg.Type == "user" {
					meta.FirstPrompt = msg.Content
					break
				}
			}

			sessions = append(sessions, meta)
		}
	}

	// Sort by modified time desc
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModifiedAt.After(sessions[j].ModifiedAt)
	})

	return sessions, nil
}

// GetSessionMeta returns session metadata.
func (s *Store) GetSessionMeta(ctx context.Context, sessionID string) (*thinkt.SessionMeta, error) {
	// Scan all projects to find session ID? Expensive.
	// Or assume caller knows the project.
	// The interface doesn't take projectID here.
	
	// Fast path: if sessionID is a path
	if filepath.IsAbs(sessionID) {
		f, err := os.Open(sessionID)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		var sess Session
		if err := json.NewDecoder(f).Decode(&sess); err != nil {
			return nil, err
		}

		info, _ := f.Stat()
		ws := s.Workspace()
		
		// Extract project hash from path
		// .../tmp/<hash>/chats/session.json
		parts := strings.Split(sessionID, string(os.PathSeparator))
		projectID := ""
		for i, part := range parts {
			if part == "chats" && i > 0 {
				projectID = parts[i-1]
				break
			}
		}

		return &thinkt.SessionMeta{
			ID:          sess.SessionID,
			ProjectPath: projectID,
			FullPath:    sessionID,
			CreatedAt:   sess.StartTime,
			ModifiedAt:  sess.LastUpdated,
			Source:      thinkt.SourceGemini,
			WorkspaceID: ws.ID,
			FileSize:    info.Size(),
			EntryCount:  len(sess.Messages),
		}, nil
	}

	// Slow path: Search all projects
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

	var geminiSess Session
	if err := json.NewDecoder(f).Decode(&geminiSess); err != nil {
		return nil, err
	}

	return convertSession(&geminiSess, meta), nil
}

// OpenSession returns a streaming reader.
// For Gemini JSON, we currently read the whole file anyway since it's a single JSON object,
// not JSONL. So streaming is faked by loading all and iterating.
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

// Conversion helpers

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
			// Assistant entry
			asstEntry := thinkt.Entry{
				UUID:        msg.ID,
				Role:        thinkt.RoleAssistant,
				Timestamp:   msg.Timestamp,
				Source:      thinkt.SourceGemini,
				WorkspaceID: meta.WorkspaceID,
				Model:       msg.Model,
			}

			// Add Tokens
			if msg.Tokens != nil {
				asstEntry.Usage = &thinkt.TokenUsage{
					InputTokens:  msg.Tokens.Input,
					OutputTokens: msg.Tokens.Output,
				}
			}

			// Content Blocks
			// 1. Text
			if msg.Content != "" {
				asstEntry.ContentBlocks = append(asstEntry.ContentBlocks, thinkt.ContentBlock{
					Type: "text",
					Text: msg.Content,
				})
			}

			// 2. Thoughts
			for _, t := range msg.Thoughts {
				asstEntry.ContentBlocks = append(asstEntry.ContentBlocks, thinkt.ContentBlock{
					Type:     "thinking",
					Thinking: fmt.Sprintf("[%s] %s", t.Subject, t.Description),
				})
			}

			// 3. Tool Calls
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
					// Extract output string from map if possible
					output := ""
					if o, ok := res.FunctionResponse.Response["output"]; ok {
						if s, ok := o.(string); ok {
							output = s
						} else {
							b, _ := json.Marshal(o)
							output = string(b)
						}
					}

					entries = append(entries, thinkt.Entry{
						UUID:        res.FunctionResponse.ID, // Use result ID if available
						Role:        thinkt.RoleTool, // Use Tool role
						Timestamp:   msg.Timestamp, // Approx same time
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
