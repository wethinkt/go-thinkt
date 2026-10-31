//go:build cgo

package collect

import (
	"context"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// IngestBatch returns only once the payload is written, and reports a
// write failure, so the collector never acknowledges data it lost.
func TestIngestBatchReportsWriteFailure(t *testing.T) {
	s := newModeStore(t, SupersededKeep)
	if err := s.IngestBatch(context.Background(), payload("m", "s", 1, []thinkt.Entry{entry("u1", "a")})); err != nil {
		t.Fatal(err)
	}
	if got := liveTexts(t, s, "s"); got["u1"] != "a" {
		t.Fatalf("not written when IngestBatch returned: %v", got)
	}
	if _, err := s.db.Exec(`DROP TABLE entries`); err != nil {
		t.Fatal(err)
	}
	if err := s.IngestBatch(context.Background(), payload("m", "s", 2, []thinkt.Entry{entry("u2", "b")})); err == nil {
		t.Fatal("write failure not reported")
	}
}
