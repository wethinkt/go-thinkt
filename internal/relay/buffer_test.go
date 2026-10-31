package relay

import (
	"context"
	"os"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/tracewire"
)

func newTestBuffer(t *testing.T) *DiskBuffer {
	t.Helper()
	b, err := NewDiskBuffer(t.TempDir(), 10)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBufferNameSafeForSubagentIDs(t *testing.T) {
	b := newTestBuffer(t)
	p := tracewire.Payload{Source: "kimi", Session: tracewire.Session{ID: "session_y/agent-0"}}
	if err := b.Write(p); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(b.dir)
	if len(entries) != 1 || entries[0].IsDir() {
		t.Fatalf("buffer dir holds %v", entries)
	}
	var got []string
	if _, err := b.Drain(context.Background(), func(p tracewire.Payload) error {
		got = append(got, p.Session.ID)
		return nil
	}); err != nil || len(got) != 1 || got[0] != "session_y/agent-0" {
		t.Fatalf("drained %v %v", got, err)
	}
}

func TestBufferSameNanosecondNoOverwrite(t *testing.T) {
	b := newTestBuffer(t)
	for i := 0; i < 50; i++ {
		if err := b.Write(tracewire.Payload{Source: "gemini", Session: tracewire.Session{ID: "s", Generation: int64(i)}}); err != nil {
			t.Fatal(err)
		}
	}
	var gens []int64
	_, _ = b.Drain(context.Background(), func(p tracewire.Payload) error {
		gens = append(gens, p.Session.Generation)
		return nil
	})
	if len(gens) != 50 {
		t.Fatalf("drained %d of 50", len(gens))
	}
	for i, g := range gens {
		if g != int64(i) {
			t.Fatalf("out of order at %d: %v", i, gens)
		}
	}
}

func TestBufferDiscardDisabledSources(t *testing.T) {
	b := newTestBuffer(t)
	_ = b.Write(tracewire.Payload{Source: "codex", Session: tracewire.Session{ID: "a"}})
	_ = b.Write(tracewire.Payload{Source: "claude", Session: tracewire.Session{ID: "b"}})
	n, err := b.Discard(func(p tracewire.Payload) bool { return p.Source == "claude" })
	if err != nil || n != 1 {
		t.Fatalf("removed %d %v", n, err)
	}
	if c, _ := b.Count(); c != 1 {
		t.Fatalf("left %d", c)
	}
}
