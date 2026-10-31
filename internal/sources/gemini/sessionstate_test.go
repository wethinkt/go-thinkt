package gemini

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeJSONL(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "session-x.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func userLine(id, text string) string {
	return `{"id":"` + id + `","timestamp":"2026-05-17T11:00:01Z","type":"user","content":[{"text":"` + text + `"}]}`
}

func TestScanJSONLFixture(t *testing.T) {
	s, err := scanJSONL(mainFixture, mainStem, "")
	if err != nil || s == nil {
		t.Fatalf("%v %v", s, err)
	}
	m := s.Meta
	want := time.Date(2026, 5, 17, 15, 7, 20, 824e6, time.UTC)
	if m.ID != mainStem || s.HeaderID != "17503250-82e4-479a-ab9d-69002dcb6d31" || m.ParentSessionID != "" {
		t.Fatalf("ids: %q %q %q", m.ID, s.HeaderID, m.ParentSessionID)
	}
	if !m.CreatedAt.Equal(time.Date(2026, 5, 17, 15, 5, 1, 725e6, time.UTC)) || !m.ModifiedAt.Equal(want) {
		t.Fatalf("times: %v %v", m.CreatedAt, m.ModifiedAt)
	}
	if m.FirstPrompt != "text 1" || m.Title != "" || m.Model != "gemini-3-flash-preview" || m.EntryCount != 38 {
		t.Fatalf("meta: %+v", m)
	}
	if m.Cost == nil || m.Cost.InputTokens != 203729 || m.Cost.OutputTokens != 5107 || m.Cost.CachedInputTokens != 149438 {
		t.Fatalf("cost: %+v", m.Cost)
	}
	if len(s.MessageIDs) != 11 || m.FullPath != mainFixture || m.FileSize == 0 {
		t.Fatalf("ids %d path %q size %d", len(s.MessageIDs), m.FullPath, m.FileSize)
	}
}

func TestScanJSONLTitleRules(t *testing.T) {
	for summary, want := range map[string]string{
		`"Fix the build"`:             "Fix the build",
		`"{\"RelevantLocations\":1}"`: "",
		`"line one\nline two"`:        "",
	} {
		p := writeJSONL(t, hdr, userLine("u", "hi"), `{"$set":{"summary":`+summary+`}}`)
		s, err := scanJSONL(p, "session-x", "")
		if err != nil || s.Meta.Title != want {
			t.Errorf("summary %s: title %q, want %q (%v)", summary, s.Meta.Title, want, err)
		}
	}
}

func TestScanJSONLFirstPromptSkipsSlash(t *testing.T) {
	p := writeJSONL(t, hdr, userLine("u1", "/model"), userLine("u2", "fix it"))
	s, err := scanJSONL(p, "session-x", "")
	if err != nil || s.Meta.FirstPrompt != "fix it" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestScanJSONLSubagentIDs(t *testing.T) {
	s, err := scanJSONL(subFixture, "92szal", mainStem)
	if err != nil || s == nil {
		t.Fatal(err)
	}
	if s.Meta.ID != mainStem+"/92szal" || s.Meta.ParentSessionID != mainStem || s.HeaderID != "92szal" {
		t.Fatalf("%+v", s.Meta)
	}
}

func TestScanJSONLNoHeader(t *testing.T) {
	p := writeJSONL(t, userLine("u", "hi"))
	if s, err := scanJSONL(p, "session-x", ""); s != nil || err != nil {
		t.Fatalf("%+v %v", s, err)
	}
}
