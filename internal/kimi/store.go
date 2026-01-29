// Package kimi provides Kimi Code session storage implementation.
package kimi

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
)

// Store implements thinkt.Store for Kimi Code sessions.
type Store struct {
	baseDir string
}

// NewStore creates a new Kimi store.
func NewStore(baseDir string) *Store {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".kimi")
	}
	return &Store{baseDir: baseDir}
}

// Source returns the store type.
func (s *Store) Source() thinkt.Source {
	return thinkt.SourceKimi
}

// Workspace returns information about this store's workspace.
func (s *Store) Workspace() thinkt.Workspace {
	hostname, _ := os.Hostname()

	// Try to read device_id
	workspaceID := hostname // fallback
	deviceIDPath := filepath.Join(s.baseDir, "device_id")
	if data, err := os.ReadFile(deviceIDPath); err == nil {
		workspaceID = strings.TrimSpace(string(data))
	}

	return thinkt.Workspace{
		ID:       workspaceID,
		Name:     hostname,
		Hostname: hostname,
		Source:   thinkt.SourceKimi,
		BasePath: s.baseDir,
	}
}

// workDirHash returns the MD5 hash of a working directory path.
func workDirHash(path string) string {
	h := md5.New()
	h.Write([]byte(path))
	return hex.EncodeToString(h.Sum(nil))
}

// ListProjects returns all Kimi projects.
func (s *Store) ListProjects(ctx context.Context) ([]thinkt.Project, error) {
	sessionsDir := filepath.Join(s.baseDir, "sessions")

	// Read kimi.json for work directory mapping
	workDirs, err := s.loadWorkDirs()
	if err != nil {
		// Fallback: scan sessions directory
		return s.scanProjects(sessionsDir)
	}

	ws := s.Workspace()
	projects := make([]thinkt.Project, 0, len(workDirs))
	for _, wd := range workDirs {
		hash := workDirHash(wd.Path)
		sessionDir := filepath.Join(sessionsDir, hash)

		info, err := os.Stat(sessionDir)
		if err != nil {
			continue
		}

		sessions, _ := s.listSessionsForHash(hash)

		projects = append(projects, thinkt.Project{
			ID:           wd.Path,
			Name:         filepath.Base(wd.Path),
			Path:         wd.Path,
			DisplayPath:  wd.Path,
			SessionCount: len(sessions),
			LastModified: info.ModTime(),
			Source:       thinkt.SourceKimi,
			WorkspaceID:  ws.ID,
		})
	}

	return projects, nil
}

// GetProject returns a specific project.
func (s *Store) GetProject(ctx context.Context, id string) (*thinkt.Project, error) {
	// id is the project path
	hash := workDirHash(id)
	sessionDir := filepath.Join(s.baseDir, "sessions", hash)

	info, err := os.Stat(sessionDir)
	if err != nil {
		return nil, err
	}

	sessions, _ := s.listSessionsForHash(hash)
	ws := s.Workspace()

	return &thinkt.Project{
		ID:           id,
		Name:         filepath.Base(id),
		Path:         id,
		DisplayPath:  id,
		SessionCount: len(sessions),
		LastModified: info.ModTime(),
		Source:       thinkt.SourceKimi,
		WorkspaceID:  ws.ID,
	}, nil
}

// ListSessions returns sessions for a project.
func (s *Store) ListSessions(ctx context.Context, projectID string) ([]thinkt.SessionMeta, error) {
	hash := workDirHash(projectID)
	return s.listSessionsForHash(hash)
}

func (s *Store) listSessionsForHash(hash string) ([]thinkt.SessionMeta, error) {
	sessionDir := filepath.Join(s.baseDir, "sessions", hash)
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return nil, err
	}

	ws := s.Workspace()
	sessions := make([]thinkt.SessionMeta, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessionID := entry.Name()
		contextPath := filepath.Join(sessionDir, sessionID, "context.jsonl")

		info, err := os.Stat(contextPath)
		if err != nil {
			continue
		}

		// Count entries by reading the file
		count, firstPrompt := s.countEntriesAndFirstPrompt(contextPath)

		sessions = append(sessions, thinkt.SessionMeta{
			ID:          sessionID,
			ProjectPath: hash,
			FullPath:    contextPath,
			FirstPrompt: firstPrompt,
			EntryCount:  count,
			CreatedAt:   info.ModTime(),
			ModifiedAt:  info.ModTime(),
			Source:      thinkt.SourceKimi,
			WorkspaceID: ws.ID,
		})
	}

	return sessions, nil
}

func (s *Store) countEntriesAndFirstPrompt(path string) (int, string) {
	f, err := os.Open(path)
	if err != nil {
		return 0, ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	firstPrompt := ""

	for scanner.Scan() {
		count++
		if firstPrompt == "" {
			var entry struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
				if entry.Role == "user" && entry.Content != "" {
					firstPrompt = entry.Content
					if len(firstPrompt) > 50 {
						firstPrompt = firstPrompt[:50] + "..."
					}
				}
			}
		}
	}

	return count, firstPrompt
}

// GetSessionMeta returns session metadata.
func (s *Store) GetSessionMeta(ctx context.Context, sessionID string) (*thinkt.SessionMeta, error) {
	// sessionID format: {hash}/{uuid} or just {uuid}
	parts := strings.Split(sessionID, "/")
	
	var hash, uuid string
	if len(parts) == 2 {
		hash, uuid = parts[0], parts[1]
	} else {
		// Need to find which hash contains this session
		sessionsDir := filepath.Join(s.baseDir, "sessions")
		hashes, _ := os.ReadDir(sessionsDir)
		for _, h := range hashes {
			if !h.IsDir() {
				continue
			}
			potentialPath := filepath.Join(sessionsDir, h.Name(), sessionID, "context.jsonl")
			if _, err := os.Stat(potentialPath); err == nil {
				hash = h.Name()
				uuid = sessionID
				break
			}
		}
	}

	if hash == "" || uuid == "" {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	contextPath := filepath.Join(s.baseDir, "sessions", hash, uuid, "context.jsonl")
	info, err := os.Stat(contextPath)
	if err != nil {
		return nil, err
	}

	count, firstPrompt := s.countEntriesAndFirstPrompt(contextPath)
	ws := s.Workspace()

	return &thinkt.SessionMeta{
		ID:          uuid,
		ProjectPath: hash,
		FullPath:    contextPath,
		FirstPrompt: firstPrompt,
		EntryCount:  count,
		CreatedAt:   info.ModTime(),
		ModifiedAt:  info.ModTime(),
		Source:      thinkt.SourceKimi,
		WorkspaceID: ws.ID,
	}, nil
}

// LoadSession loads a complete session.
func (s *Store) LoadSession(ctx context.Context, sessionID string) (*thinkt.Session, error) {
	meta, err := s.GetSessionMeta(ctx, sessionID)
	if err != nil {
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

	meta.EntryCount = len(entries)
	return &thinkt.Session{
		Meta:    *meta,
		Entries: entries,
	}, nil
}

// OpenSession returns a streaming reader for a session.
func (s *Store) OpenSession(ctx context.Context, sessionID string) (thinkt.SessionReader, error) {
	// Resolve session ID to path
	meta, err := s.GetSessionMeta(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(meta.FullPath)
	if err != nil {
		return nil, err
	}

	return &kimiReader{
		scanner: bufio.NewScanner(f),
		file:    f,
		meta:    *meta,
	}, nil
}

// kimiReader implements thinkt.SessionReader for Kimi format.
type kimiReader struct {
	scanner *bufio.Scanner
	file    *os.File
	meta    thinkt.SessionMeta
	closed  bool
	done    bool
}

func (r *kimiReader) ReadNext() (*thinkt.Entry, error) {
	if r.closed {
		return nil, io.ErrClosedPipe
	}
	if r.done {
		return nil, io.EOF
	}

	for r.scanner.Scan() {
		line := r.scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		entry, err := parseKimiEntry(line)
		if err != nil {
			continue // Skip malformed entries
		}
		if entry != nil {
			return entry, nil
		}
	}

	if err := r.scanner.Err(); err != nil {
		return nil, err
	}

	r.done = true
	return nil, io.EOF
}

func (r *kimiReader) Metadata() thinkt.SessionMeta {
	return r.meta
}

func (r *kimiReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.file.Close()
}

// parseKimiEntry parses a single line from context.jsonl.
func parseKimiEntry(data []byte) (*thinkt.Entry, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	role, _ := raw["role"].(string)
	
	// Skip internal entries
	if role == "" || role == "_checkpoint" || role == "_usage" {
		return nil, nil
	}

	entry := &thinkt.Entry{
		Role:     convertKimiRole(role),
		Metadata: make(map[string]any),
	}

	// Extract timestamp if present
	if ts, ok := raw["timestamp"].(float64); ok {
		entry.Timestamp = time.Unix(int64(ts), 0)
	}

	// Handle content based on role
	switch role {
	case "user":
		if content, ok := raw["content"].(string); ok {
			entry.Text = content
		}
		// Handle array content (tool results, etc.)
		if contentArr, ok := raw["content"].([]any); ok {
			entry.ContentBlocks = convertKimiContentBlocks(contentArr)
			entry.Text = extractTextFromBlocks(entry.ContentBlocks)
		}
	case "assistant":
		if content, ok := raw["content"].([]any); ok {
			entry.ContentBlocks = convertKimiContentBlocks(content)
			entry.Text = extractTextFromBlocks(entry.ContentBlocks)
		}
		// Extract tool calls
		if toolCalls, ok := raw["tool_calls"].([]any); ok {
			for _, tc := range toolCalls {
				if toolCall, ok := tc.(map[string]any); ok {
					cb := thinkt.ContentBlock{
						Type:       "tool_use",
						ToolUseID:  getString(toolCall, "id"),
						ToolName:   getString(toolCall, "name"),
						ToolInput:  toolCall["input"],
					}
					entry.ContentBlocks = append(entry.ContentBlocks, cb)
				}
			}
		}
	case "tool":
		entry.ContentBlocks = []thinkt.ContentBlock{{
			Type:       "tool_result",
			ToolUseID:  getString(raw, "tool_call_id"),
			ToolResult: getString(raw, "content"),
		}}
	}

	// Extract usage if present
	if usage, ok := raw["usage"].(map[string]any); ok {
		entry.Usage = &thinkt.TokenUsage{
			InputTokens:  getInt(usage, "input_tokens"),
			OutputTokens: getInt(usage, "output_tokens"),
		}
	}

	return entry, nil
}

func convertKimiRole(role string) thinkt.Role {
	switch role {
	case "user":
		return thinkt.RoleUser
	case "assistant":
		return thinkt.RoleAssistant
	case "tool":
		return thinkt.RoleTool
	case "system":
		return thinkt.RoleSystem
	default:
		return thinkt.RoleSystem
	}
}

func convertKimiContentBlocks(arr []any) []thinkt.ContentBlock {
	result := make([]thinkt.ContentBlock, 0, len(arr))
	for _, item := range arr {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		
		cb := thinkt.ContentBlock{
			Type: getString(block, "type"),
		}
		
		switch cb.Type {
		case "text":
			cb.Text = getString(block, "text")
		case "thinking":
			cb.Thinking = getString(block, "thinking")
			cb.Signature = getString(block, "signature")
		case "tool_result":
			cb.ToolUseID = getString(block, "tool_use_id")
			if content, ok := block["content"].(string); ok {
				cb.ToolResult = content
			} else if contentArr, ok := block["content"].([]any); ok && len(contentArr) > 0 {
				// Extract text from content array
				for _, c := range contentArr {
					if textMap, ok := c.(map[string]any); ok {
						if text, ok := textMap["text"].(string); ok {
							cb.ToolResult += text
						}
					}
				}
			}
			cb.IsError = getBool(block, "is_error")
		}
		
		result = append(result, cb)
	}
	return result
}

func extractTextFromBlocks(blocks []thinkt.ContentBlock) string {
	var texts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			texts = append(texts, b.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func getBool(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

// kimiWorkDir represents an entry in kimi.json.
type kimiWorkDir struct {
	Path          string `json:"path"`
	Kaos          string `json:"kaos"`
	LastSessionID string `json:"last_session_id"`
}

// kimiJSON represents the structure of ~/.kimi/kimi.json.
type kimiJSON struct {
	WorkDirs []kimiWorkDir `json:"work_dirs"`
}

func (s *Store) loadWorkDirs() ([]kimiWorkDir, error) {
	path := filepath.Join(s.baseDir, "kimi.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var kj kimiJSON
	if err := json.Unmarshal(data, &kj); err != nil {
		return nil, err
	}

	return kj.WorkDirs, nil
}

func (s *Store) scanProjects(sessionsDir string) ([]thinkt.Project, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, err
	}

	ws := s.Workspace()
	projects := make([]thinkt.Project, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		hash := entry.Name()
		sessions, _ := s.listSessionsForHash(hash)

		info, _ := entry.Info()

		projects = append(projects, thinkt.Project{
			ID:           hash,
			Name:         hash[:8], // Show first 8 chars of hash
			Path:         hash,
			DisplayPath:  hash[:8] + "...",
			SessionCount: len(sessions),
			LastModified: info.ModTime(),
			Source:       thinkt.SourceKimi,
			WorkspaceID:  ws.ID,
		})
	}

	return projects, nil
}
