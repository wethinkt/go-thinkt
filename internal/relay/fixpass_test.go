package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tracewire"
)

// A --source filter narrows what this run ships; it must not delete the
// buffered payloads of other enabled sources.
func TestSourceFilterKeepsOtherSourcesBuffered(t *testing.T) {
	h := layoutHomes(t)
	writeFile(t, filepath.Join(os.Getenv("THINKT_HOME"), "config.json"), `{"sources":{"claude":{"enabled":true},"codex":{"enabled":true}}}`)
	reg := sources.Restrict(sources.RegistryFor(nil), map[thinkt.Source]bool{thinkt.SourceClaude: true})
	rec := &recorder{fail: map[int]bool{}}
	r := newTestRelay(t, h, rec, reg, BackfillAll)
	r.shipper = nil // buffer only
	_ = r.buffer.Write(tracewire.Payload{Version: tracewire.Version, MachineID: "m", Source: "codex", Session: tracewire.Session{ID: "x"}})
	r.discardDisabled()
	if n, _ := r.buffer.Count(); n != 1 {
		t.Fatalf("codex payload discarded by a --source filter (buffer %d)", n)
	}
}

// A payload of a source the config turned off is dropped, and so is its
// state, so turning the source back on re-ships the session.
func TestDiscardDropsStateOfDisabledSource(t *testing.T) {
	h := layoutHomes(t)
	writeFile(t, filepath.Join(os.Getenv("THINKT_HOME"), "config.json"), `{"sources":{"claude":{"enabled":true},"codex":{"enabled":false}}}`)
	r := newTestRelay(t, h, &recorder{}, sources.RegistryFor(nil), BackfillAll)
	r.state.Sessions[stateKey(thinkt.SourceCodex, "x")] = &sessionState{Hashes: map[string]string{"u": "h"}}
	_ = r.buffer.Write(tracewire.Payload{Version: tracewire.Version, MachineID: "m", Source: "codex", Session: tracewire.Session{ID: "x"}})
	r.discardDisabled()
	if n, _ := r.buffer.Count(); n != 0 {
		t.Fatalf("buffer %d", n)
	}
	if r.state.Sessions[stateKey(thinkt.SourceCodex, "x")] != nil {
		t.Fatal("state of the discarded session kept")
	}
}

// rejecting answers 400 for one session's payloads, like a collector that
// will never accept them.
type rejecting struct {
	*recorder
	session string
}

func (r *rejecting) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/v1/traces" {
		body := readBody(req)
		if sessionOf(body) == r.session {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		req.Body = bodyFrom(body)
	}
	r.recorder.ServeHTTP(w, req)
}

// A payload the collector permanently rejects is moved aside, and does not
// block the payloads behind it.
func TestRejectedPayloadDoesNotBlockBuffer(t *testing.T) {
	h := layoutHomes(t)
	rec := &rejecting{recorder: &recorder{}, session: "s1"}
	r := newTestRelay(t, h, rec, sources.Restrict(sources.RegistryFor(nil), map[thinkt.Source]bool{thinkt.SourceQwen: true, thinkt.SourceClaude: true}), BackfillAll)
	r.shipper.backoff = 1
	if err := r.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.FlushBuffer(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n, _ := r.buffer.Count(); n != 0 {
		t.Fatalf("buffer still holds %d", n)
	}
	if _, ok := rec.sessions()["claude/11111111-2222-4333-8444-555555555555"]; !ok {
		t.Fatalf("claude session blocked: %v", keys(rec.sessions()))
	}
	q, _ := os.ReadDir(r.quarantineDir())
	if len(q) == 0 {
		t.Fatal("rejected payload not kept in quarantine")
	}
}

// With the collector down, a sync of many payloads tries the collector
// once, not once per payload.
func TestCollectorDownTriesOnce(t *testing.T) {
	h := layoutHomes(t)
	rec := &recorder{fail: map[int]bool{}}
	for i := 1; i <= 1000; i++ {
		rec.fail[i] = true
	}
	r := newTestRelay(t, h, rec, nil, BackfillAll)
	r.cfg.BatchSize = 5
	r.shipper.backoff = 1
	if err := r.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rec.posts > maxRetries+1 {
		t.Fatalf("%d POSTs while the collector was down", rec.posts)
	}
	if n, _ := r.buffer.Count(); n < 10 {
		t.Fatalf("only %d payloads buffered", n)
	}
}

// A file event does not rewrite the state file; the relay saves when dirty.
func TestSyncPathDoesNotSaveState(t *testing.T) {
	h := layoutHomes(t)
	r := newTestRelay(t, h, &recorder{}, nil, BackfillAll)
	r.syncPath(context.Background(), filepath.Join(h.qwen, "projects", "abc123", "chats", "s1.jsonl"))
	if _, err := os.Stat(r.cfg.StatePath); err == nil {
		t.Fatal("state written on a file event")
	}
	r.saveIfDirty()
	if _, err := os.Stat(r.cfg.StatePath); err != nil {
		t.Fatalf("state not saved when dirty: %v", err)
	}
}

// cancelAfterOpen cancels the reconcile's context once a session is read.
type cancelAfterOpen struct {
	thinkt.Store
	cancel context.CancelFunc
}

func (c *cancelAfterOpen) OpenSession(ctx context.Context, id string) (thinkt.SessionReader, error) {
	defer c.cancel()
	return c.Store.OpenSession(ctx, id)
}

// Interrupting a reconcile keeps the progress it made.
func TestInterruptedReconcileSavesProgress(t *testing.T) {
	h := layoutHomes(t)
	full := sources.RegistryFor(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg := thinkt.NewRegistry()
	for _, s := range full.All() {
		reg.Register(&cancelAfterOpen{Store: s, cancel: cancel})
	}
	r := newTestRelay(t, h, &recorder{}, reg, BackfillAll)
	_ = r.reconcile(ctx)
	st, err := loadState(r.cfg.StatePath)
	if err != nil || len(st.Sessions) == 0 {
		t.Fatalf("no progress saved: %v %v", st, err)
	}
}

func readBody(req *http.Request) []byte {
	b, _ := io.ReadAll(req.Body)
	return b
}

func bodyFrom(b []byte) io.ReadCloser { return io.NopCloser(bytes.NewReader(b)) }

func sessionOf(body []byte) string {
	var p tracewire.Payload
	_ = json.Unmarshal(body, &p)
	return p.Session.ID
}
