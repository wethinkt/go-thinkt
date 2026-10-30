package sources

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/sources/antigravity"
	"github.com/wethinkt/go-thinkt/internal/sources/claude"
	"github.com/wethinkt/go-thinkt/internal/sources/codex"
	"github.com/wethinkt/go-thinkt/internal/sources/copilot"
	"github.com/wethinkt/go-thinkt/internal/sources/gemini"
	"github.com/wethinkt/go-thinkt/internal/sources/kimi"
	"github.com/wethinkt/go-thinkt/internal/sources/qwen"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// oldSwitchSource is the source the TUI's OpenLazySession switch picked
// before resolvers existed; it is the oracle the resolution order must match.
// Its content sniff is the old one, which named every Claude transcript
// (whose records carry sessionId) as Qwen.
func oldSwitchSource(path string) string {
	switch {
	case kimi.IsSessionPath(path):
		return "kimi"
	case antigravity.IsSessionPath(path):
		return "antigravity"
	case codex.IsSessionPath(path):
		return "codex"
	case copilot.IsSessionPath(path):
		return "copilot"
	case qwen.IsSessionPath(path):
		return "qwen"
	case gemini.IsSessionPath(path):
		return "gemini"
	case filepath.Ext(path) == ".jsonl":
		return oldSniff(path)
	}
	return ""
}

// oldSniff is the first-record rule of the old detectJSONLSessionType,
// reduced to the cases these fixtures reach.
func oldSniff(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		var obj map[string]any
		if json.Unmarshal([]byte(line), &obj) != nil {
			continue
		}
		if _, ok := obj["sessionId"]; ok {
			return "qwen"
		}
		if t, _ := obj["type"].(string); t == "session_meta" {
			return "codex"
		}
		if _, ok := obj["message"]; ok {
			return "claude"
		}
	}
	return ""
}

const (
	claudeLine  = `{"type":"last-prompt","lastPrompt":"hi","sessionId":"s1"}` + "\n" + `{"type":"user","uuid":"u1","sessionId":"s1","message":{"role":"user","content":"hi"}}`
	codexLine   = `{"timestamp":"2026-10-01T10:00:00Z","type":"session_meta","payload":{"id":"r1","cwd":"/w"}}`
	qwenLine    = `{"sessionId":"q1","type":"user","message":{"role":"user","parts":[{"text":"hi"}]}}`
	copilotLine = `{"type":"session.start","data":{"sessionId":"c1"}}`
	geminiJSON  = `{"sessionId":"g1","messages":[]}`
)

type homes struct {
	root, claude, codex, qwen, copilot, gemini, agy, kimi string
}

// writeFile creates path with content, making parent directories.
func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// copyFixture copies a source's testdata file to dst.
func copyFixture(t *testing.T, src, dst string) string {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	return writeFile(t, dst, string(b))
}

func setHomes(t *testing.T) homes {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := homes{
		root:    root,
		claude:  filepath.Join(root, "claudehome"),
		codex:   filepath.Join(root, "codexhome"),
		qwen:    filepath.Join(root, "qwenhome"),
		copilot: filepath.Join(root, "copilothome"),
		gemini:  filepath.Join(root, "geminihome"),
		kimi:    filepath.Join(root, "kimihome"),
	}
	h.agy = filepath.Join(h.gemini, "antigravity-cli")
	for _, d := range []string{h.claude, h.codex, h.qwen, h.copilot, h.gemini, h.agy, h.kimi} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("THINKT_CLAUDE_HOME", h.claude)
	t.Setenv("THINKT_CODEX_HOME", h.codex)
	t.Setenv("THINKT_QWEN_HOME", h.qwen)
	t.Setenv("THINKT_COPILOT_HOME", h.copilot)
	t.Setenv("THINKT_GEMINI_HOME", h.gemini)
	t.Setenv("THINKT_ANTIGRAVITY_HOME", h.agy)
	t.Setenv("THINKT_KIMI_HOME", h.kimi)
	return h
}

func TestResolveSessionPathMatchesOldSwitch(t *testing.T) {
	h := setHomes(t)
	// oracle is what the old switch picked when it differs from want: the
	// old sniff read Claude transcripts as Qwen, and the direct open then
	// failed over to the registry scan.
	rows := []struct {
		name, path, want, oracle string
	}{
		{"events.jsonl under claude home", writeFile(t, filepath.Join(h.claude, "projects", "-x", "events.jsonl"), copilotLine), "copilot", ""},
		{"codex rollout in claude projects", writeFile(t, filepath.Join(h.claude, "projects", "-x", "r.jsonl"), codexLine), "codex", ""},
		{"claude transcript", writeFile(t, filepath.Join(h.claude, "projects", "-x", "s.jsonl"), claudeLine), "claude", "qwen"},
		{"jsonl under codex home", writeFile(t, filepath.Join(h.codex, "sessions", "2026", "10", "30", "rollout-a.jsonl"), codexLine), "codex", ""},
		{"jsonl under qwen home", writeFile(t, filepath.Join(h.qwen, "projects", "h", "chats", "s.jsonl"), qwenLine), "qwen", ""},
		{"antigravity transcript inside gemini home", copyFixture(t, "antigravity/testdata/main.jsonl", filepath.Join(h.agy, "brain", "c1", ".system_generated", "logs", "transcript.jsonl")), "antigravity", ""},
		{"kimi wire outside any home", copyFixture(t, "kimi/testdata/wire-1.5.jsonl", filepath.Join(h.root, "elsewhere", "sessions", "wd", "s1", "agents", "main", "wire.jsonl")), "kimi", ""},
		{"codex layout outside home", writeFile(t, filepath.Join(h.root, "elsewhere", ".codex", "sessions", "r.jsonl"), codexLine), "codex", ""},
		{"claude copy outside any home", writeFile(t, filepath.Join(h.root, "elsewhere", "copy.jsonl"), claudeLine), "claude", "qwen"},
		{"gemini chat", writeFile(t, filepath.Join(h.gemini, "tmp", "h", "chats", "session-1.json"), geminiJSON), "gemini", ""},
		{"not a session", writeFile(t, filepath.Join(h.root, "elsewhere", "notes.txt"), "hi"), "", ""},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			oracle := r.oracle
			if oracle == "" {
				oracle = r.want
			}
			if got := oldSwitchSource(r.path); got != oracle {
				t.Fatalf("old switch = %q, want %q", got, oracle)
			}
			sp, res, ok := ResolveSessionPath(r.path)
			if r.want == "" {
				if ok {
					t.Fatalf("resolved %+v, want no match", sp)
				}
				return
			}
			if !ok || res == nil || string(sp.Source) != r.want {
				t.Fatalf("resolved %+v ok=%v, want %s", sp, ok, r.want)
			}
		})
	}
}

func TestResolvedSessionsOpen(t *testing.T) {
	h := setHomes(t)
	paths := []string{
		copyFixture(t, "claude/testdata/modern-session.jsonl", filepath.Join(h.claude, "projects", "-x", "s.jsonl")),
		copyFixture(t, "codex/testdata/rollout-fixture.jsonl", filepath.Join(h.codex, "sessions", "2026", "10", "30", "rollout-a.jsonl")),
		kimiSession(t, h.kimi),
		copyFixture(t, "antigravity/testdata/main.jsonl", filepath.Join(h.agy, "brain", "c1", ".system_generated", "logs", "transcript.jsonl")),
		writeFile(t, filepath.Join(h.qwen, "projects", "h", "chats", "s.jsonl"), qwenLine),
		writeFile(t, filepath.Join(h.copilot, "session-state", "c1", "events.jsonl"), copilotLine),
		writeFile(t, filepath.Join(h.gemini, "tmp", "h", "chats", "session-1.json"), geminiJSON),
	}
	for _, p := range paths {
		ls, err := OpenSessionPath(context.Background(), p)
		if err != nil {
			t.Errorf("OpenSessionPath(%s): %v", p, err)
			continue
		}
		_ = ls.Close()
	}
	if _, err := OpenSessionPath(context.Background(), filepath.Join(h.root, "notes.txt")); err == nil {
		t.Error("expected an error for a path no source claims")
	}
}

func TestProjectIDMatchesStore(t *testing.T) {
	h := setHomes(t)
	cases := []struct {
		factory thinkt.StoreFactory
		path    string
	}{
		{claude.Factory(), copyFixture(t, "claude/testdata/modern-session.jsonl", filepath.Join(h.claude, "projects", "-Users-x-proj", "s.jsonl"))},
		{gemini.Factory(), writeFile(t, filepath.Join(h.gemini, "tmp", "abc123", "chats", "session-1.json"), geminiJSON)},
		{qwen.Factory(), writeFile(t, filepath.Join(h.qwen, "projects", "def456", "chats", "s.jsonl"), qwenLine)},
	}
	for _, c := range cases {
		sp, _, ok := ResolveSessionPath(c.path)
		if !ok || sp.ProjectID == "" {
			t.Fatalf("%s: resolved %+v ok=%v", c.factory.Source(), sp, ok)
		}
		store, err := c.factory.Create()
		if err != nil || store == nil {
			t.Fatalf("%s: create store: %v", c.factory.Source(), err)
		}
		projects, err := store.ListProjects(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, p := range projects {
			found = found || p.ID == sp.ProjectID
		}
		if !found {
			t.Errorf("%s: ProjectID %q not among %+v", c.factory.Source(), sp.ProjectID, projects)
		}
	}
}

func TestResolveThroughSymlinkedHome(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(root, "real-codex")
	link := filepath.Join(root, "link-codex")
	path := writeFile(t, filepath.Join(real, "sessions", "2026", "10", "30", "rollout-a.jsonl"), codexLine)
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THINKT_CODEX_HOME", link)
	t.Setenv("THINKT_CLAUDE_HOME", filepath.Join(root, "none"))
	for _, p := range []string{path, filepath.Join(link, "sessions", "2026", "10", "30", "rollout-a.jsonl")} {
		sp, _, ok := ResolveSessionPath(p)
		if !ok || sp.Source != thinkt.SourceCodex || sp.BaseDir == "" {
			t.Errorf("%s: resolved %+v ok=%v", p, sp, ok)
		}
	}
}

// kimiSession lays out one Kimi session (state.json plus the main wire)
// under base and returns the wire path.
func kimiSession(t *testing.T, base string) string {
	t.Helper()
	dir := filepath.Join(base, "sessions", "wd_proj_abc", "session_p1")
	writeFile(t, filepath.Join(dir, "state.json"), `{"id":"session_p1","version":2,"cwd":"/proj","agents":{"main":{"homedir":"/x/agents/main","type":"main"}}}`)
	return copyFixture(t, "kimi/testdata/wire-1.5.jsonl", filepath.Join(dir, "agents", "main", "wire.jsonl"))
}
