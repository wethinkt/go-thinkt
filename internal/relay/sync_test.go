package relay

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tracewire"
)

func ent(uuid, text string) thinkt.Entry {
	return thinkt.Entry{UUID: uuid, Role: thinkt.RoleUser, Text: text}
}

func meta() thinkt.SessionMeta {
	return thinkt.SessionMeta{ID: "s1", Title: "Title", ProjectPath: "/p", Source: thinkt.SourceGemini}
}

var base = tracewire.Payload{Version: tracewire.Version, InstanceID: "i", MachineID: "m", Source: "gemini"}

func uuids(es []thinkt.Entry) string {
	var out []string
	for _, e := range es {
		out = append(out, e.UUID)
	}
	return strings.Join(out, ",")
}

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay", "state.json")
	st := &relayState{Sessions: map[string]*sessionState{
		stateKey(thinkt.SourceGemini, "s1"): {Hashes: map[string]string{"u1": "h"}, ModTime: time.Now(), Size: 9, Generation: 3, Path: "/x"},
	}}
	if err := st.save(path); err != nil {
		t.Fatal(err)
	}
	got, err := loadState(path)
	if err != nil {
		t.Fatal(err)
	}
	s := got.Sessions[stateKey(thinkt.SourceGemini, "s1")]
	want := st.Sessions[stateKey(thinkt.SourceGemini, "s1")]
	if s == nil || s.Generation != 3 || s.Size != 9 || !s.ModTime.Equal(want.ModTime) || s.Hashes["u1"] != "h" {
		t.Fatalf("%+v", s)
	}
}

func TestStateMissingFileIsEmpty(t *testing.T) {
	st, err := loadState(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || st == nil || len(st.Sessions) != 0 {
		t.Fatalf("%+v %v", st, err)
	}
}

func TestDiffNewSession(t *testing.T) {
	ps := diffSession(meta(), []thinkt.Entry{ent("u1", "a"), ent("u2", "b"), ent("u3", "c")}, &sessionState{}, base, 100, gen(&sessionState{}))
	if len(ps) != 1 || uuids(ps[0].Entries) != "u1,u2,u3" || ps[0].Session.Generation != 1 || ps[0].Removed != nil {
		t.Fatalf("%+v", ps)
	}
	if ps[0].Session.Title != "Title" || ps[0].Session.ProjectPath != "/p" || ps[0].Session.ID != "s1" || ps[0].MachineID != "m" {
		t.Fatalf("session %+v", ps[0].Session)
	}
}

func TestDiffNoChange(t *testing.T) {
	es := []thinkt.Entry{ent("u1", "a"), ent("u2", "b")}
	st := &sessionState{}
	for _, p := range diffSession(meta(), es, st, base, 100, gen(st)) {
		st.commit(p)
	}
	if ps := diffSession(meta(), es, st, base, 100, gen(st)); len(ps) != 0 {
		t.Fatalf("%+v", ps)
	}
}

func TestDiffChangedAndRemoved(t *testing.T) {
	st := &sessionState{}
	for _, p := range diffSession(meta(), []thinkt.Entry{ent("u1", "a"), ent("u2", "b"), ent("u3", "c")}, st, base, 100, gen(st)) {
		st.commit(p)
	}
	ps := diffSession(meta(), []thinkt.Entry{ent("u1", "a2"), ent("u2", "b")}, st, base, 100, gen(st))
	if len(ps) != 1 || uuids(ps[0].Entries) != "u1" || strings.Join(ps[0].Removed, ",") != "u3" || ps[0].Session.Generation != 2 {
		t.Fatalf("%+v", ps)
	}
}

func TestDiffRemovalOnly(t *testing.T) {
	st := &sessionState{}
	for _, p := range diffSession(meta(), []thinkt.Entry{ent("u1", "a"), ent("u2", "b")}, st, base, 100, gen(st)) {
		st.commit(p)
	}
	ps := diffSession(meta(), []thinkt.Entry{ent("u1", "a")}, st, base, 100, gen(st))
	if len(ps) != 1 || len(ps[0].Entries) != 0 || strings.Join(ps[0].Removed, ",") != "u2" {
		t.Fatalf("%+v", ps)
	}
	st.commit(ps[0])
	if _, ok := st.Hashes["u2"]; ok {
		t.Fatal("commit kept a removed uuid")
	}
}

func TestDiffMetadataOnly(t *testing.T) {
	es := []thinkt.Entry{ent("u1", "a")}
	st := &sessionState{}
	for _, p := range diffSession(meta(), es, st, base, 100, gen(st)) {
		st.commit(p)
	}
	m := meta()
	m.Title = "Renamed"
	ps := diffSession(m, es, st, base, 100, gen(st))
	if len(ps) != 1 || len(ps[0].Entries) != 0 || ps[0].Session.Title != "Renamed" {
		t.Fatalf("%+v", ps)
	}
}

func TestDiffSplitsByBatchSize(t *testing.T) {
	var es []thinkt.Entry
	for i := 0; i < 250; i++ {
		es = append(es, ent(fmt.Sprintf("u%03d", i), "x"))
	}
	st := &sessionState{Hashes: map[string]string{"gone": "h"}, Generation: 0}
	ps := diffSession(meta(), es, st, base, 100, gen(st))
	if len(ps) != 3 {
		t.Fatalf("%d payloads", len(ps))
	}
	for i, p := range ps {
		if p.Session.Generation != 1 || p.Session.Title != "Title" {
			t.Errorf("payload %d: %+v", i, p.Session)
		}
		if (i == 2) != (len(p.Removed) == 1) {
			t.Errorf("payload %d removed %v", i, p.Removed)
		}
	}
}

func TestDiffOversizedEntryAlone(t *testing.T) {
	big := ent("big", strings.Repeat("x", 9<<20))
	ps := diffSession(meta(), []thinkt.Entry{ent("a", "1"), big, ent("b", "2")}, &sessionState{}, base, 100, gen(&sessionState{}))
	for _, p := range ps {
		for _, e := range p.Entries {
			if e.UUID == "big" && len(p.Entries) != 1 {
				t.Fatalf("big entry shares a payload: %s", uuids(p.Entries))
			}
		}
	}
	if len(ps) < 2 {
		t.Fatalf("%d payloads", len(ps))
	}
}

func TestAssignIDs(t *testing.T) {
	es := []thinkt.Entry{ent("", "same"), ent("", "same"), ent("", "other"), ent("keep", "x")}
	assignIDs(es)
	seen := map[string]bool{}
	for _, e := range es {
		if e.UUID == "" || seen[e.UUID] {
			t.Fatalf("ids %s", uuids(es))
		}
		seen[e.UUID] = true
	}
	if es[3].UUID != "keep" || !strings.HasPrefix(es[0].UUID, "relay-noid:") {
		t.Fatalf("ids %s", uuids(es))
	}
	again := []thinkt.Entry{ent("", "same"), ent("", "same"), ent("", "other"), ent("keep", "x")}
	assignIDs(again)
	if uuids(again) != uuids(es) {
		t.Fatalf("not stable: %s vs %s", uuids(again), uuids(es))
	}
}

// gen is the generation the old tests expect: one past the state's.
func gen(st *sessionState) int64 { return st.Generation + 1 }

func TestNextGenerationMonotonic(t *testing.T) {
	now := time.Unix(100, 0)
	if g := nextGeneration(&sessionState{}, now); g != now.UnixNano() {
		t.Fatalf("fresh state: %d", g)
	}
	if g := nextGeneration(&sessionState{Generation: now.UnixNano() + 5}, now); g != now.UnixNano()+6 {
		t.Fatalf("clock behind state: %d", g)
	}
}
