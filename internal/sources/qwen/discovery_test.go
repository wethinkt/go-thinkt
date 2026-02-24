package qwen

import (
	"path/filepath"
	"testing"
)

func TestIsSessionPath_BoundarySafe(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), ".qwen")
	t.Setenv("THINKT_QWEN_HOME", baseDir)

	valid := filepath.Join(baseDir, "projects", "p1", "trace.jsonl")
	if !IsSessionPath(valid) {
		t.Fatalf("expected path under base dir to match: %s", valid)
	}

	prefixCollision := filepath.Join(filepath.Dir(baseDir), filepath.Base(baseDir)+"x", "projects", "p1", "trace.jsonl")
	if IsSessionPath(prefixCollision) {
		t.Fatalf("expected prefix-collision path to not match: %s", prefixCollision)
	}
}

func TestIsSessionPath_FallbackQwenProjectsPath(t *testing.T) {
	path := filepath.Join(string(filepath.Separator), "tmp", ".qwen", "projects", "p1", "trace.jsonl")
	if !IsSessionPath(path) {
		t.Fatalf("expected fallback .qwen/projects path to match: %s", path)
	}
}

