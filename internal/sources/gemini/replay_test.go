package gemini

import (
	"os"
	"strings"
	"testing"
)

func replayFile(t *testing.T, path string) *replayed {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := replay(f)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func replayLines(t *testing.T, lines ...string) *replayed {
	t.Helper()
	r, err := replay(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func ids(r *replayed) string {
	var out []string
	for _, m := range r.Messages {
		out = append(out, m.ID)
	}
	return strings.Join(out, " ")
}

const hdr = `{"sessionId":"s","projectHash":"h","startTime":"2026-05-17T11:00:00Z","lastUpdated":"2026-05-17T11:00:00Z","kind":"main"}`

func msg(id, text string) string {
	return `{"id":"` + id + `","timestamp":"2026-05-17T11:00:01Z","type":"gemini","content":"` + text + `"}`
}

func TestReplayFixture(t *testing.T) {
	r := replayFile(t, "testdata/replay.jsonl")
	if r == nil {
		t.Fatal("nil replay")
	}
	if got := ids(r); got != "u1 g1 u3" {
		t.Fatalf("ids = %q", got)
	}
	g1 := r.Messages[1]
	if len(g1.ToolCalls) != 1 || !strings.Contains(string(g1.ToolCalls[0].Result), `"new"`) {
		t.Fatalf("g1 calls = %+v", g1.ToolCalls)
	}
	if r.Meta.Summary != "Title" || r.Header.SessionID != "s1" || r.Header.ProjectHash != "h1" {
		t.Fatalf("meta %+v header %+v", r.Meta, r.Header)
	}
	for _, n := range []int{2, 4, 7, 9} {
		if !r.Live[n] {
			t.Errorf("line %d not live", n)
		}
	}
	for _, n := range []int{3, 5, 6} {
		if r.Live[n] {
			t.Errorf("line %d live", n)
		}
	}
	if r.Unknown.Snapshot()["mystery"] != 1 {
		t.Errorf("unknown = %v", r.Unknown.Snapshot())
	}
}

func TestReplayRepeatKeepsFirstPosition(t *testing.T) {
	r := replayLines(t, hdr, msg("a", "x"), msg("b", "y"), msg("a", "z"))
	if ids(r) != "a b" || string(r.Messages[0].Content) != `"z"` {
		t.Fatalf("%q %s", ids(r), r.Messages[0].Content)
	}
}

func TestReplayRewindUnknownIDClears(t *testing.T) {
	r := replayLines(t, hdr, msg("a", "x"), msg("b", "y"), `{"$rewindTo":"zz"}`)
	if len(r.Messages) != 0 {
		t.Fatalf("%q", ids(r))
	}
}

func TestReplaySetMessagesReplaces(t *testing.T) {
	r := replayLines(t, hdr, msg("x", "1"), msg("y", "2"), `{"$set":{"messages":[`+msg("z", "3")+`]}}`)
	if ids(r) != "z" || !r.Live[4] || r.Live[2] {
		t.Fatalf("%q live %v", ids(r), r.Live)
	}
}

func TestReplayOrderIDs(t *testing.T) {
	r := replayLines(t, hdr, msg("a", "1"), msg("b", "2"), msg("c", "3"), `{"$patch":{"orderIds":["c","a"]}}`)
	if ids(r) != "b c a" {
		t.Fatalf("%q", ids(r))
	}
}

func TestReplayRemoveIDs(t *testing.T) {
	r := replayLines(t, hdr, msg("a", "1"), msg("b", "2"), `{"$patch":{"removeIds":["a"]}}`)
	if ids(r) != "b" || r.Live[2] {
		t.Fatalf("%q", ids(r))
	}
}

func TestReplayPatchContent(t *testing.T) {
	r := replayLines(t, hdr, msg("a", "1"), `{"$patch":{"updates":[{"id":"a","content":"edited"}]}}`)
	if string(r.Messages[0].Content) != `"edited"` || !r.Live[3] {
		t.Fatalf("%s live %v", r.Messages[0].Content, r.Live)
	}
}

func TestReplayNoHeaderIsNil(t *testing.T) {
	if r := replayLines(t, msg("a", "1")); r != nil {
		t.Fatalf("%+v", r)
	}
}

func TestReplayPartialLastLine(t *testing.T) {
	r := replayLines(t, hdr, msg("a", "1"), `{"id":"b","timestamp":"2026-05-17T11:0`)
	if r == nil || ids(r) != "a" {
		t.Fatalf("%v", r)
	}
}
