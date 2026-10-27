package claude

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestConvertToolResultContentArray(t *testing.T) {
	text, kids := convertToolResultContent(json.RawMessage(`[{"type":"text","text":"a"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}},{"type":"tool_reference","tool_name":"X"}]`))
	if text != "a" {
		t.Fatalf("text %q", text)
	}
	if len(kids) != 3 {
		t.Fatalf("children %d", len(kids))
	}
	if kids[0].Type != "text" || kids[0].Text != "a" {
		t.Fatalf("text child %+v", kids[0])
	}
	if kids[1].Type != "image" || kids[1].MediaType != "image/png" || kids[1].MediaData != "AAAA" {
		t.Fatalf("image child %+v", kids[1])
	}
	if kids[2].Type != "tool_reference" || !strings.Contains(kids[2].Text, `"tool_name"`) {
		t.Fatalf("unknown child %+v", kids[2])
	}
	text2, _ := convertToolResultContent(json.RawMessage(`[{"type":"text","text":"a"},{"type":"text","text":"b"}]`))
	if text2 != "a\nb" {
		t.Fatalf("joined text %q", text2)
	}
}

func TestConvertToolResultContentString(t *testing.T) {
	text, kids := convertToolResultContent(json.RawMessage(`"plain"`))
	if text != "plain" || kids != nil {
		t.Fatalf("string content: %q %v", text, kids)
	}
	if text, kids := convertToolResultContent(nil); text != "" || kids != nil {
		t.Fatalf("empty content: %q %v", text, kids)
	}
}

func TestParseToolUseResultShapes(t *testing.T) {
	r, ok := parseToolUseResult(json.RawMessage(`{"filePath":"/p/a.md","structuredPatch":[{"oldStart":1,"oldLines":1,"newStart":1,"newLines":1,"lines":["-a","+b"]}],"stdout":"","interrupted":false}`))
	if !ok || r.FilePath != "/p/a.md" || len(r.StructuredPatch) != 1 || r.StructuredPatch[0].Lines[1] != "+b" {
		t.Fatalf("object: ok=%v %+v", ok, r)
	}
	for _, raw := range []string{`"a string"`, `[1,2]`, ``, `null`} {
		if _, ok := parseToolUseResult(json.RawMessage(raw)); ok {
			t.Fatalf("%q must not parse as a typed result", raw)
		}
	}
}

func TestUnifiedDiff(t *testing.T) {
	got := unifiedDiff("/p/a.md", []patchHunk{{OldStart: 34, OldLines: 2, NewStart: 34, NewLines: 3, Lines: []string{" keep", "-old", "+new", "+more"}}})
	want := "--- a//p/a.md\n+++ b//p/a.md\n@@ -34,2 +34,3 @@\n keep\n-old\n+new\n+more\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestConvertEntryToolResultWithPatch(t *testing.T) {
	line := `{"type":"user","uuid":"r","sessionId":"sess","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"The file has been updated."}]},"toolUseResult":{"filePath":"/p/a.md","oldString":"old","newString":"new","structuredPatch":[{"oldStart":1,"oldLines":1,"newStart":1,"newLines":1,"lines":["-old","+new"]}]}}`
	e := convertLine(t, line)
	block := e.ContentBlocks[0]
	if block.ToolResult != "The file has been updated." {
		t.Fatalf("tool result %q", block.ToolResult)
	}
	if len(block.Children) != 1 || block.Children[0].Type != thinkt.BlockTypeDiff || !strings.HasPrefix(block.Children[0].Text, "--- a/") {
		t.Fatalf("diff child %+v", block.Children)
	}
}

func TestConvertEntryToolResultAgentID(t *testing.T) {
	line := `{"type":"user","uuid":"r","sessionId":"sess","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"launched"}]},"toolUseResult":{"agentId":"a9","status":"async_launched","isAsync":true}}`
	e := convertLine(t, line)
	if e.ChildSessionID != "sess/agent-a9" || e.ContentBlocks[0].ChildSessionID != "sess/agent-a9" {
		t.Fatalf("child session: entry=%q block=%q", e.ChildSessionID, e.ContentBlocks[0].ChildSessionID)
	}
	noSession := convertLine(t, `{"type":"user","uuid":"r","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"x"}]},"toolUseResult":{"agentId":"a9","status":"async_launched"}}`)
	if noSession.ChildSessionID != "agent-a9" {
		t.Fatalf("unqualified child session: %q", noSession.ChildSessionID)
	}
}

func TestConvertEntryToolResultStringToolUseResultIgnored(t *testing.T) {
	line := `{"type":"user","uuid":"r","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"x"}]},"toolUseResult":"ignored"}`
	e := convertLine(t, line)
	block := e.ContentBlocks[0]
	if block.ToolResult != "x" || block.Children != nil {
		t.Fatalf("block %+v", block)
	}
	for _, key := range []string{"interrupted", "persisted_output_path"} {
		if _, ok := e.Metadata[key]; ok {
			t.Fatalf("metadata %q must not be set", key)
		}
	}
}

func TestConvertEntryToolResultStdoutFallback(t *testing.T) {
	line := `{"type":"user","uuid":"r","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":""}]},"toolUseResult":{"stdout":"out","stderr":"err","interrupted":true,"persistedOutputPath":"/s/tool-results/x.txt"}}`
	e := convertLine(t, line)
	block := e.ContentBlocks[0]
	if block.ToolResult != "out" {
		t.Fatalf("stdout fallback %q", block.ToolResult)
	}
	if len(block.Children) != 1 || block.Children[0].Type != "text" || block.Children[0].Text != "err" {
		t.Fatalf("stderr child %+v", block.Children)
	}
	if e.Metadata["interrupted"] != true || e.Metadata["persisted_output_path"] != "/s/tool-results/x.txt" {
		t.Fatalf("metadata %v", e.Metadata)
	}
}

func TestConvertEntryToolResultImageArray(t *testing.T) {
	line := `{"type":"user","uuid":"r","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"shot"},{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"QUJD"}}]}]},"toolUseResult":{"stdout":"ignored because content is set","interrupted":false}}`
	e := convertLine(t, line)
	block := e.ContentBlocks[0]
	if block.ToolResult != "shot" || len(block.Children) != 2 || block.Children[1].MediaType != "image/jpeg" {
		t.Fatalf("image array %+v", block)
	}
}
