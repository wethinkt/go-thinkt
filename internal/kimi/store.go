// Package kimi provides Kimi Code session storage implementation.
package kimi

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/jsonl"
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
		sessionPath := filepath.Join(sessionDir, sessionID)
		contextPath := filepath.Join(sessionPath, "context.jsonl")

		info, err := os.Stat(contextPath)
		if err != nil {
			continue
		}

		// Count entries by reading the file(s)
		count, firstPrompt := s.countEntriesAndFirstPrompt(contextPath)
		
		// Count chunks (context_sub_*.jsonl files)
		chunkCount := countChunks(sessionPath)

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
			ChunkCount:  chunkCount,
		})
	}

	return sessions, nil
}

// countChunks counts the number of context files (context.jsonl + context_sub_*.jsonl)
func countChunks(sessionPath string) int {
	entries, err := os.ReadDir(sessionPath)
	if err != nil {
		return 1 // Assume single file if we can't read
	}
	
	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if name == "context.jsonl" || strings.HasPrefix(name, "context_sub_") && strings.HasSuffix(name, ".jsonl") {
			count++
		}
	}
	if count == 0 {
		return 1 // Default to 1 if we find nothing
	}
	return count
}

func (s *Store) countEntriesAndFirstPrompt(path string) (int, string) {
	reader, err := jsonl.NewReader(path)
	if err != nil {
		return 0, ""
	}
	defer reader.Close()

	count := 0
	firstPrompt := ""

	for {
		line, err := reader.ReadLine()
		if err == io.EOF {
			if len(line) > 0 {
				count++
				if firstPrompt == "" {
					var entry struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					}
					if err := json.Unmarshal(line, &entry); err == nil {
						if entry.Role == "user" && entry.Content != "" {
							firstPrompt = entry.Content
							if len(firstPrompt) > 50 {
								firstPrompt = firstPrompt[:50] + "..."
							}
						}
					}
				}
			}
			break
		}
		if err != nil {
			break
		}
		if len(line) == 0 {
			continue
		}

		count++
		if firstPrompt == "" {
			var entry struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(line, &entry); err == nil {
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
	
	// Count chunks
	sessionPath := filepath.Join(s.baseDir, "sessions", hash, uuid)
	chunkCount := countChunks(sessionPath)
	
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
		ChunkCount:  chunkCount,
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
// For chunked sessions (context_sub_*.jsonl), it transparently stitches all files together.
func (s *Store) OpenSession(ctx context.Context, sessionID string) (thinkt.SessionReader, error) {
	// Resolve session ID to path
	meta, err := s.GetSessionMeta(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Check if session is chunked
	if meta.ChunkCount > 1 {
		return s.openChunkedSession(*meta)
	}

	reader, err := jsonl.NewReader(meta.FullPath)
	if err != nil {
		return nil, err
	}

	return &kimiReader{
		reader: reader,
		meta:   *meta,
		source: thinkt.SourceKimi,
		wsID:   s.Workspace().ID,
	}, nil
}

// openChunkedSession creates a reader that transparently reads across chunked files.
func (s *Store) openChunkedSession(meta thinkt.SessionMeta) (thinkt.SessionReader, error) {
	sessionPath := filepath.Dir(meta.FullPath)
	
	// Build list of chunk files in order
	var chunkFiles []string
	chunkFiles = append(chunkFiles, meta.FullPath) // context.jsonl
	
	// Find and sort context_sub_*.jsonl files
	entries, err := os.ReadDir(sessionPath)
	if err != nil {
		return nil, err
	}
	
	var subChunks []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "context_sub_") && strings.HasSuffix(name, ".jsonl") {
			subChunks = append(subChunks, filepath.Join(sessionPath, name))
		}
	}
	// Sort numerically (context_sub_1.jsonl, context_sub_2.jsonl, etc.)
	sort.Slice(subChunks, func(i, j int) bool {
		// Extract number from filename for proper numeric sorting
		numI := extractChunkNumber(subChunks[i])
		numJ := extractChunkNumber(subChunks[j])
		return numI < numJ
	})
	
	chunkFiles = append(chunkFiles, subChunks...)
	
	return &chunkedKimiReader{
		files:   chunkFiles,
		meta:    meta,
		source:  thinkt.SourceKimi,
		wsID:    s.Workspace().ID,
	}, nil
}

// extractChunkNumber extracts the number from context_sub_N.jsonl
func extractChunkNumber(path string) int {
	base := filepath.Base(path)
	// Remove prefix and suffix
	numStr := strings.TrimPrefix(base, "context_sub_")
	numStr = strings.TrimSuffix(numStr, ".jsonl")
	num, _ := strconv.Atoi(numStr)
	return num
}

// kimiReader implements thinkt.SessionReader for Kimi format.
type kimiReader struct {
	reader     *jsonl.Reader
	meta       thinkt.SessionMeta
	closed     bool
	done       bool
	lineNum    int
	source     thinkt.Source
	wsID       string
}

func (r *kimiReader) ReadNext() (*thinkt.Entry, error) {
	if r.closed {
		return nil, io.ErrClosedPipe
	}
	if r.done {
		return nil, io.EOF
	}

	for {
		line, err := r.reader.ReadLine()
		r.lineNum++
		if err == io.EOF {
			if len(line) > 0 {
				entry, parseErr := parseKimiEntry(line, r.lineNum, r.source, r.wsID)
				if parseErr == nil && entry != nil {
					return entry, nil
				}
			}
			r.done = true
			return nil, io.EOF
		}
		if err != nil {
			return nil, err
		}
		if len(line) == 0 {
			continue
		}

		entry, err := parseKimiEntry(line, r.lineNum, r.source, r.wsID)
		if err != nil {
			continue // Skip malformed entries
		}
		if entry != nil {
			return entry, nil
		}
	}
}

func (r *kimiReader) Metadata() thinkt.SessionMeta {
	return r.meta
}

func (r *kimiReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.reader.Close()
}

// chunkedKimiReader reads across multiple chunk files transparently.
type chunkedKimiReader struct {
	files      []string
	meta       thinkt.SessionMeta
	currentIdx int
	current    *jsonl.Reader
	lineNum    int
	closed     bool
	source     thinkt.Source
	wsID       string
}

func (r *chunkedKimiReader) ReadNext() (*thinkt.Entry, error) {
	if r.closed {
		return nil, io.ErrClosedPipe
	}

	// If no current reader, open first file
	if r.current == nil && r.currentIdx < len(r.files) {
		reader, err := jsonl.NewReader(r.files[r.currentIdx])
		if err != nil {
			return nil, err
		}
		r.current = reader
	}

	for r.current != nil {
		line, err := r.current.ReadLine()
		r.lineNum++
		
		if err == io.EOF {
			// Try to process any final line
			if len(line) > 0 {
				entry, parseErr := parseKimiEntry(line, r.lineNum, r.source, r.wsID)
				if parseErr == nil && entry != nil {
					return entry, nil
				}
			}
			// Move to next file
			r.current.Close()
			r.currentIdx++
			if r.currentIdx >= len(r.files) {
				return nil, io.EOF
			}
			reader, err := jsonl.NewReader(r.files[r.currentIdx])
			if err != nil {
				return nil, err
			}
			r.current = reader
			continue
		}
		
		if err != nil {
			return nil, err
		}
		
		if len(line) == 0 {
			continue
		}

		entry, err := parseKimiEntry(line, r.lineNum, r.source, r.wsID)
		if err != nil {
			continue // Skip malformed entries
		}
		if entry != nil {
			return entry, nil
		}
	}

	return nil, io.EOF
}

func (r *chunkedKimiReader) Metadata() thinkt.SessionMeta {
	return r.meta
}

func (r *chunkedKimiReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if r.current != nil {
		r.current.Close()
	}
	return nil
}

// parseKimiEntry parses a single line from context.jsonl.
// lineNum is used to generate a deterministic UUID ("L{lineNum}").
// source and wsID are set for provenance tracking.
func parseKimiEntry(data []byte, lineNum int, source thinkt.Source, wsID string) (*thinkt.Entry, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	role, _ := raw["role"].(string)
	
	// Skip empty entries and usage metadata entries
	if role == "" || role == "_usage" {
		return nil, nil
	}

	entry := &thinkt.Entry{
		UUID:        fmt.Sprintf("L%d", lineNum), // Deterministic UUID from line number
		Role:        convertKimiRole(role),
		Source:      source,
		WorkspaceID: wsID,
		Metadata:    make(map[string]any),
	}

	// Handle checkpoint entries as first-class role
	if role == "_checkpoint" {
		entry.Role = thinkt.RoleCheckpoint
		entry.IsCheckpoint = true
		return entry, nil // Return checkpoint entries now
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
		if os.IsNotExist(err) {
			return []thinkt.Project{}, nil
		}
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
