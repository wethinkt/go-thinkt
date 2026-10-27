package claude

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

var convertFixtureLines = []string{
	`{"type":"user","uuid":"u1","parentUuid":null,"isSidechain":true,"agentId":"a1","timestamp":"2026-10-20T10:00:00Z","gitBranch":"main","cwd":"/p","sessionId":"s","version":"2.1.310","message":{"role":"user","content":"hi"}}`,
	`{"type":"assistant","uuid":"a1","parentUuid":"u1","timestamp":"2026-10-20T10:00:01Z","sessionId":"s","message":{"id":"msg_1","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"yo"}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
	`{"type":"file-history-snapshot","messageId":"m","snapshot":{"trackedFileBackups":{"/a":{}}}}`,
}

func parseFixtureLine(t *testing.T, line string) *Entry {
	t.Helper()
	var e Entry
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		t.Fatal(err)
	}
	return &e
}

func TestToThinktEntryMatchesConvertEntry(t *testing.T) {
	for _, line := range convertFixtureLines {
		e := parseFixtureLine(t, line)
		a := e.ToThinktEntry()
		b := *convertEntry(e, thinkt.SourceClaude, "")
		a.Metadata, b.Metadata = nil, nil
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("paths differ for %s:\n%+v\n%+v", e.Type, a, b)
		}
	}
}

func TestConvertEntrySetsSidechainAndCheckpoint(t *testing.T) {
	user := convertEntry(parseFixtureLine(t, convertFixtureLines[0]), thinkt.SourceClaude, "ws")
	if !user.IsSidechain || user.SourceAgentID != "a1" || user.Source != thinkt.SourceClaude || user.WorkspaceID != "ws" {
		t.Fatalf("sidechain/provenance lost: %+v", user)
	}
	snap := convertEntry(parseFixtureLine(t, convertFixtureLines[2]), thinkt.SourceClaude, "")
	if !snap.IsCheckpoint || snap.Role != thinkt.RoleCheckpoint || snap.Text != "File History Snapshot (1 file: /a)" {
		t.Fatalf("checkpoint lost: %+v", snap)
	}
	assistant := convertEntry(parseFixtureLine(t, convertFixtureLines[1]), thinkt.SourceClaude, "")
	if assistant.Usage == nil || assistant.Usage.OutputTokens != 2 || assistant.Text != "yo" {
		t.Fatalf("assistant usage/text lost: %+v", assistant)
	}
}
