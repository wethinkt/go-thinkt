package copilot

import (
	"path/filepath"
	"testing"
)

func TestIsSessionPath_BoundarySafe(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), ".copilot")
	t.Setenv("THINKT_COPILOT_HOME", baseDir)

	valid := filepath.Join(baseDir, "projects", "p1", "trace.jsonl")
	if !IsSessionPath(valid) {
		t.Fatalf("expected path under base dir to match: %s", valid)
	}

	prefixCollision := filepath.Join(filepath.Dir(baseDir), filepath.Base(baseDir)+"x", "projects", "p1", "trace.jsonl")
	if IsSessionPath(prefixCollision) {
		t.Fatalf("expected prefix-collision path to not match: %s", prefixCollision)
	}
}

func TestIsSessionPath_EventsFileName(t *testing.T) {
	if !IsSessionPath(filepath.Join("any", "events.jsonl")) {
		t.Fatalf("expected events.jsonl to be recognized")
	}
}

