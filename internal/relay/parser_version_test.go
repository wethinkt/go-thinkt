package relay

import (
	"context"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// A new parser version resyncs unchanged files, so the archive gets what the
// readers now produce, and only changed entries ship.
func TestReconcileResyncsOnNewParserVersion(t *testing.T) {
	h := layoutHomes(t)
	ctx := context.Background()
	first := newTestRelay(t, h, &recorder{}, nil, BackfillAll)
	if err := first.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	key := stateKey(thinkt.SourceQwen, "s1")
	st, err := loadState(first.cfg.StatePath)
	if err != nil || st.Sessions[key] == nil {
		t.Fatalf("%+v %v", st, err)
	}
	if st.Sessions[key].ParserVersion != thinkt.ParserVersion {
		t.Fatalf("state records parser %d", st.Sessions[key].ParserVersion)
	}
	st.Sessions[key].ParserVersion = thinkt.ParserVersion - 1
	st.Sessions[key].Hashes["q1"] = "older"
	if err := st.save(first.cfg.StatePath); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	if err := newTestRelay(t, h, rec, nil, BackfillAll).reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	got := rec.sessions()["qwen/s1"]
	if len(got) != 1 || !got["q1"] {
		t.Fatalf("%v", rec.sessions())
	}
}
