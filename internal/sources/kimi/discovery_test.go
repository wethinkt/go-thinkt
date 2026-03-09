package kimi

import (
	"path/filepath"
	"testing"
)

func TestIsSessionPath_BoundarySafe(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), ".kimi")
	t.Setenv("THINKT_KIMI_HOME", baseDir)

	valid := filepath.Join(baseDir, "projects", "p1", "trace.jsonl")
	if !IsSessionPath(valid) {
		t.Fatalf("expected path under base dir to match: %s", valid)
	}

	prefixCollision := filepath.Join(filepath.Dir(baseDir), filepath.Base(baseDir)+"x", "projects", "p1", "trace.jsonl")
	if IsSessionPath(prefixCollision) {
		t.Fatalf("expected prefix-collision path to not match: %s", prefixCollision)
	}
}

func TestIsSessionPath_KnownContextFileNames(t *testing.T) {
	if !IsSessionPath(filepath.Join("any", "context.jsonl")) {
		t.Fatalf("expected context.jsonl to be recognized")
	}
	if !IsSessionPath(filepath.Join("any", "context_sub_1.jsonl")) {
		t.Fatalf("expected context_sub_*.jsonl to be recognized")
	}
}
