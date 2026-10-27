package claude

import (
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

const modernFixture = "testdata/modern-session.jsonl"

// The fixture has 14 lines; 7 are conversation records:
// user, attachment(instructions), assistant, assistant, user(tool result),
// system(turn_duration), file-history-snapshot.
const modernFixtureEntries = 7

func TestModernFixtureEntryCount(t *testing.T) {
	p, err := NewParserFromFile(modernFixture)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := p.ReadAllEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != modernFixtureEntries {
		t.Fatalf("want %d entries, got %d", modernFixtureEntries, len(entries))
	}
	if got := countConversationRecords(modernFixture); got != modernFixtureEntries {
		t.Fatalf("countConversationRecords = %d, want %d", got, modernFixtureEntries)
	}
	if len(p.Errors()) != 0 {
		t.Fatalf("skipped lines must not be errors: %v", p.Errors())
	}
}

func TestLazySessionSkipsBookkeeping(t *testing.T) {
	ls, err := OpenLazySession(modernFixture)
	if err != nil {
		t.Fatal(err)
	}
	defer ls.Close()
	if err := ls.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if ls.EntryCount() != modernFixtureEntries {
		t.Fatalf("got %d entries", ls.EntryCount())
	}
	for _, e := range ls.Entries() {
		if e.Role == thinkt.RoleSystem && e.Text == "" {
			t.Fatalf("empty system entry leaked: %+v", e)
		}
	}
}

func TestLoadSessionWindowSkipsBookkeeping(t *testing.T) {
	w, err := LoadSessionWindow(modernFixture, 0, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if w.EntryCount != modernFixtureEntries || len(w.Session.Entries) != modernFixtureEntries {
		t.Fatalf("window entries = %d / %d", w.EntryCount, len(w.Session.Entries))
	}
}

func TestUnknownTypeKeepsRaw(t *testing.T) {
	p := NewParser(strings.NewReader(`{"type":"brand-new","uuid":"n","x":1}` + "\n"))
	entry, err := p.NextEntry()
	if err != nil || entry == nil {
		t.Fatalf("entry %v err %v", entry, err)
	}
	if len(entry.Raw) == 0 {
		t.Fatal("unknown record must keep its raw line")
	}
	out := convertEntry(entry, thinkt.SourceClaude, "")
	if !strings.Contains(out.Text, `"x": 1`) || out.Kind != thinkt.KindUnknown {
		t.Fatalf("unknown conversion: %+v", out)
	}
}
