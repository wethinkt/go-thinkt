package thinkt

import "testing"

func TestRecordClassStringAndIsEntry(t *testing.T) {
	tests := []struct {
		class RecordClass
		name  string
		entry bool
	}{
		{ClassTurn, "turn", true},
		{ClassEvent, "event", true},
		{ClassContext, "context", true},
		{ClassCheckpoint, "checkpoint", true},
		{ClassSessionState, "session-state", false},
		{ClassBookkeeping, "bookkeeping", false},
		{ClassUnknown, "unknown", true},
		{RecordClass(99), "invalid", false},
	}
	for _, tt := range tests {
		if got := tt.class.String(); got != tt.name {
			t.Errorf("String(%d) = %q, want %q", tt.class, got, tt.name)
		}
		if got := tt.class.IsEntry(); got != tt.entry {
			t.Errorf("%s.IsEntry() = %v, want %v", tt.name, got, tt.entry)
		}
	}
}
