package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectDeleter_Delete_NotFound(t *testing.T) {
	// Create a temp directory as base
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	deleter := NewProjectDeleter(tmpDir, DeleteOptions{
		Force:  true,
		Stdout: &stdout,
	})

	err := deleter.Delete("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent project")
	}
	if !strings.Contains(err.Error(), "project not found") {
		t.Errorf("expected 'project not found' error, got: %v", err)
	}
}

func TestProjectDeleter_Delete_NoSessions(t *testing.T) {
	// Create a temp directory with a project but no sessions
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projDir := filepath.Join(projectsDir, "-test-emptyproject")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a non-session file (should not count)
	nonSessionFile := filepath.Join(projDir, "readme.txt")
	if err := os.WriteFile(nonSessionFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	deleter := NewProjectDeleter(tmpDir, DeleteOptions{
		Force:  true, // Even with force, should refuse
		Stdout: &stdout,
	})

	err := deleter.Delete("/test/emptyproject")
	if err == nil {
		t.Error("expected error for project with no sessions")
	}
	if !strings.Contains(err.Error(), "no sessions found") {
		t.Errorf("expected 'no sessions found' error, got: %v", err)
	}

	// Directory should still exist
	if _, err := os.Stat(projDir); os.IsNotExist(err) {
		t.Error("project directory should NOT be deleted when it has no sessions")
	}
}

func TestProjectDeleter_Delete_Force(t *testing.T) {
	// Create a temp directory with a project
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projDir := filepath.Join(projectsDir, "-test-myproject")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file
	sessionFile := filepath.Join(projDir, "session1.jsonl")
	if err := os.WriteFile(sessionFile, []byte(`{"type":"user"}`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	deleter := NewProjectDeleter(tmpDir, DeleteOptions{
		Force:  true,
		Stdout: &stdout,
	})

	err := deleter.Delete("/test/myproject")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Project should be deleted without prompting
	if _, err := os.Stat(projDir); !os.IsNotExist(err) {
		t.Error("project directory should be deleted")
	}

	output := stdout.String()
	if !strings.Contains(output, "Deleted") {
		t.Error("expected 'Deleted' message in output")
	}
}

func TestProjectDeleter_Delete_ForceMultipleSessions(t *testing.T) {
	// Create a temp directory with a project containing multiple sessions
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projDir := filepath.Join(projectsDir, "-test-myproject")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create multiple session files
	for _, name := range []string{"session1.jsonl", "session2.jsonl", "session3.jsonl"} {
		sessionFile := filepath.Join(projDir, name)
		if err := os.WriteFile(sessionFile, []byte(`{"type":"user"}`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var stdout bytes.Buffer
	deleter := NewProjectDeleter(tmpDir, DeleteOptions{
		Force:  true,
		Stdout: &stdout,
	})

	err := deleter.Delete("/test/myproject")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Project should be deleted
	if _, err := os.Stat(projDir); !os.IsNotExist(err) {
		t.Error("project directory should be deleted")
	}

	output := stdout.String()
	if !strings.Contains(output, "3 sessions") {
		t.Errorf("expected '3 sessions' in output, got: %s", output)
	}
}

func TestEncodePathToDirName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/Users/evan/myproject", "-Users-evan-myproject"},
		{"/test/path", "-test-path"},
		{"", "-"},
		{"relative/path", "-relative-path"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := encodePathToDirName(tc.input)
			if result != tc.expected {
				t.Errorf("encodePathToDirName(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// Note: Interactive confirmation tests are not included because huh.Confirm
// uses a TUI that requires an actual terminal. The interactive flow is
// tested manually. The --force flag bypasses the confirmation and is
// thoroughly tested above.
