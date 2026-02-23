package thinkt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseIDELockFile(t *testing.T) {
	// Create a temp lock file with concatenated JSON objects (like real lock files)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "12345.lock")

	entry1 := ideLockEntry{
		PID:              os.Getpid(), // current process, guaranteed alive
		WorkspaceFolders: []string{"/Users/test/project-a"},
		IDEName:          "Visual Studio Code",
	}
	entry2 := ideLockEntry{
		PID:              os.Getpid(),
		WorkspaceFolders: []string{"/Users/test/project-b"},
		IDEName:          "Cursor",
	}

	data1, _ := json.Marshal(entry1)
	data2, _ := json.Marshal(entry2)
	// Lock files have concatenated JSON (no newlines)
	os.WriteFile(lockPath, append(data1, data2...), 0644)

	entries, err := parseIDELockFile(lockPath)
	if err != nil {
		t.Fatalf("parseIDELockFile: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].IDEName != "Visual Studio Code" {
		t.Errorf("entry[0].IDEName = %q, want %q", entries[0].IDEName, "Visual Studio Code")
	}
	if entries[1].IDEName != "Cursor" {
		t.Errorf("entry[1].IDEName = %q, want %q", entries[1].IDEName, "Cursor")
	}
	if entries[0].PID != os.Getpid() {
		t.Errorf("entry[0].PID = %d, want %d", entries[0].PID, os.Getpid())
	}
}

func TestParseIDELockFile_Empty(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "empty.lock")
	os.WriteFile(lockPath, []byte{}, 0644)

	entries, err := parseIDELockFile(lockPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseIDELockFile_NotExist(t *testing.T) {
	_, err := parseIDELockFile("/nonexistent/path.lock")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestIsProcessAlive(t *testing.T) {
	// Current process should be alive
	if !isProcessAlive(os.Getpid()) {
		t.Error("current process should be alive")
	}

	// PID 0 should not be considered alive
	if isProcessAlive(0) {
		t.Error("PID 0 should not be alive")
	}

	// Negative PID should not be alive
	if isProcessAlive(-1) {
		t.Error("negative PID should not be alive")
	}
}

func TestDetectIDELock(t *testing.T) {
	// Set up a fake Claude directory structure
	dir := t.TempDir()

	// Create IDE lock file
	ideDir := filepath.Join(dir, "ide")
	os.MkdirAll(ideDir, 0755)

	lockEntry := ideLockEntry{
		PID:              os.Getpid(),
		WorkspaceFolders: []string{"/Users/test/my-project"},
		IDEName:          "Visual Studio Code",
	}
	data, _ := json.Marshal(lockEntry)
	lockFile := filepath.Join(ideDir, fmt.Sprintf("%d.lock", os.Getpid()))
	os.WriteFile(lockFile, data, 0644)

	// Create matching project directory with a session file
	// Path encoding: /Users/test/my-project -> -Users-test-my-project
	projectDir := filepath.Join(dir, "projects", "-Users-test-my-project")
	os.MkdirAll(projectDir, 0755)

	sessionFile := filepath.Join(projectDir, "abc-123-def.jsonl")
	os.WriteFile(sessionFile, []byte(`{"type":"user"}`), 0644)

	// Create detector
	registry := NewRegistry()
	detector := NewActiveSessionDetector(registry)
	detector.SetClaudeDir(dir)

	sessions, err := detector.detectIDELock(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("detectIDELock: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.Method != "ide_lock" {
		t.Errorf("Method = %q, want %q", s.Method, "ide_lock")
	}
	if s.IDE != "Visual Studio Code" {
		t.Errorf("IDE = %q, want %q", s.IDE, "Visual Studio Code")
	}
	if s.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", s.PID, os.Getpid())
	}
	if s.ProjectPath != "/Users/test/my-project" {
		t.Errorf("ProjectPath = %q, want %q", s.ProjectPath, "/Users/test/my-project")
	}
	if s.SessionID != "abc-123-def" {
		t.Errorf("SessionID = %q, want %q", s.SessionID, "abc-123-def")
	}
	if s.Source != SourceClaude {
		t.Errorf("Source = %q, want %q", s.Source, SourceClaude)
	}
}

func TestDetectIDELock_DeadProcess(t *testing.T) {
	dir := t.TempDir()
	ideDir := filepath.Join(dir, "ide")
	os.MkdirAll(ideDir, 0755)

	// Use a PID that's very unlikely to be alive
	lockEntry := ideLockEntry{
		PID:              999999999,
		WorkspaceFolders: []string{"/Users/test/project"},
		IDEName:          "VS Code",
	}
	data, _ := json.Marshal(lockEntry)
	os.WriteFile(filepath.Join(ideDir, "999999999.lock"), data, 0644)

	registry := NewRegistry()
	detector := NewActiveSessionDetector(registry)
	detector.SetClaudeDir(dir)

	sessions, err := detector.detectIDELock(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("detectIDELock: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for dead process, got %d", len(sessions))
	}
}

func TestDetectMtime(t *testing.T) {
	// Create a mock store that returns sessions with recent mtime
	registry := NewRegistry()
	registry.Register(&mockActiveStore{
		source: SourceClaude,
		projects: []Project{
			{ID: "proj1", Path: "/test/proj1"},
		},
		sessions: map[string][]SessionMeta{
			"proj1": {
				{
					ID:         "recent-session",
					FullPath:   "/test/proj1/recent.jsonl",
					ModifiedAt: time.Now().Add(-1 * time.Minute),
					Source:     SourceClaude,
				},
				{
					ID:         "old-session",
					FullPath:   "/test/proj1/old.jsonl",
					ModifiedAt: time.Now().Add(-1 * time.Hour),
					Source:     SourceClaude,
				},
			},
		},
	})

	detector := NewActiveSessionDetector(registry)
	detector.SetActiveWindow(5 * time.Minute)

	sessions, err := detector.detectMtime(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("detectMtime: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 active session, got %d", len(sessions))
	}

	if sessions[0].SessionID != "recent-session" {
		t.Errorf("SessionID = %q, want %q", sessions[0].SessionID, "recent-session")
	}
	if sessions[0].Method != "mtime" {
		t.Errorf("Method = %q, want %q", sessions[0].Method, "mtime")
	}
}

func TestDetect_Dedup(t *testing.T) {
	// Set up a fake Claude dir with IDE lock that maps to a recent session
	dir := t.TempDir()
	ideDir := filepath.Join(dir, "ide")
	os.MkdirAll(ideDir, 0755)

	lockEntry := ideLockEntry{
		PID:              os.Getpid(),
		WorkspaceFolders: []string{"/test/proj1"},
		IDEName:          "VS Code",
	}
	data, _ := json.Marshal(lockEntry)
	os.WriteFile(filepath.Join(ideDir, fmt.Sprintf("%d.lock", os.Getpid())), data, 0644)

	projectDir := filepath.Join(dir, "projects", "-test-proj1")
	os.MkdirAll(projectDir, 0755)
	sessionFile := filepath.Join(projectDir, "session-1.jsonl")
	os.WriteFile(sessionFile, []byte(`{"type":"user"}`), 0644)

	// Also register a store that returns this session as recent by mtime
	registry := NewRegistry()
	registry.Register(&mockActiveStore{
		source: SourceClaude,
		projects: []Project{
			{ID: "proj1", Path: "/test/proj1"},
		},
		sessions: map[string][]SessionMeta{
			"proj1": {
				{
					ID:         "session-1",
					FullPath:   sessionFile,
					ModifiedAt: time.Now(),
					Source:     SourceClaude,
				},
			},
		},
	})

	detector := NewActiveSessionDetector(registry)
	detector.SetClaudeDir(dir)

	sessions, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	// Should be deduplicated - IDE lock takes precedence
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session (deduplicated), got %d", len(sessions))
	}
	if sessions[0].Method != "ide_lock" {
		t.Errorf("Method = %q, want %q (IDE lock should take precedence)", sessions[0].Method, "ide_lock")
	}
}

func TestFormatActiveSession(t *testing.T) {
	s := ActiveSession{
		Source:      SourceClaude,
		ProjectPath: "/Users/evan/my-project",
		SessionID:   "abc-123-def-456",
		Method:      "ide_lock",
		IDE:         "Visual Studio Code",
	}
	got := FormatActiveSession(s)
	if got == "" {
		t.Error("FormatActiveSession returned empty string")
	}
	// Should contain key elements
	for _, want := range []string{"claude", "Visual Studio Code", "abc-123-", "ide_lock"} {
		if !contains(got, want) {
			t.Errorf("FormatActiveSession missing %q in %q", want, got)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// mockActiveStore is a minimal Store implementation for testing.
type mockActiveStore struct {
	source   Source
	projects []Project
	sessions map[string][]SessionMeta
}

func (m *mockActiveStore) Source() Source         { return m.source }
func (m *mockActiveStore) Workspace() Workspace   { return Workspace{ID: "test", Source: m.source} }

func (m *mockActiveStore) ListProjects(_ context.Context) ([]Project, error) {
	return m.projects, nil
}

func (m *mockActiveStore) GetProject(_ context.Context, id string) (*Project, error) {
	for _, p := range m.projects {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, nil
}

func (m *mockActiveStore) ListSessions(_ context.Context, projectID string) ([]SessionMeta, error) {
	return m.sessions[projectID], nil
}

func (m *mockActiveStore) GetSessionMeta(_ context.Context, _ string) (*SessionMeta, error) {
	return nil, nil
}

func (m *mockActiveStore) LoadSession(_ context.Context, _ string) (*Session, error) {
	return nil, nil
}

func (m *mockActiveStore) OpenSession(_ context.Context, _ string) (SessionReader, error) {
	return nil, nil
}
