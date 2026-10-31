package gemini

import (
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

const (
	mainFixture = "testdata/chats/session-2026-05-17T15-05-17503250.jsonl"
	subFixture  = "testdata/chats/17503250-82e4-479a-ab9d-69002dcb6d31/92szal.jsonl"
	mainStem    = "session-2026-05-17T15-05-17503250"
)

func convertFixture(t *testing.T) []thinkt.Entry {
	t.Helper()
	return convert(replayFile(t, mainFixture), mainStem)
}

func isResult(e thinkt.Entry) bool {
	return len(e.ContentBlocks) == 1 && e.ContentBlocks[0].Type == "tool_result"
}

func TestConvertFixtureShape(t *testing.T) {
	entries := convertFixture(t)
	var users, assistants, results, system int
	turnOfCall := map[string]string{}
	for _, e := range entries {
		switch {
		case e.Role == thinkt.RoleAssistant:
			assistants++
			if e.TurnID != e.UUID {
				t.Errorf("assistant %s TurnID %q", e.UUID, e.TurnID)
			}
			for _, b := range e.ContentBlocks {
				if b.Type == "tool_use" {
					turnOfCall[b.ToolUseID] = e.TurnID
				}
			}
		case isResult(e):
			results++
			if want := turnOfCall[e.ContentBlocks[0].ToolUseID]; e.TurnID != want || want == "" {
				t.Errorf("result %s TurnID %q, want %q", e.UUID, e.TurnID, want)
			}
		case e.Role == thinkt.RoleUser:
			users++
		case e.Role == thinkt.RoleSystem:
			system++
		}
	}
	if users != 3 || assistants != 7 || results != 27 || system != 1 || len(entries) != 38 {
		t.Fatalf("users %d assistants %d results %d system %d total %d", users, assistants, results, system, len(entries))
	}
}

func TestConvertErrorResult(t *testing.T) {
	found := false
	for _, e := range convertFixture(t) {
		if isResult(e) && e.ContentBlocks[0].IsError {
			found = true
			if !strings.Contains(e.ContentBlocks[0].ToolResult, "File not found") {
				t.Errorf("error text %q", e.ContentBlocks[0].ToolResult)
			}
		}
	}
	if !found {
		t.Fatal("no error result")
	}
}

func TestConvertContinuationHidden(t *testing.T) {
	entries := convertFixture(t)
	for i, e := range entries {
		if e.UUID != "cont-1" {
			continue
		}
		if e.Kind != thinkt.KindMeta || !e.IsSecondary() {
			t.Fatalf("continuation = %+v", e)
		}
		prev := entries[i-1].PromptID
		for _, n := range entries[i+1:] {
			if n.Role == thinkt.RoleAssistant {
				if n.PromptID != prev || prev == "" {
					t.Fatalf("PromptID after continuation %q, before %q", n.PromptID, prev)
				}
				return
			}
		}
		t.Fatal("no assistant after continuation")
	}
	t.Fatal("no continuation entry")
}

func TestConvertInvokeAgentLinksChild(t *testing.T) {
	for _, e := range convertFixture(t) {
		for _, b := range e.ContentBlocks {
			if b.Type == "tool_use" && b.ToolName == "invoke_agent" {
				if b.ChildSessionID != mainStem+"/92szal" {
					t.Fatalf("child = %q", b.ChildSessionID)
				}
				return
			}
		}
	}
	t.Fatal("no invoke_agent call")
}

func TestConvertUpdateTopicSecondary(t *testing.T) {
	calls := map[string]bool{}
	for _, e := range convertFixture(t) {
		for _, b := range e.ContentBlocks {
			if b.Type == "tool_use" && b.ToolName == "update_topic" {
				calls[b.ToolUseID] = true
			}
		}
	}
	if len(calls) != 6 {
		t.Fatalf("%d update_topic calls", len(calls))
	}
	for _, e := range convertFixture(t) {
		if isResult(e) && calls[e.ContentBlocks[0].ToolUseID] && !e.IsSecondary() {
			t.Errorf("update_topic result %s visible", e.UUID)
		}
	}
}

func TestConvertInfoSecondary(t *testing.T) {
	for _, e := range convertFixture(t) {
		if e.Role == thinkt.RoleSystem {
			if !e.IsSecondary() || e.Subtype != "info" || e.Text == "" || e.TurnID != "" {
				t.Fatalf("info = %+v", e)
			}
			return
		}
	}
	t.Fatal("no info entry")
}

func TestConvertFunctionResponseUserMessageDropped(t *testing.T) {
	r := replayLines(t, hdr,
		`{"id":"g","timestamp":"2026-05-17T11:00:01Z","type":"gemini","content":"","toolCalls":[{"id":"c","name":"read_file","args":{},"status":"success","result":[{"functionResponse":{"id":"c","name":"read_file","response":{"output":"x"}}}]}]}`,
		`{"id":"u","timestamp":"2026-05-17T11:00:02Z","type":"user","content":[{"functionResponse":{"id":"c","name":"read_file","response":{"output":"x"}}}]}`)
	entries := convert(r, "s")
	if len(entries) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestConvertUsage(t *testing.T) {
	r := replayLines(t, hdr, `{"id":"g","timestamp":"2026-05-17T11:00:01Z","type":"gemini","content":"hi","tokens":{"input":10,"output":3,"cached":4,"thoughts":2,"tool":0,"total":15}}`)
	e := convert(r, "s")[0]
	if e.Usage == nil || e.Usage.OutputTokens != 5 || e.Usage.ThinkingTokens != 2 || e.Usage.CacheReadInputTokens != 4 {
		t.Fatalf("usage = %+v", e.Usage)
	}
}

func TestReplayRealFixtureRepeats(t *testing.T) {
	for _, path := range []string{mainFixture, subFixture} {
		r := replayFile(t, path)
		seen := map[string]bool{}
		for _, m := range r.Messages {
			if seen[m.ID] {
				t.Fatalf("%s: id %s twice", path, m.ID)
			}
			seen[m.ID] = true
		}
	}
	r := replayFile(t, mainFixture)
	withCalls := 0
	for _, m := range r.Messages {
		if len(m.ToolCalls) > 0 {
			withCalls++
		}
	}
	if withCalls != 6 {
		t.Fatalf("%d gemini messages kept their toolCalls copy", withCalls)
	}
}

func TestClassifyLine(t *testing.T) {
	cases := map[string]thinkt.RecordClass{
		hdr:                            thinkt.ClassSessionState,
		`{"$set":{"lastUpdated":"x"}}`: thinkt.ClassSessionState,
		`{"$set":{"messages":[]}}`:     thinkt.ClassBookkeeping,
		`{"$patch":{"id":"a"}}`:        thinkt.ClassBookkeeping,
		`{"$rewindTo":"a"}`:            thinkt.ClassBookkeeping,
		msg("a", "x"):                  thinkt.ClassTurn,
		`{"id":"i","type":"info"}`:     thinkt.ClassEvent,
		`{"id":"w","type":"warning"}`:  thinkt.ClassEvent,
		`{"mystery":1}`:                thinkt.ClassUnknown,
	}
	for line, want := range cases {
		if got := classifyLine([]byte(line)); got != want {
			t.Errorf("%s: %v, want %v", line, got, want)
		}
	}
}
