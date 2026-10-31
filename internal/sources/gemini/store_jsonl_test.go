package gemini

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

const (
	mainHeaderID = "17503250-82e4-479a-ab9d-69002dcb6d31"
	mainFirstMsg = "1dc77037-8568-4abb-869e-f2a2b837afe5"
)

// copyTo copies a testdata file to base/rel.
func copyTo(t *testing.T, src, base, rel string) string {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	return put(t, base, rel, string(b))
}

func jsonSession(sessionID, hash, lastMsg string) string {
	return `{"sessionId":"` + sessionID + `","projectHash":"` + hash + `","startTime":"2026-02-06T13:11:00Z","lastUpdated":"2026-02-06T13:12:00Z","messages":[` +
		`{"id":"` + lastMsg + `","timestamp":"2026-02-06T13:11:30Z","type":"user","content":[{"text":"old prompt"}]}]}`
}

// newHome lays out a slug folder "proj" holding the main fixture and its
// subagent, and returns the base directory and a store over it.
func newHome(t *testing.T) (string, *Store) {
	t.Helper()
	base := t.TempDir()
	put(t, base, "tmp/proj/.project_root", devProj+"\n")
	copyTo(t, mainFixture, base, "tmp/proj/chats/"+mainStem+".jsonl")
	copyTo(t, subFixture, base, "tmp/proj/chats/"+mainHeaderID+"/92szal.jsonl")
	return base, NewStoreWithCacheDir(base, t.TempDir())
}

func onlyProject(t *testing.T, s *Store) thinkt.Project {
	t.Helper()
	ps, err := s.ListProjects(context.Background())
	if err != nil || len(ps) != 1 {
		t.Fatalf("projects = %+v, %v", ps, err)
	}
	return ps[0]
}

func sessionIDs(ms []thinkt.SessionMeta) map[string]thinkt.SessionMeta {
	out := map[string]thinkt.SessionMeta{}
	for _, m := range ms {
		out[m.ID] = m
	}
	return out
}

func TestListProjectsMergesHashAndSlugFolders(t *testing.T) {
	base, s := newHome(t)
	h := hashOf(devProj)
	put(t, base, "tmp/"+h+"/chats/session-2026-02-06T13-11-dac0f3bc.json", jsonSession("old", h, "m1"))
	p := onlyProject(t, s)
	if p.ID != h || p.Path != devProj || p.Name != "proj" || p.SessionCount != 2 || p.SubagentCount != 1 {
		t.Fatalf("project = %+v", p)
	}
}

func TestListSessionsBothFormats(t *testing.T) {
	base, s := newHome(t)
	h := hashOf(devProj)
	put(t, base, "tmp/proj/chats/session-2026-02-06T13-11-dac0f3bc.json", jsonSession("old", h, "m1"))
	ms, err := s.ListSessions(context.Background(), h)
	if err != nil || len(ms) != 2 {
		t.Fatalf("%+v %v", ms, err)
	}
	got := sessionIDs(ms)
	if _, ok := got[mainStem]; !ok {
		t.Fatalf("ids %v", got)
	}
	if _, ok := got["session-2026-02-06T13-11-dac0f3bc"]; !ok {
		t.Fatalf("ids %v", got)
	}
	if got[mainStem].ProjectPath != devProj || got[mainStem].SubagentCount != 1 {
		t.Fatalf("main = %+v", got[mainStem])
	}
}

func TestListSessionsSkipsSupersededJSON(t *testing.T) {
	base, s := newHome(t)
	put(t, base, "tmp/proj/chats/"+mainStem+".json", jsonSession(mainHeaderID, hashOf(devProj), mainFirstMsg))
	ms, _ := s.ListSessions(context.Background(), hashOf(devProj))
	if len(ms) != 1 || ms[0].ID != mainStem || filepath.Ext(ms[0].FullPath) != ".jsonl" {
		t.Fatalf("%+v", ms)
	}
}

func TestListSessionsKeepsJSONBranchWithSameID(t *testing.T) {
	base, s := newHome(t)
	put(t, base, "tmp/proj/chats/session-2026-02-06T13-11-17503250.json", jsonSession(mainHeaderID, hashOf(devProj), "other"))
	ms, _ := s.ListSessions(context.Background(), hashOf(devProj))
	if len(ms) != 2 {
		t.Fatalf("%+v", ms)
	}
}

func TestListSessionsDedupesAcrossFolders(t *testing.T) {
	base, s := newHome(t)
	h := hashOf(devProj)
	name := "session-2026-02-06T13-11-dac0f3bc.json"
	put(t, base, "tmp/proj/chats/"+name, jsonSession("old", h, "m1"))
	put(t, base, "tmp/"+h+"/chats/"+name, jsonSession("old", h, "m1"))
	ms, _ := s.ListSessions(context.Background(), h)
	if len(ms) != 2 {
		t.Fatalf("%+v", ms)
	}
	for _, m := range ms {
		if filepath.Ext(m.FullPath) == ".json" && filepath.Base(filepath.Dir(filepath.Dir(m.FullPath))) != "proj" {
			t.Fatalf("kept %s", m.FullPath)
		}
	}
	loser := filepath.Join(base, "tmp", h, "chats", name)
	if m, err := s.GetSessionMeta(context.Background(), loser); m != nil || err != nil {
		t.Fatalf("loser resolved to %+v %v", m, err)
	}
}

func TestSubagentsOnlyWithOption(t *testing.T) {
	_, s := newHome(t)
	h := hashOf(devProj)
	ms, _ := s.ListSessions(context.Background(), h)
	if len(ms) != 1 {
		t.Fatalf("%+v", ms)
	}
	all, _ := s.ListSessions(context.Background(), h, thinkt.WithSubagents())
	got := sessionIDs(all)
	child, ok := got[mainStem+"/92szal"]
	if len(all) != 2 || !ok || child.ParentSessionID != mainStem {
		t.Fatalf("%+v", all)
	}
}

func TestOpenJSONLSession(t *testing.T) {
	base, s := newHome(t)
	path := filepath.Join(base, "tmp/proj/chats/"+mainStem+".jsonl")
	sess, err := s.LoadSession(context.Background(), path)
	if err != nil || sess == nil {
		t.Fatalf("%v %v", sess, err)
	}
	want := convert(replayFile(t, mainFixture), mainStem)
	for i := range want {
		want[i].WorkspaceID = sess.Meta.WorkspaceID
	}
	if !reflect.DeepEqual(sess.Entries, want) {
		t.Fatalf("entries differ: %d vs %d", len(sess.Entries), len(want))
	}
	if sess.Meta.ID != mainStem {
		t.Fatalf("meta %+v", sess.Meta)
	}
}

func TestUnknownPathProjectOpens(t *testing.T) {
	base := t.TempDir()
	h := hashOf("/somewhere/else")
	b, err := os.ReadFile(mainFixture)
	if err != nil {
		t.Fatal(err)
	}
	data := string(b)
	data = replaceAll(data, hashOf(devProj), h)
	path := put(t, base, "tmp/"+h+"/chats/"+mainStem+".jsonl", data)
	s := NewStoreWithCacheDir(base, t.TempDir())
	p := onlyProject(t, s)
	if p.ID != h || p.Path != "" || p.DisplayPath != "gemini://"+h[:8] {
		t.Fatalf("%+v", p)
	}
	r, err := s.OpenSession(context.Background(), path)
	if err != nil || r == nil {
		t.Fatalf("%v", err)
	}
	defer r.Close()
	if _, err := r.ReadNext(); err != nil {
		t.Fatal(err)
	}
}

func TestNoHeaderFileNotListed(t *testing.T) {
	base, s := newHome(t)
	put(t, base, "tmp/proj/chats/session-2026-05-18T00-00-deadbeef.jsonl", userLine("u", "hi")+"\n")
	ms, _ := s.ListSessions(context.Background(), hashOf(devProj))
	if len(ms) != 1 {
		t.Fatalf("%+v", ms)
	}
}

func TestResumeCommand(t *testing.T) {
	bin := t.TempDir()
	put(t, bin, "gemini", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(bin, "gemini"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	_, s := newHome(t)
	all, _ := s.ListSessions(context.Background(), hashOf(devProj), thinkt.WithSubagents())
	for _, m := range all {
		info, err := s.ResumeCommand(m)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"gemini", "--resume", mainHeaderID}
		if !reflect.DeepEqual(info.Args, want) || info.Dir != devProj {
			t.Fatalf("%s: %+v", m.ID, info)
		}
	}
}

func TestWatchConfigDepth(t *testing.T) {
	wc := NewStore(t.TempDir()).WatchConfig()
	if wc.MaxDepth != 4 || len(wc.IncludeDirs) != 1 || wc.IncludeDirs[0] != "tmp" {
		t.Fatalf("%+v", wc)
	}
}
