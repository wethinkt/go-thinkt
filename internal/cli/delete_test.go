package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestProjectDeleter_Delete_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "workspace", "myproject")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatal(err)
	}
	session := filepath.Join(tmpDir, "session.jsonl")
	if err := os.WriteFile(session, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	registry := makeSingleProjectRegistry(thinkt.SourceClaude, "project-1", projectPath, []string{session})
	deleter := NewProjectDeleter(registry, DeleteOptions{Force: true})

	err := deleter.Delete(filepath.Join(tmpDir, "missing"))
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
	if !strings.Contains(err.Error(), "project not found") {
		t.Fatalf("expected project not found error, got: %v", err)
	}
}

func TestProjectDeleter_Delete_NoSessions(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "workspace", "empty")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatal(err)
	}

	registry := makeSingleProjectRegistry(thinkt.SourceGemini, "project-empty", projectPath, nil)
	deleter := NewProjectDeleter(registry, DeleteOptions{Force: true})

	err := deleter.Delete(projectPath)
	if err == nil {
		t.Fatal("expected error for project with no sessions")
	}
	if !strings.Contains(err.Error(), "no sessions found") {
		t.Fatalf("expected no sessions found error, got: %v", err)
	}
}

func TestProjectDeleter_Delete_Force(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "workspace", "myproject")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatal(err)
	}

	sessionFile := filepath.Join(tmpDir, "session1.jsonl")
	if err := os.WriteFile(sessionFile, []byte(`{"type":"user"}`), 0644); err != nil {
		t.Fatal(err)
	}

	registry := makeSingleProjectRegistry(thinkt.SourceCopilot, "project-1", projectPath, []string{sessionFile})

	var stdout bytes.Buffer
	deleter := NewProjectDeleter(registry, DeleteOptions{
		Force:  true,
		Stdout: &stdout,
	})

	if err := deleter.Delete(projectPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(sessionFile); !os.IsNotExist(err) {
		t.Fatal("session file should be deleted")
	}

	if !strings.Contains(stdout.String(), "Deleted") {
		t.Fatalf("expected Deleted output, got: %s", stdout.String())
	}
}

func TestProjectDeleter_Delete_ForceMultipleSessions(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "workspace", "myproject")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatal(err)
	}

	sessionPaths := []string{
		filepath.Join(tmpDir, "session1.jsonl"),
		filepath.Join(tmpDir, "session2.jsonl"),
		filepath.Join(tmpDir, "session3.jsonl"),
	}
	for _, p := range sessionPaths {
		if err := os.WriteFile(p, []byte(`{"type":"user"}`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	registry := makeSingleProjectRegistry(thinkt.SourceCodex, "project-1", projectPath, sessionPaths)

	var stdout bytes.Buffer
	deleter := NewProjectDeleter(registry, DeleteOptions{
		Force:  true,
		Stdout: &stdout,
	})

	if err := deleter.Delete(projectPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, p := range sessionPaths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("session file should be deleted: %s", p)
		}
	}

	if !strings.Contains(stdout.String(), "3 sessions") {
		t.Fatalf("expected session count in output, got: %s", stdout.String())
	}
}
