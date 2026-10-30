package antigravity

import (
	"bytes"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// parseFixture reads every entry of a testdata fixture.
func parseFixture(t *testing.T, name, conversationID, parentID string) []thinkt.Entry {
	t.Helper()
	p := NewParser(bytes.NewReader(fixtureBytes(t, name)), conversationID, parentID)
	var out []thinkt.Entry
	for {
		e, err := p.NextEntry()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, *e)
	}
}

// entriesByStep indexes entries by the step index in their UUID.
func entriesByStep(entries []thinkt.Entry) map[int]thinkt.Entry {
	m := map[int]thinkt.Entry{}
	for _, e := range entries {
		_, idx, _ := strings.Cut(e.UUID, ":")
		n, _ := strconv.Atoi(idx)
		m[n] = e
	}
	return m
}

func TestParserFixtureEntries(t *testing.T) {
	entries := parseFixture(t, "main.jsonl", "11111111-aaaa", "")
	// 15 entries: steps 0,2,3,4,5,6,7,8,9,10,11,12,13,14,15 (step 1 is empty history)
	if len(entries) != 15 {
		t.Fatalf("got %d entries", len(entries))
	}
	seen := map[string]bool{}
	for i, e := range entries {
		if seen[e.UUID] || (e.Text == "" && len(e.ContentBlocks) == 0) {
			t.Fatalf("duplicate or empty: %+v", e)
		}
		seen[e.UUID] = true
		if i > 0 && e.Timestamp.Before(entries[i-1].Timestamp) {
			t.Fatalf("out of order at %s", e.UUID)
		}
	}
}

func TestParserPairsResultsByOrder(t *testing.T) {
	byStep := entriesByStep(parseFixture(t, "main.jsonl", "11111111-aaaa", ""))
	if byStep[3].ContentBlocks[0].ToolUseID != "2.0" || byStep[4].ContentBlocks[0].ToolUseID != "2.1" {
		t.Fatal("results 3 and 4 not paired with calls 2.0 and 2.1")
	}
}

func TestParserNewResponseDropsStaleCalls(t *testing.T) {
	byStep := entriesByStep(parseFixture(t, "main.jsonl", "11111111-aaaa", ""))
	if got := byStep[9].ContentBlocks[0].ToolUseID; got != "8.0" {
		t.Fatalf("step 9 paired with %q, want 8.0 (6.x are superseded)", got)
	}
}

func TestParserLinksSubagentChild(t *testing.T) {
	byStep := entriesByStep(parseFixture(t, "main.jsonl", "11111111-aaaa", ""))
	if byStep[8].ContentBlocks[0].ChildSessionID != "22222222-bbbb" {
		t.Fatalf("tool_use child = %q", byStep[8].ContentBlocks[0].ChildSessionID)
	}
	if ids, _ := byStep[9].Metadata["child_session_ids"].([]string); !slices.Equal(ids, []string{"22222222-bbbb", "33333333-cccc"}) {
		t.Fatalf("result child ids %v", byStep[9].Metadata["child_session_ids"])
	}
}

func TestParserTurnIDsAndModel(t *testing.T) {
	byStep := entriesByStep(parseFixture(t, "main.jsonl", "11111111-aaaa", ""))
	if byStep[2].PromptID != "0" || byStep[14].PromptID != "13" || byStep[14].Model != "Gemini 4 Argon (High)" {
		t.Fatalf("prompt %q %q model %q", byStep[2].PromptID, byStep[14].PromptID, byStep[14].Model)
	}
}

func TestParserSkipsPartialLastLine(t *testing.T) {
	data := string(fixtureBytes(t, "main.jsonl")) + `{"step_index":16,"type":"PLANNER_RESP`
	p := NewParser(strings.NewReader(data), "11111111-aaaa", "")
	n := 0
	for {
		_, err := p.NextEntry()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 15 {
		t.Fatalf("got %d", n)
	}
}

func TestParserCountsUnknownTypes(t *testing.T) {
	p := NewParser(bytes.NewReader(fixtureBytes(t, "main.jsonl")), "11111111-aaaa", "")
	for _, err := p.NextEntry(); err == nil; _, err = p.NextEntry() {
	}
	if p.UnknownTypes()["MYSTERY_STEP"] != 1 {
		t.Fatalf("%v", p.UnknownTypes())
	}
}

func TestParserResultWithoutCallIsUnknown(t *testing.T) {
	line := `{"step_index":0,"type":"GENERIC","status":"DONE","content":"orphan"}` + "\n"
	p := NewParser(strings.NewReader(line), "c", "")
	e, err := p.NextEntry()
	if err != nil || e.Kind != thinkt.KindUnknown || !e.IsSecondary() || e.Text != "orphan" || e.TurnID != "" {
		t.Fatalf("%+v %v", e, err)
	}
}

func TestParserEmptyOrphanResultIsNotEmpty(t *testing.T) {
	line := `{"step_index":0,"type":"GENERIC","status":"DONE"}` + "\n"
	e, err := NewParser(strings.NewReader(line), "c", "").NextEntry()
	if err != nil || (e.Text == "" && len(e.ContentBlocks) == 0) {
		t.Fatalf("%+v %v", e, err)
	}
}

// A file that quotes the subagent marker is not a subagent result.
func TestParserLinksChildOnlyFromInvokeSubagent(t *testing.T) {
	data := `{"step_index":0,"type":"PLANNER_RESPONSE","tool_calls":[{"name":"view_file","args":{"AbsolutePath":"/w/README.md"}}]}
{"step_index":1,"type":"VIEW_FILE","content":"Created the following subagents:\n{\"conversationId\": \"99999999-zzzz\"}"}
`
	var got []thinkt.Entry
	p := NewParser(strings.NewReader(data), "c", "")
	for e, err := p.NextEntry(); err == nil; e, err = p.NextEntry() {
		got = append(got, *e)
	}
	if len(got) != 2 || got[0].ContentBlocks[0].ChildSessionID != "" || got[1].Metadata["child_session_ids"] != nil {
		t.Fatalf("%+v", got)
	}
}

// A planner step is one model response: its entry's TurnID is the step
// index, its paired results carry the same id, and user input has none.
func TestPlannerAndResultShareTurnID(t *testing.T) {
	entries := parseFixture(t, "main.jsonl", "11111111-aaaa", "")
	callTurn := map[string]string{}
	for _, e := range entries {
		for _, b := range e.ContentBlocks {
			if b.Type == "tool_use" {
				if want := strings.TrimPrefix(e.UUID, "11111111-aaaa:"); e.TurnID != want {
					t.Errorf("planner %s TurnID = %q, want %q", e.UUID, e.TurnID, want)
				}
				callTurn[b.ToolUseID] = e.TurnID
			}
		}
	}
	results := 0
	for _, e := range entries {
		for _, b := range e.ContentBlocks {
			if b.Type == "tool_result" {
				results++
				if e.TurnID != callTurn[b.ToolUseID] || e.TurnID == "" {
					t.Errorf("result %s TurnID = %q, want %q", e.UUID, e.TurnID, callTurn[b.ToolUseID])
				}
			}
		}
		if e.Role == thinkt.RoleUser && len(e.ContentBlocks) == 0 && e.TurnID != "" {
			t.Errorf("user entry %s TurnID = %q", e.UUID, e.TurnID)
		}
	}
	if len(callTurn) == 0 || results == 0 {
		t.Fatalf("fixture has %d calls, %d results", len(callTurn), results)
	}
}
