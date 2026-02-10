package codex

import (
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
