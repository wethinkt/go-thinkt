package codex

import (
	"io"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// parseAll drains a parser built over the given lines.
func parseAll(t *testing.T, lines ...string) []thinkt.Entry {
	t.Helper()
	p := NewParser(strings.NewReader(strings.Join(lines, "\n")), "sess")
	var out []thinkt.Entry
	for {
		e, err := p.NextEntry()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("NextEntry: %v", err)
		}
		out = append(out, *e)
	}
}

const (
	fixAgentItemA2 = `{"timestamp":"2026-10-01T10:00:14Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"AgentMessage","id":"msg-a2","content":[{"type":"Text","text":"Build fixed."}],"phase":"final_answer"}}}`
	fixWireA2      = `{"timestamp":"2026-10-01T10:00:14Z","type":"response_item","payload":{"type":"message","id":"msg-a2","role":"assistant","content":[{"type":"output_text","text":"Build fixed."}],"phase":"final_answer"}}`
	fixWireA2WS    = `{"timestamp":"2026-10-01T10:00:14Z","type":"response_item","payload":{"type":"message","id":"msg-a2","role":"assistant","content":[{"type":"output_text","text":"Build fixed.\n"}],"phase":"final_answer"}}`
)

func TestParserDropsWireAssistantTwin(t *testing.T) {
	entries := parseAll(t, fixAgentItemA2, fixWireA2)
	if len(entries) != 1 || entries[0].UUID != "msg-a2" || entries[0].Text != "Build fixed." {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParserDropsWireAssistantTwinIgnoringWhitespace(t *testing.T) {
	entries := parseAll(t, fixAgentItemA2, fixWireA2WS)
	if len(entries) != 1 || entries[0].UUID != "msg-a2" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParserKeepsWireOnlyAssistantReply(t *testing.T) {
	entries := parseAll(t, fixWireA2)
	if len(entries) != 1 || entries[0].Role != thinkt.RoleAssistant || entries[0].Text != "Build fixed." || entries[0].UUID != "msg-a2" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParserKeepsWireAssistantAfterDifferentItem(t *testing.T) {
	other := `{"timestamp":"2026-10-01T10:00:14Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"AgentMessage","id":"msg-a1","content":[{"type":"Text","text":"Checking."}],"phase":"commentary"}}}`
	entries := parseAll(t, other, fixWireA2)
	if len(entries) != 2 || entries[1].Text != "Build fixed." {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParserDropsWireReasoningTwin(t *testing.T) {
	item := `{"timestamp":"2026-10-01T10:00:02Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"Reasoning","id":"rs-2","summary_text":[{"type":"summary_text","text":"why"}],"raw_content":[]}}}`
	wire := `{"timestamp":"2026-10-01T10:00:02Z","type":"response_item","payload":{"type":"reasoning","id":"rs-2","summary":[{"type":"summary_text","text":"why "}],"content":null,"encrypted_content":"xxx"}}`
	entries := parseAll(t, item, wire)
	if len(entries) != 1 || entries[0].UUID != "rs-2" || entries[0].ContentBlocks[0].Thinking != "why" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParserStampsTurnModelAndEffort(t *testing.T) {
	ctx := `{"timestamp":"2026-10-01T10:00:01Z","type":"turn_context","payload":{"turn_id":"t1","cwd":"/work/proj","model":"gpt-5.5","effort":"medium"}}`
	item := `{"timestamp":"2026-10-01T10:00:03Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"msg-a1","content":[{"type":"Text","text":"Checking the build."}],"phase":"commentary"}}}`
	entries := parseAll(t, ctx, item)
	if len(entries) != 1 || entries[0].Model != "gpt-5.5" || entries[0].Effort != "medium" || entries[0].TurnID != "t1" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParserEncryptedReasoningProducesNothing(t *testing.T) {
	item := `{"timestamp":"2026-10-01T10:00:02Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"Reasoning","id":"rs-1","summary_text":[],"raw_content":[]}}}`
	wire := `{"timestamp":"2026-10-01T10:00:02Z","type":"response_item","payload":{"type":"reasoning","id":"rs-1","summary":[],"content":null,"encrypted_content":"xxx"}}`
	if entries := parseAll(t, item, wire); len(entries) != 0 {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParserUnknownTypesTally(t *testing.T) {
	p := NewParser(strings.NewReader(`{"timestamp":"2026-10-01T10:00:19Z","type":"mystery_record","payload":{"x":1}}`+"\n"+`{"timestamp":"2026-10-01T10:00:19Z","type":"mystery_record","payload":{"x":2}}`), "sess")
	n := 0
	for {
		_, err := p.NextEntry()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 2 || p.UnknownTypes()["mystery_record"] != 2 {
		t.Fatalf("n = %d, unknown = %v", n, p.UnknownTypes())
	}
	if fresh := NewParser(strings.NewReader(""), "other"); fresh.UnknownTypes() != nil {
		t.Fatalf("new parser tally = %v", fresh.UnknownTypes())
	}
}

func TestParserPendingFlushedBeforeLegacyEvent(t *testing.T) {
	item := `{"timestamp":"2026-10-01T10:00:03Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"msg-a1","content":[{"type":"Text","text":"first"}]}}}`
	legacy := `{"timestamp":"2026-10-01T10:00:04Z","type":"event_msg","payload":{"type":"user_message","message":"second"}}`
	entries := parseAll(t, item, legacy)
	if len(entries) != 2 || entries[0].Text != "first" || entries[1].Text != "second" {
		t.Fatalf("entries = %+v", entries)
	}
}
