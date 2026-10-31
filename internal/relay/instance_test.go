package relay

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tracewire"
)

func TestSecondRelayRefuses(t *testing.T) {
	h := layoutHomes(t)
	if err := config.RegisterInstance(config.Instance{Type: config.InstanceRelay, PID: os.Getppid(), StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	r := newTestRelay(t, h, &recorder{}, nil, BackfillAll)
	err := r.ExportOnce(context.Background())
	if err == nil || !strings.Contains(err.Error(), "relay already running") {
		t.Fatalf("err = %v", err)
	}
}

func TestRelayUnregistersWhenDone(t *testing.T) {
	h := layoutHomes(t)
	r := newTestRelay(t, h, &recorder{}, nil, BackfillAll)
	if err := r.ExportOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if inst := config.FindInstanceByType(config.InstanceRelay); inst != nil {
		t.Fatalf("instance left registered: %+v", inst)
	}
}

func TestStartDiscardsDisabledSourcesBuffered(t *testing.T) {
	h := layoutHomes(t)
	writeFile(t, filepath.Join(os.Getenv("THINKT_HOME"), "config.json"), `{"sources":{"qwen":{"enabled":true},"codex":{"enabled":false}}}`)
	reg := sources.Restrict(sources.RegistryFor(nil), map[thinkt.Source]bool{thinkt.SourceQwen: true})
	rec := &recorder{}
	r := newTestRelay(t, h, rec, reg, BackfillAll)
	_ = r.buffer.Write(tracewire.Payload{Version: tracewire.Version, MachineID: "m", Source: "codex", Session: tracewire.Session{ID: "x"}})
	if err := r.ExportOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, p := range rec.payloads {
		if p.Source == "codex" {
			t.Fatal("shipped a buffered payload of a disabled source")
		}
	}
	if n, _ := r.buffer.Count(); n != 0 {
		t.Fatalf("buffer still holds %d", n)
	}
}

func TestActivityEventUsesSessionID(t *testing.T) {
	h := layoutHomes(t)
	rec := &recorder{}
	r := newTestRelay(t, h, rec, nil, BackfillAll)
	wire := filepath.Join(h.kimi, "sessions", "wd_proj_abc", "session_p1", "agents", "main", "wire.jsonl")
	r.recordSessionWrite(context.Background(), wire, "kimi")
	if len(rec.activity) != 1 {
		t.Fatalf("%+v", rec.activity)
	}
	ev := rec.activity[0]
	if ev.SessionID != "session_p1" || ev.MachineID == "" || ev.Source != "kimi" || ev.Event != "session_start" {
		t.Fatalf("%+v", ev)
	}
	r.doSweepAt(context.Background(), time.Now().Add(time.Hour))
	if len(rec.activity) != 2 || rec.activity[1].SessionID != "session_p1" || rec.activity[1].Event != "session_end" {
		t.Fatalf("%+v", rec.activity)
	}
}
