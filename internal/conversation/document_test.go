package conversation

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func sampleEntries(t *testing.T) []thinkt.Entry {
	t.Helper()
	data, err := os.ReadFile("testdata/session.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []thinkt.Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestBuildPreservesContentAndToolOrder(t *testing.T) {
	entries := sampleEntries(t)
	doc, err := Build(entries, BuildOptions{SessionID: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Messages) != len(entries) {
		t.Fatal("messages were dropped")
	}
	for i, message := range doc.Messages {
		if message.Role != entries[i].Role || message.Origin.EntryUUID != entries[i].UUID || message.Origin.EntryIndex != i {
			t.Errorf("message %d lost its role or source identity: %+v", i, message)
		}
	}
	call := doc.Messages[1].Blocks[2]
	result := doc.Messages[2].Blocks[0]
	if call.Kind != "tool_use" || result.Kind != "tool_result" || !result.IsError {
		t.Fatal("tool call/result order or error state changed")
	}
	if !reflect.DeepEqual(call.Related, []string{result.ID}) || !reflect.DeepEqual(result.Related, []string{call.ID}) {
		t.Fatal("tool relationships are not reciprocal")
	}
	media := doc.Messages[3].Blocks[1]
	if media.MediaData != "fixture-image-payload" || media.MediaType != "image/png" || media.Origin.BlockIndex != 1 {
		t.Fatal("media reference or payload lost")
	}
	if doc.Messages[1].Blocks[0].Text != entries[1].ContentBlocks[0].Thinking {
		t.Fatal("thinking was truncated")
	}
}

func TestAnchorsSurviveBatching(t *testing.T) {
	entries := sampleEntries(t)
	// Empty and repeated UUIDs must still produce distinct, batch-stable IDs.
	entries[0].UUID, entries[1].UUID, entries[2].UUID = "", "same", "same"
	whole, err := Build(entries, BuildOptions{SessionID: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for i, entry := range entries {
		batch, err := Build([]thinkt.Entry{entry}, BuildOptions{SessionID: "session-1", EntryOffset: i})
		if err != nil {
			t.Fatal(err)
		}
		message := whole.Messages[i]
		if message.ID != batch.Messages[0].ID || seen[message.ID] {
			t.Fatal("message anchors collided or changed with batching")
		}
		seen[message.ID] = true
		for j, block := range message.Blocks {
			if block.ID != batch.Messages[0].Blocks[j].ID {
				t.Fatal("block anchor changed with batching")
			}
		}
	}
	other, err := Build(entries, BuildOptions{SessionID: "session-2"})
	if err != nil {
		t.Fatal(err)
	}
	if whole.Messages[0].ID == other.Messages[0].ID {
		t.Fatal("session namespaces collided")
	}
}

func TestBuildRetainsUnfamiliarRolesAndBlocks(t *testing.T) {
	entries := []thinkt.Entry{{Role: "future-role", ContentBlocks: []thinkt.ContentBlock{{Type: "future-block", Text: "preserve me"}}}}
	doc, err := Build(entries, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var raw thinkt.ContentBlock
	if err := json.Unmarshal([]byte(doc.Messages[0].Blocks[0].Text), &raw); err != nil {
		t.Fatal(err)
	}
	if doc.Messages[0].Role != "future-role" || !reflect.DeepEqual(raw, entries[0].ContentBlocks[0]) {
		t.Fatal("unfamiliar content was lost")
	}
}

func TestBuildRejectsUnserializableInput(t *testing.T) {
	_, err := Build([]thinkt.Entry{{ContentBlocks: []thinkt.ContentBlock{{Type: "tool_use", ToolInput: make(chan int)}}}}, BuildOptions{})
	if err == nil {
		t.Fatal("invalid tool input silently discarded")
	}
}
