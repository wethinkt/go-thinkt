package relay

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSourceUsesResolver(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gemini := filepath.Join(root, ".gemini")
	agy := filepath.Join(gemini, "antigravity-cli")
	t.Setenv("THINKT_GEMINI_HOME", gemini)
	t.Setenv("THINKT_ANTIGRAVITY_HOME", agy)
	t.Setenv("THINKT_QWEN_HOME", filepath.Join(root, "qwen-home"))

	write := func(path string) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	transcript := write(filepath.Join(agy, "brain", "c1", ".system_generated", "logs", "transcript.jsonl"))
	qwenFile := write(filepath.Join(root, "elsewhere", ".qwen", "projects", "h", "chats", "s.jsonl"))

	if got := detectSource(transcript); got != "antigravity" {
		t.Errorf("antigravity transcript: %q", got)
	}
	if got := detectSource(qwenFile); got != "qwen" {
		t.Errorf("qwen layout: %q", got)
	}
	// A brand-new empty Claude file is not claimed by any resolver yet; the
	// path fallback still names it.
	if got := detectSource(write(filepath.Join(root, ".claude", "projects", "x", "new.jsonl"))); got != "claude" {
		t.Errorf("empty claude file: %q", got)
	}
}
