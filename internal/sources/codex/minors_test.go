package codex

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Minor 1: legacy event/wire pairs count once, and wire messages with no
// content count zero, so EntryCount matches what the parser emits.
func TestScanSessionFileEntryCountMatchesParserOnLegacyRollout(t *testing.T) {
	lines := []string{
		`{"timestamp":"2026-02-10T00:00:00Z","type":"session_meta","payload":{"id":"sess-dup","cwd":"/w"}}`,
		`{"timestamp":"2026-02-10T00:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"same text"}}`,
		`{"timestamp":"2026-02-10T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"same text"}]}}`,
		`{"timestamp":"2026-02-10T00:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"assistant line"}}`,
		`{"timestamp":"2026-02-10T00:00:03Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"assistant line"}]}}`,
		`{"timestamp":"2026-02-10T00:00:04Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"thinking once"}}`,
		`{"timestamp":"2026-02-10T00:00:05Z","type":"response_item","payload":{"type":"reasoning","text":"thinking once"}}`,
		`{"timestamp":"2026-02-10T00:00:06Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"   "}]}}`,
		`{"timestamp":"2026-02-10T00:00:07Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[]}}`,
	}
	path := filepath.Join(t.TempDir(), "legacy.jsonl")
	writeLines(t, path, lines...)
	parsed := parseAll(t, lines...)
	s := scanSessionFile(path, nil)
	if len(parsed) != 3 || s.EntryCount != len(parsed) {
		t.Fatalf("parser emitted %d, scan counted %d", len(parsed), s.EntryCount)
	}
}

func TestScanSessionFileEntryCountMatchesParserOnFixture(t *testing.T) {
	if n := scanSessionFile("testdata/rollout-fixture.jsonl", nil).EntryCount; n != len(readFixture(t)) {
		t.Fatalf("scan counted %d, parser emitted %d", n, len(readFixture(t)))
	}
}

// Minor 2: a wire call without a call_id must not share the UUID "wire:"
// with every other such call.
func TestWireCallWithoutCallIDGetsDistinctUUIDs(t *testing.T) {
	a := `{"timestamp":"2026-02-10T00:00:03Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"pwd\"}"}}`
	b := `{"timestamp":"2026-02-10T00:00:04Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"ls\"}"}}`
	entries := parseAll(t, a, b)
	if len(entries) != 2 || entries[0].UUID == entries[1].UUID || strings.HasSuffix(entries[0].UUID, "wire:") {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].ContentBlocks[0].ToolUseID == "" || entries[0].ContentBlocks[0].ToolUseID == entries[1].ContentBlocks[0].ToolUseID {
		t.Fatalf("tool use ids must be distinct and non-empty: %+v", entries)
	}
}

// Minor 3: only the file header is skipped, so removed lines that begin
// with "--" (SQL or YAML) and added lines that begin with "++" still count.
func TestDiffLineCountsKeepsDashDashBodyLines(t *testing.T) {
	added, removed := diffLineCounts("--- a/q.sql\n+++ b/q.sql\n@@ -1,2 +1,2 @@\n--- old comment\n+++ new comment\n-x\n+y\n")
	if added != 2 || removed != 2 {
		t.Fatalf("counts = %d/%d, want 2/2", added, removed)
	}
}

// Minor 4: a failed command with exit code 0 is still an error.
func TestCommandExecutionFailedStatusIsError(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:05Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"CommandExecution","id":"exec-9","command":["ls"],"cwd":"file:///w","source":"unified_exec_startup","status":"failed","stdout":"","stderr":"","aggregated_output":"","exit_code":0,"duration":{"secs":0,"nanos":0}}}}`
	_, _, _, result := pairOf(t, convertLine(t, line))
	if !result.IsError {
		t.Fatalf("result = %+v", result)
	}
}

// Minor 5: the dedupe memory covers only the item's adjacent twin; a later
// wire-only reply with the same text is a new entry.
func TestParserKeepsLaterWireReplyWithSameText(t *testing.T) {
	entries := parseAll(t, fixAgentItemA2, fixWireA2, fixWireA2)
	if len(entries) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
	user := `{"timestamp":"2026-10-01T10:00:15Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"again"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["user.text"]}}}`
	entries = parseAll(t, fixAgentItemA2, user, fixWireA2)
	if len(entries) != 3 {
		t.Fatalf("entries = %+v", entries)
	}
}

// Minor 6: MCP content that does not decode still gets a typed child.
func TestMcpToolCallUndecodableContentChildHasType(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:07Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"McpToolCall","id":"m3","server":"s","tool":"t","arguments":{},"status":"completed","result":{"content":["bare string",{"data":"x"}],"isError":false},"duration":{"secs":0,"nanos":0}}}}`
	_, _, _, result := pairOf(t, convertLine(t, line))
	if len(result.Children) != 2 {
		t.Fatalf("children = %+v", result.Children)
	}
	for _, ch := range result.Children {
		if ch.Type == "" || ch.Text == "" {
			t.Fatalf("child = %+v", ch)
		}
	}
	_ = thinkt.BlockTypeDiff
}

// A wire assistant reply that does not repeat the item before it is an entry
// for the parser and counts one for the scan.
func TestScanSessionFileCountsNonTwinWireReply(t *testing.T) {
	other := `{"timestamp":"2026-10-01T10:00:14Z","type":"response_item","payload":{"type":"message","id":"msg-x","role":"assistant","content":[{"type":"output_text","text":"Something else."}]}}`
	path := filepath.Join(t.TempDir(), "r.jsonl")
	writeLines(t, path, fixAgentItemA2, other, fixAgentItemA2, fixWireA2)
	parsed := parseAll(t, fixAgentItemA2, other, fixAgentItemA2, fixWireA2)
	if s := scanSessionFile(path, nil); len(parsed) != 3 || s.EntryCount != 3 {
		t.Fatalf("parser %d, scan %d", len(parsed), s.EntryCount)
	}
}
