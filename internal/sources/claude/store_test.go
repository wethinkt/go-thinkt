package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// mockStatsigStableID creates a minimal statsig stable_id file
func mockStatsigStableID(t *testing.T, dir string, stableID string) {
	t.Helper()

	statsigDir := filepath.Join(dir, "statsig")
	if err := os.MkdirAll(statsigDir, 0755); err != nil {
		t.Fatalf("creating statsig dir: %v", err)
	}

	// Claude uses a file pattern like statsig.stable_id.*
	if err := os.WriteFile(filepath.Join(statsigDir, "statsig.stable_id.test"), []byte(stableID), 0644); err != nil {
		t.Fatalf("writing statsig stable_id: %v", err)
	}
}

// createTestSessionFile creates a JSONL file with test entries
func createTestSessionFile(t *testing.T, path string, entries []map[string]any) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating session file: %v", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			t.Fatalf("encoding entry: %v", err)
		}
	}
}

func TestStore_Workspace(t *testing.T) {
	tmp := t.TempDir()
	mockStatsigStableID(t, tmp, "stable-123")

	store := NewStore(tmp)

	ws := store.Workspace()

	if ws.ID != "stable-123" {
		t.Errorf("expected workspace ID 'stable-123', got '%s'", ws.ID)
	}
	if ws.Source != thinkt.SourceClaude {
		t.Errorf("expected source %v, got %v", thinkt.SourceClaude, ws.Source)
	}
	if ws.BasePath != tmp {
		t.Errorf("expected BasePath '%s', got '%s'", tmp, ws.BasePath)
	}
}

func TestStore_Workspace_Defaults(t *testing.T) {
	tmp := t.TempDir()
	// No statsig file created - should use hostname as fallback

	store := NewStore(tmp)

	ws := store.Workspace()

	// Should fall back to hostname
	hostname, _ := os.Hostname()
	if ws.ID != hostname {
		t.Errorf("expected hostname fallback '%s', got '%s'", hostname, ws.ID)
	}
}

func TestStore_Source(t *testing.T) {
	store := NewStore(t.TempDir())

	if store.Source() != thinkt.SourceClaude {
		t.Errorf("expected source %v, got %v", thinkt.SourceClaude, store.Source())
	}
}

func TestStore_ListProjects(t *testing.T) {
	tmp := t.TempDir()
	mockStatsigStableID(t, tmp, "ws-123")

	// Create project directories using Claude's format (escaped paths)
	// Note: ListProjects filters out projects with 0 sessions, so we need at least one JSONL file
	projectsDir := filepath.Join(tmp, "projects")
	project1Dir := filepath.Join(projectsDir, "-Users-evan-project1")
	project2Dir := filepath.Join(projectsDir, "-Users-evan-project2")
	if err := os.MkdirAll(project1Dir, 0755); err != nil {
		t.Fatalf("creating project1: %v", err)
	}
	if err := os.MkdirAll(project2Dir, 0755); err != nil {
		t.Fatalf("creating project2: %v", err)
	}

	// Create at least one session in each project
	createTestSessionFile(t, filepath.Join(project1Dir, "sess1.jsonl"), []map[string]any{
		{"type": "user", "uuid": "u1", "timestamp": "2024-01-15T10:00:00Z", "message": map[string]any{
			"content": map[string]any{"text": "Hello"},
		}},
	})
	createTestSessionFile(t, filepath.Join(project2Dir, "sess2.jsonl"), []map[string]any{
		{"type": "user", "uuid": "u2", "timestamp": "2024-01-15T11:00:00Z", "message": map[string]any{
			"content": map[string]any{"text": "World"},
		}},
	})

	store := NewStore(tmp)

	projects, err := store.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}

	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}

	// Check project properties
	for _, p := range projects {
		if p.Source != thinkt.SourceClaude {
			t.Errorf("expected source %v, got %v", thinkt.SourceClaude, p.Source)
		}
		if p.WorkspaceID != "ws-123" {
			t.Errorf("expected workspace ID 'ws-123', got '%s'", p.WorkspaceID)
		}
		// Path is the decoded full path (e.g., /Users/evan/project1)
		if !strings.HasPrefix(p.Path, "/Users/evan/") {
			t.Errorf("unexpected project path: %s", p.Path)
		}
	}
}

func TestStore_ListProjects_Empty(t *testing.T) {
	tmp := t.TempDir()
	mockStatsigStableID(t, tmp, "ws-123")

	// Create projects dir but leave it empty
	projectsDir := filepath.Join(tmp, "projects")
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatalf("creating projects dir: %v", err)
	}

	store := NewStore(tmp)

	projects, err := store.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}

	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

func TestStore_ListSessions(t *testing.T) {
	tmp := t.TempDir()
	mockStatsigStableID(t, tmp, "ws-123")

	// Create project with sessions (sessions are directly in project dir, not in sessions/)
	projectDir := filepath.Join(tmp, "projects", "-Users-evan-testproject")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	// Create test session files directly in project dir
	createTestSessionFile(t, filepath.Join(projectDir, "session1.jsonl"), []map[string]any{
		{"type": "user", "uuid": "entry-1", "timestamp": "2024-01-15T10:00:00Z", "message": map[string]any{
			"content": map[string]any{"text": "Hello"},
		}},
		{"type": "assistant", "uuid": "entry-2", "timestamp": "2024-01-15T10:01:00Z", "message": map[string]any{
			"content": []any{},
		}},
	})

	createTestSessionFile(t, filepath.Join(projectDir, "session2.jsonl"), []map[string]any{
		{"type": "user", "uuid": "entry-3", "timestamp": "2024-01-15T11:00:00Z", "message": map[string]any{
			"content": map[string]any{"text": "Test prompt"},
		}},
	})

	store := NewStore(tmp)

	// Note: ListSessions takes projectID (path), not project struct
	sessions, err := store.ListSessions(context.Background(), projectDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}

	// Verify session metadata
	for _, sess := range sessions {
		if sess.Source != thinkt.SourceClaude {
			t.Errorf("expected source %v, got %v", thinkt.SourceClaude, sess.Source)
		}
		if sess.WorkspaceID != "ws-123" {
			t.Errorf("expected workspace ID 'ws-123', got '%s'", sess.WorkspaceID)
		}
		if sess.ChunkCount != 1 { // Claude sessions are not chunked
			t.Errorf("expected ChunkCount=1, got %d", sess.ChunkCount)
		}
		if !strings.HasSuffix(sess.FullPath, ".jsonl") {
			t.Errorf("expected .jsonl extension in path: %s", sess.FullPath)
		}
	}
}

func TestStore_LoadSession(t *testing.T) {
	tmp := t.TempDir()
	mockStatsigStableID(t, tmp, "ws-123")

	// Create project with session (sessions are directly in project dir)
	projectDir := filepath.Join(tmp, "projects", "-Users-evan-testproject")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	sessionPath := filepath.Join(projectDir, "test-session.jsonl")
	createTestSessionFile(t, sessionPath, []map[string]any{
		{
			"type":        "user",
			"uuid":        "user-uuid",
			"timestamp":   "2024-01-15T10:00:00Z",
			"sessionId":   "test-session",
			"gitBranch":   "main",
			"cwd":         "/Users/evan/testproject",
			"isSidechain": false,
			"message": map[string]any{
				"content": map[string]any{"text": "Hello, Claude!"},
			},
		},
		{
			"type":      "assistant",
			"uuid":      "assistant-uuid",
			"timestamp": "2024-01-15T10:01:00Z",
			"sessionId": "test-session",
			"message": map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "Hello! How can I help?"},
				},
			},
		},
	})

	store := NewStore(tmp)

	// LoadSession takes sessionID, finds it via GetSessionMeta
	session, err := store.LoadSession(context.Background(), "test-session")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}

	// Verify metadata
	if session.Meta.ID != "test-session" {
		t.Errorf("expected session ID 'test-session', got '%s'", session.Meta.ID)
	}
	if session.Meta.Source != thinkt.SourceClaude {
		t.Errorf("expected source %v, got %v", thinkt.SourceClaude, session.Meta.Source)
	}
	if session.Meta.WorkspaceID != "ws-123" {
		t.Errorf("expected workspace ID 'ws-123', got '%s'", session.Meta.WorkspaceID)
	}
	if session.Meta.GitBranch != "main" {
		t.Errorf("expected git branch 'main', got '%s'", session.Meta.GitBranch)
	}

	// Verify entries
	if len(session.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(session.Entries))
	}

	// First entry (user)
	if session.Entries[0].UUID != "user-uuid" {
		t.Errorf("expected UUID 'user-uuid', got '%s'", session.Entries[0].UUID)
	}
	if session.Entries[0].Role != thinkt.RoleUser {
		t.Errorf("expected role %v, got %v", thinkt.RoleUser, session.Entries[0].Role)
	}
	// Note: Text extraction depends on message parsing which may need the full UserMessage structure
	if session.Entries[0].GitBranch != "main" {
		t.Errorf("expected git branch 'main', got '%s'", session.Entries[0].GitBranch)
	}
	if session.Entries[0].CWD != "/Users/evan/testproject" {
		t.Errorf("expected CWD '/Users/evan/testproject', got '%s'", session.Entries[0].CWD)
	}
	if session.Entries[0].Source != thinkt.SourceClaude {
		t.Errorf("expected source %v, got %v", thinkt.SourceClaude, session.Entries[0].Source)
	}
	if session.Entries[0].WorkspaceID != "ws-123" {
		t.Errorf("expected workspace ID 'ws-123', got '%s'", session.Entries[0].WorkspaceID)
	}

	// Second entry (assistant)
	if session.Entries[1].UUID != "assistant-uuid" {
		t.Errorf("expected UUID 'assistant-uuid', got '%s'", session.Entries[1].UUID)
	}
	if session.Entries[1].Role != thinkt.RoleAssistant {
		t.Errorf("expected role %v, got %v", thinkt.RoleAssistant, session.Entries[1].Role)
	}
}

func TestStore_OpenSession(t *testing.T) {
	tmp := t.TempDir()
	mockStatsigStableID(t, tmp, "ws-123")

	// Create project with session (sessions are directly in project dir)
	projectDir := filepath.Join(tmp, "projects", "-Users-evan-testproject")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	sessionPath := filepath.Join(projectDir, "stream-session.jsonl")
	createTestSessionFile(t, sessionPath, []map[string]any{
		{"type": "user", "uuid": "s1", "sessionId": "stream-session", "timestamp": "2024-01-15T10:00:00Z", "message": map[string]any{
			"content": "First message", // User content can be a plain string
		}},
		{"type": "assistant", "uuid": "s2", "sessionId": "stream-session", "timestamp": "2024-01-15T10:01:00Z", "message": map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Response"}},
		}},
		{"type": "user", "uuid": "s3", "sessionId": "stream-session", "timestamp": "2024-01-15T10:02:00Z", "message": map[string]any{
			"content": "Second message", // User content can be a plain string
		}},
	})

	store := NewStore(tmp)

	// OpenSession takes sessionID
	reader, err := store.OpenSession(context.Background(), "stream-session")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer reader.Close()

	// Verify metadata
	meta := reader.Metadata()
	if meta.ID != "stream-session" {
		t.Errorf("expected session ID 'stream-session', got '%s'", meta.ID)
	}
	if meta.Source != thinkt.SourceClaude {
		t.Errorf("expected source %v, got %v", thinkt.SourceClaude, meta.Source)
	}
	if meta.WorkspaceID != "ws-123" {
		t.Errorf("expected workspace ID 'ws-123', got '%s'", meta.WorkspaceID)
	}

	// Read all entries
	var entries []*thinkt.Entry
	for {
		entry, err := reader.ReadNext()
		if err != nil {
			break
		}
		entries = append(entries, entry)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Verify entry order and content
	if len(entries) >= 1 {
		if entries[0].UUID != "s1" {
			t.Errorf("first entry UUID mismatch: got %s, want s1", entries[0].UUID)
		}
		if entries[0].Text != "First message" {
			t.Errorf("first entry text mismatch: got %s, want 'First message'", entries[0].Text)
		}
	}
	if len(entries) >= 2 {
		if entries[1].UUID != "s2" || entries[1].Role != thinkt.RoleAssistant {
			t.Errorf("second entry mismatch: UUID=%s, Role=%v", entries[1].UUID, entries[1].Role)
		}
	}
	if len(entries) >= 3 {
		if entries[2].UUID != "s3" {
			t.Errorf("third entry UUID mismatch: got %s, want s3", entries[2].UUID)
		}
		if entries[2].Text != "Second message" {
			t.Errorf("third entry text mismatch: got %s, want 'Second message'", entries[2].Text)
		}
	}
}

func TestStore_FileHistorySnapshot(t *testing.T) {
	tmp := t.TempDir()
	mockStatsigStableID(t, tmp, "ws-123")

	projectDir := filepath.Join(tmp, "projects", "-Users-evan-testproject")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	sessionPath := filepath.Join(projectDir, "snapshot-session.jsonl")
	createTestSessionFile(t, sessionPath, []map[string]any{
		{"type": "user", "uuid": "u1", "timestamp": "2024-01-15T10:00:00Z", "message": map[string]any{
			"content": map[string]any{"text": "Edit the file"},
		}},
		{
			"type":             "file-history-snapshot",
			"uuid":             "snap-1",
			"timestamp":        "2024-01-15T10:01:00Z",
			"messageId":        "u1",
			"isSnapshotUpdate": true,
			"snapshot": map[string]any{
				"messageId":          "u1",
				"trackedFileBackups": map[string]any{"file.txt": "content"},
				"timestamp":          "2024-01-15T10:01:00Z",
			},
		},
	})

	store := NewStore(tmp)

	session, err := store.LoadSession(context.Background(), "snapshot-session")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}

	if len(session.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(session.Entries))
	}

	// Check snapshot entry
	snapshot := session.Entries[1]
	if snapshot.UUID != "snap-1" {
		t.Errorf("expected UUID 'snap-1', got '%s'", snapshot.UUID)
	}
	if snapshot.Role != thinkt.RoleCheckpoint {
		t.Errorf("expected role %v for file-history-snapshot, got %v", thinkt.RoleCheckpoint, snapshot.Role)
	}
	if !snapshot.IsCheckpoint {
		t.Errorf("expected IsCheckpoint=true for file-history-snapshot")
	}
	if snapshot.Source != thinkt.SourceClaude {
		t.Errorf("expected source %v, got %v", thinkt.SourceClaude, snapshot.Source)
	}
	if snapshot.WorkspaceID != "ws-123" {
		t.Errorf("expected workspace ID 'ws-123', got '%s'", snapshot.WorkspaceID)
	}
	wantText := "File History Snapshot (1 file: file.txt)"
	if snapshot.Text != wantText {
		t.Errorf("expected Text %q, got %q", wantText, snapshot.Text)
	}
}

func TestStore_SystemProgressSummaryRoles(t *testing.T) {
	tmp := t.TempDir()
	mockStatsigStableID(t, tmp, "ws-123")

	projectDir := filepath.Join(tmp, "projects", "-Users-evan-testproject")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	sessionPath := filepath.Join(projectDir, "roles-session.jsonl")
	createTestSessionFile(t, sessionPath, []map[string]any{
		{"type": "system", "uuid": "sys-1", "timestamp": "2024-01-15T10:00:00Z", "content": "System message"},
		{"type": "progress", "uuid": "prog-1", "timestamp": "2024-01-15T10:01:00Z", "data": map[string]any{}},
		{"type": "summary", "uuid": "sum-1", "timestamp": "2024-01-15T10:02:00Z", "summary": "Summary"},
		{"type": "unknown", "uuid": "unk-1", "timestamp": "2024-01-15T10:03:00Z"},
	})

	store := NewStore(tmp)

	session, err := store.LoadSession(context.Background(), "roles-session")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}

	if len(session.Entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(session.Entries))
	}

	tests := []struct {
		idx      int
		expected thinkt.Role
	}{
		{0, thinkt.RoleSystem},
		{1, thinkt.RoleProgress},
		{2, thinkt.RoleSummary},
		{3, thinkt.RoleSystem}, // unknown defaults to system
	}

	for _, tt := range tests {
		if session.Entries[tt.idx].Role != tt.expected {
			t.Errorf("entry %d: expected role %v, got %v", tt.idx, tt.expected, session.Entries[tt.idx].Role)
		}
	}
}

func TestStore_GetSessionMeta(t *testing.T) {
	tmp := t.TempDir()
	mockStatsigStableID(t, tmp, "ws-123")

	projectDir := filepath.Join(tmp, "projects", "-Users-evan-testproject")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	// Create the actual session file
	createTestSessionFile(t, filepath.Join(projectDir, "meta-session.jsonl"), []map[string]any{
		{"type": "user", "uuid": "m1", "timestamp": "2024-01-15T10:00:00Z", "message": map[string]any{
			"content": "First prompt text",
		}},
	})

	// Create sessions-index.json for rich metadata (EntryCount, FirstPrompt)
	indexData := map[string]any{
		"version": 1,
		"entries": []map[string]any{
			{
				"sessionId":    "meta-session",
				"created":      "2024-01-15T10:00:00Z",
				"modified":     "2024-01-15T10:01:00Z",
				"messageCount": 2,
				"firstPrompt":  "First prompt text",
				"summary":      "Test summary",
				"gitBranch":    "main",
			},
		},
	}
	indexPath := filepath.Join(projectDir, "sessions-index.json")
	indexBytes, _ := json.Marshal(indexData)
	if err := os.WriteFile(indexPath, indexBytes, 0644); err != nil {
		t.Fatalf("writing sessions-index.json: %v", err)
	}

	store := NewStore(tmp)

	meta, err := store.GetSessionMeta(context.Background(), "meta-session")
	if err != nil {
		t.Fatalf("GetSessionMeta: %v", err)
	}
	if meta == nil {
		t.Fatal("expected session meta, got nil")
	}

	if meta.ID != "meta-session" {
		t.Errorf("expected ID 'meta-session', got '%s'", meta.ID)
	}
	if meta.Source != thinkt.SourceClaude {
		t.Errorf("expected source %v, got %v", thinkt.SourceClaude, meta.Source)
	}
	if meta.WorkspaceID != "ws-123" {
		t.Errorf("expected workspace ID 'ws-123', got '%s'", meta.WorkspaceID)
	}
	if meta.EntryCount != 2 {
		t.Errorf("expected EntryCount=2, got %d", meta.EntryCount)
	}
	if meta.ChunkCount != 1 {
		t.Errorf("expected ChunkCount=1, got %d", meta.ChunkCount)
	}
	if meta.FirstPrompt != "First prompt text" {
		t.Errorf("expected FirstPrompt='First prompt text', got '%s'", meta.FirstPrompt)
	}
	if meta.Summary != "Test summary" {
		t.Errorf("expected Summary='Test summary', got '%s'", meta.Summary)
	}
	if meta.GitBranch != "main" {
		t.Errorf("expected GitBranch='main', got '%s'", meta.GitBranch)
	}
}

func TestStore_NonExistentSession(t *testing.T) {
	tmp := t.TempDir()
	mockStatsigStableID(t, tmp, "ws-123")

	store := NewStore(tmp)

	session, err := store.LoadSession(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session != nil {
		t.Error("expected nil session for non-existent ID")
	}
}

func TestConvertRole(t *testing.T) {
	tests := []struct {
		entryType EntryType
		expected  thinkt.Role
	}{
		{EntryTypeUser, thinkt.RoleUser},
		{EntryTypeAssistant, thinkt.RoleAssistant},
		{EntryTypeSystem, thinkt.RoleSystem},
		{EntryTypeProgress, thinkt.RoleProgress},
		{EntryTypeSummary, thinkt.RoleSummary},
		{EntryTypeFileHistorySnapshot, thinkt.RoleCheckpoint},
		{EntryTypeQueueOperation, thinkt.RoleSystem}, // unknown defaults to system
		{"unknown-type", thinkt.RoleSystem},
	}

	for _, tt := range tests {
		got := convertRole(tt.entryType)
		if got != tt.expected {
			t.Errorf("convertRole(%q) = %v, want %v", tt.entryType, got, tt.expected)
		}
	}
}

func TestConvertEntry_Provenance(t *testing.T) {
	timestamp := "2024-01-15T10:00:00Z"
	claudeEntry := &Entry{
		Type:        EntryTypeUser,
		UUID:        "test-uuid",
		Timestamp:   timestamp,
		GitBranch:   "main",
		CWD:         "/home/project",
		IsSidechain: false,
		Message:     json.RawMessage(`{"content":{"text":"Hello"}}`),
	}

	entry := convertEntry(claudeEntry, thinkt.SourceClaude, "ws-test")

	if entry == nil {
		t.Fatal("convertEntry returned nil")
	}

	if entry.UUID != "test-uuid" {
		t.Errorf("expected UUID 'test-uuid', got '%s'", entry.UUID)
	}
	if entry.Source != thinkt.SourceClaude {
		t.Errorf("expected Source %v, got %v", thinkt.SourceClaude, entry.Source)
	}
	if entry.WorkspaceID != "ws-test" {
		t.Errorf("expected WorkspaceID 'ws-test', got '%s'", entry.WorkspaceID)
	}
	if entry.Role != thinkt.RoleUser {
		t.Errorf("expected Role %v, got %v", thinkt.RoleUser, entry.Role)
	}

	// Check timestamp parsing
	expectedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	if !entry.Timestamp.Equal(expectedTime) {
		t.Errorf("expected timestamp %v, got %v", expectedTime, entry.Timestamp)
	}
}

func TestConvertEntry_Checkpoint(t *testing.T) {
	claudeEntry := &Entry{
		Type:             EntryTypeFileHistorySnapshot,
		UUID:             "snapshot-uuid",
		Timestamp:        "2024-01-15T10:00:00Z",
		IsSnapshotUpdate: true,
	}

	entry := convertEntry(claudeEntry, thinkt.SourceClaude, "ws-test")

	if entry == nil {
		t.Fatal("convertEntry returned nil")
	}

	if entry.Role != thinkt.RoleCheckpoint {
		t.Errorf("expected Role %v for file-history-snapshot, got %v", thinkt.RoleCheckpoint, entry.Role)
	}
	if !entry.IsCheckpoint {
		t.Error("expected IsCheckpoint=true for file-history-snapshot")
	}
}

func TestToThinktEntry_FileHistorySnapshot(t *testing.T) {
	e := &Entry{
		Type:             EntryTypeFileHistorySnapshot,
		MessageID:        "msg-1",
		IsSnapshotUpdate: true,
		Snapshot:         json.RawMessage(`{"messageId":"msg-1","trackedFileBackups":{"src/main.go":"content","src/util.go":"content"},"timestamp":"2024-01-15T10:00:00Z"}`),
	}

	entry := e.ToThinktEntry()

	if entry.Role != thinkt.RoleCheckpoint {
		t.Errorf("expected Role %v, got %v", thinkt.RoleCheckpoint, entry.Role)
	}
	if !entry.IsCheckpoint {
		t.Error("expected IsCheckpoint=true")
	}
	want := "File History Snapshot (2 files)"
	if entry.Text != want {
		t.Errorf("expected Text %q, got %q", want, entry.Text)
	}
}

func TestConvertEntry_Nil(t *testing.T) {
	entry := convertEntry(nil, thinkt.SourceClaude, "ws-test")
	if entry != nil {
		t.Error("expected nil for nil input")
	}
}

func TestConvertEntry_AssistantWithUsage(t *testing.T) {
	claudeEntry := &Entry{
		Type:      EntryTypeAssistant,
		UUID:      "assistant-uuid",
		Timestamp: "2024-01-15T10:00:00Z",
		Message: json.RawMessage(`{
			"content": [{"type": "text", "text": "Response"}],
			"usage": {
				"input_tokens": 100,
				"output_tokens": 50,
				"cache_creation_input_tokens": 20,
				"cache_read_input_tokens": 30
			}
		}`),
	}

	entry := convertEntry(claudeEntry, thinkt.SourceClaude, "ws-test")

	if entry == nil {
		t.Fatal("convertEntry returned nil")
	}

	if entry.Role != thinkt.RoleAssistant {
		t.Errorf("expected Role %v, got %v", thinkt.RoleAssistant, entry.Role)
	}
	if entry.Usage == nil {
		t.Fatal("expected Usage to be set")
	}
	if entry.Usage.InputTokens != 100 {
		t.Errorf("expected InputTokens=100, got %d", entry.Usage.InputTokens)
	}
	if entry.Usage.OutputTokens != 50 {
		t.Errorf("expected OutputTokens=50, got %d", entry.Usage.OutputTokens)
	}
	if entry.Usage.CacheCreationInputTokens != 20 {
		t.Errorf("expected CacheCreationInputTokens=20, got %d", entry.Usage.CacheCreationInputTokens)
	}
	if entry.Usage.CacheReadInputTokens != 30 {
		t.Errorf("expected CacheReadInputTokens=30, got %d", entry.Usage.CacheReadInputTokens)
	}
}

func TestStore_GetProject(t *testing.T) {
	tmp := t.TempDir()
	mockStatsigStableID(t, tmp, "ws-123")

	// Create project directories (with at least one session to not be filtered out)
	projectsDir := filepath.Join(tmp, "projects")
	project1Dir := filepath.Join(projectsDir, "-Users-evan-project1")
	if err := os.MkdirAll(project1Dir, 0755); err != nil {
		t.Fatalf("creating project1: %v", err)
	}
	// Create a session so project is not filtered out
	createTestSessionFile(t, filepath.Join(project1Dir, "sess.jsonl"), []map[string]any{
		{"type": "user", "uuid": "u1", "timestamp": "2024-01-15T10:00:00Z", "message": map[string]any{
			"content": map[string]any{"text": "Test"},
		}},
	})

	store := NewStore(tmp)

	// Project ID is the full directory path
	projectID := project1Dir
	
	project, err := store.GetProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if project == nil {
		t.Fatal("expected project, got nil")
	}
	if project.ID != projectID {
		t.Errorf("expected project ID '%s', got '%s'", projectID, project.ID)
	}
	// Check the decoded path
	if project.Path != "/Users/evan/project1" {
		t.Errorf("expected project Path '/Users/evan/project1', got '%s'", project.Path)
	}

	// Test non-existent project
	project, err = store.GetProject(context.Background(), "/nonexistent/path")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if project != nil {
		t.Error("expected nil for non-existent project")
	}
}

func TestStore_ListSessions_EntryCount_NoIndex(t *testing.T) {
	tmp := t.TempDir()
	mockStatsigStableID(t, tmp, "ws-entry-count")

	projectDir := filepath.Join(tmp, "projects", "-Users-evan-testproject")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	// Create a session file with 3 entries but NO sessions-index.json.
	createTestSessionFile(t, filepath.Join(projectDir, "no-index-session.jsonl"), []map[string]any{
		{"type": "user", "uuid": "u1", "timestamp": "2024-01-15T10:00:00Z", "message": map[string]any{"content": "hello"}},
		{"type": "assistant", "uuid": "a1", "timestamp": "2024-01-15T10:00:01Z", "message": map[string]any{"content": []map[string]any{{"type": "text", "text": "hi"}}, "model": "claude-sonnet-4-20250514"}},
		{"type": "user", "uuid": "u2", "timestamp": "2024-01-15T10:00:02Z", "message": map[string]any{"content": "thanks"}},
	})

	store := NewStore(tmp)
	sessions, err := store.ListSessions(context.Background(), projectDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].EntryCount == 0 {
		t.Errorf("EntryCount should not be 0 when JSONL file has entries (got %d)", sessions[0].EntryCount)
	}
	if sessions[0].EntryCount != 3 {
		t.Errorf("expected EntryCount=3, got %d", sessions[0].EntryCount)
	}
}

func TestStore_ListSessions_EntryCount_IndexZero(t *testing.T) {
	tmp := t.TempDir()
	mockStatsigStableID(t, tmp, "ws-entry-count-2")

	projectDir := filepath.Join(tmp, "projects", "-Users-evan-testproject")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	// Create a session file with 4 entries.
	createTestSessionFile(t, filepath.Join(projectDir, "zero-count-session.jsonl"), []map[string]any{
		{"type": "user", "uuid": "u1", "timestamp": "2024-01-15T10:00:00Z", "message": map[string]any{"content": "start"}},
		{"type": "assistant", "uuid": "a1", "timestamp": "2024-01-15T10:00:01Z", "message": map[string]any{"content": []map[string]any{{"type": "text", "text": "ok"}}, "model": "claude-sonnet-4-20250514"}},
		{"type": "user", "uuid": "u2", "timestamp": "2024-01-15T10:00:02Z", "message": map[string]any{"content": "more"}},
		{"type": "assistant", "uuid": "a2", "timestamp": "2024-01-15T10:00:03Z", "message": map[string]any{"content": []map[string]any{{"type": "text", "text": "done"}}, "model": "claude-sonnet-4-20250514"}},
	})

	// Create sessions-index.json with messageCount: 0 (the bug scenario).
	indexData := map[string]any{
		"version": 1,
		"entries": []map[string]any{
			{
				"sessionId":    "zero-count-session",
				"created":      "2024-01-15T10:00:00Z",
				"modified":     "2024-01-15T10:00:03Z",
				"messageCount": 0,
				"firstPrompt":  "start",
			},
		},
	}
	indexBytes, _ := json.Marshal(indexData)
	if err := os.WriteFile(filepath.Join(projectDir, "sessions-index.json"), indexBytes, 0644); err != nil {
		t.Fatalf("writing sessions-index.json: %v", err)
	}

	store := NewStore(tmp)
	sessions, err := store.ListSessions(context.Background(), projectDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].EntryCount == 0 {
		t.Errorf("EntryCount should not be 0 when index has messageCount:0 but JSONL has entries (got %d)", sessions[0].EntryCount)
	}
	if sessions[0].EntryCount != 4 {
		t.Errorf("expected EntryCount=4, got %d", sessions[0].EntryCount)
	}
}
