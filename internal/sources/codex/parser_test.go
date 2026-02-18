package codex

import (
	"io"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestParser_NextEntry(t *testing.T) {
	input := strings.Join([]string{
		`{"timestamp":"2026-02-10T00:00:00Z","type":"session_meta","payload":{"id":"sess-1","cwd":"/tmp/proj"}}`,
		`{"timestamp":"2026-02-10T00:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"hello"}}`,
		`{"timestamp":"2026-02-10T00:00:02Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"thinking..."}}`,
		`{"timestamp":"2026-02-10T00:00:03Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"call_1","arguments":"{\"cmd\":\"pwd\"}"}}`,
		`{"timestamp":"2026-02-10T00:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_1","output":"ok"}}`,
	}, "\n")

	p := NewParser(strings.NewReader(input), "sess-1")

	e1, err := p.NextEntry()
	if err != nil {
		t.Fatalf("first entry: %v", err)
	}
	if e1.Role != thinkt.RoleUser || e1.Text != "hello" {
		t.Fatalf("unexpected first entry: %+v", e1)
	}

	e2, err := p.NextEntry()
	if err != nil {
		t.Fatalf("second entry: %v", err)
	}
	if e2.Role != thinkt.RoleAssistant || len(e2.ContentBlocks) != 1 || e2.ContentBlocks[0].Type != "thinking" {
		t.Fatalf("unexpected second entry: %+v", e2)
	}

	e3, err := p.NextEntry()
	if err != nil {
		t.Fatalf("third entry: %v", err)
	}
	if e3.Role != thinkt.RoleAssistant || len(e3.ContentBlocks) != 1 || e3.ContentBlocks[0].Type != "tool_use" {
		t.Fatalf("unexpected third entry: %+v", e3)
	}
	if e3.ContentBlocks[0].ToolName != "exec_command" {
		t.Fatalf("unexpected tool name: %+v", e3.ContentBlocks[0])
	}

	e4, err := p.NextEntry()
	if err != nil {
		t.Fatalf("fourth entry: %v", err)
	}
	if e4.Role != thinkt.RoleTool || len(e4.ContentBlocks) != 1 || e4.ContentBlocks[0].ToolResult != "ok" {
		t.Fatalf("unexpected fourth entry: %+v", e4)
	}
}

func TestParser_DeduplicatesEventMessageWhenResponseMessageMatches(t *testing.T) {
	input := strings.Join([]string{
		`{"timestamp":"2026-02-10T00:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"same text"}}`,
		`{"timestamp":"2026-02-10T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"same text"}]}}`,
		`{"timestamp":"2026-02-10T00:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"assistant line"}}`,
		`{"timestamp":"2026-02-10T00:00:03Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"assistant line"}]}}`,
		`{"timestamp":"2026-02-10T00:00:04Z","type":"response_item","payload":{"type":"reasoning","text":"thinking..."}}`,
	}, "\n")

	p := NewParser(strings.NewReader(input), "sess-dup")

	var entries []*thinkt.Entry
	for {
		e, err := p.NextEntry()
		if err != nil {
			if err != io.EOF {
				t.Fatalf("NextEntry returned unexpected error: %v", err)
			}
			break
		}
		entries = append(entries, e)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 deduplicated entries, got %d", len(entries))
	}
	if entries[0].Role != thinkt.RoleUser || entries[0].Text != "same text" {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
	if entries[1].Role != thinkt.RoleAssistant || entries[1].Text != "assistant line" {
		t.Fatalf("unexpected second entry: %+v", entries[1])
	}
	if len(entries[2].ContentBlocks) != 1 || entries[2].ContentBlocks[0].Type != "thinking" {
		t.Fatalf("unexpected third entry: %+v", entries[2])
	}
}

func TestParser_DeduplicatesEventReasoningWhenResponseReasoningMatches(t *testing.T) {
	input := strings.Join([]string{
		`{"timestamp":"2026-02-10T00:00:00Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"thinking once"}}`,
		`{"timestamp":"2026-02-10T00:00:01Z","type":"response_item","payload":{"type":"reasoning","text":"thinking once"}}`,
		`{"timestamp":"2026-02-10T00:00:02Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}}`,
	}, "\n")

	p := NewParser(strings.NewReader(input), "sess-reasoning-dup")

	var entries []*thinkt.Entry
	for {
		e, err := p.NextEntry()
		if err != nil {
			if err != io.EOF {
				t.Fatalf("NextEntry returned unexpected error: %v", err)
			}
			break
		}
		entries = append(entries, e)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 deduplicated entries, got %d", len(entries))
	}
	if len(entries[0].ContentBlocks) != 1 || entries[0].ContentBlocks[0].Type != "thinking" || entries[0].ContentBlocks[0].Thinking != "thinking once" {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
	if entries[1].Role != thinkt.RoleAssistant || entries[1].Text != "answer" {
		t.Fatalf("unexpected second entry: %+v", entries[1])
	}
}
