package thinkt

import (
	"maps"
	"testing"
)

func TestUnknownTallyCountsAndSnapshots(t *testing.T) {
	u := UnknownTally[string]{Source: SourceCodex}
	if u.Snapshot() != nil {
		t.Fatal("zero tally snapshot should be nil")
	}
	u.Note("a")
	u.Note("a", "line", 3)
	u.Note("b")
	want := map[string]int{"a": 2, "b": 1}
	got := u.Snapshot()
	if !maps.Equal(got, want) {
		t.Fatalf("snapshot = %v, want %v", got, want)
	}
	got["a"] = 99
	if again := u.Snapshot(); !maps.Equal(again, want) {
		t.Fatalf("snapshot shares storage: %v", again)
	}
}
