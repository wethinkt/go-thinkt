package claude

import (
	"io"
	"strings"
	"testing"
)

// Unknown record types are emitted as entries so drift is visible; each
// reader also tallies them per file so a caller can report format drift.
func TestParserCountsUnknownTypes(t *testing.T) {
	jsonl := `{"type":"user","uuid":"1","message":{"role":"user","content":"hi"}}
{"type":"brand-new","uuid":"2"}
{"type":"brand-new","uuid":"3"}
{"type":"other-new","uuid":"4"}
`
	p := NewParser(strings.NewReader(jsonl))
	entries, err := p.ReadAllEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("entries = %d, want 4 (unknown types still emit)", len(entries))
	}
	got := p.UnknownTypes()
	if got["brand-new"] != 2 || got["other-new"] != 1 || len(got) != 2 {
		t.Fatalf("unknown tally = %v", got)
	}
	if NewParser(strings.NewReader(jsonl)).UnknownTypes() != nil {
		t.Fatal("tally must be nil before any unknown type is seen")
	}
}

func TestLazySessionCountsUnknownTypes(t *testing.T) {
	path := writeTempSession(t, t.TempDir(), "s.jsonl",
		`{"type":"user","uuid":"1","message":{"role":"user","content":"hi"}}`+"\n"+
			`{"type":"brand-new","uuid":"2"}`+"\n"+
			`{"type":"brand-new","uuid":"3"}`+"\n")
	ls, err := OpenLazySession(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ls.Close()
	for !ls.IsFullyLoaded() {
		if _, err := ls.LoadMore(1 << 20); err != nil && err != io.EOF {
			t.Fatal(err)
		}
	}
	if got := ls.UnknownTypes(); got["brand-new"] != 2 || len(got) != 1 {
		t.Fatalf("unknown tally = %v", got)
	}
}
