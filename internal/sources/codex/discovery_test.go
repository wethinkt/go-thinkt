package codex

import (
	"path/filepath"
	"testing"
)

func TestIsSessionPath_BoundarySafe(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), ".codex")
	t.Setenv("THINKT_CODEX_HOME", baseDir)

	valid := filepath.Join(baseDir, "sessions", "2026", "session.jsonl")
	if !IsSessionPath(valid) {
		t.Fatalf("expected path under base dir to match: %s", valid)
	}

	prefixCollision := filepath.Join(filepath.Dir(baseDir), filepath.Base(baseDir)+"x", "sessions", "2026", "session.jsonl")
	if IsSessionPath(prefixCollision) {
		t.Fatalf("expected prefix-collision path to not match: %s", prefixCollision)
	}
}

func TestIsSessionPath_FallbackCodexSessionsPath(t *testing.T) {
	path := filepath.Join(string(filepath.Separator), "tmp", ".codex", "sessions", "2026", "session.jsonl")
	if !IsSessionPath(path) {
		t.Fatalf("expected fallback .codex/sessions path to match: %s", path)
	}
}

