package kimi

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestIsSessionPath(t *testing.T) {
	cases := map[string]bool{
		"/h/.kimi-code/sessions/wd_x_1/session_y/agents/main/wire.jsonl":    true,
		"/h/.kimi-code/sessions/wd_x_1/session_y/agents/agent-0/wire.jsonl": true,
		"/h/.kimi-code/sessions/wd_x_1/session_y/state.json":                false,
		"/h/.kimi/sessions/abc/uuid/context.jsonl":                          false,
		"/h/elsewhere/agents/main/wire.jsonl":                               false,
		"wire.jsonl":                                                        false,
	}
	for path, want := range cases {
		if got := IsSessionPath(filepath.FromSlash(path)); got != want {
			t.Errorf("IsSessionPath(%s) = %v, want %v", path, got, want)
		}
	}
}

func TestBaseDirForPath(t *testing.T) {
	if got := BaseDirForPath(filepath.FromSlash("/h/.kimi-code/sessions/wd_x_1/session_y/agents/main/wire.jsonl")); got != filepath.FromSlash("/h/.kimi-code") {
		t.Fatalf("base = %q", got)
	}
	if got := BaseDirForPath(filepath.FromSlash("/h/nowhere/wire.jsonl")); got != "" {
		t.Fatalf("base = %q", got)
	}
}

func TestSessionIDForPath(t *testing.T) {
	if got := SessionIDForPath(filepath.FromSlash("/h/.kimi-code/sessions/wd_x_1/session_y/agents/main/wire.jsonl")); got != "session_y" {
		t.Fatalf("main = %q", got)
	}
	if got := SessionIDForPath(filepath.FromSlash("/h/.kimi-code/sessions/wd_x_1/session_y/agents/agent-0/wire.jsonl")); got != "session_y/agent-0" {
		t.Fatalf("agent = %q", got)
	}
	if got := SessionIDForPath(filepath.FromSlash("/h/.kimi-code/sessions/wd_x_1/session_y/state.json")); got != "" {
		t.Fatalf("state = %q", got)
	}
}

func TestDefaultDirUsesKimiCode(t *testing.T) {
	t.Setenv("THINKT_KIMI_HOME", "")
	dir, err := DefaultDir()
	if err != nil || !strings.HasSuffix(dir, ".kimi-code") {
		t.Fatalf("dir = %q err = %v", dir, err)
	}
	t.Setenv("THINKT_KIMI_HOME", "/custom")
	if dir, _ := DefaultDir(); dir != "/custom" {
		t.Fatalf("dir = %q", dir)
	}
}

func TestDiscovererAvailability(t *testing.T) {
	base := t.TempDir()
	t.Setenv("THINKT_KIMI_HOME", base)
	ok, err := NewDiscoverer().IsAvailable()
	if err != nil || ok {
		t.Fatalf("empty base: ok=%v err=%v", ok, err)
	}
	storeFixture(t)
	t.Setenv("THINKT_KIMI_HOME", storeFixture(t))
	ok, err = NewDiscoverer().IsAvailable()
	if err != nil || !ok {
		t.Fatalf("fixture base: ok=%v err=%v", ok, err)
	}
	if Factory().Source() != "kimi" {
		t.Fatal("factory source")
	}
}
