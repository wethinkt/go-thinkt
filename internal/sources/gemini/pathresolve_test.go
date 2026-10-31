package gemini

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsSessionPathGemini(t *testing.T) {
	base := filepath.Join(t.TempDir(), ".gemini")
	t.Setenv("THINKT_GEMINI_HOME", base)
	t.Setenv("THINKT_ANTIGRAVITY_HOME", "")
	for rel, want := range map[string]bool{
		"tmp/p/chats/a.jsonl":                 true,
		"tmp/p/chats/parent-id/x.jsonl":       true,
		"tmp/p/chats/a.json":                  true,
		"tmp/p/logs.json":                     false,
		"tmp/p/chats/parent-id/x.json":        false,
		"tmp/session.json":                    false,
		"antigravity-cli/brain/c/x.jsonl":     false,
		"tmp/p/chats/parent-id/deeper/y.json": false,
	} {
		if got := IsSessionPath(filepath.Join(base, rel)); got != want {
			t.Errorf("%s: %v, want %v", rel, got, want)
		}
	}
	other := filepath.Join(filepath.Dir(base), ".geminix", "tmp", "p", "chats", "a.jsonl")
	if IsSessionPath(other) {
		t.Error("prefix-collision path matched")
	}
}

func TestResolveGeminiProjectID(t *testing.T) {
	base := t.TempDir()
	t.Setenv("THINKT_GEMINI_HOME", base)
	h := hashOf(devProj)
	path := put(t, base, "tmp/proj/chats/session-a.jsonl", header(h))
	sub := put(t, base, "tmp/proj/chats/parent/92szal.jsonl", header(h))
	for _, p := range []string{path, sub} {
		sp, ok := (&Discoverer{}).ResolveSessionPath(p)
		if !ok || sp.ProjectID != h || sp.SessionID != p {
			t.Errorf("%s: %+v %v", p, sp, ok)
		}
	}
	empty := put(t, base, "tmp/proj/chats/session-b.jsonl", "")
	if err := os.Truncate(empty, 0); err != nil {
		t.Fatal(err)
	}
	if sp, ok := (&Discoverer{}).ResolveSessionPath(empty); !ok || sp.ProjectID != "" {
		t.Errorf("empty file: %+v %v", sp, ok)
	}
}
