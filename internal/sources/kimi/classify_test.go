package kimi

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestClassifyLineTable(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		key   string
		class RecordClass
	}{
		{"metadata", `{"type":"metadata","protocol_version":"1.5"}`, "metadata", ClassSessionState},
		{"step begin", `{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s","turnId":"0"}}`, "loop/step.begin", ClassEvent},
		{"step end", `{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"s","turnId":"0","finishReason":"end_turn"}}`, "loop/step.end", ClassEvent},
		{"interrupted", `{"type":"turn.step.interrupted","turnId":0,"step":3,"reason":"error","message":"x"}`, "turn.step.interrupted", ClassEvent},
		{"part", `{"type":"context.append_loop_event","event":{"type":"content.part","stepUuid":"s","part":{"type":"text","text":"x"}}}`, "loop/content.part", ClassTurn},
		{"call", `{"type":"context.append_loop_event","event":{"type":"tool.call","toolCallId":"t","name":"Bash","args":{}}}`, "loop/tool.call", ClassTurn},
		{"result", `{"type":"context.append_loop_event","event":{"type":"tool.result","toolCallId":"t","result":{"output":"o"}}}`, "loop/tool.result", ClassTurn},
		{"appended input user", `{"type":"agent.message.appended","message":{"message":{"role":"user","content":[{"type":"text","text":"hi"}]},"meta":{"source":"input","origin":{"kind":"user"},"userMessageId":"m"}}}`, "appended/user", ClassTurn},
		{"appended notify", `{"type":"agent.message.appended","message":{"message":{"role":"user","content":[{"type":"text","text":"<notification/>"}]},"meta":{"source":"notify"}}}`, "appended/user", ClassContext},
		{"appended assistant", `{"type":"agent.message.appended","message":{"message":{"role":"assistant","content":[{"type":"text","text":"a"}]},"meta":{"source":"llm","messageId":"c"}}}`, "appended/assistant", ClassTurn},
		{"appended tool", `{"type":"agent.message.appended","message":{"message":{"role":"tool","content":[{"type":"text","text":"o"}],"toolCallId":"t"},"meta":{"source":"tool"}}}`, "appended/tool", ClassTurn},
		{"ctx user", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"hi"}],"origin":{"kind":"user"},"id":"m"}}`, "ctx/user", ClassTurn},
		{"ctx injection", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"<system-reminder>x"}],"origin":{"kind":"injection"}}}`, "ctx/user", ClassContext},
		{"ctx git context trigger", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"<git-context>\nx"}],"origin":{"kind":"system_trigger"},"id":"m"}}`, "ctx/user", ClassContext},
		{"ctx task trigger", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"Review this"}],"origin":{"kind":"system_trigger"},"id":"m"}}`, "ctx/user", ClassTurn},
		{"ctx 1.0 compaction", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"<system>Previous context has been compacted."}]}}`, "ctx/user", ClassContext},
		{"ctx 1.0 plain", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"plain"}]}}`, "ctx/user", ClassTurn},
		{"ctx shell command", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"!ls"}],"origin":{"kind":"shell_command"}}}`, "ctx/user", ClassTurn},
		{"ctx assistant", `{"type":"context.append_message","message":{"role":"assistant","content":[{"type":"text","text":"a"}],"toolCalls":[]}}`, "ctx/assistant", ClassTurn},
		{"ctx tool", `{"type":"context.append_message","message":{"role":"tool","content":[{"type":"text","text":"o"}],"toolCallId":"t"}}`, "ctx/tool", ClassTurn},
		{"steer", `{"type":"turn.steer","input":[{"type":"text","text":"<notification/>"}],"origin":{"kind":"background_task"}}`, "turn.steer", ClassContext},
		{"turn prompt", `{"type":"turn.prompt","input":[{"type":"text","text":"hi"}]}`, "turn.prompt", ClassBookkeeping},
		{"turn ended", `{"type":"turn.ended","turnId":0,"reason":"completed","durationMs":1}`, "turn.ended", ClassEvent},
		{"approval", `{"type":"permission.record_approval_result","toolCallId":"t","result":{"decision":"approved"}}`, "permission.record_approval_result", ClassEvent},
		{"spawned", `{"type":"subagent.spawned","subagentId":"agent-0","parentToolCallId":"t"}`, "subagent.spawned", ClassEvent},
		{"subagent completed", `{"type":"subagent.completed","subagentId":"agent-0","resultSummary":"r"}`, "subagent.completed", ClassEvent},
		{"subagent started", `{"type":"subagent.started","subagentId":"agent-0"}`, "subagent.started", ClassBookkeeping},
		{"task started", `{"type":"task.started","info":{"taskId":"bash-1","description":"d"}}`, "task.started", ClassEvent},
		{"plan revision", `{"type":"plan.revision","key":"plan/x/v1.md","version":1}`, "plan.revision", ClassEvent},
		{"agent turn started", `{"turnId":0,"type":"agent.turn.started","kind":"event"}`, "agent.turn.started", ClassBookkeeping},
		{"llm request", `{"type":"llm.request","modelAlias":"kimi-code/k3"}`, "llm.request", ClassBookkeeping},
		{"usage record", `{"type":"usage.record","usage":{}}`, "usage.record", ClassBookkeeping},
		{"interaction", `{"type":"interaction.request","kind":"approval"}`, "interaction.request", ClassBookkeeping},
		{"unknown", `{"type":"mystery.record","x":1}`, "mystery.record", ClassUnknown},
		{"not json", `nope`, "", ClassBookkeeping},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			head, class := ClassifyLine([]byte(tc.line))
			if head.Key != tc.key || class != tc.class {
				t.Fatalf("got (%q, %s), want (%q, %s)", head.Key, class, tc.key, tc.class)
			}
		})
	}
}

func TestClassifyLineHeadFields(t *testing.T) {
	h, _ := ClassifyLine([]byte(`{"type":"agent.message.appended","message":{"message":{"role":"user","content":[{"type":"text","text":" hi "},{"type":"text","text":"there"}]},"meta":{"source":"input","origin":{"kind":"user"},"userMessageId":"m"}},"time":1790000001000}`))
	if h.Role != "user" || h.Origin != "user" || h.Source != "input" || h.ID != "m" || h.LeadText != "hi \nthere" || h.Time != 1790000001000 {
		t.Fatalf("appended head = %+v", h)
	}
	h, _ = ClassifyLine([]byte(`{"type":"context.append_loop_event","event":{"type":"tool.call","uuid":"c1","turnId":"0","step":1,"stepUuid":"s","toolCallId":"t","name":"Bash","args":{}}}`))
	if h.ToolCallID != "t" || h.StepUUID != "s" || h.TurnID != "0" {
		t.Fatalf("call head = %+v", h)
	}
	h, _ = ClassifyLine([]byte(`{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s2","turnId":"1","step":1}}`))
	if h.StepUUID != "s2" || h.TurnID != "1" {
		t.Fatalf("step.begin head = %+v", h)
	}
	h, _ = ClassifyLine([]byte(`{"type":"turn.ended","turnId":0,"reason":"completed","durationMs":1}`))
	if h.TurnID != "0" {
		t.Fatalf("turn.ended head = %+v", h)
	}
	h, _ = ClassifyLine([]byte(`{"type":"agent.message.appended","message":{"message":{"role":"assistant","content":[{"type":"text","text":"a"}]},"meta":{"source":"llm","messageId":"c"}}}`))
	if h.ID != "c" || h.Role != "assistant" {
		t.Fatalf("appended assistant head = %+v", h)
	}
	h, _ = ClassifyLine([]byte(`{"type":"context.append_message","message":{"role":"tool","content":[{"type":"text","text":"o"}],"toolCallId":"t"}}`))
	if h.ToolCallID != "t" {
		t.Fatalf("ctx tool head = %+v", h)
	}
	h, _ = ClassifyLine([]byte(`{"type":"permission.record_approval_result","toolCallId":"t","result":{"decision":"approved"}}`))
	if h.ToolCallID != "t" {
		t.Fatalf("approval head = %+v", h)
	}
	h, _ = ClassifyLine([]byte(`{"type":"turn.steer","input":[{"type":"text","text":"<notification/>"}],"origin":{"kind":"background_task"}}`))
	if h.Role != "user" || h.Origin != "background_task" || h.LeadText != "<notification/>" {
		t.Fatalf("steer head = %+v", h)
	}
}

func TestUserClass(t *testing.T) {
	cases := []struct {
		name    string
		head    recordHead
		class   RecordClass
		kind    thinkt.EntryKind
		subtype string
	}{
		{"typed prompt", recordHead{Origin: "user", LeadText: "hi"}, ClassTurn, thinkt.KindNone, ""},
		{"no origin plain", recordHead{LeadText: "hi"}, ClassTurn, thinkt.KindNone, ""},
		{"git context trigger", recordHead{Origin: "system_trigger", LeadText: "<git-context>\nx"}, ClassContext, thinkt.KindMeta, "system_trigger"},
		{"working directory trigger", recordHead{Origin: "system_trigger", LeadText: "Working directory: /w"}, ClassContext, thinkt.KindMeta, "system_trigger"},
		{"continue trigger", recordHead{Origin: "system_trigger", LeadText: "continue"}, ClassContext, thinkt.KindMeta, "system_trigger"},
		{"task trigger", recordHead{Origin: "system_trigger", LeadText: "Review the code"}, ClassTurn, thinkt.KindNone, "system_trigger"},
		{"injection", recordHead{Origin: "injection", LeadText: "<system-reminder>x"}, ClassContext, thinkt.KindMeta, "injection"},
		{"no origin reminder", recordHead{LeadText: "<system-reminder>x"}, ClassContext, thinkt.KindMeta, "injection"},
		{"no origin system", recordHead{LeadText: "<system>something</system>"}, ClassContext, thinkt.KindMeta, "injection"},
		{"compaction", recordHead{LeadText: "<system>Previous context has been compacted. x"}, ClassContext, thinkt.KindCompactSummary, "compaction"},
		{"task", recordHead{Origin: "task", LeadText: "<notification/>"}, ClassContext, thinkt.KindTaskNotification, "task"},
		{"background task", recordHead{Origin: "background_task", LeadText: "<notification/>"}, ClassContext, thinkt.KindTaskNotification, "background_task"},
		{"notify", recordHead{Source: "notify", LeadText: "<notification/>"}, ClassContext, thinkt.KindTaskNotification, "notify"},
		{"skill", recordHead{Origin: "skill_activation", LeadText: "User activated"}, ClassContext, thinkt.KindMeta, "skill_activation"},
		{"shell", recordHead{Origin: "shell_command", LeadText: "!ls"}, ClassTurn, thinkt.KindCommandOutput, "shell_command"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			class, kind, subtype := userClass(tc.head)
			if class != tc.class || kind != tc.kind || subtype != tc.subtype {
				t.Fatalf("got (%s, %q, %q), want (%s, %q, %q)", class, kind, subtype, tc.class, tc.kind, tc.subtype)
			}
		})
	}
}

func TestIsSearchableKimi(t *testing.T) {
	cases := map[string]bool{
		`{"type":"agent.message.appended","message":{"message":{"role":"user","content":[{"type":"text","text":"needle"}]},"meta":{"source":"input","origin":{"kind":"user"},"userMessageId":"m"}}}`: true,
		`{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"needle"}],"origin":{"kind":"user"},"id":"m"}}`:                                                  false,
		`{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"needle"}]}}`:                                                                                    true,
		`{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"<system-reminder>needle"}],"origin":{"kind":"injection"}}}`:                                     false,
		`{"type":"context.append_loop_event","event":{"type":"content.part","part":{"type":"text","text":"needle"}}}`:                                                                                true,
		`{"type":"context.append_loop_event","event":{"type":"tool.call","toolCallId":"t","name":"Bash","args":{"command":"needle"}}}`:                                                               true,
		`{"type":"context.append_loop_event","event":{"type":"tool.result","toolCallId":"t","result":{"output":"needle"}}}`:                                                                          true,
		`{"type":"agent.message.appended","message":{"message":{"role":"assistant","content":[{"type":"text","text":"needle"}]},"meta":{"source":"llm","messageId":"c"}}}`:                           false,
		`{"type":"agent.message.appended","message":{"message":{"role":"user","content":[{"type":"text","text":"needle"}]},"meta":{"source":"notify"}}}`:                                             false,
		`{"type":"turn.prompt","input":[{"type":"text","text":"needle"}]}`:                                                                                                                           false,
		`{"type":"context.append_message","message":{"role":"assistant","content":[{"type":"text","text":"needle"}],"toolCalls":[]}}`:                                                                true,
		`{"type":"context.append_message","message":{"role":"tool","content":[{"type":"text","text":"needle"}],"toolCallId":"t"}}`:                                                                   true,
		`{"type":"usage.record","usage":{"inputOther":1}}`:                                                                                                                                           false,
		`{"type":"mystery.record","x":"needle"}`: false,
		`not json`:                               false,
	}
	for line, want := range cases {
		if got := IsSearchable([]byte(line)); got != want {
			t.Errorf("IsSearchable(%s) = %v, want %v", line, got, want)
		}
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
