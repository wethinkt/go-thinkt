package gemini

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A long-running store must find a session file created, or see one grown,
// after it first listed the project.
func TestGetSessionMetaFindsNewAndGrownFiles(t *testing.T) {
	base, s := newHome(t)
	ctx := context.Background()
	if _, err := s.ListSessions(ctx, hashOf(devProj)); err != nil {
		t.Fatal(err)
	}
	newPath := put(t, base, "tmp/proj/chats/session-2026-05-18T00-00-bbbbbbbb.jsonl", header(hashOf(devProj))+userLine("u1", "hello")+"\n")
	m, err := s.GetSessionMeta(ctx, newPath)
	if err != nil || m == nil || m.EntryCount != 1 {
		t.Fatalf("new file: %+v %v", m, err)
	}
	f, err := os.OpenFile(newPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(userLine("u2", "again") + "\n")
	f.Close()
	later := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(newPath, later, later)
	if m, err := s.GetSessionMeta(ctx, newPath); err != nil || m == nil || m.EntryCount != 2 {
		t.Fatalf("grown file: %+v %v", m, err)
	}
}

// The CLI's masking sync stores the whole user turn as each call's result;
// a call takes the functionResponse with its own id.
func TestConvertPicksResultByCallID(t *testing.T) {
	both := `[{"functionResponse":{"id":"c1","name":"read_file","response":{"output":"A"}}},{"functionResponse":{"id":"c2","name":"read_file","response":{"output":"B"}}}]`
	r := replayLines(t, hdr, `{"id":"g","timestamp":"2026-05-17T11:00:01Z","type":"gemini","content":"","toolCalls":[`+
		`{"id":"c1","name":"read_file","args":{},"status":"success","result":`+both+`},`+
		`{"id":"c2","name":"read_file","args":{},"status":"success","result":`+both+`}]}`)
	for _, e := range convert(r, "s") {
		if !isResult(e) {
			continue
		}
		want := map[string]string{"c1": "A", "c2": "B"}[e.ContentBlocks[0].ToolUseID]
		if e.ContentBlocks[0].ToolResult != want {
			t.Errorf("%s: %q, want %q", e.UUID, e.ContentBlocks[0].ToolResult, want)
		}
	}
}

// The CLI converts <name>.json into <name>.jsonl; once converted, the .json
// is superseded even when a rewind removed its last message.
func TestConvertedJSONWithSameStemSuperseded(t *testing.T) {
	base, s := newHome(t)
	put(t, base, "tmp/proj/chats/"+mainStem+".json", jsonSession(mainHeaderID, hashOf(devProj), "rewound-away"))
	ms, _ := s.ListSessions(context.Background(), hashOf(devProj))
	if len(ms) != 1 || filepath.Ext(ms[0].FullPath) != ".jsonl" {
		t.Fatalf("%+v", ms)
	}
}

// Identical copies in both folders: the slug folder's wins whatever their
// mtimes and whether the metadata cache is warm.
func TestDedupeSlugWinsForIdenticalCopies(t *testing.T) {
	base, s := newHome(t)
	h := hashOf(devProj)
	name := "session-2026-02-06T13-11-dac0f3bc.json"
	slug := put(t, base, "tmp/proj/chats/"+name, jsonSession("old", h, "m1"))
	hashCopy := put(t, base, "tmp/"+h+"/chats/"+name, jsonSession("old", h, "m1"))
	newer := time.Now().Add(time.Hour)
	_ = os.Chtimes(hashCopy, newer, newer)
	ctx := context.Background()
	if _, err := s.ListProjects(ctx); err != nil { // warms the metadata cache
		t.Fatal(err)
	}
	s.ResetCache()
	ms, _ := s.ListSessions(ctx, h)
	for _, m := range ms {
		if filepath.Base(m.FullPath) == name && m.FullPath != slug {
			t.Fatalf("kept %s", m.FullPath)
		}
	}
}

// A converted session carries its summary in the header line.
func TestReplayHeaderSummary(t *testing.T) {
	r := replayLines(t, `{"sessionId":"s","projectHash":"h","startTime":"2026-05-17T11:00:00Z","lastUpdated":"2026-05-17T11:00:00Z","summary":"Converted title"}`, msg("a", "x"))
	if r.Meta.Summary != "Converted title" {
		t.Fatalf("summary %q", r.Meta.Summary)
	}
	r = replayLines(t, `{"sessionId":"s","projectHash":"h","summary":"old"}`, `{"$set":{"summary":"new"}}`)
	if r.Meta.Summary != "new" {
		t.Fatalf("later $set summary %q", r.Meta.Summary)
	}
}
