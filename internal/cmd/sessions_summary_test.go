package cmd

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// The summary prints counts, prompts and related folders, so it must not
// return before the sessions are scanned.
func TestSessionsSummaryEnrichesSynchronously(t *testing.T) {
	if !thinkt.ResolveListOptions(sessionsSummaryListOptions()).EnrichSync {
		t.Fatal("sessions summary lists without WithEnrichSync")
	}
}
