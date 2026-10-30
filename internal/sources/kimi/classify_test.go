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
		class thinkt.RecordClass
	}{
		{"metadata", `{"type":"metadata","protocol_version":"1.5"}`, "metadata", thinkt.ClassSessionState},
		{"step begin", `{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s","turnId":"0"}}`, "loop/step.begin", thinkt.ClassEvent},
		{"step end", `{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"s","turnId":"0","finishReason":"end_turn"}}`, "loop/step.end", thinkt.ClassEvent},
		{"interrupted", `{"type":"turn.step.interrupted","turnId":0,"step":3,"reason":"error","message":"x"}`, "turn.step.interrupted", thinkt.ClassEvent},
		{"part", `{"type":"context.append_loop_event","event":{"type":"content.part","stepUuid":"s","part":{"type":"text","text":"x"}}}`, "loop/content.part", thinkt.ClassTurn},
		{"call", `{"type":"context.append_loop_event","event":{"type":"tool.call","toolCallId":"t","name":"Bash","args":{}}}`, "loop/tool.call", thinkt.ClassTurn},
		{"result", `{"type":"context.append_loop_event","event":{"type":"tool.result","toolCallId":"t","result":{"output":"o"}}}`, "loop/tool.result", thinkt.ClassTurn},
		{"appended input user", `{"type":"agent.message.appended","message":{"message":{"role":"user","content":[{"type":"text","text":"hi"}]},"meta":{"source":"input","origin":{"kind":"user"},"userMessageId":"m"}}}`, "appended/user", thinkt.ClassTurn},
		{"appended notify", `{"type":"agent.message.appended","message":{"message":{"role":"user","content":[{"type":"text","text":"<notification/>"}]},"meta":{"source":"notify"}}}`, "appended/user", thinkt.ClassContext},
		{"appended assistant", `{"type":"agent.message.appended","message":{"message":{"role":"assistant","content":[{"type":"text","text":"a"}]},"meta":{"source":"llm","messageId":"c"}}}`, "appended/assistant", thinkt.ClassTurn},
		{"appended tool", `{"type":"agent.message.appended","message":{"message":{"role":"tool","content":[{"type":"text","text":"o"}],"toolCallId":"t"},"meta":{"source":"tool"}}}`, "appended/tool", thinkt.ClassTurn},
		{"ctx user", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"hi"}],"origin":{"kind":"user"},"id":"m"}}`, "ctx/user", thinkt.ClassTurn},
		{"ctx injection", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"<system-reminder>x"}],"origin":{"kind":"injection"}}}`, "ctx/user", thinkt.ClassContext},
		{"ctx git context trigger", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"<git-context>\nx"}],"origin":{"kind":"system_trigger"},"id":"m"}}`, "ctx/user", thinkt.ClassContext},
		{"ctx task trigger", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"Review this"}],"origin":{"kind":"system_trigger"},"id":"m"}}`, "ctx/user", thinkt.ClassTurn},
		{"ctx 1.0 compaction", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"<system>Previous context has been compacted."}]}}`, "ctx/user", thinkt.ClassContext},
		{"ctx 1.0 plain", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"plain"}]}}`, "ctx/user", thinkt.ClassTurn},
		{"ctx shell command", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"!ls"}],"origin":{"kind":"shell_command"}}}`, "ctx/user", thinkt.ClassTurn},
		{"ctx assistant", `{"type":"context.append_message","message":{"role":"assistant","content":[{"type":"text","text":"a"}],"toolCalls":[]}}`, "ctx/assistant", thinkt.ClassTurn},
		{"ctx tool", `{"type":"context.append_message","message":{"role":"tool","content":[{"type":"text","text":"o"}],"toolCallId":"t"}}`, "ctx/tool", thinkt.ClassTurn},
		{"steer", `{"type":"turn.steer","input":[{"type":"text","text":"<notification/>"}],"origin":{"kind":"background_task"}}`, "turn.steer", thinkt.ClassContext},
		{"turn prompt", `{"type":"turn.prompt","input":[{"type":"text","text":"hi"}]}`, "turn.prompt", thinkt.ClassBookkeeping},
		{"turn ended", `{"type":"turn.ended","turnId":0,"reason":"completed","durationMs":1}`, "turn.ended", thinkt.ClassEvent},
		{"approval", `{"type":"permission.record_approval_result","toolCallId":"t","result":{"decision":"approved"}}`, "permission.record_approval_result", thinkt.ClassEvent},
		{"spawned", `{"type":"subagent.spawned","subagentId":"agent-0","parentToolCallId":"t"}`, "subagent.spawned", thinkt.ClassEvent},
		{"subagent completed", `{"type":"subagent.completed","subagentId":"agent-0","resultSummary":"r"}`, "subagent.completed", thinkt.ClassEvent},
		{"subagent started", `{"type":"subagent.started","subagentId":"agent-0"}`, "subagent.started", thinkt.ClassBookkeeping},
		{"task started", `{"type":"task.started","info":{"taskId":"bash-1","description":"d"}}`, "task.started", thinkt.ClassEvent},
		{"plan revision", `{"type":"plan.revision","key":"plan/x/v1.md","version":1}`, "plan.revision", thinkt.ClassEvent},
		{"agent turn started", `{"turnId":0,"type":"agent.turn.started","kind":"event"}`, "agent.turn.started", thinkt.ClassBookkeeping},
		{"llm request", `{"type":"llm.request","modelAlias":"kimi-code/k3"}`, "llm.request", thinkt.ClassBookkeeping},
		{"usage record", `{"type":"usage.record","usage":{}}`, "usage.record", thinkt.ClassBookkeeping},
		{"interaction", `{"type":"interaction.request","kind":"approval"}`, "interaction.request", thinkt.ClassBookkeeping},
		{"unknown", `{"type":"mystery.record","x":1}`, "mystery.record", thinkt.ClassUnknown},
		{"not json", `nope`, "", thinkt.ClassBookkeeping},
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
		class   thinkt.RecordClass
		kind    thinkt.EntryKind
		subtype string
	}{
		{"typed prompt", recordHead{Origin: "user", LeadText: "hi"}, thinkt.ClassTurn, thinkt.KindNone, ""},
		{"no origin plain", recordHead{LeadText: "hi"}, thinkt.ClassTurn, thinkt.KindNone, ""},
		{"git context trigger", recordHead{Origin: "system_trigger", LeadText: "<git-context>\nx"}, thinkt.ClassContext, thinkt.KindMeta, "system_trigger"},
		{"working directory trigger", recordHead{Origin: "system_trigger", LeadText: "Working directory: /w"}, thinkt.ClassContext, thinkt.KindMeta, "system_trigger"},
		{"continue trigger", recordHead{Origin: "system_trigger", LeadText: "continue"}, thinkt.ClassContext, thinkt.KindMeta, "system_trigger"},
		{"task trigger", recordHead{Origin: "system_trigger", LeadText: "Review the code"}, thinkt.ClassTurn, thinkt.KindNone, "system_trigger"},
		{"injection", recordHead{Origin: "injection", LeadText: "<system-reminder>x"}, thinkt.ClassContext, thinkt.KindMeta, "injection"},
		{"no origin reminder", recordHead{LeadText: "<system-reminder>x"}, thinkt.ClassContext, thinkt.KindMeta, "injection"},
		{"no origin system", recordHead{LeadText: "<system>something</system>"}, thinkt.ClassContext, thinkt.KindMeta, "injection"},
		{"compaction", recordHead{LeadText: "<system>Previous context has been compacted. x"}, thinkt.ClassContext, thinkt.KindCompactSummary, "compaction"},
		{"task", recordHead{Origin: "task", LeadText: "<notification/>"}, thinkt.ClassContext, thinkt.KindTaskNotification, "task"},
		{"background task", recordHead{Origin: "background_task", LeadText: "<notification/>"}, thinkt.ClassContext, thinkt.KindTaskNotification, "background_task"},
		{"notify", recordHead{Source: "notify", LeadText: "<notification/>"}, thinkt.ClassContext, thinkt.KindTaskNotification, "notify"},
		{"skill", recordHead{Origin: "skill_activation", LeadText: "User activated"}, thinkt.ClassContext, thinkt.KindMeta, "skill_activation"},
		{"shell", recordHead{Origin: "shell_command", LeadText: "!ls"}, thinkt.ClassTurn, thinkt.KindCommandOutput, "shell_command"},
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
		`{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"needle"}],"origin":{"kind":"user"},"id":"m"}}`:                                                  true,
		`{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"!needle"}],"origin":{"kind":"shell_command"}}}`:                                                 false,
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

// The per-file filter keeps whichever of a prompt's two records comes first.
func TestNewSearchFilterKeepsFirstOfTwins(t *testing.T) {
	appended := []byte(`{"type":"agent.message.appended","message":{"message":{"role":"user","content":[{"type":"text","text":"needle"}]},"meta":{"source":"input","origin":{"kind":"user"},"userMessageId":"m"}}}`)
	ctx := []byte(`{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"needle"}],"origin":{"kind":"user"},"id":"m"}}`)
	f := NewSearchFilter()
	if !f(appended) || f(ctx) {
		t.Fatal("appended first: appended kept, context twin dropped")
	}
	g := NewSearchFilter()
	if !g(ctx) || g(appended) {
		t.Fatal("context first: context kept, appended twin dropped")
	}
}
