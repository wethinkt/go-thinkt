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

func TestIsSessionPathSkipsAntigravityHome(t *testing.T) {
	base := filepath.Join(t.TempDir(), ".gemini")
	t.Setenv("THINKT_GEMINI_HOME", base)
	t.Setenv("THINKT_ANTIGRAVITY_HOME", "")
	if IsSessionPath(filepath.Join(base, "antigravity-cli", "settings.json")) {
		t.Fatal("claimed default antigravity home")
	}
	if !IsSessionPath(filepath.Join(base, "tmp", "s.json")) {
		t.Fatal("lost a real gemini path")
	}
	t.Setenv("THINKT_ANTIGRAVITY_HOME", filepath.Join(base, "agy"))
	if IsSessionPath(filepath.Join(base, "agy", "brain", "c1", "task.md.metadata.json")) {
		t.Fatal("claimed THINKT_ANTIGRAVITY_HOME")
	}
}
