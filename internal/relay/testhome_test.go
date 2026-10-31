package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tracewire"
)

// homes are the per-source session homes a test lays out.
type homes struct {
	root                                                     string
	claude, codex, kimi, gemini, antigravity, copilot, qwen string
}

func copyFile(t *testing.T, from, to string) string {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	return writeFile(t, to, string(b))
}

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const kimiState = `{"id":"session_p1","version":2,"cwd":"/proj","createdAt":1790891444054,"updatedAt":1790894174521,"title":"t","agents":{"main":{"homedir":"/x/agents/main","type":"main"}}}`

// layoutHomes copies each source's fixtures into the layout its store
// lists, and points every source's home (and THINKT_HOME) at them.
func layoutHomes(t *testing.T) homes {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := homes{root: root}
	for _, p := range []*string{&h.claude, &h.codex, &h.kimi, &h.gemini, &h.antigravity, &h.copilot, &h.qwen} {
		*p = root
	}
	h.claude = filepath.Join(root, "claude")
	h.codex = filepath.Join(root, "codex")
	h.kimi = filepath.Join(root, "kimi")
	h.gemini = filepath.Join(root, "gemini")
	h.antigravity = filepath.Join(root, "agy")
	h.copilot = filepath.Join(root, "copilot")
	h.qwen = filepath.Join(root, "qwen")
	src := filepath.Join("..", "sources")

	copyFile(t, filepath.Join(src, "claude", "testdata", "modern-session.jsonl"),
		filepath.Join(h.claude, "projects", "-tmp-proj", "11111111-2222-4333-8444-555555555555.jsonl"))
	copyFile(t, filepath.Join(src, "codex", "testdata", "rollout-fixture.jsonl"),
		filepath.Join(h.codex, "sessions", "2026", "10", "01", "rollout-2026-10-01T10-00-00-fixture.jsonl"))
	kimiSession(t, h.kimi, "session_p1")
	writeFile(t, filepath.Join(h.gemini, "tmp", "proj", ".project_root"), "/tmp/proj\n")
	copyFile(t, filepath.Join(src, "gemini", "testdata", "chats", "session-2026-05-17T15-05-17503250.jsonl"),
		filepath.Join(h.gemini, "tmp", "proj", "chats", "session-2026-05-17T15-05-17503250.jsonl"))
	copyFile(t, filepath.Join(src, "gemini", "testdata", "chats", "17503250-82e4-479a-ab9d-69002dcb6d31", "92szal.jsonl"),
		filepath.Join(h.gemini, "tmp", "proj", "chats", "17503250-82e4-479a-ab9d-69002dcb6d31", "92szal.jsonl"))
	copyFile(t, filepath.Join(src, "antigravity", "testdata", "main.jsonl"),
		filepath.Join(h.antigravity, "brain", "c1", ".system_generated", "logs", "transcript_full.jsonl"))
	writeFile(t, filepath.Join(h.copilot, "session-state", "c1", "events.jsonl"), strings.Join([]string{
		`{"type":"session.start","id":"e0","timestamp":"2026-10-01T10:00:00Z","data":{"sessionId":"c1"}}`,
		`{"type":"user.message","id":"e1","timestamp":"2026-10-01T10:00:01Z","data":{"content":"hello copilot"}}`,
		`{"type":"assistant.message","id":"e2","timestamp":"2026-10-01T10:00:02Z","data":{"messageId":"m1","content":"hi there"}}`,
	}, "\n")+"\n")
	writeFile(t, filepath.Join(h.qwen, "projects", "abc123", "chats", "s1.jsonl"), strings.Join([]string{
		`{"uuid":"q1","sessionId":"s1","type":"user","timestamp":"2026-03-15T10:00:00Z","cwd":"/tmp/proj","message":{"role":"user","parts":[{"text":"hello qwen"}]}}`,
		`{"uuid":"q2","sessionId":"s1","type":"assistant","timestamp":"2026-03-15T10:00:01Z","cwd":"/tmp/proj","message":{"role":"model","parts":[{"text":"hi"}]}}`,
	}, "\n")+"\n")

	t.Setenv("THINKT_HOME", filepath.Join(root, "thinkt-home"))
	t.Setenv("THINKT_CLAUDE_HOME", h.claude)
	t.Setenv("THINKT_CODEX_HOME", h.codex)
	t.Setenv("THINKT_KIMI_HOME", h.kimi)
	t.Setenv("THINKT_GEMINI_HOME", h.gemini)
	t.Setenv("THINKT_ANTIGRAVITY_HOME", h.antigravity)
	t.Setenv("THINKT_COPILOT_HOME", h.copilot)
	t.Setenv("THINKT_QWEN_HOME", h.qwen)
	return h
}

// kimiSession lays out one Kimi session and returns its main wire path.
func kimiSession(t *testing.T, home, id string) string {
	t.Helper()
	dir := filepath.Join(home, "sessions", "wd_proj_abc", id)
	writeFile(t, filepath.Join(dir, "state.json"), strings.Replace(kimiState, "session_p1", id, 1))
	return copyFile(t, filepath.Join("..", "sources", "kimi", "testdata", "wire-1.5.jsonl"), filepath.Join(dir, "agents", "main", "wire.jsonl"))
}

// recorder is a collector stub that records the payloads it accepts. fail
// makes the n-th POST (1-based) answer 503 once.
type recorder struct {
	mu       sync.Mutex
	payloads []tracewire.Payload
	posts    int
	fail     map[int]bool
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/v1/traces" {
		w.WriteHeader(http.StatusOK)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.posts++
	if r.fail[r.posts] {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	var p tracewire.Payload
	if err := json.NewDecoder(req.Body).Decode(&p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	r.payloads = append(r.payloads, p)
	w.WriteHeader(http.StatusOK)
}

// sessions groups the recorded payloads' entries by source and session id.
func (r *recorder) sessions() map[string]map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]map[string]bool{}
	for _, p := range r.payloads {
		k := p.Source + "/" + p.Session.ID
		if out[k] == nil {
			out[k] = map[string]bool{}
		}
		for _, e := range p.Entries {
			out[k][e.UUID] = true
		}
	}
	return out
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.payloads)
}

// newTestRelay builds a relay over every source's registry (or reg), with
// its buffer and state under the test root, shipping to rec.
func newTestRelay(t *testing.T, h homes, rec http.Handler, reg *thinkt.StoreRegistry, backfill string) *Relay {
	t.Helper()
	if reg == nil {
		reg = sources.RegistryFor(nil)
	}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)
	r, err := New(RelayConfig{
		CollectorURL: srv.URL,
		BufferDir:    filepath.Join(h.root, "buffer"),
		StatePath:    filepath.Join(h.root, "relay", "state.json"),
		Registry:     reg,
		Backfill:     backfill,
		Quiet:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r.shipper = NewShipper(srv.URL+"/v1/traces", "")
	return r
}

// storeEntryCounts reads every session of every store in reg, subagents
// included, keyed like recorder.sessions.
func storeEntryCounts(t *testing.T, reg *thinkt.StoreRegistry) map[string]int {
	t.Helper()
	ctx := context.Background()
	out := map[string]int{}
	for _, store := range reg.All() {
		projects, _ := store.ListProjects(ctx)
		for _, p := range projects {
			ms, _ := store.ListSessions(ctx, p.ID, thinkt.WithSubagents())
			for _, m := range ms {
				s, err := store.LoadSession(ctx, m.FullPath)
				if err != nil || s == nil {
					t.Fatalf("%s %s: load: %v", store.Source(), m.ID, err)
				}
				out[string(store.Source())+"/"+m.ID] = len(s.Entries)
			}
		}
	}
	return out
}
