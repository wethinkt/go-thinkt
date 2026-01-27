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

func TestProjectDeleter_Delete_Cancelled(t *testing.T) {
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
	stdin := strings.NewReader("n\n") // User says no

	deleter := NewProjectDeleter(tmpDir, DeleteOptions{
		Force:  false,
		Stdin:  stdin,
		Stdout: &stdout,
	})

	err := deleter.Delete("/test/myproject")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Project should still exist
	if _, err := os.Stat(projDir); os.IsNotExist(err) {
		t.Error("project directory should still exist after cancellation")
	}

	if !strings.Contains(stdout.String(), "Cancelled") {
		t.Error("expected 'Cancelled' message in output")
	}
}

func TestProjectDeleter_Delete_Confirmed(t *testing.T) {
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
	stdin := strings.NewReader("y\n") // User says yes

	deleter := NewProjectDeleter(tmpDir, DeleteOptions{
		Force:  false,
		Stdin:  stdin,
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

	if !strings.Contains(stdout.String(), "Deleted") {
		t.Error("expected 'Deleted' message in output")
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
	if strings.Contains(output, "Are you sure") {
		t.Error("force mode should not prompt for confirmation")
	}
	if !strings.Contains(output, "Deleted") {
		t.Error("expected 'Deleted' message in output")
	}
}

func TestProjectDeleter_Delete_ShowsInfo(t *testing.T) {
	// Create a temp directory with a project
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
	stdin := strings.NewReader("n\n") // Cancel to see the info

	deleter := NewProjectDeleter(tmpDir, DeleteOptions{
		Force:  false,
		Stdin:  stdin,
		Stdout: &stdout,
	})

	_ = deleter.Delete("/test/myproject")

	output := stdout.String()

	// Should show project info
	if !strings.Contains(output, "/test/myproject") {
		t.Error("expected project path in output")
	}
	if !strings.Contains(output, "Sessions: 3") {
		t.Errorf("expected 'Sessions: 3' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Are you sure") {
		t.Error("expected confirmation prompt in output")
	}
}

func TestProjectDeleter_Delete_YesVariants(t *testing.T) {
	variants := []string{"y\n", "Y\n", "yes\n", "YES\n", "Yes\n"}

	for _, input := range variants {
		t.Run(input, func(t *testing.T) {
			// Create a temp directory with a project
			tmpDir := t.TempDir()
			projectsDir := filepath.Join(tmpDir, "projects")
			projDir := filepath.Join(projectsDir, "-test-myproject")
			if err := os.MkdirAll(projDir, 0755); err != nil {
				t.Fatal(err)
			}

			sessionFile := filepath.Join(projDir, "session1.jsonl")
			if err := os.WriteFile(sessionFile, []byte(`{"type":"user"}`), 0644); err != nil {
				t.Fatal(err)
			}

			var stdout bytes.Buffer
			stdin := strings.NewReader(input)

			deleter := NewProjectDeleter(tmpDir, DeleteOptions{
				Stdin:  stdin,
				Stdout: &stdout,
			})

			err := deleter.Delete("/test/myproject")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Project should be deleted
			if _, err := os.Stat(projDir); !os.IsNotExist(err) {
				t.Errorf("project directory should be deleted for input %q", input)
			}
		})
	}
}

func TestProjectDeleter_Delete_NoVariants(t *testing.T) {
	variants := []string{"n\n", "N\n", "no\n", "NO\n", "\n", "x\n", "anything\n"}

	for _, input := range variants {
		t.Run(input, func(t *testing.T) {
			// Create a temp directory with a project
			tmpDir := t.TempDir()
			projectsDir := filepath.Join(tmpDir, "projects")
			projDir := filepath.Join(projectsDir, "-test-myproject")
			if err := os.MkdirAll(projDir, 0755); err != nil {
				t.Fatal(err)
			}

			sessionFile := filepath.Join(projDir, "session1.jsonl")
			if err := os.WriteFile(sessionFile, []byte(`{"type":"user"}`), 0644); err != nil {
				t.Fatal(err)
			}

			var stdout bytes.Buffer
			stdin := strings.NewReader(input)

			deleter := NewProjectDeleter(tmpDir, DeleteOptions{
				Stdin:  stdin,
				Stdout: &stdout,
			})

			err := deleter.Delete("/test/myproject")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Project should NOT be deleted
			if _, err := os.Stat(projDir); os.IsNotExist(err) {
				t.Errorf("project directory should NOT be deleted for input %q", input)
			}
		})
	}
}
