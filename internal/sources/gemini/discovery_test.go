package gemini

import (
	"path/filepath"
	"testing"
)

func TestIsSessionPath_BoundarySafe(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), ".gemini")
	t.Setenv("THINKT_GEMINI_HOME", baseDir)

	valid := filepath.Join(baseDir, "tmp", "session.json")
	if !IsSessionPath(valid) {
		t.Fatalf("expected path under base dir to match: %s", valid)
	}

	prefixCollision := filepath.Join(filepath.Dir(baseDir), filepath.Base(baseDir)+"x", "tmp", "session.json")
	if IsSessionPath(prefixCollision) {
		t.Fatalf("expected prefix-collision path to not match: %s", prefixCollision)
	}

	wrongExt := filepath.Join(baseDir, "tmp", "session.jsonl")
	if IsSessionPath(wrongExt) {
		t.Fatalf("expected non-json path to not match: %s", wrongExt)
	}
}

