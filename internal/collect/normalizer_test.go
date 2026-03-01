package collect

import (
	"testing"
)

func TestNormalizeEntry_DerivesHasThinking(t *testing.T) {
	e := IngestEntry{
		UUID:        "test-1",
		Role:        "assistant",
		ThinkingLen: 150,
	}
	if err := normalizeEntry(&e); err != nil {
		t.Fatal(err)
	}
	if !e.HasThinking {
		t.Error("HasThinking = false, want true (derived from ThinkingLen > 0)")
	}
}

func TestNormalizeEntry_DerivesHasToolUse(t *testing.T) {
	e := IngestEntry{
		UUID:     "test-2",
		Role:     "assistant",
		ToolName: "Read",
	}
	if err := normalizeEntry(&e); err != nil {
		t.Fatal(err)
	}
	if !e.HasToolUse {
		t.Error("HasToolUse = false, want true (derived from ToolName)")
	}
}

func TestNormalizeEntry_PreservesExplicitFlags(t *testing.T) {
	// If the exporter already set the flags, normalizer should not override.
	e := IngestEntry{
		UUID:        "test-3",
		Role:        "assistant",
		HasThinking: true,
		HasToolUse:  true,
		ThinkingLen: 0, // Even with zero thinking_len, explicit flag is preserved
		ToolName:    "", // Even with empty tool_name, explicit flag is preserved
	}
	if err := normalizeEntry(&e); err != nil {
		t.Fatal(err)
	}
	if !e.HasThinking {
		t.Error("HasThinking was overridden to false")
	}
	if !e.HasToolUse {
		t.Error("HasToolUse was overridden to false")
	}
}

func TestNormalizeEntry_NoFlagsWhenNoSignals(t *testing.T) {
	e := IngestEntry{
		UUID: "test-4",
		Role: "user",
	}
	if err := normalizeEntry(&e); err != nil {
		t.Fatal(err)
	}
	if e.HasThinking {
		t.Error("HasThinking = true for user entry with no thinking data")
	}
	if e.HasToolUse {
		t.Error("HasToolUse = true for user entry with no tool data")
	}
}

func TestNormalizeRequest_ClassificationEndToEnd(t *testing.T) {
	req := IngestRequest{
		SessionID: "sess-1",
		Source:    "claude",
		Entries: []IngestEntry{
			{UUID: "e1", Role: "assistant", ThinkingLen: 500},
			{UUID: "e2", Role: "assistant", ToolName: "Bash"},
			{UUID: "e3", Role: "user"},
		},
	}

	dropped, err := NormalizeRequest(&req)
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0", dropped)
	}

	if !req.Entries[0].HasThinking {
		t.Error("entry 0: HasThinking should be true")
	}
	if req.Entries[0].HasToolUse {
		t.Error("entry 0: HasToolUse should be false")
	}
	if !req.Entries[1].HasToolUse {
		t.Error("entry 1: HasToolUse should be true")
	}
	if req.Entries[1].HasThinking {
		t.Error("entry 1: HasThinking should be false")
	}
	if req.Entries[2].HasThinking || req.Entries[2].HasToolUse {
		t.Error("entry 2: user entry should have no classification flags")
	}
}
