package kimi

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// readFixture parses testdata/<name> to EOF with the given session id.
func readFixture(t *testing.T, name, sessionID string) []thinkt.Entry {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return drain(t, NewParser(f, sessionID))
}

func drain(t *testing.T, p *Parser) []thinkt.Entry {
	t.Helper()
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

func parseLines(t *testing.T, lines ...string) []thinkt.Entry {
	t.Helper()
	return drain(t, NewParser(strings.NewReader(strings.Join(lines, "\n")), "sess"))
}

func blockTypes(e thinkt.Entry) string {
	var ts []string
	for _, b := range e.ContentBlocks {
		ts = append(ts, b.Type)
	}
	return strings.Join(ts, ",")
}

func TestParserFixture15Table(t *testing.T) {
	entries := readFixture(t, "wire-1.5.jsonl", "session_fix1")
	if len(entries) != 17 {
		t.Fatalf("entries = %d, want 17", len(entries))
	}
	s := true
	type row struct {
		role   thinkt.Role
		kind   thinkt.EntryKind
		sec    bool
		uuid   string // "" = not asserted
		text   string // "" = not asserted
		blocks string // "" = not asserted
	}
	want := []row{
		{thinkt.RoleUser, thinkt.KindNone, false, "msg_u1", "fix the build", ""},
		{thinkt.RoleUser, thinkt.KindMeta, s, "", "", ""},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "step-1", "Running the build.", "thinking,text,tool_use,tool_use"},
		{thinkt.RoleSystem, thinkt.KindAgentMessage, false, "", "spawned explore: Explore", ""},
		{thinkt.RoleUser, thinkt.KindNone, false, "tool_b1:result", "", "tool_result"},
		{thinkt.RoleUser, thinkt.KindNone, false, "tool_a1:result", "", "tool_result"},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "step-2", "Build is fixed.", "text"},
		{thinkt.RoleAssistant, thinkt.KindAPIError, s, "step-3", "[provider.auth_error] 403", ""},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "step-4", "Partial answer", "text"},
		{thinkt.RoleSystem, thinkt.KindTurnDuration, s, "", "", ""},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "", "", "thinking"},
		{thinkt.RoleUser, thinkt.KindTaskNotification, s, "", "", ""},
		{thinkt.RoleUser, thinkt.KindTaskNotification, s, "", "", ""},
		{thinkt.RoleUser, thinkt.KindMeta, s, "msg_u2", "", ""},
		{thinkt.RoleUser, thinkt.KindNone, false, "msg_u3", "Review the code structure", ""},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "step-5", "Unclosed", "text"},
		{thinkt.RoleSystem, thinkt.KindUnknown, s, "", "", ""},
	}
	for i, w := range want {
		e := entries[i]
		if e.Role != w.role || e.Kind != w.kind || e.IsSecondary() != w.sec {
			t.Errorf("row %d: role/kind/secondary = %s/%q/%v, want %s/%q/%v (uuid %s)", i+1, e.Role, e.Kind, e.IsSecondary(), w.role, w.kind, w.sec, e.UUID)
		}
		if w.uuid != "" && e.UUID != w.uuid {
			t.Errorf("row %d: uuid = %q, want %q", i+1, e.UUID, w.uuid)
		}
		if w.text != "" && e.Text != w.text {
			t.Errorf("row %d: text = %q, want %q", i+1, e.Text, w.text)
		}
		if w.blocks != "" && blockTypes(e) != w.blocks {
			t.Errorf("row %d: blocks = %q, want %q", i+1, blockTypes(e), w.blocks)
		}
	}

	// Row 1: before any turn; row 2 after agent.turn.started 0.
	if entries[0].TurnID != "" || entries[1].TurnID != "0" || entries[1].Subtype != "injection" {
		t.Errorf("turn ids: %q %q subtype %q", entries[0].TurnID, entries[1].TurnID, entries[1].Subtype)
	}
	// Row 3: step entry details.
	step1 := entries[2]
	if step1.TurnID != "0" || step1.Model != "kimi-code/k3" || step1.Effort != "high" || step1.StopReason != "tool_use" || step1.Metadata["message_id"] != "chatcmpl-1" {
		t.Errorf("step-1 = %+v", step1)
	}
	if u := step1.Usage; u == nil || u.InputTokens != 100 || u.OutputTokens != 20 || u.CacheReadInputTokens != 40 || u.CacheCreationInputTokens != 5 {
		t.Errorf("step-1 usage = %+v", step1.Usage)
	}
	b := step1.ContentBlocks
	if b[0].Thinking != "Need to build." || b[1].Text != "Running the build." || b[2].ToolUseID != "tool_b1" || b[2].ToolName != "Bash" || b[3].ToolUseID != "tool_a1" || b[3].ToolName != "Agent" {
		t.Errorf("step-1 blocks = %+v", b)
	}
	if in, _ := b[2].ToolInput.(map[string]any); in["command"] != "go build ./..." {
		t.Errorf("tool_b1 input = %#v", b[2].ToolInput)
	}
	if b[3].ChildSessionID != "session_fix1/agent-0" {
		t.Errorf("tool_a1 block ChildSessionID = %q", b[3].ChildSessionID)
	}
	// Row 4: spawn event.
	if entries[3].ChildSessionID != "session_fix1/agent-0" || entries[3].Subtype != "spawned" {
		t.Errorf("spawned = %+v", entries[3])
	}
	// Row 5: approved result with metadata.
	r1 := entries[4]
	if r1.ContentBlocks[0].ToolResult != "ok" || r1.ContentBlocks[0].ToolUseID != "tool_b1" || r1.Permission == nil || r1.Permission.Decision != "approved" || r1.Permission.Source != "user" || r1.Metadata["scope"] != "session" || r1.Metadata["duration_ms"] != int64(12) {
		t.Errorf("tool_b1 result = %+v perm = %+v", r1, r1.Permission)
	}
	// Row 6: array output, image child, child session link on entry and block.
	r2 := entries[5]
	if r2.ContentBlocks[0].ToolResult != "<image path=\"/x.png\">\ndone" || len(r2.ContentBlocks[0].Children) != 1 || r2.ContentBlocks[0].Children[0].Type != "image" || r2.ChildSessionID != "session_fix1/agent-0" || r2.ContentBlocks[0].ChildSessionID != "session_fix1/agent-0" {
		t.Errorf("tool_a1 result = %+v", r2)
	}
	// Row 7 usage; row 8 and 9 errors.
	if u := entries[6].Usage; u == nil || u.InputTokens != 50 || u.OutputTokens != 10 || entries[6].StopReason != "end_turn" {
		t.Errorf("step-2 = %+v", entries[6])
	}
	if e := entries[7]; e.Error != "[provider.auth_error] 403" || e.StopReason != "error" || e.Usage != nil || len(e.ContentBlocks) != 0 {
		t.Errorf("step-3 = %+v", e)
	}
	if e := entries[8]; e.Error != "Timed out" || e.StopReason != "interrupted" || e.Usage != nil {
		t.Errorf("step-4 = %+v", e)
	}
	if entries[9].Metadata["duration_ms"] != int64(9000) {
		t.Errorf("turn.ended = %+v", entries[9])
	}
	if entries[10].ContentBlocks[0].Thinking != "leftover" {
		t.Errorf("salvaged = %+v", entries[10])
	}
	if entries[11].Subtype != "background_task" || entries[11].TurnID != "0" || entries[12].Subtype != "notify" || entries[12].TurnID != "1" || !strings.Contains(entries[12].Text, "bash-2") {
		t.Errorf("notifications = %+v / %+v", entries[11], entries[12])
	}
	if entries[13].Subtype != "system_trigger" || entries[14].Subtype != "system_trigger" {
		t.Errorf("triggers = %+v / %+v", entries[13], entries[14])
	}
	if e := entries[15]; e.TurnID != "1" || e.Usage != nil || e.StopReason != "" {
		t.Errorf("step-5 = %+v", e)
	}
	if !strings.HasPrefix(entries[16].Text, `{"type":"mystery.record"`) {
		t.Errorf("unknown = %+v", entries[16])
	}
}

func TestParserFixture15Counts(t *testing.T) {
	entries := readFixture(t, "wire-1.5.jsonl", "session_fix1")
	if len(entries) != 17 {
		t.Fatalf("entries = %d", len(entries))
	}
	secondary := 0
	seen := map[string]bool{}
	uses := map[string]bool{}
	results := map[string]bool{}
	for _, e := range entries {
		if e.IsSecondary() {
			secondary++
		}
		if e.Text == "" && len(e.ContentBlocks) == 0 {
			t.Errorf("empty entry %s", e.UUID)
		}
		if seen[e.UUID] {
			t.Errorf("duplicate UUID %s", e.UUID)
		}
		seen[e.UUID] = true
		for _, b := range e.ContentBlocks {
			switch b.Type {
			case "tool_use":
				uses[b.ToolUseID] = true
			case "tool_result":
				results[b.ToolUseID] = true
			}
		}
	}
	if secondary != 7 {
		t.Errorf("secondary = %d, want 7", secondary)
	}
	for id := range uses {
		if !results[id] {
			t.Errorf("tool_use %s has no tool_result", id)
		}
	}
}

func TestParserFixture10(t *testing.T) {
	entries := readFixture(t, "wire-1.0.jsonl", "ses_fix0")
	if len(entries) != 5 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Kind != thinkt.KindCompactSummary || !entries[0].IsSecondary() {
		t.Errorf("row 1 = %+v", entries[0])
	}
	if entries[1].Text != "hello legacy" || entries[1].Kind != thinkt.KindNone {
		t.Errorf("row 2 = %+v", entries[1])
	}
	if blockTypes(entries[2]) != "thinking,text,tool_use" || entries[2].ContentBlocks[2].ToolUseID != "tool_l1" {
		t.Errorf("row 3 = %+v", entries[2])
	}
	if entries[3].UUID != "tool_l1:result" || !entries[3].ContentBlocks[0].IsError {
		t.Errorf("row 4 = %+v", entries[3])
	}
	if entries[4].Text != "Done" || entries[4].Role != thinkt.RoleAssistant {
		t.Errorf("row 5 = %+v", entries[4])
	}
	for _, e := range entries {
		if !e.Timestamp.IsZero() {
			t.Errorf("1.0 entry %s has a timestamp", e.UUID)
		}
	}
}

func TestParserFixture14(t *testing.T) {
	entries := readFixture(t, "wire-1.4.jsonl", "session_fix4")
	if len(entries) != 3 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Text != "hi 1.4" || entries[0].Role != thinkt.RoleUser || !strings.HasPrefix(entries[0].UUID, "session_fix4:") {
		t.Errorf("row 1 = %+v", entries[0])
	}
	if e := entries[1]; e.UUID != "s14-1" || e.Model != "k2" || e.Effort != "medium" || blockTypes(e) != "text,tool_use" || e.ContentBlocks[1].ToolName != "Read" || e.Usage == nil || e.Usage.InputTokens != 7 {
		t.Errorf("row 2 = %+v", e)
	}
	if e := entries[2]; e.UUID != "tool_x:result" || e.ContentBlocks[0].ToolResult != "contents" {
		t.Errorf("row 3 = %+v", e)
	}
}

func TestParserFlushesErrorStepAtEOF(t *testing.T) {
	entries := parseLines(t,
		`{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s1","turnId":"0","step":1},"time":1}`,
		`{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"s1","turnId":"0","step":1,"finishReason":"error"},"time":2}`,
	)
	if len(entries) != 1 || entries[0].Kind != thinkt.KindAPIError || entries[0].Error != "error" || entries[0].StopReason != "error" || !entries[0].IsSecondary() {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParserApprovalWithoutCallIsIgnored(t *testing.T) {
	entries := parseLines(t,
		`{"type":"permission.record_approval_result","toolCallId":"tool_z","result":{"decision":"rejected","feedback":"no"},"time":1}`,
		`{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s1","turnId":"0","step":1},"time":2}`,
		`{"type":"context.append_loop_event","event":{"type":"tool.call","uuid":"c","turnId":"0","step":1,"stepUuid":"s1","toolCallId":"tool_b","name":"Bash","args":{}},"time":3}`,
		`{"type":"context.append_loop_event","event":{"type":"tool.result","toolCallId":"tool_b","result":{"output":"ok"}},"time":4}`,
		`{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"s1","turnId":"0","step":1,"finishReason":"end_turn","usage":{"inputOther":1,"output":1}},"time":5}`,
	)
	if len(entries) != 2 || entries[1].Permission != nil {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParserMergesConsecutiveParts(t *testing.T) {
	entries := parseLines(t,
		`{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s1","turnId":"0","step":1},"time":1}`,
		`{"type":"context.append_loop_event","event":{"type":"content.part","stepUuid":"s1","part":{"type":"think","think":"a"}},"time":2}`,
		`{"type":"context.append_loop_event","event":{"type":"content.part","stepUuid":"s1","part":{"type":"think","think":"b"}},"time":3}`,
		`{"type":"context.append_loop_event","event":{"type":"content.part","stepUuid":"s1","part":{"type":"text","text":"c"}},"time":4}`,
		`{"type":"context.append_loop_event","event":{"type":"content.part","stepUuid":"s1","part":{"type":"text","text":"d"}},"time":5}`,
		`{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"s1","turnId":"0","step":1,"finishReason":"end_turn"},"time":6}`,
	)
	if len(entries) != 1 || blockTypes(entries[0]) != "thinking,text" || entries[0].ContentBlocks[0].Thinking != "ab" || entries[0].ContentBlocks[1].Text != "cd" || entries[0].Text != "cd" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParserResultWithoutStepEmitsDirectly(t *testing.T) {
	entries := parseLines(t,
		`{"type":"context.append_loop_event","event":{"type":"tool.result","toolCallId":"tool_q","result":{"output":"late"}},"time":1}`,
	)
	if len(entries) != 1 || entries[0].UUID != "tool_q:result" || entries[0].ContentBlocks[0].ToolResult != "late" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParserPromptAppearsOnce(t *testing.T) {
	n := 0
	for _, e := range readFixture(t, "wire-1.5.jsonl", "session_fix1") {
		if e.Text == "fix the build" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("prompt appears %d times", n)
	}
}

func TestParserTimestamps(t *testing.T) {
	entries := readFixture(t, "wire-1.5.jsonl", "session_fix1")
	if got := entries[0].Timestamp; !got.Equal(time.UnixMilli(1790000001000)) {
		t.Fatalf("timestamp = %v", got)
	}
}

// An in-turn (steered) message is written as the context message first, then
// turn.steer with the same text, then the appended input record with the
// same id at turn end: one entry.
func TestParserSteeredPromptAppearsOnce(t *testing.T) {
	entries := parseLines(t,
		`{"turnId":0,"type":"agent.turn.started","time":1,"kind":"event"}`,
		`{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"also check the docs"}],"toolCalls":[],"origin":{"kind":"user","inTurn":true},"id":"msg_s1"},"time":2}`,
		`{"type":"turn.steer","input":[{"type":"text","text":"also check the docs"}],"origin":{"kind":"user","inTurn":true},"time":3}`,
		`{"message":{"message":{"role":"user","content":[{"type":"text","text":"also check the docs"}]},"meta":{"source":"input","origin":{"kind":"user","inTurn":true},"userMessageId":"msg_s1"}},"type":"agent.message.appended","time":4,"kind":"event"}`,
	)
	if len(entries) != 1 || entries[0].UUID != "msg_s1" {
		t.Fatalf("entries = %+v", entries)
	}
}

// A batched assistant record the loop never covered is emitted, and so is
// the batched tool result for its call.
func TestParserAppendedFallbackKeepsItsToolResult(t *testing.T) {
	entries := parseLines(t,
		`{"message":{"message":{"role":"assistant","content":[{"type":"text","text":"checking"}],"toolCalls":[{"type":"function","id":"tool_f1","name":"Bash","arguments":"{\"command\":\"ls\"}"}]},"meta":{"source":"llm","messageId":"chatcmpl-f1"}},"type":"agent.message.appended","time":1,"kind":"event"}`,
		`{"message":{"message":{"role":"tool","content":[{"type":"text","text":"a b c"}],"toolCallId":"tool_f1"},"meta":{"source":"tool"}},"type":"agent.message.appended","time":1,"kind":"event"}`,
	)
	if len(entries) != 2 || entries[1].UUID != "tool_f1:result" || entries[1].ContentBlocks[0].ToolResult != "a b c" {
		t.Fatalf("entries = %+v", entries)
	}
}

// A failed step is usually followed by turn.step.retrying, which carries the
// readable error; a turn.step.interrupted after it still wins.
func TestParserErrorStepTakesRetryingMessage(t *testing.T) {
	entries := parseLines(t,
		`{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s1","turnId":"0","step":1},"time":1}`,
		`{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"s1","turnId":"0","step":1,"finishReason":"error"},"time":2}`,
		`{"type":"turn.step.retrying","agentId":"main","turnId":0,"step":1,"failedAttempt":1,"nextAttempt":2,"errorName":"APIConnectionError","errorMessage":"Connection error.","time":3}`,
		`{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s2","turnId":"0","step":1},"time":4}`,
		`{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"s2","turnId":"0","step":1,"finishReason":"error"},"time":5}`,
		`{"type":"turn.step.retrying","agentId":"main","turnId":0,"step":1,"errorName":"APIConnectionError","errorMessage":"Connection error.","time":6}`,
		`{"type":"turn.step.interrupted","agentId":"main","turnId":0,"step":1,"reason":"error","message":"gave up","time":7}`,
	)
	if len(entries) != 2 || entries[0].Error != "APIConnectionError: Connection error." || entries[1].Error != "gave up" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParserUnknownTypes(t *testing.T) {
	f, err := os.Open("testdata/wire-1.5.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p := NewParser(f, "session_fix1")
	drain(t, p)
	if p.UnknownTypes()["mystery.record"] != 1 {
		t.Fatalf("unknown = %v", p.UnknownTypes())
	}
}
