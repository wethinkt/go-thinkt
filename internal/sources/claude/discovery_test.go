package claude

import (
	"path/filepath"
	"testing"
)

func TestIsSessionPath_BoundarySafe(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), ".claude")
	t.Setenv("THINKT_CLAUDE_HOME", baseDir)

	valid := filepath.Join(baseDir, "projects", "p1", "session.jsonl")
	if !IsSessionPath(valid) {
		t.Fatalf("expected path under base dir to match: %s", valid)
	}

	prefixCollision := filepath.Join(filepath.Dir(baseDir), filepath.Base(baseDir)+"x", "projects", "p1", "session.jsonl")
	if IsSessionPath(prefixCollision) {
		t.Fatalf("expected prefix-collision path to not match: %s", prefixCollision)
	}

	wrongExt := filepath.Join(baseDir, "projects", "p1", "session.txt")
	if IsSessionPath(wrongExt) {
		t.Fatalf("expected non-jsonl path to not match: %s", wrongExt)
	}
}

