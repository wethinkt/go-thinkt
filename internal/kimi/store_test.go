// Package kimi provides tests for Kimi Code session storage implementation.
package kimi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
)

// createTestStore creates a temporary Kimi store structure for testing.
func createTestStore(t *testing.T) (string, *Store) {
	t.Helper()
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	return tmpDir, store
}

// writeJSONL writes lines to a JSONL file.
func writeJSONL(t *testing.T, path string, lines []map[string]any) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	defer f.Close()

	for _, line := range lines {
		data, _ := json.Marshal(line)
		f.Write(data)
		f.WriteString("\n")
	}
}

// TestWorkspace tests workspace detection.
func TestWorkspace(t *testing.T) {
	tmpDir, store := createTestStore(t)

	// Test without device_id (should use hostname)
	ws := store.Workspace()
	if ws.ID == "" {
		t.Error("expected non-empty workspace ID")
	}
	if ws.Source != thinkt.SourceKimi {
		t.Errorf("expected source %q, got %q", thinkt.SourceKimi, ws.Source)
	}
	if ws.BasePath != tmpDir {
		t.Errorf("expected base path %q, got %q", tmpDir, ws.BasePath)
	}

	// Test with device_id
	deviceID := "test-device-id-12345"
	os.WriteFile(filepath.Join(tmpDir, "device_id"), []byte(deviceID), 0600)
	
	ws = store.Workspace()
	if ws.ID != deviceID {
		t.Errorf("expected ID %q, got %q", deviceID, ws.ID)
	}
}

// TestListProjectsEmpty tests listing projects when none exist.
func TestListProjectsEmpty(t *testing.T) {
	_, store := createTestStore(t)
	ctx := context.Background()

	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

// TestListProjectsWithSessions tests listing projects with existing sessions.
func TestListProjectsWithSessions(t *testing.T) {
	tmpDir, store := createTestStore(t)
	ctx := context.Background()

	// Create a session
	projectPath := "/Users/test/myproject"
	hash := workDirHash(projectPath)
	sessionDir := filepath.Join(tmpDir, "sessions", hash, "session-123")
	os.MkdirAll(sessionDir, 0755)

	// Create context.jsonl
	writeJSONL(t, filepath.Join(sessionDir, "context.jsonl"), []map[string]any{
		{"role": "user", "content": "Hello"},
	})

	// Create kimi.json with work directory
	kimiJSON := map[string]any{
		"work_dirs": []map[string]any{
			{"path": projectPath, "kaos": "local", "last_session_id": "session-123"},
		},
	}
	kimiJSONPath := filepath.Join(tmpDir, "kimi.json")
	data, _ := json.Marshal(kimiJSON)
	os.WriteFile(kimiJSONPath, data, 0644)

	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}

	proj := projects[0]
	if proj.Path != projectPath {
		t.Errorf("expected path %q, got %q", projectPath, proj.Path)
	}
	if proj.Name != "myproject" {
		t.Errorf("expected name %q, got %q", "myproject", proj.Name)
	}
	if proj.SessionCount != 1 {
		t.Errorf("expected 1 session, got %d", proj.SessionCount)
	}
	if proj.Source != thinkt.SourceKimi {
		t.Errorf("expected source %q, got %q", thinkt.SourceKimi, proj.Source)
	}
}

// TestListSessions tests listing sessions for a project.
func TestListSessions(t *testing.T) {
	tmpDir, store := createTestStore(t)
	ctx := context.Background()

	projectPath := "/Users/test/myproject"
	hash := workDirHash(projectPath)

	// Create multiple sessions
	for _, sessionID := range []string{"session-1", "session-2"} {
		sessionDir := filepath.Join(tmpDir, "sessions", hash, sessionID)
		os.MkdirAll(sessionDir, 0755)
		writeJSONL(t, filepath.Join(sessionDir, "context.jsonl"), []map[string]any{
			{"role": "user", "content": "Hello from " + sessionID},
		})
	}

	sessions, err := store.ListSessions(ctx, projectPath)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

// TestSessionChunkCount tests chunk detection.
func TestSessionChunkCount(t *testing.T) {
	tmpDir, store := createTestStore(t)
	ctx := context.Background()

	projectPath := "/Users/test/myproject"
	hash := workDirHash(projectPath)

	// Create chunked session
	sessionDir := filepath.Join(tmpDir, "sessions", hash, "chunked-session")
	os.MkdirAll(sessionDir, 0755)

	// Main context file
	writeJSONL(t, filepath.Join(sessionDir, "context.jsonl"), []map[string]any{
		{"role": "user", "content": "Part 1"},
	})
	// Chunk files
	writeJSONL(t, filepath.Join(sessionDir, "context_sub_1.jsonl"), []map[string]any{
		{"role": "user", "content": "Part 2"},
	})
	writeJSONL(t, filepath.Join(sessionDir, "context_sub_2.jsonl"), []map[string]any{
		{"role": "user", "content": "Part 3"},
	})

	sessions, err := store.ListSessions(ctx, projectPath)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	if sessions[0].ChunkCount != 3 {
		t.Errorf("expected chunk count 3, got %d", sessions[0].ChunkCount)
	}
}

// TestOpenSession tests reading a single session.
func TestOpenSession(t *testing.T) {
	tmpDir, store := createTestStore(t)
	ctx := context.Background()

	projectPath := "/Users/test/myproject"
	hash := workDirHash(projectPath)
	sessionID := "test-session"
	sessionDir := filepath.Join(tmpDir, "sessions", hash, sessionID)
	os.MkdirAll(sessionDir, 0755)

	writeJSONL(t, filepath.Join(sessionDir, "context.jsonl"), []map[string]any{
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": []map[string]any{
			{"type": "text", "text": "Hi there"},
		}},
	})

	reader, err := store.OpenSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}
	defer reader.Close()

	// Read first entry
	entry, err := reader.ReadNext()
	if err != nil {
		t.Fatalf("ReadNext failed: %v", err)
	}
	if entry.Role != thinkt.RoleUser {
		t.Errorf("expected role %q, got %q", thinkt.RoleUser, entry.Role)
	}
	if entry.Text != "Hello" {
		t.Errorf("expected text %q, got %q", "Hello", entry.Text)
	}
	if entry.UUID != "L1" {
		t.Errorf("expected UUID %q, got %q", "L1", entry.UUID)
	}

	// Read second entry
	entry, err = reader.ReadNext()
	if err != nil {
		t.Fatalf("ReadNext failed: %v", err)
	}
	if entry.Role != thinkt.RoleAssistant {
		t.Errorf("expected role %q, got %q", thinkt.RoleAssistant, entry.Role)
	}

	// Should be EOF
	_, err = reader.ReadNext()
	if err == nil {
		t.Error("expected EOF, got nil")
	}
}

// TestOpenChunkedSession tests reading across multiple chunk files.
func TestOpenChunkedSession(t *testing.T) {
	tmpDir, store := createTestStore(t)
	ctx := context.Background()

	projectPath := "/Users/test/myproject"
	hash := workDirHash(projectPath)
	sessionID := "chunked-session"
	sessionDir := filepath.Join(tmpDir, "sessions", hash, sessionID)
	os.MkdirAll(sessionDir, 0755)

	// Create chunked files
	writeJSONL(t, filepath.Join(sessionDir, "context.jsonl"), []map[string]any{
		{"role": "user", "content": "First"},
	})
	writeJSONL(t, filepath.Join(sessionDir, "context_sub_1.jsonl"), []map[string]any{
		{"role": "user", "content": "Second"},
	})
	writeJSONL(t, filepath.Join(sessionDir, "context_sub_2.jsonl"), []map[string]any{
		{"role": "user", "content": "Third"},
	})

	reader, err := store.OpenSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}
	defer reader.Close()

	// Read all entries across chunks
	var contents []string
	for {
		entry, err := reader.ReadNext()
		if err != nil {
			break
		}
		contents = append(contents, entry.Text)
	}

	if len(contents) != 3 {
		t.Errorf("expected 3 entries, got %d: %v", len(contents), contents)
	}
	if contents[0] != "First" || contents[1] != "Second" || contents[2] != "Third" {
		t.Errorf("unexpected contents: %v", contents)
	}
}

// TestParseCheckpointEntry tests checkpoint entries.
func TestParseCheckpointEntry(t *testing.T) {
	entry, err := parseKimiEntry([]byte(`{"role": "_checkpoint", "id": 5}`), 1, thinkt.SourceKimi, "ws-1")
	if err != nil {
		t.Fatalf("parseKimiEntry failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Role != thinkt.RoleCheckpoint {
		t.Errorf("expected role %q, got %q", thinkt.RoleCheckpoint, entry.Role)
	}
	if !entry.IsCheckpoint {
		t.Error("expected IsCheckpoint to be true")
	}
}

// TestParseUsageEntry tests that usage entries are skipped.
func TestParseUsageEntry(t *testing.T) {
	entry, err := parseKimiEntry([]byte(`{"role": "_usage", "token_count": 100}`), 1, thinkt.SourceKimi, "ws-1")
	if err != nil {
		t.Fatalf("parseKimiEntry failed: %v", err)
	}
	if entry != nil {
		t.Error("expected nil entry for usage role")
	}
}

// TestParseToolCalls tests parsing tool calls from assistant entries.
func TestParseToolCalls(t *testing.T) {
	data := []byte(`{
		"role": "assistant",
		"content": [{"type": "text", "text": "I'll help"}],
		"tool_calls": [
			{"id": "call-1", "name": "ReadFile", "input": {"path": "/tmp/test"}}
		]
	}`)

	entry, err := parseKimiEntry(data, 1, thinkt.SourceKimi, "ws-1")
	if err != nil {
		t.Fatalf("parseKimiEntry failed: %v", err)
	}

	// Should have text block + tool_use block
	if len(entry.ContentBlocks) != 2 {
		t.Errorf("expected 2 content blocks, got %d", len(entry.ContentBlocks))
	}

	// Find tool_use block
	var toolBlock *thinkt.ContentBlock
	for i := range entry.ContentBlocks {
		if entry.ContentBlocks[i].Type == "tool_use" {
			toolBlock = &entry.ContentBlocks[i]
			break
		}
	}

	if toolBlock == nil {
		t.Fatal("expected tool_use block")
	}
	if toolBlock.ToolUseID != "call-1" {
		t.Errorf("expected tool use ID %q, got %q", "call-1", toolBlock.ToolUseID)
	}
	if toolBlock.ToolName != "ReadFile" {
		t.Errorf("expected tool name %q, got %q", "ReadFile", toolBlock.ToolName)
	}
}

// TestParseToolResult tests parsing tool results.
func TestParseToolResult(t *testing.T) {
	data := []byte(`{
		"role": "tool",
		"tool_call_id": "call-1",
		"content": "File contents here"
	}`)

	entry, err := parseKimiEntry(data, 1, thinkt.SourceKimi, "ws-1")
	if err != nil {
		t.Fatalf("parseKimiEntry failed: %v", err)
	}

	if entry.Role != thinkt.RoleTool {
		t.Errorf("expected role %q, got %q", thinkt.RoleTool, entry.Role)
	}
	if len(entry.ContentBlocks) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(entry.ContentBlocks))
	}

	block := entry.ContentBlocks[0]
	if block.Type != "tool_result" {
		t.Errorf("expected type %q, got %q", "tool_result", block.Type)
	}
	if block.ToolUseID != "call-1" {
		t.Errorf("expected tool use ID %q, got %q", "call-1", block.ToolUseID)
	}
	if block.ToolResult != "File contents here" {
		t.Errorf("expected result %q, got %q", "File contents here", block.ToolResult)
	}
}

// TestProvenance tests that source and workspace ID are set.
func TestProvenance(t *testing.T) {
	data := []byte(`{"role": "user", "content": "Hello"}`)
	entry, err := parseKimiEntry(data, 42, thinkt.SourceKimi, "my-workspace")
	if err != nil {
		t.Fatalf("parseKimiEntry failed: %v", err)
	}

	if entry.Source != thinkt.SourceKimi {
		t.Errorf("expected source %q, got %q", thinkt.SourceKimi, entry.Source)
	}
	if entry.WorkspaceID != "my-workspace" {
		t.Errorf("expected workspace ID %q, got %q", "my-workspace", entry.WorkspaceID)
	}
	if entry.UUID != "L42" {
		t.Errorf("expected UUID %q, got %q", "L42", entry.UUID)
	}
}

// TestWorkDirHash tests the hash function.
func TestWorkDirHash(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/Users/test/project", "a7c44d1f2e8b3c9f5d6e7a8b9c0d1e2f"}, // actual hash will differ
	}

	for _, tt := range tests {
		hash := workDirHash(tt.input)
		if hash == "" {
			t.Error("expected non-empty hash")
		}
		if len(hash) != 32 {
			t.Errorf("expected 32-char hash, got %d chars", len(hash))
		}
	}
}

// TestFirstPromptExtraction tests that first prompt is extracted correctly.
func TestFirstPromptExtraction(t *testing.T) {
	tmpDir, store := createTestStore(t)

	projectPath := "/Users/test/myproject"
	hash := workDirHash(projectPath)
	sessionDir := filepath.Join(tmpDir, "sessions", hash, "session-1")
	os.MkdirAll(sessionDir, 0755)

	// Create session with multiple entries
	writeJSONL(t, filepath.Join(sessionDir, "context.jsonl"), []map[string]any{
		{"role": "_checkpoint", "id": 0},
		{"role": "user", "content": "This is the first prompt"},
		{"role": "assistant", "content": []map[string]any{{"type": "text", "text": "Response"}}},
		{"role": "user", "content": "Second prompt"},
	})

	count, firstPrompt := store.countEntriesAndFirstPrompt(filepath.Join(sessionDir, "context.jsonl"))
	
	if count != 4 { // includes checkpoint
		t.Errorf("expected count 4, got %d", count)
	}
	if firstPrompt != "This is the first prompt" {
		t.Errorf("expected first prompt %q, got %q", "This is the first prompt", firstPrompt)
	}
}

// TestLoadSession tests loading a complete session.
func TestLoadSession(t *testing.T) {
	tmpDir, store := createTestStore(t)
	ctx := context.Background()

	projectPath := "/Users/test/myproject"
	hash := workDirHash(projectPath)
	sessionID := "full-session"
	sessionDir := filepath.Join(tmpDir, "sessions", hash, sessionID)
	os.MkdirAll(sessionDir, 0755)

	writeJSONL(t, filepath.Join(sessionDir, "context.jsonl"), []map[string]any{
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": []map[string]any{{"type": "text", "text": "Hi"}}},
		{"role": "user", "content": "Bye"},
	})

	session, err := store.LoadSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}

	if len(session.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(session.Entries))
	}
	if session.Meta.EntryCount != 3 {
		t.Errorf("expected EntryCount 3, got %d", session.Meta.EntryCount)
	}
}
