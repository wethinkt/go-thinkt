package relay

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestReconcileShipsEverySource(t *testing.T) {
	h := layoutHomes(t)
	rec := &recorder{}
	r := newTestRelay(t, h, rec, nil, BackfillAll)
	if err := r.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := storeEntryCounts(t, r.cfg.Registry)
	got := rec.sessions()
	for _, src := range []string{"claude", "codex", "kimi", "gemini", "antigravity", "copilot", "qwen"} {
		found := false
		for k := range want {
			if strings.HasPrefix(k, src+"/") {
				found = true
			}
		}
		if !found {
			t.Errorf("store for %s lists no session from its layout", src)
		}
	}
	for k, n := range want {
		if len(got[k]) != n {
			t.Errorf("%s: shipped %d entries, store has %d", k, len(got[k]), n)
		}
	}
	if _, ok := got["kimi/session_p1"]; !ok {
		t.Errorf("kimi session id: shipped %v", keys(got))
	}
	sub := false
	for _, p := range rec.payloads {
		if p.Source == "gemini" && p.Session.ParentSessionID != "" {
			sub = true
		}
	}
	if !sub {
		t.Error("no subagent session shipped")
	}
}

func keys(m map[string]map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestReconcileSkipsUnchanged(t *testing.T) {
	h := layoutHomes(t)
	ctx := context.Background()
	if err := newTestRelay(t, h, &recorder{}, nil, BackfillAll).reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	if err := newTestRelay(t, h, rec, nil, BackfillAll).reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if rec.count() != 0 {
		t.Fatalf("restart shipped %d payloads: %v", rec.count(), keys(rec.sessions()))
	}
	path := filepath.Join(h.qwen, "projects", "abc123", "chats", "s1.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"uuid":"q3","sessionId":"s1","type":"user","timestamp":"2026-03-15T10:00:02Z","cwd":"/tmp/proj","message":{"role":"user","parts":[{"text":"more"}]}}` + "\n")
	f.Close()
	rec = &recorder{}
	if err := newTestRelay(t, h, rec, nil, BackfillAll).reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	got := rec.sessions()
	if len(got) != 1 || len(got["qwen/s1"]) != 1 || !got["qwen/s1"]["q3"] {
		t.Fatalf("after one change: %v", got)
	}
}

func TestBackfillNone(t *testing.T) {
	h := layoutHomes(t)
	ctx := context.Background()
	rec := &recorder{}
	if err := newTestRelay(t, h, rec, nil, BackfillNone).reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if rec.count() != 0 {
		t.Fatalf("backfill none shipped %d payloads", rec.count())
	}
	path := filepath.Join(h.qwen, "projects", "abc123", "chats", "s1.jsonl")
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	_, _ = f.WriteString(`{"uuid":"q3","sessionId":"s1","type":"user","timestamp":"2026-03-15T10:00:02Z","cwd":"/tmp/proj","message":{"role":"user","parts":[{"text":"more"}]}}` + "\n")
	f.Close()
	rec = &recorder{}
	if err := newTestRelay(t, h, rec, nil, BackfillNone).reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	got := rec.sessions()
	if len(got) != 1 || len(got["qwen/s1"]) != 1 {
		t.Fatalf("%v", got)
	}
}

func TestVanishedSessionSendsNothing(t *testing.T) {
	h := layoutHomes(t)
	ctx := context.Background()
	r := newTestRelay(t, h, &recorder{}, nil, BackfillAll)
	if err := r.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(h.qwen, "projects", "abc123", "chats", "s1.jsonl")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	r2 := newTestRelay(t, h, rec, nil, BackfillAll)
	r2.syncPath(ctx, path)
	if err := r2.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	for _, p := range rec.payloads {
		if len(p.Removed) > 0 {
			t.Fatalf("vanished session sent removals: %+v", p.Removed)
		}
	}
	if st := r2.state.Sessions[stateKey(thinkt.SourceQwen, "s1")]; st != nil {
		t.Fatal("state kept a vanished session")
	}
}

func TestDisabledSourceNotShipped(t *testing.T) {
	h := layoutHomes(t)
	reg := sources.Restrict(sources.RegistryFor(nil), map[thinkt.Source]bool{thinkt.SourceClaude: true, thinkt.SourceQwen: true})
	rec := &recorder{}
	r := newTestRelay(t, h, rec, reg, BackfillAll)
	if err := r.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.syncPath(context.Background(), filepath.Join(h.codex, "sessions", "2026", "10", "01", "rollout-2026-10-01T10-00-00-fixture.jsonl"))
	for _, p := range rec.payloads {
		if p.Source != "claude" && p.Source != "qwen" {
			t.Fatalf("shipped disabled source %s", p.Source)
		}
	}
}

func TestNewKimiSessionFoundAfterReconcile(t *testing.T) {
	h := layoutHomes(t)
	ctx := context.Background()
	rec := &recorder{}
	r := newTestRelay(t, h, rec, nil, BackfillAll)
	if err := r.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	before := len(rec.sessions())
	wire := kimiSession(t, h.kimi, "session_p2")
	r.syncPath(ctx, wire)
	if _, ok := rec.sessions()["kimi/session_p2"]; !ok || len(rec.sessions()) != before+1 {
		t.Fatalf("new kimi session not shipped: %v", keys(rec.sessions()))
	}
}

func TestGeminiRewindShipsRemoved(t *testing.T) {
	h := layoutHomes(t)
	ctx := context.Background()
	rec := &recorder{}
	r := newTestRelay(t, h, rec, nil, BackfillAll)
	if err := r.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(h.gemini, "tmp", "proj", "chats", "session-2026-05-17T15-05-17503250.jsonl")
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	// Rewind to the second user message: it and everything after it go.
	_, _ = f.WriteString(`{"$rewindTo":"cont-1"}` + "\n")
	f.Close()
	later := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(path, later, later)
	n := rec.count()
	r.syncPath(ctx, path)
	var removed []string
	for _, p := range rec.payloads[n:] {
		removed = append(removed, p.Removed...)
	}
	if len(removed) == 0 {
		t.Fatal("rewind shipped no removals")
	}
}
