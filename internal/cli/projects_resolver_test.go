package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestResolveProject_ByID(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "workspace", "myproject")
	registry := makeSingleProjectRegistry(thinkt.SourceClaude, "project-id", projectPath, nil)

	project, err := ResolveProject(registry, "project-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if project.Path != projectPath {
		t.Fatalf("expected path %s, got %s", projectPath, project.Path)
	}
}

func TestResolveProject_ByRelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	workingDir := filepath.Join(tmpDir, "workspace")
	projectPath := filepath.Join(workingDir, "myproject")
	if err := os.MkdirAll(workingDir, 0755); err != nil {
		t.Fatal(err)
	}
	registry := makeSingleProjectRegistry(thinkt.SourceKimi, "kimi-id", projectPath, nil)

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	if err := os.Chdir(workingDir); err != nil {
		t.Fatal(err)
	}

	project, err := ResolveProject(registry, "myproject")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if project.Path != projectPath {
		t.Fatalf("expected path %s, got %s", projectPath, project.Path)
	}
}

func TestResolveProject_AmbiguousSuffix(t *testing.T) {
	tmpDir := t.TempDir()

	store := &testProjectStore{
		source: thinkt.SourceCodex,
		projects: []thinkt.Project{
			{ID: "p1", Path: filepath.Join(tmpDir, "a", "repo"), Source: thinkt.SourceCodex},
			{ID: "p2", Path: filepath.Join(tmpDir, "b", "repo"), Source: thinkt.SourceCodex},
		},
		sessionsByProject: map[string][]thinkt.SessionMeta{},
	}

	registry := thinkt.NewRegistry()
	registry.Register(store)

	_, err := ResolveProject(registry, "repo")
	if err == nil {
		t.Fatal("expected ambiguous error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous error, got: %v", err)
	}
}
