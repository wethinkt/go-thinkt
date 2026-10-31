//go:build cgo

package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/collect"
	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tracewire"
)

// failing wraps the collector so POST /v1/traces fails while down is set,
// or for the posts of session failSession numbered in failPosts (1-based,
// counting that session's posts only, retries included).
type failing struct {
	next        http.Handler
	mu          sync.Mutex
	posts       int
	failSession string
	failPosts   map[int]bool
	down        bool
}

func (f *failing) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.URL.Path == "/v1/traces" {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		var p tracewire.Payload
		_ = json.Unmarshal(body, &p)
		f.mu.Lock()
		if p.Session.ID == f.failSession {
			f.posts++
		}
		fail := f.down || (p.Session.ID == f.failSession && f.failPosts[f.posts])
		f.mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
	}
	f.next.ServeHTTP(w, r)
}

type e2e struct {
	h     homes
	col   *collect.Server
	gate  *failing
	relay *Relay
}

func newE2E(t *testing.T, only map[thinkt.Source]bool, batchSize int) *e2e {
	t.Helper()
	h := layoutHomes(t)
	reg := sources.Restrict(sources.RegistryFor(nil), only)
	col, err := collect.NewServer(collect.CollectorConfig{DBPath: filepath.Join(h.root, "collector.duckdb"), Quiet: true, FlushInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { col.Close() })
	gate := &failing{next: col.Handler(), failPosts: map[int]bool{}}
	r := newTestRelay(t, h, gate, reg, BackfillAll)
	r.cfg.BatchSize = batchSize
	r.shipper.backoff = time.Millisecond
	return &e2e{h: h, col: col, gate: gate, relay: r}
}

func (x *e2e) flush() { x.col.Store().(*collect.DuckDBStore).Flush() }

// live returns the collector's live entry uuids for a session.
func (x *e2e) live(t *testing.T, sessionID string) map[string]bool {
	t.Helper()
	x.flush()
	es, err := x.col.Store().QueryEntries(context.Background(), sessionID, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, e := range es {
		out[e.UUID] = true
	}
	return out
}

// storeUUIDs reads a session through its store, with the relay's
// composed ids for entries that have none.
func storeUUIDs(t *testing.T, store thinkt.Store, path string) map[string]bool {
	t.Helper()
	s, err := store.LoadSession(context.Background(), path)
	if err != nil || s == nil {
		t.Fatalf("load %s: %v", path, err)
	}
	assignIDs(s.Entries)
	out := map[string]bool{}
	for _, e := range s.Entries {
		out[e.UUID] = true
	}
	return out
}

func TestEndToEndAllSources(t *testing.T) {
	x := newE2E(t, nil, 100)
	ctx := context.Background()
	if err := x.relay.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	x.flush()
	sessions, err := x.col.Store().QuerySessions(ctx, collect.SessionFilter{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	bySource := map[string]int{}
	for _, s := range sessions {
		bySource[s.Source]++
	}
	for _, src := range []string{"claude", "codex", "kimi", "gemini", "antigravity", "copilot", "qwen"} {
		if bySource[src] == 0 {
			t.Errorf("no %s session in the collector (%v)", src, bySource)
		}
	}
	for k, n := range storeEntryCounts(t, x.relay.cfg.Registry) {
		id := k[len(sourceOf(k))+1:]
		if got := len(x.live(t, id)); got != n {
			t.Errorf("%s: collector has %d live entries, store %d", k, got, n)
		}
	}
}

func sourceOf(key string) string {
	for i := range key {
		if key[i] == '/' {
			return key[:i]
		}
	}
	return key
}

const geminiMain = "session-2026-05-17T15-05-17503250"

func geminiPath(h homes) string {
	return filepath.Join(h.gemini, "tmp", "proj", "chats", geminiMain+".jsonl")
}

var geminiOnly = map[thinkt.Source]bool{thinkt.SourceGemini: true}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(line + "\n")
	f.Close()
	later := time.Now().Add(time.Second)
	_ = os.Chtimes(path, later, later)
}

func TestEndToEndPartialFailure(t *testing.T) {
	x := newE2E(t, geminiOnly, 10)
	// Every attempt (first try plus three retries) of the main session's
	// second POST fails, so that chunk is buffered.
	x.gate.failSession = geminiMain
	for p := 2; p <= 2+maxRetries; p++ {
		x.gate.failPosts[p] = true
	}
	ctx := context.Background()
	if err := x.relay.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	appendLine(t, geminiPath(x.h), `{"id":"late-1","timestamp":"2026-05-17T15:08:00.000Z","type":"user","content":[{"text":"one more"}]}`)
	x.relay.syncPath(ctx, geminiPath(x.h))
	if err := x.relay.FlushBuffer(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := x.relay.buffer.Count(); n != 0 {
		t.Fatalf("buffer still holds %d", n)
	}
	store := mustStore(t, x)
	sub := filepath.Join(x.h.gemini, "tmp", "proj", "chats", "17503250-82e4-479a-ab9d-69002dcb6d31", "92szal.jsonl")
	for id, path := range map[string]string{geminiMain: geminiPath(x.h), geminiMain + "/92szal": sub} {
		want := storeUUIDs(t, store, path)
		got := x.live(t, id)
		if len(got) != len(want) {
			t.Errorf("%s: collector has %d live entries, session has %d", id, len(got), len(want))
		}
		for u := range want {
			if !got[u] {
				t.Errorf("%s: missing %s", id, u)
			}
		}
	}
}

func TestEndToEndBufferedRewrites(t *testing.T) {
	x := newE2E(t, geminiOnly, 100)
	ctx := context.Background()
	x.gate.down = true
	if err := x.relay.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	before := storeUUIDs(t, mustStore(t, x), geminiPath(x.h))
	appendLine(t, geminiPath(x.h), `{"$rewindTo":"cont-1"}`)
	x.relay.syncPath(ctx, geminiPath(x.h))
	if n, _ := x.relay.buffer.Count(); n < 2 {
		t.Fatalf("expected both syncs buffered, buffer holds %d", n)
	}
	x.gate.mu.Lock()
	x.gate.down = false
	x.gate.mu.Unlock()
	if err := x.relay.FlushBuffer(ctx); err != nil {
		t.Fatal(err)
	}
	want := storeUUIDs(t, mustStore(t, x), geminiPath(x.h))
	got := x.live(t, geminiMain)
	if len(got) != len(want) || len(want) >= len(before) {
		t.Fatalf("live %d, session now %d, before rewind %d", len(got), len(want), len(before))
	}
	for u := range before {
		if !want[u] && got[u] {
			t.Errorf("rewound entry %s still live", u)
		}
	}
}

func mustStore(t *testing.T, x *e2e) thinkt.Store {
	t.Helper()
	s, ok := x.relay.cfg.Registry.Get(thinkt.SourceGemini)
	if !ok {
		t.Fatal("no gemini store")
	}
	return s
}

var _ = httptest.NewServer
