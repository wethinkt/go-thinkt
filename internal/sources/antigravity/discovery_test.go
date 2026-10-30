package antigravity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsSessionPath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "antigravity-cli")
	t.Setenv("THINKT_ANTIGRAVITY_HOME", home)
	logs := filepath.Join(home, "brain", "c1", ".system_generated", "logs")
	for path, want := range map[string]bool{
		filepath.Join(logs, "transcript_full.jsonl"):                                            true,
		filepath.Join(logs, "transcript.jsonl"):                                                 true,
		filepath.Join(logs, "chunks", "transcript_full", "00000001.jsonl"):                      false,
		filepath.Join(home, "brain", "c1", "task.md.metadata.json"):                             false,
		filepath.Join(home+"x", "brain", "c1", ".system_generated", "logs", "transcript.jsonl"): false,
	} {
		if IsSessionPath(path) != want {
			t.Errorf("%s: want %v", path, want)
		}
	}
	if sessionIDForPath(filepath.Join(logs, "transcript.jsonl")) != "c1" ||
		baseDirForPath(filepath.Join(logs, "transcript.jsonl")) != home {
		t.Fatal("id or base")
	}
	if sessionIDForPath(filepath.Join(home, "brain", "c1", "task.md")) != "" || baseDirForPath("/x/y.jsonl") != "" {
		t.Fatal("non-session path resolved")
	}
}

func TestSessionFilePrefersFull(t *testing.T) {
	home := t.TempDir()
	logs := func(id string) string { return filepath.Join(home, "brain", id, ".system_generated", "logs") }
	write := func(id, name string) {
		if err := os.MkdirAll(logs(id), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(logs(id), name), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("both", "transcript.jsonl")
	write("both", "transcript_full.jsonl")
	write("plain", "transcript.jsonl")
	if got := sessionFile(home, "both"); got != filepath.Join(logs("both"), "transcript_full.jsonl") {
		t.Fatalf("both: %q", got)
	}
	if got := sessionFile(home, "plain"); got != filepath.Join(logs("plain"), "transcript.jsonl") {
		t.Fatalf("plain: %q", got)
	}
	if got := sessionFile(home, "none"); got != "" {
		t.Fatalf("none: %q", got)
	}
}

func TestDefaultDir(t *testing.T) {
	t.Setenv("THINKT_ANTIGRAVITY_HOME", "/x/agy")
	if d, err := DefaultDir(); err != nil || d != "/x/agy" {
		t.Fatalf("%q %v", d, err)
	}
	t.Setenv("THINKT_ANTIGRAVITY_HOME", "")
	userHome, _ := os.UserHomeDir()
	if d, _ := DefaultDir(); d != filepath.Join(userHome, ".gemini", "antigravity-cli") {
		t.Fatalf("%q", d)
	}
}
