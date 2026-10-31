package tracewire

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func sampleEntry() thinkt.Entry {
	return thinkt.Entry{
		UUID:      "u1",
		Role:      thinkt.RoleAssistant,
		Timestamp: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC),
		Source:    thinkt.SourceGemini,
		Text:      "hello",
		TurnID:    "t1",
		PromptID:  "p1",
		Usage:     &thinkt.TokenUsage{InputTokens: 10, OutputTokens: 5, ThinkingTokens: 2},
		ContentBlocks: []thinkt.ContentBlock{
			{Type: "thinking", Thinking: "hmm"},
			{Type: "tool_use", ToolUseID: "c1", ToolName: "read_file", ToolInput: map[string]any{"path": "a.go"}},
		},
		Metadata: map[string]any{"a": "1", "b": "2"},
	}
}

func TestPayloadRoundTrip(t *testing.T) {
	p := Payload{
		Version: Version, InstanceID: "i", MachineID: "m", Source: "gemini",
		Session: Session{ID: "s", Title: "T", Generation: 3},
		Entries: []thinkt.Entry{sampleEntry()},
		Removed: []string{"u9"},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var got Payload
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, p) {
		t.Fatalf("round trip:\n got %+v\nwant %+v", got, p)
	}
}

func TestEntryHashStable(t *testing.T) {
	a, b := sampleEntry(), sampleEntry()
	if EntryHash(a) != EntryHash(b) {
		t.Fatal("same entry, different hashes")
	}
	b.Text = "changed"
	if EntryHash(a) == EntryHash(b) {
		t.Fatal("changed text, same hash")
	}
	c := sampleEntry()
	c.Metadata = map[string]any{}
	c.Metadata["b"] = "2"
	c.Metadata["a"] = "1"
	if EntryHash(a) != EntryHash(c) {
		t.Fatal("metadata key order changed the hash")
	}
	if len(EntryHash(a)) != 64 {
		t.Fatalf("hash %q", EntryHash(a))
	}
}

func TestSessionFromMeta(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	m := thinkt.SessionMeta{
		ID: "s", ProjectPath: "/p", ParentSessionID: "parent", Title: "T", Model: "m",
		CreatedAt: now, ModifiedAt: now.Add(time.Hour), EntryCount: 7,
		Cost: &thinkt.SessionCost{InputTokens: 3},
	}
	want := Session{
		ID: "s", ProjectPath: "/p", ParentSessionID: "parent", Title: "T", Model: "m",
		CreatedAt: now, ModifiedAt: now.Add(time.Hour), EntryCount: 7,
		Cost: &thinkt.SessionCost{InputTokens: 3},
	}
	if got := SessionFromMeta(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
}
