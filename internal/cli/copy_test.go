package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectCopier_Copy_Success(t *testing.T) {
	// Create a temp directory with a project
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projDir := filepath.Join(projectsDir, "-test-myproject")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create session files
	sessions := []string{"session1.jsonl", "session2.jsonl"}
	for _, name := range sessions {
		content := []byte(`{"type":"user","message":"hello"}`)
		if err := os.WriteFile(filepath.Join(projDir, name), content, 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create an index file
	indexContent := []byte(`{"version":1}`)
	if err := os.WriteFile(filepath.Join(projDir, "sessions-index.json"), indexContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Create target directory
	targetDir := filepath.Join(tmpDir, "backup")

	var stdout bytes.Buffer
	copier := NewProjectCopier(tmpDir, CopyOptions{Stdout: &stdout})

	err := copier.Copy("/test/myproject", targetDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify files were copied
	for _, name := range sessions {
		dstPath := filepath.Join(targetDir, name)
		if _, err := os.Stat(dstPath); os.IsNotExist(err) {
			t.Errorf("expected %s to be copied", name)
		}
	}

	// Verify index was copied
	indexPath := filepath.Join(targetDir, "sessions-index.json")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Error("expected sessions-index.json to be copied")
	}

	// Verify output message
	if !strings.Contains(stdout.String(), "Copied 3 files") {
		t.Errorf("expected 'Copied 3 files' in output, got: %s", stdout.String())
	}
}

func TestProjectCopier_Copy_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	copier := NewProjectCopier(tmpDir, CopyOptions{Stdout: &stdout})

	err := copier.Copy("/nonexistent/path", filepath.Join(tmpDir, "backup"))
	if err == nil {
		t.Error("expected error for nonexistent project")
	}
	if !strings.Contains(err.Error(), "project not found") {
		t.Errorf("expected 'project not found' error, got: %v", err)
	}
}

func TestProjectCopier_Copy_NoSessions(t *testing.T) {
	// Create a temp directory with a project but no sessions
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projDir := filepath.Join(projectsDir, "-test-emptyproject")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a non-session file
	if err := os.WriteFile(filepath.Join(projDir, "readme.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	copier := NewProjectCopier(tmpDir, CopyOptions{Stdout: &stdout})

	err := copier.Copy("/test/emptyproject", filepath.Join(tmpDir, "backup"))
	if err == nil {
		t.Error("expected error for project with no sessions")
	}
	if !strings.Contains(err.Error(), "no sessions found") {
		t.Errorf("expected 'no sessions found' error, got: %v", err)
	}
}

func TestProjectCopier_Copy_CreatesTargetDir(t *testing.T) {
	// Create a temp directory with a project
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projDir := filepath.Join(projectsDir, "-test-myproject")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file
	if err := os.WriteFile(filepath.Join(projDir, "session.jsonl"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Target directory doesn't exist yet
	targetDir := filepath.Join(tmpDir, "nested", "backup", "dir")

	var stdout bytes.Buffer
	copier := NewProjectCopier(tmpDir, CopyOptions{Stdout: &stdout})

	err := copier.Copy("/test/myproject", targetDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify target directory was created
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		t.Error("target directory should be created")
	}

	// Verify file was copied
	if _, err := os.Stat(filepath.Join(targetDir, "session.jsonl")); os.IsNotExist(err) {
		t.Error("session file should be copied")
	}
}

func TestProjectCopier_Copy_PreservesContent(t *testing.T) {
	// Create a temp directory with a project
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projDir := filepath.Join(projectsDir, "-test-myproject")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file with specific content
	originalContent := []byte(`{"type":"user","message":"test content 12345"}`)
	if err := os.WriteFile(filepath.Join(projDir, "session.jsonl"), originalContent, 0644); err != nil {
		t.Fatal(err)
	}

	targetDir := filepath.Join(tmpDir, "backup")

	var stdout bytes.Buffer
	copier := NewProjectCopier(tmpDir, CopyOptions{Stdout: &stdout})

	err := copier.Copy("/test/myproject", targetDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify content matches
	copiedContent, err := os.ReadFile(filepath.Join(targetDir, "session.jsonl"))
	if err != nil {
		t.Fatalf("failed to read copied file: %v", err)
	}

	if string(copiedContent) != string(originalContent) {
		t.Errorf("content mismatch: expected %q, got %q", originalContent, copiedContent)
	}
}

func TestProjectCopier_Copy_SkipsDirectories(t *testing.T) {
	// Create a temp directory with a project
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projDir := filepath.Join(projectsDir, "-test-myproject")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file
	if err := os.WriteFile(filepath.Join(projDir, "session.jsonl"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a subdirectory (should be skipped)
	subDir := filepath.Join(projDir, "subagents")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "agent.jsonl"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	targetDir := filepath.Join(tmpDir, "backup")

	var stdout bytes.Buffer
	copier := NewProjectCopier(tmpDir, CopyOptions{Stdout: &stdout})

	err := copier.Copy("/test/myproject", targetDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Subdirectory should NOT be copied
	if _, err := os.Stat(filepath.Join(targetDir, "subagents")); !os.IsNotExist(err) {
		t.Error("subdirectories should not be copied")
	}

	// Only the top-level session should be copied
	if !strings.Contains(stdout.String(), "Copied 1 file") {
		t.Errorf("expected 'Copied 1 file' in output, got: %s", stdout.String())
	}
}
