package target

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestFilterEntriesHidesSecondaryUnlessSystem(t *testing.T) {
	entries := []thinkt.Entry{
		{UUID: "1", Role: thinkt.RoleUser, Text: "hello"},
		{UUID: "2", Role: thinkt.RoleUser, Kind: thinkt.KindMeta, Visibility: thinkt.VisibilitySecondary, Text: "<system-reminder>x</system-reminder>"},
		{UUID: "3", Role: thinkt.RoleSystem, Kind: thinkt.KindTurnDuration, Visibility: thinkt.VisibilitySecondary, Text: "turn_duration"},
		{UUID: "4", Role: thinkt.RoleSystem, Kind: thinkt.KindAwaySummary, Text: "recap"},
	}
	got := FilterEntries(entries, DefaultFilter())
	if len(got) != 1 || got[0].UUID != "1" {
		t.Fatalf("default filter must drop secondary and system entries, got %+v", got)
	}
	f := DefaultFilter()
	f.IncludeSystem = true
	got = FilterEntries(entries, f)
	if len(got) != 4 {
		t.Fatalf("IncludeSystem must reveal secondary entries, got %d", len(got))
	}
}
