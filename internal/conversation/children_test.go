package conversation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func childEntries() []thinkt.Entry {
	return []thinkt.Entry{
		{UUID: "u1", Role: thinkt.RoleUser, Text: "edit the file"},
		{UUID: "a1", Role: thinkt.RoleAssistant, TurnID: "msg_1", Kind: thinkt.KindNone,
			Attribution: &thinkt.Attribution{Agent: "general-purpose", Skill: "tdd"}, StopReason: "tool_use",
			ContentBlocks: []thinkt.ContentBlock{{Type: "tool_use", ToolUseID: "t1", ToolName: "Edit", ToolInput: map[string]any{"file_path": "/p/a.md"}}}},
		{UUID: "r1", Role: thinkt.RoleUser, ContentBlocks: []thinkt.ContentBlock{{
			Type: "tool_result", ToolUseID: "t1", ToolResult: "The file has been updated.",
			Children: []thinkt.ContentBlock{
				{Type: thinkt.BlockTypeDiff, Text: "--- a//p/a.md\n+++ b//p/a.md\n@@ -1,1 +1,1 @@\n-old\n+new\n"},
				{Type: "image", MediaType: "image/png", MediaData: "QUJD"},
			}}}},
		{UUID: "s1", Role: thinkt.RoleSystem, Kind: thinkt.KindTurnDuration, Subtype: "turn_duration", Visibility: thinkt.VisibilitySecondary, Text: "turn_duration"},
	}
}

func TestBuildChildrenAndDiff(t *testing.T) {
	doc, err := Build(childEntries(), BuildOptions{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	result := doc.Messages[2].Blocks[0]
	if len(result.Children) != 2 {
		t.Fatalf("children %d", len(result.Children))
	}
	diff := result.Children[0]
	if diff.Kind != thinkt.BlockTypeDiff || !strings.HasPrefix(diff.Text, "--- a/") {
		t.Fatalf("diff child %+v", diff)
	}
	if diff.ID != result.ID+"-child-0" || result.Children[1].ID != result.ID+"-child-1" {
		t.Fatalf("child ids %q %q", diff.ID, result.Children[1].ID)
	}
	if diff.Origin != result.Origin {
		t.Fatalf("child origin must copy the parent block origin: %+v vs %+v", diff.Origin, result.Origin)
	}
	if result.Children[1].Kind != "image" || result.Children[1].MediaType != "image/png" {
		t.Fatalf("image child %+v", result.Children[1])
	}
}

func TestBuildMessageKindAndTurn(t *testing.T) {
	doc, err := Build(childEntries(), BuildOptions{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	assistant := doc.Messages[1]
	if assistant.TurnID != "msg_1" || assistant.StopReason != "tool_use" || assistant.Attribution == nil || assistant.Attribution.Skill != "tdd" {
		t.Fatalf("assistant message lost turn metadata: %+v", assistant)
	}
	system := doc.Messages[3]
	if system.Kind != thinkt.KindTurnDuration || system.Visibility != thinkt.VisibilitySecondary {
		t.Fatalf("system message lost kind/visibility: %+v", system)
	}
}

func TestWritePageHidesSecondaryByDefault(t *testing.T) {
	doc, err := Build(childEntries(), BuildOptions{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	secondaryID := doc.Messages[3].ID
	var page bytes.Buffer
	if err := WritePage(&page, doc, View{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page.String(), secondaryID) {
		t.Fatal("WritePage must hide secondary messages by default")
	}
	var fragment bytes.Buffer
	if err := WriteHTML(&fragment, doc, View{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fragment.String(), secondaryID) {
		t.Fatal("WriteHTML with a zero View hides nothing")
	}
	var revealed bytes.Buffer
	if err := WritePage(&revealed, doc, View{HiddenVisibility: map[thinkt.Visibility]bool{}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(revealed.String(), secondaryID) {
		t.Fatal("an explicit empty HiddenVisibility reveals secondary messages")
	}
}

func TestWriteHTMLDiffAndChildren(t *testing.T) {
	doc, err := Build(childEntries(), BuildOptions{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := WriteHTML(&out, doc, View{}); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{
		`<pre class="diff">`,
		`<span class="diff-add">+new</span>`,
		`<span class="diff-del">-old</span>`,
		`<span class="diff-hunk">@@ -1,1 +1,1 @@</span>`,
		`data-turn-id="msg_1"`,
		`data-kind="turn_duration"`,
		`class="block block-child block-image"`,
		`general-purpose · tdd`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
	parent := doc.Messages[2].Blocks[0]
	parentAt := strings.Index(html, `id="`+parent.ID+`"`)
	childAt := strings.Index(html, `id="`+parent.Children[0].ID+`"`)
	parentEnd := strings.Index(html[parentAt:], "</section>\n</section>")
	if parentAt < 0 || childAt < 0 || childAt < parentAt || childAt > parentAt+parentEnd {
		t.Fatalf("child section must be nested inside its parent block (parent %d, child %d, end %d)", parentAt, childAt, parentEnd)
	}
}
