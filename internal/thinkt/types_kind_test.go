package thinkt

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEntryNewFieldsRoundTrip(t *testing.T) {
	in := Entry{UUID: "u", Role: RoleSystem, Kind: KindTurnDuration, Subtype: "turn_duration",
		Visibility: VisibilitySecondary, TurnID: "msg_1",
		Attribution:   &Attribution{Agent: "Explore"},
		Compaction:    &CompactionInfo{Trigger: "auto", PreTokens: 12},
		ContentBlocks: []ContentBlock{{Type: "tool_result", Children: []ContentBlock{{Type: BlockTypeDiff, Text: "@@"}}}}}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Entry
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != KindTurnDuration || !out.IsSecondary() || out.TurnID != "msg_1" {
		t.Fatalf("round trip lost fields: %+v", out)
	}
	if out.Attribution == nil || out.Attribution.Agent != "Explore" || out.Compaction == nil || out.Compaction.PreTokens != 12 {
		t.Fatalf("round trip lost nested fields: %+v", out)
	}
	if out.ContentBlocks[0].Children[0].Type != BlockTypeDiff {
		t.Fatal("children lost")
	}
	if strings.Contains(string(data), `"visibility":""`) || strings.Contains(string(data), `"kind":""`) {
		t.Fatal("zero values must be omitted")
	}
}

func TestEntryZeroValueJSONUnchanged(t *testing.T) {
	data, err := json.Marshal(Entry{UUID: "u", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"kind", "visibility", "turn_id", "attribution", "permission", "compaction", "children", "subtype"} {
		if strings.Contains(string(data), `"`+key+`"`) {
			t.Fatalf("%s leaked into zero-value JSON: %s", key, data)
		}
	}
}

func TestSessionMetaNewFieldsOmitted(t *testing.T) {
	data, err := json.Marshal(SessionMeta{ID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"title", "last_prompt", "subagent_count", "pr_links", "worktree", "cost", "parent_session_id"} {
		if strings.Contains(string(data), `"`+key+`"`) {
			t.Fatalf("%s leaked into zero-value JSON: %s", key, data)
		}
	}
	if ParserVersion < 2 {
		t.Fatalf("ParserVersion must start at 2, got %d", ParserVersion)
	}
}
