// Package qwen provides Qwen Code session storage implementation.
package qwen

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// Store implements thinkt.Store for Qwen Code sessions.
type Store struct {
	baseDir string
	cache   thinkt.StoreCache
}

// NewStore creates a new Qwen store.
func NewStore(baseDir string) *Store {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".qwen")
	}
	return &Store{baseDir: baseDir}
}

// SetCacheTTL sets the cache time-to-live for this store.
func (s *Store) SetCacheTTL(d time.Duration) {
	s.cache.SetName("qwen")
	s.cache.SetTTL(d)
}

// Source returns the store type.
func (s *Store) Source() thinkt.Source {
	return thinkt.SourceQwen
}

// Workspace returns information about this store's workspace.
func (s *Store) Workspace() thinkt.Workspace {
	hostname, _ := os.Hostname()

	// Try to read installation_id
	workspaceID := hostname // fallback
	installIDPath := filepath.Join(s.baseDir, "installation_id")
	if data, err := os.ReadFile(installIDPath); err == nil {
		workspaceID = strings.TrimSpace(string(data))
	}

	return thinkt.Workspace{
		ID:       workspaceID,
		Name:     hostname,
		Hostname: hostname,
		Source:   thinkt.SourceQwen,
		BasePath: s.baseDir,
	}
}

// ListProjects returns all Qwen projects.
// Projects are organized by working directory under ~/.qwen/projects/
func (s *Store) ListProjects(ctx context.Context) ([]thinkt.Project, error) {
	if cached, err, ok := s.cache.GetProjects(); ok {
		return cached, err
	}

	projectsDir := filepath.Join(s.baseDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			s.cache.SetProjects([]thinkt.Project{}, nil)
			return []thinkt.Project{}, nil
		}
		s.cache.SetProjects(nil, err)
		return nil, err
	}

	ws := s.Workspace()
	var projects []thinkt.Project

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectHash := entry.Name()
		projectPath := filepath.Join(projectsDir, projectHash)
		chatsDir := filepath.Join(projectPath, "chats")

		// Check if chats directory exists
		if _, err := os.Stat(chatsDir); os.IsNotExist(err) {
			continue
		}

		// Try to get a human-readable name from debug logs
		projectName := s.extractProjectName(projectHash)
		if projectName == "" {
			projectName = projectHash
		}

		// Count sessions and get last modified
		sessions, _ := s.listSessionsForProject(projectHash)
		sessionCount := len(sessions)

		var lastMod time.Time
		for _, sess := range sessions {
			if sess.ModifiedAt.After(lastMod) {
				lastMod = sess.ModifiedAt
			}
		}

		if sessionCount > 0 {
			// Decode the project hash back to original path if possible
			displayPath := s.decodeProjectPath(projectHash)
			if displayPath == "" {
				// Show hash with qwen:// prefix for unknown encoding
				displayPath = "qwen://" + projectHash[:min(8, len(projectHash))]
			}

			projects = append(projects, thinkt.Project{
				ID:             projectHash,
				Name:           projectName,
				Path:           displayPath,
				DisplayPath:    displayPath,
				SessionCount:   sessionCount,
				LastModified:   lastMod,
				Source:         thinkt.SourceQwen,
				WorkspaceID:    ws.ID,
				SourceBasePath: ws.BasePath,
				PathExists:     true,
			})
		}
	}

	s.cache.SetProjects(projects, nil)
	return projects, nil
}

// extractProjectName tries to get a human-readable project name from debug logs
func (s *Store) extractProjectName(projectHash string) string {
	// Try to find logs in tmp directory that might contain project info
	tmpDir := filepath.Join(s.baseDir, "tmp")
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		logsPath := filepath.Join(tmpDir, entry.Name(), "logs.json")
		if data, err := os.ReadFile(logsPath); err == nil {
			var logs []struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			}
			if err := json.Unmarshal(data, &logs); err == nil {
				// Find first user message to use as a name hint
				for _, l := range logs {
					if l.Type == "user" && l.Message != "" {
						name := l.Message
						if len(name) > 40 {
							name = name[:37] + "..."
						}
						return name
					}
				}
			}
		}
	}
	return ""
}

// decodeProjectPath attempts to decode the project hash back to original path
// Qwen uses dash-encoded directory names (e.g., -Users-evan-project)
func (s *Store) decodeProjectPath(hash string) string {
	// If hash starts with '-', it's likely an encoded path
	if strings.HasPrefix(hash, "-") {
		// Replace - with / to reconstruct path
		decoded := strings.ReplaceAll(hash, "-", "/")
		// Remove leading slash and restore leading dash if needed
		decoded = strings.TrimPrefix(decoded, "/")
		// On macOS, paths don't start with / in user display
		return decoded
	}
	return ""
}

// ResetCache clears all cached data.
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

	sessions, err := s.listSessionsForProject(projectID)
	if err != nil {
		s.cache.SetSessions(projectID, nil, err)
		return nil, err
	}

	s.cache.SetSessions(projectID, sessions, nil)
	return sessions, nil
}

func (s *Store) listSessionsForProject(projectHash string) ([]thinkt.SessionMeta, error) {
	chatsDir := filepath.Join(s.baseDir, "projects", projectHash, "chats")
	entries, err := os.ReadDir(chatsDir)
	if err != nil {
		return nil, err
	}

	ws := s.Workspace()
	var sessions []thinkt.SessionMeta

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Qwen stores sessions as .jsonl files
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		fullPath := filepath.Join(chatsDir, entry.Name())

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Parse session to get metadata
		meta, err := s.readSessionMeta(fullPath, projectHash, info.Size(), info.ModTime(), ws.ID)
		if err != nil {
			tuilog.Log.Error("failed to read qwen session meta", "path", fullPath, "error", err)
			continue
		}

		sessions = append(sessions, *meta)
	}

	return sessions, nil
}

func (s *Store) readSessionMeta(path, projectHash string, size int64, modTime time.Time, wsID string) (*thinkt.SessionMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	meta := &thinkt.SessionMeta{
		ID:          strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		ProjectPath: projectHash,
		FullPath:    path,
		FileSize:    size,
		CreatedAt:   modTime,
		ModifiedAt:  modTime,
		Source:      thinkt.SourceQwen,
		WorkspaceID: wsID,
		ChunkCount:  1,
	}

	// Read first few lines to extract metadata
	scanner := thinkt.NewScannerWithMaxCapacity(f)

	lineCount := 0
	maxLines := 100 // Read first 100 lines to get metadata

	for scanner.Scan() && lineCount < maxLines {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry struct {
			Type      string          `json:"type"`
			Role      string          `json:"role"`
			Message   json.RawMessage `json:"message"`
			Timestamp string          `json:"timestamp"`
			Model     string          `json:"model"`
		}

		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		lineCount++

		// Extract first prompt
		if meta.FirstPrompt == "" && entry.Type == "user" {
			var msg struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			}
			if err := json.Unmarshal(entry.Message, &msg); err == nil {
				for _, part := range msg.Parts {
					if part.Text != "" {
						meta.FirstPrompt = thinkt.TruncateString(part.Text, thinkt.DefaultTruncateLength)
						break
					}
				}
			}
		}

		// Extract model
		if meta.Model == "" && entry.Model != "" {
			meta.Model = entry.Model
		}

		// Parse timestamp
		if entry.Timestamp != "" && meta.CreatedAt.IsZero() {
			if t, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
				meta.CreatedAt = t
			}
		}
	}

	// Count total entries
	meta.EntryCount = s.countEntries(path)

	return meta, nil
}

func (s *Store) countEntries(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := thinkt.NewScannerWithMaxCapacity(f)

	for scanner.Scan() {
		if len(strings.TrimSpace(scanner.Text())) > 0 {
			count++
		}
	}
	return count
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

	reader, err := s.OpenSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	entries := make([]thinkt.Entry, 0)
	for {
		entry, err := reader.ReadNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
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

	f, err := os.Open(meta.FullPath)
	if err != nil {
		return nil, err
	}

	scanner := thinkt.NewScannerWithMaxCapacity(f)

	return &qwenReader{
		scanner: scanner,
		file:    f,
		meta:    *meta,
		source:  thinkt.SourceQwen,
		wsID:    meta.WorkspaceID,
	}, nil
}

// qwenReader implements thinkt.SessionReader for Qwen format.
type qwenReader struct {
	scanner *bufio.Scanner
	file    *os.File
	closed  bool
	lineNum int
	meta    thinkt.SessionMeta
	source  thinkt.Source
	wsID    string
}

func (r *qwenReader) ReadNext() (*thinkt.Entry, error) {
	if r.closed {
		return nil, io.ErrClosedPipe
	}

	for r.scanner.Scan() {
		r.lineNum++
		line := r.scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		entry, err := parseQwenEntry(line, r.lineNum, r.source, r.wsID)
		if err != nil {
			tuilog.Log.Debug("failed to parse qwen entry", "line", r.lineNum, "error", err)
			continue // Skip malformed entries
		}
		if entry != nil {
			return entry, nil
		}
	}

	if err := r.scanner.Err(); err != nil {
		return nil, err
	}

	return nil, io.EOF
}

func (r *qwenReader) Metadata() thinkt.SessionMeta {
	return r.meta
}

func (r *qwenReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.file.Close()
}

// parseQwenEntry parses a single line from the session JSONL file.
func parseQwenEntry(data []byte, lineNum int, source thinkt.Source, wsID string) (*thinkt.Entry, error) {
	var raw struct {
		UUID          string          `json:"uuid"`
		ParentUUID    string          `json:"parentUuid"`
		Type          string          `json:"type"`
		Role          string          `json:"role"`
		Message       json.RawMessage `json:"message"`
		Timestamp     string          `json:"timestamp"`
		Model         string          `json:"model"`
		CWD           string          `json:"cwd"`
		GitBranch     string          `json:"gitBranch"`
		Subtype       string          `json:"subtype"`
		SystemPayload json.RawMessage `json:"systemPayload"`
		Version       string          `json:"version"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	entry := &thinkt.Entry{
		UUID:        raw.UUID,
		Source:      source,
		WorkspaceID: wsID,
		CWD:         raw.CWD,
		GitBranch:   raw.GitBranch,
		Metadata:    make(map[string]any),
	}

	// Parse timestamp
	if raw.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339, raw.Timestamp); err == nil {
			entry.Timestamp = t
		}
	}

	// Handle parent UUID
	if raw.ParentUUID != "" {
		entry.ParentUUID = &raw.ParentUUID
	}

	// Store metadata
	if raw.Version != "" {
		entry.Metadata["version"] = raw.Version
	}

	// Determine role and parse content based on type
	switch raw.Type {
	case "user":
		entry.Role = thinkt.RoleUser
		if err := parseQwenMessage(raw.Message, entry); err != nil {
			return nil, err
		}

	case "assistant":
		entry.Role = thinkt.RoleAssistant
		entry.Model = raw.Model
		if err := parseQwenMessage(raw.Message, entry); err != nil {
			return nil, err
		}

	case "tool_result":
		entry.Role = thinkt.RoleTool
		if err := parseQwenMessage(raw.Message, entry); err != nil {
			return nil, err
		}

	case "system":
		entry.Role = thinkt.RoleSystem
		if len(raw.SystemPayload) > 0 {
			var payload map[string]any
			if err := json.Unmarshal(raw.SystemPayload, &payload); err == nil {
				entry.Metadata["systemPayload"] = payload
			}
		}
		if raw.Subtype != "" {
			entry.Metadata["subtype"] = raw.Subtype
		}
		// Extract text from system payload for display
		if raw.Subtype == "slash_command" {
			if payload, ok := entry.Metadata["systemPayload"].(map[string]any); ok {
				if cmd, ok := payload["rawCommand"].(string); ok {
					entry.Text = "Slash command: " + cmd
				}
			}
		}

	default:
		// Try to use role field directly
		if raw.Role != "" {
			entry.Role = convertQwenRole(raw.Role)
		} else {
			entry.Role = thinkt.RoleSystem
		}
	}

	return entry, nil
}

// qwenMessage represents the message structure in Qwen entries
type qwenMessage struct {
	Role  string          `json:"role"`
	Parts []qwenMessagePart `json:"parts"`
}

// qwenMessagePart represents a part in the message
type qwenMessagePart struct {
	Text           string          `json:"text"`
	Thought        bool            `json:"thought"`
	FunctionCall   *qwenFunctionCall `json:"functionCall"`
	FunctionResponse *qwenFunctionResponse `json:"functionResponse"`
}

// qwenFunctionCall represents a function call
type qwenFunctionCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// qwenFunctionResponse represents a function response
type qwenFunctionResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Response struct {
		Output string `json:"output"`
	} `json:"response"`
}

// parseQwenMessage parses the message field and populates entry content
func parseQwenMessage(msgData json.RawMessage, entry *thinkt.Entry) error {
	if len(msgData) == 0 {
		return nil
	}

	var msg qwenMessage
	if err := json.Unmarshal(msgData, &msg); err != nil {
		return err
	}

	var textParts []string
	for _, part := range msg.Parts {
		// Handle text parts
		if part.Text != "" {
			if part.Thought {
				// Thinking content
				entry.ContentBlocks = append(entry.ContentBlocks, thinkt.ContentBlock{
					Type:     "thinking",
					Thinking: part.Text,
				})
			} else {
				// Regular text
				textParts = append(textParts, part.Text)
				entry.ContentBlocks = append(entry.ContentBlocks, thinkt.ContentBlock{
					Type: "text",
					Text: part.Text,
				})
			}
		}

		// Handle function calls (tool_use)
		if part.FunctionCall != nil {
			entry.ContentBlocks = append(entry.ContentBlocks, thinkt.ContentBlock{
				Type:      "tool_use",
				ToolUseID: part.FunctionCall.ID,
				ToolName:  part.FunctionCall.Name,
				ToolInput: part.FunctionCall.Args,
			})
		}

		// Handle function responses (tool_result)
		if part.FunctionResponse != nil {
			output := part.FunctionResponse.Response.Output
			entry.ContentBlocks = append(entry.ContentBlocks, thinkt.ContentBlock{
				Type:       "tool_result",
				ToolUseID:  part.FunctionResponse.ID,
				ToolResult: output,
			})
			// Also add to text for display
			if output != "" {
				textParts = append(textParts, "[Tool result: "+output+"]")
			}
		}
	}

	// Set Text field from text parts
	entry.Text = strings.Join(textParts, "\n")
	return nil
}

func convertQwenRole(role string) thinkt.Role {
	switch role {
	case "user":
		return thinkt.RoleUser
	case "assistant", "model":
		return thinkt.RoleAssistant
	case "tool":
		return thinkt.RoleTool
	case "system":
		return thinkt.RoleSystem
	default:
		return thinkt.RoleSystem
	}
}
