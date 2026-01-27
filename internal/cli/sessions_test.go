package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

func TestSessionsFormatter_FormatList(t *testing.T) {
	sessions := []sessionMetaForTest{
		{FullPath: "/path/to/session1.jsonl", SessionID: "abc123"},
		{FullPath: "/path/to/session2.jsonl", SessionID: "def456"},
	}

	var buf bytes.Buffer
	formatter := NewSessionsFormatter(&buf)
	err := formatter.FormatList(toClaudeSessionMeta(sessions))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "/path/to/session1.jsonl") {
		t.Error("expected session1 path in output")
	}
	if !strings.Contains(output, "/path/to/session2.jsonl") {
		t.Error("expected session2 path in output")
	}
}

func TestSessionsFormatter_FormatSummary(t *testing.T) {
	now := time.Now()
	sessions := []sessionMetaForTest{
		{
			FullPath:     "/path/to/session1.jsonl",
			SessionID:    "abc123",
			MessageCount: 10,
			Created:      now.Add(-time.Hour),
			Modified:     now,
			Summary:      "Test summary",
		},
	}

	var buf bytes.Buffer
	formatter := NewSessionsFormatter(&buf)
	err := formatter.FormatSummary(toClaudeSessionMeta(sessions), "", SessionListOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "abc123") {
		t.Error("expected session ID in output")
	}
	if !strings.Contains(output, "Messages: 10") {
		t.Error("expected message count in output")
	}
	if !strings.Contains(output, "Test summary") {
		t.Error("expected summary in output")
	}
}

func TestSessionsFormatter_FormatSummary_SortByTime(t *testing.T) {
	now := time.Now()
	sessions := []sessionMetaForTest{
		{SessionID: "older", Modified: now.Add(-time.Hour)},
		{SessionID: "newer", Modified: now},
	}

	// Sort ascending (oldest first)
	var buf bytes.Buffer
	formatter := NewSessionsFormatter(&buf)
	err := formatter.FormatSummary(toClaudeSessionMeta(sessions), "", SessionListOptions{
		SortBy:     "time",
		Descending: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	olderIdx := strings.Index(output, "older")
	newerIdx := strings.Index(output, "newer")
	if olderIdx > newerIdx {
		t.Error("expected older session first when sorting ascending by time")
	}
}

func TestSessionsFormatter_FormatSummary_SortByName(t *testing.T) {
	sessions := []sessionMetaForTest{
		{SessionID: "zebra"},
		{SessionID: "alpha"},
	}

	// Sort ascending (A-Z)
	var buf bytes.Buffer
	formatter := NewSessionsFormatter(&buf)
	err := formatter.FormatSummary(toClaudeSessionMeta(sessions), "", SessionListOptions{
		SortBy:     "name",
		Descending: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	alphaIdx := strings.Index(output, "alpha")
	zebraIdx := strings.Index(output, "zebra")
	if alphaIdx > zebraIdx {
		t.Error("expected alpha before zebra when sorting ascending by name")
	}
}

func TestSessionDeleter_Delete_Force(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projDir := filepath.Join(projectsDir, "-test-myproject")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file
	sessionFile := filepath.Join(projDir, "session123.jsonl")
	if err := os.WriteFile(sessionFile, []byte(`{"type":"user"}`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	deleter := NewSessionDeleter(tmpDir, SessionDeleteOptions{
		Force:   true,
		Stdout:  &stdout,
		Project: "/test/myproject",
	})

	err := deleter.Delete(sessionFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Session file should be deleted
	if _, err := os.Stat(sessionFile); !os.IsNotExist(err) {
		t.Error("session file should be deleted")
	}

	if !strings.Contains(stdout.String(), "Deleted") {
		t.Error("expected 'Deleted' in output")
	}
}

func TestSessionDeleter_Delete_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	deleter := NewSessionDeleter(tmpDir, SessionDeleteOptions{
		Force:  true,
		Stdout: &stdout,
	})

	err := deleter.Delete("/nonexistent/session.jsonl")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestSessionCopier_Copy_Success(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projDir := filepath.Join(projectsDir, "-test-myproject")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file
	sessionFile := filepath.Join(projDir, "session123.jsonl")
	content := []byte(`{"type":"user","message":"test content"}`)
	if err := os.WriteFile(sessionFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	targetDir := filepath.Join(tmpDir, "backup")

	var stdout bytes.Buffer
	copier := NewSessionCopier(tmpDir, SessionCopyOptions{
		Stdout:  &stdout,
		Project: "/test/myproject",
	})

	err := copier.Copy(sessionFile, targetDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was copied
	copiedFile := filepath.Join(targetDir, "session123.jsonl")
	copiedContent, err := os.ReadFile(copiedFile)
	if err != nil {
		t.Fatalf("failed to read copied file: %v", err)
	}

	if string(copiedContent) != string(content) {
		t.Error("content mismatch")
	}

	if !strings.Contains(stdout.String(), "Copied") {
		t.Error("expected 'Copied' in output")
	}
}

func TestSessionCopier_Copy_ToSpecificFile(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projDir := filepath.Join(projectsDir, "-test-myproject")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file
	sessionFile := filepath.Join(projDir, "session123.jsonl")
	content := []byte(`{"type":"user"}`)
	if err := os.WriteFile(sessionFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	targetFile := filepath.Join(tmpDir, "backup", "renamed.jsonl")

	var stdout bytes.Buffer
	copier := NewSessionCopier(tmpDir, SessionCopyOptions{
		Stdout:  &stdout,
		Project: "/test/myproject",
	})

	err := copier.Copy(sessionFile, targetFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was copied with new name
	if _, err := os.Stat(targetFile); os.IsNotExist(err) {
		t.Error("expected file to be copied to specific path")
	}
}

// Helper types and functions for testing
type sessionMetaForTest struct {
	FullPath     string
	SessionID    string
	MessageCount int
	Created      time.Time
	Modified     time.Time
	Summary      string
	GitBranch    string
}

func toClaudeSessionMeta(sessions []sessionMetaForTest) []claude.SessionMeta {
	result := make([]claude.SessionMeta, len(sessions))
	for i, s := range sessions {
		result[i] = claude.SessionMeta{
			FullPath:     s.FullPath,
			SessionID:    s.SessionID,
			MessageCount: s.MessageCount,
			Created:      s.Created,
			Modified:     s.Modified,
			Summary:      s.Summary,
			GitBranch:    s.GitBranch,
		}
	}
	return result
}
