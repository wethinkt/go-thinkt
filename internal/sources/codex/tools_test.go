package codex

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// fixtureLine returns line n (1-based) of the fixture rollout.
func fixtureLine(t *testing.T, n int) string {
	t.Helper()
	f, err := os.Open("testdata/rollout-fixture.jsonl")
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
	t.Fatalf("fixture has no line %d", n)
	return ""
}

// convertLines converts several lines through one converter and returns the
// entries of the last line.
func convertLines(t *testing.T, lines ...string) []thinkt.Entry {
	t.Helper()
	c := &converter{sessionID: "s", turn: &turnState{}, calls: map[string]string{}}
	var out []thinkt.Entry
	for i, line := range lines {
		c.lineNo = i + 1
		head, class := ClassifyLine([]byte(line))
		out = c.convert(head, class, []byte(line))
	}
	return out
}

func pairOf(t *testing.T, entries []thinkt.Entry) (thinkt.Entry, thinkt.ContentBlock, thinkt.Entry, thinkt.ContentBlock) {
	t.Helper()
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(entries), entries)
	}
	use, res := entries[0], entries[1]
	if use.Role != thinkt.RoleAssistant || len(use.ContentBlocks) != 1 || use.ContentBlocks[0].Type != "tool_use" {
		t.Fatalf("tool_use entry = %+v", use)
	}
	if res.Role != thinkt.RoleUser || len(res.ContentBlocks) != 1 || res.ContentBlocks[0].Type != "tool_result" {
		t.Fatalf("tool_result entry = %+v", res)
	}
	if use.ContentBlocks[0].ToolUseID == "" || use.ContentBlocks[0].ToolUseID != res.ContentBlocks[0].ToolUseID {
		t.Fatalf("ToolUseID mismatch: %q vs %q", use.ContentBlocks[0].ToolUseID, res.ContentBlocks[0].ToolUseID)
	}
	if res.UUID != use.UUID+":result" {
		t.Fatalf("UUIDs = %q, %q", use.UUID, res.UUID)
	}
	return use, use.ContentBlocks[0], res, res.ContentBlocks[0]
}

func TestCommandExecutionPair(t *testing.T) {
	use, call, res, result := pairOf(t, convertLine(t, fixtureLine(t, 16)))
	if call.ToolName != "unified_exec_startup" || use.UUID != "exec-1" || call.ToolUseID != "exec-1" || use.TurnID != "turn-1" {
		t.Fatalf("tool_use = %+v", use)
	}
	in, _ := call.ToolInput.(map[string]any)
	if in["cwd"] != "/work/proj" || in["command"] != "/bin/zsh -lc go build ./..." {
		t.Fatalf("input = %+v", call.ToolInput)
	}
	if res.UUID != "exec-1:result" || result.ToolResult != "undefined: foo" || !result.IsError || len(result.Children) != 0 {
		t.Fatalf("tool_result = %+v", res)
	}
	if res.Metadata["exit_code"] != 1 || res.Metadata["duration_ms"] != int64(1500) || res.Metadata["status"] != "failed" {
		t.Fatalf("metadata = %+v", res.Metadata)
	}
	if use.IsSecondary() || res.IsSecondary() || use.Kind != thinkt.KindNone {
		t.Fatalf("pair should be visible: %+v %+v", use, res)
	}
}

func TestCommandExecutionStderrChild(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:05Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"CommandExecution","id":"exec-3","command":["ls"],"cwd":"file:///work","source":"user_shell","status":"completed","stdout":"a","stderr":"warn","aggregated_output":"a","exit_code":0,"duration":{"secs":0,"nanos":0}}}}`
	_, _, _, result := pairOf(t, convertLine(t, line))
	if result.ToolResult != "a" || result.IsError || len(result.Children) != 1 || result.Children[0].Type != "text" || result.Children[0].Text != "warn" {
		t.Fatalf("result = %+v", result)
	}
}

func TestFileChangeDiffChildrenOnlyForNonEmptyDiffs(t *testing.T) {
	use, call, _, result := pairOf(t, convertLine(t, fixtureLine(t, 19)))
	if call.ToolName != "apply_patch" || use.UUID != "call-patch-1" {
		t.Fatalf("tool_use = %+v", use)
	}
	in, _ := call.ToolInput.(map[string]any)
	if in["/work/proj/new.go"] != "add" || in["/work/proj/main.go"] != "update" || in["/work/proj/old.go"] != "delete" {
		t.Fatalf("input = %+v", call.ToolInput)
	}
	for _, want := range []string{"update /work/proj/main.go", "add /work/proj/new.go", "delete /work/proj/old.go"} {
		if !strings.Contains(result.ToolResult, want) {
			t.Fatalf("ToolResult %q lacks %q", result.ToolResult, want)
		}
	}
	if len(result.Children) != 1 || result.Children[0].Type != thinkt.BlockTypeDiff || !strings.HasPrefix(result.Children[0].Text, "--- a//work/proj/main.go\n+++ b//work/proj/main.go\n@@") {
		t.Fatalf("children = %+v", result.Children)
	}
	if result.IsError {
		t.Fatal("completed patch must not be an error")
	}
}

func TestFileChangeMoveWithoutDiff(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:06Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"FileChange","id":"fc-2","changes":{"/a.go":{"type":"update","unified_diff":"","move_path":"/b.go"}},"status":"failed","stdout":""}}}`
	_, call, _, result := pairOf(t, convertLine(t, line))
	in, _ := call.ToolInput.(map[string]any)
	if in["/a.go"] != "update -> /b.go" || len(result.Children) != 0 || !result.IsError || result.ToolResult != "update /a.go" {
		t.Fatalf("call = %+v result = %+v", call, result)
	}
}

func TestMcpToolCallPair(t *testing.T) {
	use, call, res, result := pairOf(t, convertLine(t, fixtureLine(t, 22)))
	if call.ToolName != "mcp:cua_repl/js" || use.UUID != "call-mcp-1" {
		t.Fatalf("tool_use = %+v", use)
	}
	in, _ := call.ToolInput.(map[string]any)
	if in["code"] != "1+1" {
		t.Fatalf("input = %+v", call.ToolInput)
	}
	if result.ToolResult != "2" || result.IsError || res.Metadata["duration_ms"] != int64(1) {
		t.Fatalf("result = %+v meta = %+v", result, res.Metadata)
	}
}

func TestMcpToolCallNonTextContentBecomesChild(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:07Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"McpToolCall","id":"m2","server":"s","tool":"t","arguments":{},"status":"failed","result":{"content":[{"type":"image","data":"QUJD","mimeType":"image/png"}],"isError":true},"duration":{"secs":0,"nanos":0}}}}`
	_, _, _, result := pairOf(t, convertLine(t, line))
	if !result.IsError || len(result.Children) != 1 || result.Children[0].Type != "image" || !strings.Contains(result.Children[0].Text, `"mimeType"`) {
		t.Fatalf("result = %+v", result)
	}
}

func TestWebSearchExtensionPair(t *testing.T) {
	use, call, _, result := pairOf(t, convertLine(t, fixtureLine(t, 24)))
	if call.ToolName != "web_search" || use.UUID != "ws-1" {
		t.Fatalf("tool_use = %+v", use)
	}
	in, _ := call.ToolInput.(map[string]any)
	if in["query"] != "go build undefined" {
		t.Fatalf("input = %+v", call.ToolInput)
	}
	if !strings.Contains(result.ToolResult, "Go FAQ https://go.dev/doc/faq") {
		t.Fatalf("result = %q", result.ToolResult)
	}
}

func TestWebSearchItemWithoutResultsUsesQuery(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:08Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"WebSearch","id":"ws-2","query":"q","action":{"type":"open_page","url":"https://x"}}}}`
	_, call, _, result := pairOf(t, convertLine(t, line))
	in, _ := call.ToolInput.(map[string]any)
	if in["url"] != "https://x" || result.ToolResult != "https://x" {
		t.Fatalf("call = %+v result = %+v", call, result)
	}
}

func TestClockSleepPair(t *testing.T) {
	use, call, _, result := pairOf(t, convertLine(t, fixtureLine(t, 27)))
	if call.ToolName != "sleep" || use.UUID != "call-sleep-1" || result.ToolResult != "slept 10 ms" {
		t.Fatalf("use = %+v result = %+v", use, result)
	}
	in, _ := call.ToolInput.(map[string]any)
	if in["durationMs"] != int64(10) {
		t.Fatalf("input = %#v", call.ToolInput)
	}
}

func TestImageViewPair(t *testing.T) {
	use, call, _, result := pairOf(t, convertLine(t, fixtureLine(t, 29)))
	if call.ToolName != "view_image" || use.UUID != "img-1" || result.ToolResult != "/tmp/shot.png" || len(result.Children) != 0 {
		t.Fatalf("use = %+v result = %+v", use, result)
	}
	in, _ := call.ToolInput.(map[string]any)
	if in["path"] != "/tmp/shot.png" {
		t.Fatalf("input = %+v", call.ToolInput)
	}
}

func TestWireOrchestrationCallIsSecondaryToolWire(t *testing.T) {
	callLine, outLine := fixtureLine(t, 14), fixtureLine(t, 17)
	call := convertLines(t, callLine)
	if len(call) != 1 || call[0].Kind != thinkt.KindToolWire || !call[0].IsSecondary() || call[0].UUID != "wire:call-exec-1" || call[0].ContentBlocks[0].ToolUseID != "wire:call-exec-1" || call[0].ContentBlocks[0].ToolName != "exec" {
		t.Fatalf("call = %+v", call)
	}
	out := convertLines(t, callLine, outLine)
	if len(out) != 1 || out[0].Kind != thinkt.KindToolWire || !out[0].IsSecondary() || out[0].UUID != "wire:call-exec-1:result" || out[0].Role != thinkt.RoleUser || out[0].ContentBlocks[0].ToolResult != "undefined: foo" {
		t.Fatalf("out = %+v", out)
	}
}

func TestWireExecCommandStaysVisibleWithExitCode(t *testing.T) {
	callLine := `{"timestamp":"2026-02-10T00:00:03Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"call_1","arguments":"{\"cmd\":\"pwd\"}"}}`
	outLine := `{"timestamp":"2026-02-10T00:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_1","output":"Chunk ID: x\nWall time: 0.1 seconds\nProcess exited with code 2\nOutput:\nboom"}}`
	call := convertLines(t, callLine)
	if len(call) != 1 || call[0].Kind != thinkt.KindNone || call[0].IsSecondary() {
		t.Fatalf("call = %+v", call)
	}
	out := convertLines(t, callLine, outLine)
	if len(out) != 1 || out[0].IsSecondary() || out[0].Metadata["exit_code"] != 2 {
		t.Fatalf("out = %+v", out)
	}
}

func TestDurationMs(t *testing.T) {
	if got := durationMs(itemDuration{Secs: 1, Nanos: 500000000}); got != 1500 {
		t.Fatalf("got %d", got)
	}
	if got := durationMs(itemDuration{Secs: 0, Nanos: 2000000}); got != 2 {
		t.Fatalf("got %d", got)
	}
}

func TestDiffHelpers(t *testing.T) {
	if got := wrapDiff("p/q.go", "@@\n-a\n+b\n"); got != "--- a/p/q.go\n+++ b/p/q.go\n@@\n-a\n+b\n" {
		t.Fatalf("wrapDiff = %q", got)
	}
	added, removed := diffLineCounts("--- a/x\n+++ b/x\n@@ -1,2 +1,3 @@\n-old\n+new\n+more\n ctx\n")
	if added != 2 || removed != 1 {
		t.Fatalf("counts = %d/%d", added, removed)
	}
	if stripFileURI("file:///tmp/x") != "/tmp/x" || stripFileURI("/tmp/y") != "/tmp/y" {
		t.Fatal("stripFileURI")
	}
	if code, ok := parseExitCode("Process exited with code 7\n"); !ok || code != 7 {
		t.Fatalf("parseExitCode = %d %v", code, ok)
	}
	if _, ok := parseExitCode("no code here"); ok {
		t.Fatal("parseExitCode false positive")
	}
}
