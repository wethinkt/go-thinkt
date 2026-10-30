package codex

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

const (
	turnReasoning = `{"timestamp":"2026-10-01T10:00:02Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"Reasoning","id":"rs-%s","summary_text":[{"type":"summary_text","text":"thinking %s"}],"raw_content":[]}}}`
	turnMessage   = `{"timestamp":"2026-10-01T10:00:03Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"AgentMessage","id":"msg-%s","content":[{"type":"Text","text":"answer %s"}],"phase":"commentary"}}}`
	turnWireCall  = `{"timestamp":"2026-10-01T10:00:04Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"%s","name":"exec","input":"run()"}}`
	turnFuncCall  = `{"timestamp":"2026-10-01T10:00:04Z","type":"response_item","payload":{"type":"function_call","call_id":"%s","name":"js","arguments":"{}"}}`
	turnCommand   = `{"timestamp":"2026-10-01T10:00:05Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"CommandExecution","id":"%s","command":["/bin/zsh","-lc","go build"],"cwd":"file:///w","status":"completed","stdout":"ok","stderr":"","aggregated_output":"ok","exit_code":0,"duration":{"secs":1,"nanos":0}}}}`
	turnMcp       = `{"timestamp":"2026-10-01T10:00:07Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"McpToolCall","id":"%s","server":"cua_repl","tool":"js","arguments":{},"status":"completed","result":{"content":[{"type":"text","text":"2"}],"isError":false},"duration":{"secs":0,"nanos":1000}}}}`
	turnWireOut   = `{"timestamp":"2026-10-01T10:00:05Z","type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"%s","output":"ok"}}`
	turnUsage     = `{"timestamp":"2026-10-01T10:00:06Z","type":"token_usage_record","payload":{"turn_id":"turn-1","response_id":"%s","usage":{"input_tokens":%d,"output_tokens":1,"total_tokens":%d}}}`
	turnUser      = `{"timestamp":"2026-10-01T10:00:09Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"next please"}]}}`
	turnContext   = `{"timestamp":"2026-10-01T10:00:10Z","type":"turn_context","payload":{"turn_id":"turn-2","cwd":"/w","model":"gpt-5.5","effort":"medium"}}`
)

func f(format string, args ...any) string { return strings.TrimSpace(fmt.Sprintf(format, args...)) }

func usage(id string, tokens int) string { return f(turnUsage, id, tokens, tokens) }

// byUUID indexes entries by UUID.
func byUUID(entries []thinkt.Entry) map[string]thinkt.Entry {
	m := make(map[string]thinkt.Entry, len(entries))
	for _, e := range entries {
		m[e.UUID] = e
	}
	return m
}

func TestResponseEntriesShareResponseID(t *testing.T) {
	entries := parseAll(t, f(turnReasoning, "1", "1"), f(turnMessage, "1", "1"), f(turnWireCall, "call_1"), usage("resp-1", 10))
	if len(entries) != 3 {
		t.Fatalf("entries = %+v", entries)
	}
	for _, e := range entries {
		if e.TurnID != "resp-1" {
			t.Errorf("%s: TurnID %q", e.UUID, e.TurnID)
		}
	}
	m := byUUID(entries)
	if m["msg-1"].PromptID != "turn-1" {
		t.Errorf("item PromptID %q", m["msg-1"].PromptID)
	}
	// Usage goes where it went before the hold: the newest line's
	// assistant entry.
	if m["wire:call_1"].Usage == nil || m["msg-1"].Usage != nil {
		t.Error("usage not on the newest line's assistant entry")
	}
}

func TestAdjacentUsageRecordsApplyToOneHold(t *testing.T) {
	entries := parseAll(t, f(turnReasoning, "1", "1"), f(turnMessage, "1", "1"), usage("resp-1", 10), usage("resp-2", 20), f(turnReasoning, "2", "2"))
	m := byUUID(entries)
	if m["rs-1"].TurnID != "resp-2" || m["msg-1"].TurnID != "resp-2" {
		t.Fatalf("ids: %q %q", m["rs-1"].TurnID, m["msg-1"].TurnID)
	}
	if u := m["msg-1"].Usage; u == nil || u.InputTokens != 20 {
		t.Fatalf("usage = %+v", u)
	}
	if m["rs-2"].TurnID != "" {
		t.Fatalf("next response: %q", m["rs-2"].TurnID)
	}
}

func TestCommandItemInsideHoldTakesResponseID(t *testing.T) {
	entries := parseAll(t, f(turnWireCall, "call_1"), f(turnCommand, "exec-1"), usage("resp-1", 10))
	m := byUUID(entries)
	if m["exec-1"].TurnID != "resp-1" || m["exec-1:result"].TurnID != "resp-1" {
		t.Fatalf("command halves: %q %q", m["exec-1"].TurnID, m["exec-1:result"].TurnID)
	}
}

func TestCommandItemAfterUsageHasNoTurnID(t *testing.T) {
	entries := parseAll(t, f(turnWireCall, "call_1"), usage("resp-1", 10), f(turnCommand, "exec-1"), f(turnWireOut, "call_1"))
	var order []string
	for _, e := range entries {
		order = append(order, e.UUID)
	}
	if strings.Join(order, ",") != "wire:call_1,exec-1,exec-1:result,wire:call_1:result" {
		t.Fatalf("order = %v", order)
	}
	m := byUUID(entries)
	if m["exec-1"].TurnID != "" || m["exec-1:result"].TurnID != "" {
		t.Fatalf("command halves: %q %q", m["exec-1"].TurnID, m["exec-1:result"].TurnID)
	}
	if m["wire:call_1:result"].TurnID != "resp-1" {
		t.Fatalf("wire output: %q", m["wire:call_1:result"].TurnID)
	}
}

func TestMcpToolCallAfterUsageJoinsByCallID(t *testing.T) {
	entries := parseAll(t, f(turnFuncCall, "call_9"), usage("resp-1", 10), f(turnMcp, "call_9"))
	m := byUUID(entries)
	if m["call_9"].TurnID != "resp-1" || m["call_9:result"].TurnID != "resp-1" {
		t.Fatalf("mcp halves: %q %q (%+v)", m["call_9"].TurnID, m["call_9:result"].TurnID, entries)
	}
}

func TestHoldReleasedUnstampedAtEOF(t *testing.T) {
	entries := parseAll(t, f(turnReasoning, "1", "1"), f(turnMessage, "1", "1"))
	if len(entries) != 2 || entries[0].TurnID != "" || entries[1].TurnID != "" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestSecondResponseSharesItsOwnID(t *testing.T) {
	entries := parseAll(t, f(turnReasoning, "1", "1"), f(turnMessage, "1", "1"), usage("resp-1", 10), f(turnReasoning, "2", "2"), f(turnMessage, "2", "2"), usage("resp-2", 10))
	m := byUUID(entries)
	if len(entries) != 4 || m["rs-1"].TurnID != "resp-1" || m["msg-1"].TurnID != "resp-1" || m["rs-2"].TurnID != "resp-2" || m["msg-2"].TurnID != "resp-2" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestUserMessageReleasesHoldUnstamped(t *testing.T) {
	entries := parseAll(t, f(turnReasoning, "1", "1"), turnUser, f(turnMessage, "2", "2"), usage("resp-2", 10))
	if len(entries) != 3 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].TurnID != "" || entries[1].Role != thinkt.RoleUser || entries[1].TurnID != "" {
		t.Fatalf("before: %+v / %+v", entries[0], entries[1])
	}
	if entries[2].TurnID != "resp-2" || entries[2].Usage == nil {
		t.Fatalf("assistant: %+v", entries[2])
	}
}

func TestMappedIDSurvivesLaterUsage(t *testing.T) {
	entries := parseAll(t, f(turnWireCall, "call_1"), usage("resp-1", 10), f(turnReasoning, "2", "2"), f(turnWireOut, "call_1"), usage("resp-2", 10))
	m := byUUID(entries)
	if m["wire:call_1:result"].TurnID != "resp-1" || m["rs-2"].TurnID != "resp-2" {
		t.Fatalf("output %q reasoning %q", m["wire:call_1:result"].TurnID, m["rs-2"].TurnID)
	}
}

func TestTurnContextReleasesHold(t *testing.T) {
	entries := parseAll(t, f(turnReasoning, "1", "1"), f(turnMessage, "1", "1"), turnContext, usage("resp-x", 10))
	if len(entries) != 2 || entries[0].UUID != "rs-1" || entries[1].UUID != "msg-1" || entries[0].TurnID != "" || entries[1].TurnID != "" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestTwinDropUnchangedByHold(t *testing.T) {
	entries := parseAll(t, fixAgentItemA2, fixWireA2, usage("resp-1", 10))
	if len(entries) != 1 || entries[0].UUID != "msg-a2" || entries[0].TurnID != "resp-1" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestRolloutWithoutUsageRecords(t *testing.T) {
	data, err := os.ReadFile("testdata/rollout-fixture.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if !strings.Contains(l, `"type":"token_usage_record"`) {
			lines = append(lines, l)
		}
	}
	p := NewParser(strings.NewReader(strings.Join(lines, "\n")), "thr-1")
	var without []thinkt.Entry
	for {
		e, err := p.NextEntry()
		if err != nil {
			break
		}
		without = append(without, *e)
	}
	with := readFixture(t)
	if len(without) != len(with) {
		t.Fatalf("%d entries without usage records, %d with", len(without), len(with))
	}
	for i := range without {
		a, b := without[i], with[i]
		// Line-based UUIDs shift because the usage lines are gone.
		if a.Role != b.Role || a.Kind != b.Kind || a.Text != b.Text || a.PromptID != b.PromptID {
			t.Errorf("row %d: %s/%s differs", i, a.UUID, b.UUID)
		}
		if a.TurnID != "" {
			t.Errorf("row %d: TurnID %q without usage records", i, a.TurnID)
		}
	}
}
