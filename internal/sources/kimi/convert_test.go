package kimi

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// fixtureLine returns line n (1-based) of testdata/<name>.
func fixtureLine(t *testing.T, name string, n int) string {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for i := 1; sc.Scan(); i++ {
		if i == n {
			return sc.Text()
		}
	}
	t.Fatalf("%s has no line %d", name, n)
	return ""
}

// convertLine classifies one line and converts it with a fresh converter,
// dispatching by key the way the parser does for non-loop lines.
func convertLine(t *testing.T, line string) []thinkt.Entry {
	t.Helper()
	return convertLineWith(t, line, "session_fix1")
}

func convertLineWith(t *testing.T, line, sessionID string) []thinkt.Entry {
	t.Helper()
	h, class := ClassifyLine([]byte(line))
	c := &converter{sessionID: sessionID, lineNo: 1, turn: &turnState{}}
	switch h.Key {
	case "appended/user", "ctx/user", "turn.steer":
		return c.convertUser(h)
	case "ctx/assistant", "appended/assistant":
		return c.convertLegacyAssistant(h)
	case "ctx/tool", "appended/tool":
		return c.convertLegacyTool(h)
	default:
		return c.convertEvent(h, class, []byte(line))
	}
}

func TestConvertAppendedInputUser(t *testing.T) {
	e := convertLine(t, fixtureLine(t, "wire-1.5.jsonl", 3))
	if len(e) != 1 || e[0].Role != thinkt.RoleUser || e[0].Kind != thinkt.KindNone || e[0].IsSecondary() || e[0].UUID != "msg_u1" || e[0].Text != "fix the build" {
		t.Fatalf("entries = %+v", e)
	}
	if e[0].Source != thinkt.SourceKimi || e[0].Timestamp.IsZero() {
		t.Fatalf("entry = %+v", e[0])
	}
}

func TestConvertCtxInjectionIsSecondaryMeta(t *testing.T) {
	e := convertLine(t, fixtureLine(t, "wire-1.5.jsonl", 7))
	if len(e) != 1 || e[0].Kind != thinkt.KindMeta || !e[0].IsSecondary() || e[0].Subtype != "injection" || !strings.HasPrefix(e[0].UUID, "session_fix1:000001:") {
		t.Fatalf("entries = %+v", e)
	}
}

func TestConvertSystemTriggerVariants(t *testing.T) {
	git := convertLine(t, fixtureLine(t, "wire-1.5.jsonl", 39))[0]
	if git.Kind != thinkt.KindMeta || !git.IsSecondary() || git.Subtype != "system_trigger" || git.UUID != "msg_u2" {
		t.Fatalf("git-context = %+v", git)
	}
	cont := convertLine(t, `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"continue"}],"origin":{"kind":"system_trigger"},"id":"m9"}}`)[0]
	if cont.Kind != thinkt.KindMeta || !cont.IsSecondary() {
		t.Fatalf("continue = %+v", cont)
	}
	task := convertLine(t, fixtureLine(t, "wire-1.5.jsonl", 41))[0]
	if task.Kind != thinkt.KindNone || task.IsSecondary() || task.Subtype != "system_trigger" || task.Text != "Review the code structure" || task.UUID != "msg_u3" {
		t.Fatalf("task = %+v", task)
	}
}

func TestConvertGitContextWithTaskIsVisibleTask(t *testing.T) {
	line := `{"message":{"message":{"role":"user","content":[{"type":"text","text":"<git-context>\nWorking directory: /w\nBranch: main\n</git-context>\n\nYou are reviewing the Go project. Report issues."}]},"meta":{"source":"input","origin":{"kind":"system_trigger","name":"subagent"},"userMessageId":"msg_t1"}},"type":"agent.message.appended","time":1}`
	e := convertLine(t, line)[0]
	if e.Kind != thinkt.KindNone || e.IsSecondary() || e.Subtype != "system_trigger" || e.Text != "You are reviewing the Go project. Report issues." {
		t.Fatalf("entry = %+v", e)
	}
	if gc, _ := e.Metadata["git_context"].(string); !strings.Contains(gc, "Branch: main") {
		t.Fatalf("git_context = %#v", e.Metadata["git_context"])
	}
}

func TestConvertTurnSteerIsTaskNotification(t *testing.T) {
	e := convertLine(t, fixtureLine(t, "wire-1.5.jsonl", 35))
	if len(e) != 1 || e[0].Role != thinkt.RoleUser || e[0].Kind != thinkt.KindTaskNotification || !e[0].IsSecondary() || e[0].Subtype != "background_task" || !strings.Contains(e[0].Text, "bash-1") {
		t.Fatalf("entries = %+v", e)
	}
	notify := convertLine(t, fixtureLine(t, "wire-1.5.jsonl", 38))[0]
	if notify.Kind != thinkt.KindTaskNotification || !notify.IsSecondary() || notify.Subtype != "notify" {
		t.Fatalf("notify = %+v", notify)
	}
}

func TestConvertEmptyUserProducesNothing(t *testing.T) {
	if e := convertLine(t, `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"  "}],"origin":{"kind":"user"}}}`); len(e) != 0 {
		t.Fatalf("entries = %+v", e)
	}
}

func TestConvertTurnEndedFailed(t *testing.T) {
	e := convertLine(t, `{"type":"turn.ended","agentId":"main","turnId":0,"reason":"failed","error":{"code":"internal","message":"boom"},"durationMs":42,"time":1790000001026}`)[0]
	if e.Role != thinkt.RoleSystem || e.Kind != thinkt.KindTurnDuration || !e.IsSecondary() || e.Error != "boom" || e.Metadata["duration_ms"] != int64(42) || e.PromptID != "0" || e.TurnID != "" || e.Text == "" {
		t.Fatalf("entry = %+v", e)
	}
	ok := convertLine(t, fixtureLine(t, "wire-1.5.jsonl", 29))[0]
	if ok.Error != "" || ok.Metadata["duration_ms"] != int64(9000) {
		t.Fatalf("entry = %+v", ok)
	}
}

func TestConvertSubagentSpawned(t *testing.T) {
	e := convertLine(t, fixtureLine(t, "wire-1.5.jsonl", 15))[0]
	if e.Role != thinkt.RoleSystem || e.Kind != thinkt.KindAgentMessage || e.IsSecondary() || e.Subtype != "spawned" || e.ChildSessionID != "session_fix1/agent-0" || e.Text != "spawned explore: Explore" {
		t.Fatalf("entry = %+v", e)
	}
	nested := convertLineWith(t, `{"type":"subagent.spawned","subagentId":"agent-9","subagentName":"explore","parentToolCallId":"t","description":"Deeper","time":1}`, "session_fix1/agent-0")[0]
	if nested.ChildSessionID != "session_fix1/agent-9" {
		t.Fatalf("nested = %+v", nested)
	}
	done := convertLine(t, `{"type":"subagent.completed","subagentId":"agent-0","resultSummary":"all good","usage":{},"time":2}`)[0]
	if done.Kind != thinkt.KindAgentMessage || !done.IsSecondary() || done.Subtype != "completed" || done.Text != "all good" || done.ChildSessionID != "session_fix1/agent-0" {
		t.Fatalf("completed = %+v", done)
	}
	failed := convertLine(t, `{"type":"subagent.failed","subagentId":"agent-1","error":"Subagent timed out.","time":3}`)[0]
	if failed.Text != "Subagent timed out." || failed.Error != "Subagent timed out." {
		t.Fatalf("failed = %+v", failed)
	}
}

func TestConvertTaskEvents(t *testing.T) {
	started := convertLine(t, `{"type":"task.started","agentId":"main","info":{"taskId":"bash-1","description":"Serve site","status":"running","kind":"process","command":"task serve"},"time":1}`)[0]
	if started.Role != thinkt.RoleSystem || started.Kind != thinkt.KindCommandOutput || !started.IsSecondary() || started.Subtype != "started" || started.Text != "Serve site" {
		t.Fatalf("started = %+v", started)
	}
	ended := convertLine(t, `{"type":"task.terminated","agentId":"main","info":{"taskId":"bash-1","description":"Serve site","status":"killed"},"outputTail":"bye","time":2}`)[0]
	if ended.Subtype != "terminated" || !strings.Contains(ended.Text, "Serve site") || !strings.Contains(ended.Text, "bye") {
		t.Fatalf("terminated = %+v", ended)
	}
}

func TestConvertPlanRevision(t *testing.T) {
	e := convertLine(t, `{"type":"plan.revision","agentId":"main","id":"x","version":2,"key":"plan/x/v2.md","time":1}`)[0]
	if e.Kind != thinkt.KindAttachment || !e.IsSecondary() || e.Subtype != "plan" || e.Text != "plan/x/v2.md" || e.Metadata["version"] != int64(2) {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertUnknownKeepsRawLine(t *testing.T) {
	line := fixtureLine(t, "wire-1.5.jsonl", 45)
	e := convertLine(t, line)[0]
	if e.Role != thinkt.RoleSystem || e.Kind != thinkt.KindUnknown || !e.IsSecondary() || e.Text != line {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertLegacyAssistantAndTool(t *testing.T) {
	a := convertLine(t, fixtureLine(t, "wire-1.0.jsonl", 4))
	if len(a) != 1 || a[0].Role != thinkt.RoleAssistant || len(a[0].ContentBlocks) != 3 {
		t.Fatalf("assistant = %+v", a)
	}
	b := a[0].ContentBlocks
	if b[0].Type != "thinking" || b[0].Thinking != "t" || b[1].Type != "text" || b[1].Text != "Running" || b[2].Type != "tool_use" || b[2].ToolUseID != "tool_l1" || b[2].ToolName != "Bash" {
		t.Fatalf("blocks = %+v", b)
	}
	if in, _ := b[2].ToolInput.(map[string]any); in["command"] != "ls" {
		t.Fatalf("input = %#v", b[2].ToolInput)
	}
	if a[0].Text != "Running" || a[0].Timestamp.IsZero() == false {
		t.Fatalf("assistant = %+v", a[0])
	}
	r := convertLine(t, fixtureLine(t, "wire-1.0.jsonl", 5))
	if len(r) != 1 || r[0].Role != thinkt.RoleUser || r[0].UUID != "tool_l1:result" || len(r[0].ContentBlocks) != 1 {
		t.Fatalf("tool = %+v", r)
	}
	if rb := r[0].ContentBlocks[0]; rb.Type != "tool_result" || rb.ToolUseID != "tool_l1" || !rb.IsError || !strings.HasPrefix(rb.ToolResult, "<system>ERROR") {
		t.Fatalf("result block = %+v", rb)
	}
	appended := convertLine(t, fixtureLine(t, "wire-1.5.jsonl", 34))
	if len(appended) != 1 || len(appended[0].ContentBlocks) != 1 || appended[0].ContentBlocks[0].Thinking != "leftover" {
		t.Fatalf("salvaged = %+v", appended)
	}
	if e := convertLine(t, `{"type":"context.append_message","message":{"role":"assistant","content":[],"toolCalls":[]}}`); len(e) != 0 {
		t.Fatalf("empty assistant = %+v", e)
	}
}

func TestConvertCompactionCarryOver(t *testing.T) {
	e := convertLine(t, fixtureLine(t, "wire-1.0.jsonl", 2))[0]
	if e.Role != thinkt.RoleUser || e.Kind != thinkt.KindCompactSummary || !e.IsSecondary() || e.Subtype != "compaction" {
		t.Fatalf("entry = %+v", e)
	}
	plain := convertLine(t, fixtureLine(t, "wire-1.0.jsonl", 3))[0]
	if plain.Kind != thinkt.KindNone || plain.Text != "hello legacy" {
		t.Fatalf("plain = %+v", plain)
	}
}

func TestToolResultBlockArrayOutput(t *testing.T) {
	output := []any{
		map[string]any{"type": "text", "text": "<image path=\"/x.png\">"},
		map[string]any{"type": "image_url", "imageUrl": map[string]any{"url": "data:image/png;base64,AAAA"}},
		map[string]any{"type": "text", "text": "done"},
	}
	b := toolResultBlock("t1", output, "", false)
	if b.Type != "tool_result" || b.ToolUseID != "t1" || b.ToolResult != "<image path=\"/x.png\">\ndone" || b.IsError {
		t.Fatalf("block = %+v", b)
	}
	if len(b.Children) != 1 || b.Children[0].Type != "image" || b.Children[0].MediaType != "image/png" || b.Children[0].MediaData != "AAAA" {
		t.Fatalf("children = %+v", b.Children)
	}
	s := toolResultBlock("t2", "plain", "<system>note</system>", true)
	if s.ToolResult != "plain" || !s.IsError || len(s.Children) != 1 || s.Children[0].Type != "text" || s.Children[0].Text != "<system>note</system>" {
		t.Fatalf("string block = %+v", s)
	}
}

func TestToolResultBlockBlobrefIsText(t *testing.T) {
	output := []any{map[string]any{"type": "image_url", "imageUrl": map[string]any{"url": "blobref:image/png;abc"}}}
	b := toolResultBlock("t3", output, "", false)
	if len(b.Children) != 1 || b.Children[0].Type != "text" || b.Children[0].Text != "blobref:image/png;abc" || b.Children[0].MediaData != "" {
		t.Fatalf("block = %+v", b)
	}
}

func TestDecodeUsage(t *testing.T) {
	u := decodeUsage([]byte(`{"inputOther":100,"output":20,"inputCacheRead":40,"inputCacheCreation":5}`))
	if u == nil || u.InputTokens != 100 || u.OutputTokens != 20 || u.CacheReadInputTokens != 40 || u.CacheCreationInputTokens != 5 {
		t.Fatalf("usage = %+v", u)
	}
	if decodeUsage(nil) != nil || decodeUsage([]byte("null")) != nil {
		t.Fatal("empty usage must be nil")
	}
}

func TestTurnStateApply(t *testing.T) {
	var ts turnState
	h, _ := ClassifyLine([]byte(`{"type":"llm.request","modelAlias":"kimi-code/k3","model":"k3","thinkingEffort":"high"}`))
	ts.apply(h)
	if ts.Model != "kimi-code/k3" || ts.Effort != "high" {
		t.Fatalf("state = %+v", ts)
	}
	h, _ = ClassifyLine([]byte(`{"type":"llm.request","model":"k2","thinkingEffort":"medium"}`))
	ts.apply(h)
	if ts.Model != "k2" || ts.Effort != "medium" {
		t.Fatalf("state = %+v", ts)
	}
	h, _ = ClassifyLine([]byte(`{"turnId":1,"type":"agent.turn.started","kind":"event"}`))
	ts.apply(h)
	if ts.TurnID != "1" {
		t.Fatalf("state = %+v", ts)
	}
	h, _ = ClassifyLine([]byte(`{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s","turnId":"2","step":1}}`))
	ts.apply(h)
	if ts.TurnID != "2" {
		t.Fatalf("state = %+v", ts)
	}
}

func TestChildSessionID(t *testing.T) {
	if childSessionID("session_a", "agent-0") != "session_a/agent-0" || childSessionID("session_a/agent-0", "agent-3") != "session_a/agent-3" {
		t.Fatal("childSessionID")
	}
}
