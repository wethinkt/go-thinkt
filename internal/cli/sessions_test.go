package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
)

func TestSessionsFormatter_FormatList(t *testing.T) {
	sessions := []thinkt.SessionMeta{
		{FullPath: "/path/to/session1.jsonl", ID: "abc123"},
		{FullPath: "/path/to/session2.jsonl", ID: "def456"},
	}

	var buf bytes.Buffer
	formatter := NewSessionsFormatter(&buf)
	err := formatter.FormatList(sessions)
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
	sessions := []thinkt.SessionMeta{
		{
			FullPath:    "/path/to/session1.jsonl",
			ID:          "abc123",
			EntryCount:  10,
			CreatedAt:   now.Add(-time.Hour),
			ModifiedAt:  now,
			FirstPrompt: "Test first prompt",
		},
	}

	var buf bytes.Buffer
	formatter := NewSessionsFormatter(&buf)
	err := formatter.FormatSummary(sessions, "", SessionListOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "abc123") {
		t.Error("expected session ID in output")
	}
	// The template uses "Messages" not "Entries"
	if !strings.Contains(output, "Messages: 10") {
		t.Error("expected message count in output")
	}
	// The template uses FirstPrompt as Summary
	if !strings.Contains(output, "Test first prompt") {
		t.Error("expected first prompt in output")
	}
}

func TestSessionsFormatter_FormatSummary_SortByTime(t *testing.T) {
	now := time.Now()
	sessions := []thinkt.SessionMeta{
		{ID: "older", ModifiedAt: now.Add(-time.Hour)},
		{ID: "newer", ModifiedAt: now},
	}

	// Sort ascending (oldest first)
	var buf bytes.Buffer
	formatter := NewSessionsFormatter(&buf)
	err := formatter.FormatSummary(sessions, "", SessionListOptions{
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
	sessions := []thinkt.SessionMeta{
		{ID: "zebra"},
		{ID: "alpha"},
	}

	// Sort ascending (A-Z)
	var buf bytes.Buffer
	formatter := NewSessionsFormatter(&buf)
	err := formatter.FormatSummary(sessions, "", SessionListOptions{
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

	// Create a mock registry
	registry := thinkt.NewRegistry()

	var stdout bytes.Buffer
	deleter := NewSessionDeleter(registry, SessionDeleteOptions{
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
	// Create a mock registry
	registry := thinkt.NewRegistry()

	var stdout bytes.Buffer
	deleter := NewSessionDeleter(registry, SessionDeleteOptions{
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

	// Create a mock registry
	registry := thinkt.NewRegistry()

	var stdout bytes.Buffer
	copier := NewSessionCopier(registry, SessionCopyOptions{
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

	// Create a mock registry
	registry := thinkt.NewRegistry()

	var stdout bytes.Buffer
	copier := NewSessionCopier(registry, SessionCopyOptions{
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
