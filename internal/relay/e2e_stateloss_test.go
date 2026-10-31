//go:build cgo

package relay

import (
	"context"
	"os"
	"testing"
)

// A relay whose state file is lost (deleted, new THINKT_HOME) must not
// fall below the generation the collector holds: later entries still land.
func TestEndToEndStateLoss(t *testing.T) {
	x := newE2E(t, geminiOnly, 100)
	ctx := context.Background()
	if err := x.relay.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	appendLine(t, geminiPath(x.h), `{"id":"late-1","timestamp":"2026-05-17T15:08:00.000Z","type":"user","content":[{"text":"one"}]}`)
	x.relay.syncPath(ctx, geminiPath(x.h))
	appendLine(t, geminiPath(x.h), `{"id":"late-2","timestamp":"2026-05-17T15:09:00.000Z","type":"user","content":[{"text":"two"}]}`)
	x.relay.syncPath(ctx, geminiPath(x.h))
	// state file removed; new relay instance
	os.Remove(x.relay.cfg.StatePath)
	r2 := newTestRelay(t, x.h, x.gate, x.relay.cfg.Registry, BackfillAll)
	appendLine(t, geminiPath(x.h), `{"id":"late-3","timestamp":"2026-05-17T15:10:00.000Z","type":"user","content":[{"text":"three"}]}`)
	if err := r2.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	appendLine(t, geminiPath(x.h), `{"id":"late-4","timestamp":"2026-05-17T15:11:00.000Z","type":"user","content":[{"text":"four"}]}`)
	r2.syncPath(ctx, geminiPath(x.h))
	got := x.live(t, geminiMain)
	for _, u := range []string{"late-1", "late-2", "late-3", "late-4"} {
		if !got[u] {
			t.Errorf("missing %s after state loss", u)
		}
	}
	if n, _ := r2.buffer.Count(); n != 0 {
		t.Errorf("buffer %d", n)
	}
}
