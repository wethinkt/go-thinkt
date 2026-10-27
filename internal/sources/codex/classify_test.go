package codex

import (
	"bufio"
	"os"
	"testing"
)

func TestClassifyLineTable(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		key   string
		class RecordClass
	}{
		{"user text", `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["user.text"]}}}`, "response_item/message", ClassTurn},
		{"user env context by kind", `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"x"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["environments.environment_context"]}}}`, "response_item/message", ClassContext},
		{"user env context by tag", `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n</environment_context>"}]}}`, "response_item/message", ClassContext},
		{"user no kinds plain", `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"plain prompt"}]}}`, "response_item/message", ClassTurn},
		{"developer", `{"type":"response_item","payload":{"type":"message","role":"developer","content":[]}}`, "response_item/message", ClassContext},
		{"assistant wire", `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"a"}]}}`, "response_item/message", ClassTurn},
		{"item user message", `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"i1"}}}`, "item/UserMessage", ClassBookkeeping},
		{"item reasoning empty", `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"Reasoning","id":"r1","summary_text":[]}}}`, "item/Reasoning", ClassBookkeeping},
		{"item reasoning text", `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"Reasoning","id":"r1","summary_text":[{"type":"summary_text","text":"why"}]}}}`, "item/Reasoning", ClassTurn},
		{"wire reasoning encrypted", `{"type":"response_item","payload":{"type":"reasoning","summary":[],"encrypted_content":"x"}}`, "response_item/reasoning", ClassBookkeeping},
		{"extension sleep", `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"Extension","kind":"clock.sleep","id":"c1","durationMs":5}}}`, "item/Extension", ClassTurn},
		{"extension other", `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"Extension","kind":"future.thing","id":"c2"}}}`, "item/Extension", ClassUnknown},
		{"subagent activity", `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"SubAgentActivity","id":"s1","kind":"started"}}}`, "item/SubAgentActivity", ClassEvent},
		{"wire exec call", `{"type":"response_item","payload":{"type":"custom_tool_call","call_id":"c","name":"exec","input":"x"}}`, "response_item/custom_tool_call", ClassEvent},
		{"wire exec_command call", `{"type":"response_item","payload":{"type":"function_call","call_id":"c","name":"exec_command","arguments":"{}"}}`, "response_item/function_call", ClassTurn},
		{"task complete", `{"type":"event_msg","payload":{"type":"task_complete","turn_id":"t"}}`, "event_msg/task_complete", ClassEvent},
		{"task started", `{"type":"event_msg","payload":{"type":"task_started","turn_id":"t"}}`, "event_msg/task_started", ClassBookkeeping},
		{"compacted", `{"type":"compacted","payload":{}}`, "compacted", ClassEvent},
		{"usage", `{"type":"token_usage_record","payload":{}}`, "token_usage_record", ClassEvent},
		{"turn context", `{"type":"turn_context","payload":{"turn_id":"t"}}`, "turn_context", ClassSessionState},
		{"session meta", `{"type":"session_meta","payload":{"id":"s"}}`, "session_meta", ClassSessionState},
		{"legacy user", `{"type":"event_msg","payload":{"type":"user_message","message":"hi"}}`, "event_msg/user_message", ClassTurn},
		{"unknown top", `{"type":"mystery","payload":{}}`, "mystery", ClassUnknown},
		{"not json", `nope`, "", ClassBookkeeping},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			head, class := ClassifyLine([]byte(tc.line))
			if head.Key() != tc.key || class != tc.class {
				t.Fatalf("got (%q, %s), want (%q, %s)", head.Key(), class, tc.key, tc.class)
			}
		})
	}
}

func TestEntryWeightCountsFixture(t *testing.T) {
	f, err := os.Open("testdata/rollout-fixture.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	total, lines := 0, 0
	for sc.Scan() {
		lines++
		head, class := ClassifyLine(sc.Bytes())
		total += entryWeight(head, class)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if lines != 49 {
		t.Fatalf("fixture has %d lines, want 49", lines)
	}
	if total != 39 {
		t.Fatalf("entryWeight sum = %d, want 39", total)
	}
}

func TestUnknownTallyLogsOnce(t *testing.T) {
	var u unknownTally
	u.note("mystery", "line", 1)
	u.note("mystery", "line", 2)
	if got := u.snapshot(); got["mystery"] != 2 {
		t.Fatalf("snapshot = %v", got)
	}
}
