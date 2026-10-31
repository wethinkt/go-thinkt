//go:build cgo

package collect

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tracewire"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	srv, err := NewServer(CollectorConfig{DBPath: filepath.Join(t.TempDir(), "c.duckdb"), Quiet: true, FlushInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.setupRouter())
	t.Cleanup(func() {
		ts.Close()
		srv.store.Close()
	})
	return srv, ts
}

func post(t *testing.T, ts *httptest.Server, path string, body any) (*http.Response, string) {
	t.Helper()
	var b []byte
	switch v := body.(type) {
	case []byte:
		b = v
	default:
		var err error
		if b, err = json.Marshal(v); err != nil {
			t.Fatal(err)
		}
	}
	resp, err := http.Post(ts.URL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	_, _ = out.ReadFrom(resp.Body)
	return resp, out.String()
}

func flush(srv *Server) { srv.store.(*DuckDBStore).Flush() }

func TestIngestRejectsOldVersion(t *testing.T) {
	_, ts := newTestServer(t)
	old := map[string]any{"instance_id": "i", "source": "claude", "session_id": "s", "entries": []any{map[string]any{"uuid": "u", "role": "user"}}}
	resp, body := post(t, ts, "/v1/traces", old)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "unsupported_version") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
}

func TestIngestAcceptsLargePayload(t *testing.T) {
	srv, ts := newTestServer(t)
	big := strings.Repeat("x", 20<<20)
	p := payload("m", "s", 1, []thinkt.Entry{entry("u1", big)})
	resp, body := post(t, ts, "/v1/traces", p)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%d %s", resp.StatusCode, body[:min(len(body), 200)])
	}
	flush(srv)
	if got := liveTexts(t, srv.store.(*DuckDBStore), "s"); len(got["u1"]) != len(big) {
		t.Fatalf("stored %d bytes", len(got["u1"]))
	}
}

func TestIngestRemovalOnlyBodyApplied(t *testing.T) {
	srv, ts := newTestServer(t)
	post(t, ts, "/v1/traces", payload("m", "s", 1, []thinkt.Entry{entry("u1", "a")}))
	flush(srv)
	raw := []byte(`{"version":2,"instance_id":"i","machine_id":"m","source":"gemini","session":{"id":"s","generation":2},"entries":[],"removed":["u1"]}`)
	if resp, body := post(t, ts, "/v1/traces", raw); resp.StatusCode != http.StatusOK {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	flush(srv)
	if _, marked := superseded(t, srv.store.(*DuckDBStore), "u1"); !marked {
		t.Fatal("u1 not superseded")
	}
}

func TestIngestPublishesSummaries(t *testing.T) {
	srv, ts := newTestServer(t)
	ch, unsub := srv.pubsub.Subscribe("s")
	defer unsub()
	e := entry("u1", "hello")
	e.ContentBlocks = []thinkt.ContentBlock{{Type: "text", Text: "hello"}}
	post(t, ts, "/v1/traces", payload("m", "s", 1, []thinkt.Entry{e}))
	select {
	case got := <-ch:
		if len(got) != 1 || got[0].Role != "assistant" || got[0].Text != "hello" || len(got[0].ContentBlocks) != 1 {
			t.Fatalf("%+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nothing published")
	}
}

func TestSupersededSettingFromEnv(t *testing.T) {
	t.Setenv("THINKT_COLLECTOR_SUPERSEDED", "delete")
	if m, err := ResolveSupersededMode(""); err != nil || m != SupersededDelete {
		t.Fatalf("%v %v", m, err)
	}
	if m, err := ResolveSupersededMode("keep"); err != nil || m != SupersededKeep {
		t.Fatalf("flag should win: %v %v", m, err)
	}
	t.Setenv("THINKT_COLLECTOR_SUPERSEDED", "purge")
	if _, err := ResolveSupersededMode(""); err == nil {
		t.Fatal("invalid value accepted")
	}
}

func TestActivityByFullKey(t *testing.T) {
	srv, ts := newTestServer(t)
	post(t, ts, "/v1/traces", payload("A", "s", 1, []thinkt.Entry{entry("u1", "a")}))
	post(t, ts, "/v1/traces", payload("B", "s", 1, []thinkt.Entry{entry("u1", "b")}))
	flush(srv)
	ev := SessionActivityEvent{MachineID: "A", Source: "gemini", SessionID: "s", Event: "session_active", Timestamp: time.Now()}
	if resp, body := post(t, ts, "/v1/sessions/activity", ev); resp.StatusCode != http.StatusOK {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	db := srv.store.(*DuckDBStore).db
	var a, b string
	_ = db.QueryRow(`SELECT status FROM sessions WHERE machine_id = 'A'`).Scan(&a)
	_ = db.QueryRow(`SELECT status FROM sessions WHERE machine_id = 'B'`).Scan(&b)
	if a != "active" || b == "active" {
		t.Fatalf("A %q B %q", a, b)
	}
}

var _ = tracewire.Version
