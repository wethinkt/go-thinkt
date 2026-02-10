package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestProjectCopier_Copy_Success(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "workspace", "myproject")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tmpDir, "sessions")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	sessionA := filepath.Join(srcDir, "session1.jsonl")
	sessionB := filepath.Join(srcDir, "session2.json")
	if err := os.WriteFile(sessionA, []byte(`{"type":"user","text":"hello"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionB, []byte(`{"type":"user","text":"world"}`), 0644); err != nil {
		t.Fatal(err)
	}

	registry := makeSingleProjectRegistry(thinkt.SourceClaude, "project-1", projectPath, []string{sessionA, sessionB})
	targetDir := filepath.Join(tmpDir, "backup")

	var stdout bytes.Buffer
	copier := NewProjectCopier(registry, CopyOptions{Stdout: &stdout})
	if err := copier.Copy(projectPath, targetDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, name := range []string{"session1.jsonl", "session2.json"} {
		if _, err := os.Stat(filepath.Join(targetDir, name)); os.IsNotExist(err) {
			t.Fatalf("expected %s to be copied", name)
		}
	}

	if !strings.Contains(stdout.String(), "Copied 2 files") {
		t.Fatalf("expected copied files message, got: %s", stdout.String())
	}
}

func TestProjectCopier_Copy_RelativePathQuery(t *testing.T) {
	tmpDir := t.TempDir()
	workingDir := filepath.Join(tmpDir, "work")
	projectPath := filepath.Join(workingDir, "repo")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatal(err)
	}

	session := filepath.Join(tmpDir, "session.jsonl")
	if err := os.WriteFile(session, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	registry := makeSingleProjectRegistry(thinkt.SourceKimi, "kimi-repo", projectPath, []string{session})
	targetDir := filepath.Join(tmpDir, "backup")

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	if err := os.Chdir(workingDir); err != nil {
		t.Fatal(err)
	}

	copier := NewProjectCopier(registry, CopyOptions{})
	if err := copier.Copy("repo", targetDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "session.jsonl")); os.IsNotExist(err) {
		t.Fatal("expected session file to be copied")
	}
}

func TestProjectCopier_Copy_NotFound(t *testing.T) {
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
	copier := NewProjectCopier(registry, CopyOptions{})

	err := copier.Copy(filepath.Join(tmpDir, "missing"), filepath.Join(tmpDir, "backup"))
	if err == nil {
		t.Fatal("expected error for missing project")
	}
	if !strings.Contains(err.Error(), "project not found") {
		t.Fatalf("expected project not found error, got: %v", err)
	}
}

func TestProjectCopier_Copy_NoSessions(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "workspace", "empty")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatal(err)
	}

	registry := makeSingleProjectRegistry(thinkt.SourceGemini, "gemini-empty", projectPath, nil)
	copier := NewProjectCopier(registry, CopyOptions{})

	err := copier.Copy(projectPath, filepath.Join(tmpDir, "backup"))
	if err == nil {
		t.Fatal("expected error for project with no sessions")
	}
	if !strings.Contains(err.Error(), "no sessions found") {
		t.Fatalf("expected no sessions found error, got: %v", err)
	}
}

func TestProjectCopier_Copy_NameCollision(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "workspace", "collision")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatal(err)
	}

	srcA := filepath.Join(tmpDir, "a", "session.jsonl")
	srcB := filepath.Join(tmpDir, "b", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(srcA), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(srcB), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcA, []byte(`{"session":"a"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcB, []byte(`{"session":"b"}`), 0644); err != nil {
		t.Fatal(err)
	}

	registry := makeSingleProjectRegistry(thinkt.SourceCodex, "codex-collision", projectPath, []string{srcA, srcB})
	targetDir := filepath.Join(tmpDir, "backup")

	copier := NewProjectCopier(registry, CopyOptions{})
	if err := copier.Copy(projectPath, targetDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "session.jsonl")); os.IsNotExist(err) {
		t.Fatal("expected first session filename")
	}
	if _, err := os.Stat(filepath.Join(targetDir, "session_2.jsonl")); os.IsNotExist(err) {
		t.Fatal("expected collision-resolved session filename")
	}
}
