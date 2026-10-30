package antigravity

import (
	"slices"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestSplitUserInput(t *testing.T) {
	req, meta := splitUserInput(fixtureStep(t, "main.jsonl", 0).Content)
	if req != "fix the build" || !strings.Contains(meta["additional_metadata"], "16:34:22") ||
		modelFromSettingsChange(meta["settings_change"]) != "Gemini 4 Argon (High)" {
		t.Fatalf("%q %v", req, meta)
	}
	if req, _ := splitUserInput("plain text"); req != "plain text" {
		t.Fatalf("unwrapped: %q", req)
	}
	if modelFromSettingsChange("The user changed setting `Theme` from a to b.") != "" {
		t.Fatal("non-model setting read as model")
	}
}

func TestModelTurnBlocksUsageAndIDs(t *testing.T) {
	c := &converter{conversationID: "11111111-aaaa", model: "Gemini 4 Argon (High)"}
	e, calls, ok := c.modelTurn(fixtureStep(t, "main.jsonl", 2))
	// blocks: thinking, text, tool_use 2.0 view_file, tool_use 2.1 run_command
	if !ok || e.UUID != "11111111-aaaa:2" || len(e.ContentBlocks) != 4 || len(calls) != 2 || calls[1].ID != "2.1" ||
		e.ContentBlocks[0].Thinking != "look first" || e.ContentBlocks[1].Text != "Reading." ||
		e.ContentBlocks[2].ToolName != "view_file" || e.ContentBlocks[3].ToolUseID != "2.1" ||
		e.Usage == nil || e.Usage.InputTokens != 100 || e.Usage.CacheReadInputTokens != 40 ||
		e.Usage.OutputTokens != 20 || e.Model != "Gemini 4 Argon (High)" || e.Role != thinkt.RoleAssistant {
		t.Fatalf("%+v", e)
	}
}

func TestModelTurnEmptyEmitsNothing(t *testing.T) {
	_, _, ok := (&converter{}).modelTurn(stepHead{Index: 3, Type: "PLANNER_RESPONSE", Raw: []byte(`{"step_index":3,"type":"PLANNER_RESPONSE"}`)})
	if ok {
		t.Fatal("empty response emitted")
	}
}

func TestResultEntryStripsHeaderAndFlagsError(t *testing.T) {
	e := (&converter{conversationID: "c"}).resultEntry(fixtureStep(t, "main.jsonl", 4), "2.1")
	b := e.ContentBlocks[0]
	if e.Role != thinkt.RoleUser || b.Type != "tool_result" || b.ToolUseID != "2.1" || !b.IsError ||
		strings.Contains(b.ToolResult, "Created At") || b.ToolResult != "build failed" ||
		e.Metadata["completed_at"] == nil || e.UUID != "c:4" {
		t.Fatalf("%+v", e)
	}
}

func TestSubagentIDs(t *testing.T) {
	got := subagentIDs(fixtureStep(t, "main.jsonl", 9).Content)
	if !slices.Equal(got, []string{"22222222-bbbb", "33333333-cccc"}) {
		t.Fatalf("%v", got)
	}
	if subagentIDs("no subagents here") != nil {
		t.Fatal("ids from plain text")
	}
}

func TestMessageSender(t *testing.T) {
	if got := messageSender(fixtureStep(t, "main.jsonl", 5).Content); got != "11111111-aaaa/task-4" {
		t.Fatalf("%q", got)
	}
}

func TestConvertKindsAndVisibility(t *testing.T) {
	c := &converter{conversationID: "11111111-aaaa"}
	cases := []struct {
		idx       int
		kind      thinkt.EntryKind
		secondary bool
		emitted   bool
	}{
		{0, thinkt.KindNone, false, true},
		{1, "", false, false},                        // empty history
		{5, thinkt.KindTaskNotification, true, true}, // as Kimi
		{7, thinkt.KindAPIError, true, true},
		{10, thinkt.KindAgentMessage, true, true}, // subagent report = Kimi's completion case, secondary
		{11, thinkt.KindMeta, true, true},
		{12, thinkt.KindCompactSummary, false, true}, // as Claude: VisibilityNormal
		{13, thinkt.KindNone, false, true},
		{15, thinkt.KindUnknown, true, true},
	}
	lines := fixtureLines(t, "main.jsonl")
	for _, tc := range cases {
		h, class := ClassifyLine(lines[tc.idx])
		e, ok := c.convert(h, class)
		if ok != tc.emitted {
			t.Errorf("step %d: emitted %v", tc.idx, ok)
			continue
		}
		if !ok {
			continue
		}
		if e.Kind != tc.kind || e.IsSecondary() != tc.secondary || (e.Text == "" && len(e.ContentBlocks) == 0) {
			t.Errorf("step %d: kind %q secondary %v text %q", tc.idx, e.Kind, e.IsSecondary(), e.Text)
		}
	}
	if h, class := ClassifyLine(lines[7]); class == ClassAPIError {
		if e, _ := c.convert(h, class); !strings.Contains(e.Text, "stream was interrupted") {
			t.Errorf("api error text %q", e.Text)
		}
	}
}

func TestChildTaskPromptIsVisibleUserEntry(t *testing.T) {
	c := &converter{conversationID: "22222222-bbbb", parentID: "11111111-aaaa"}
	h, class := ClassifyLine(fixtureLines(t, "child.jsonl")[0])
	e, ok := c.convert(h, class)
	if !ok || e.Role != thinkt.RoleUser || e.IsSecondary() || e.Kind != thinkt.KindNone ||
		!strings.Contains(e.Text, "Examine") {
		t.Fatalf("%+v", e)
	}
}

func TestChildTurnIDBeforeAnyUserInput(t *testing.T) {
	// design B: steps before the first USER_INPUT take the first step's index
	c := &converter{conversationID: "22222222-bbbb", parentID: "11111111-aaaa"}
	for _, line := range fixtureLines(t, "child.jsonl") {
		h, class := ClassifyLine(line)
		var e thinkt.Entry
		if class == ClassModelTurn {
			e, _, _ = c.modelTurn(h)
		} else {
			e, _ = c.convert(h, class)
		}
		if e.TurnID != "0" {
			t.Fatalf("step %d turn %q", h.Index, e.TurnID)
		}
	}
}

func TestMediaBecomesTextReference(t *testing.T) {
	e := (&converter{conversationID: "c"}).resultEntry(fixtureStep(t, "main.jsonl", 3), "2.0")
	if !strings.Contains(e.ContentBlocks[0].ToolResult, "[image/png: /w/shot.png]") {
		t.Fatalf("%+v", e)
	}
}
