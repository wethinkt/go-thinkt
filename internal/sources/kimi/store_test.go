package kimi

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// storeFixture builds a Kimi Code tree: a v2 session with one subagent in
// wd_proj_abc, an older-shape migrated session in wd_old_def, the session
// index, and workspaces.json. It returns the base directory.
func storeFixture(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	sessionDirFixture(t, base, "wd_proj_abc", "session_p1", stateV2, "agent-0")
	old := filepath.Join(base, "sessions", "wd_old_def", "ses_l1")
	writeFile(t, filepath.Join(old, "state.json"), stateOld)
	copyFixture(t, "wire-1.0.jsonl", filepath.Join(old, "agents", "main", "wire.jsonl"))
	writeFile(t, filepath.Join(base, "session_index.jsonl"),
		`{"sessionId":"session_p1","sessionDir":"`+filepath.Join(base, "sessions", "wd_proj_abc", "session_p1")+`","workDir":"/proj"}`+"\n"+
			`{"sessionId":"ses_l1","sessionDir":"`+old+`","workDir":"/Users/evan/projects/camelkoons"}`+"\n")
	writeFile(t, filepath.Join(base, "workspaces.json"), `{"version":1,"workspaces":{"wd_proj_abc":{"root":"/proj","name":"proj"},"wd_old_def":{"root":"/Users/evan/projects/camelkoons","name":"camelkoons"}}}`)
	writeFile(t, filepath.Join(base, "device_id"), "device-123\n")
	return base
}

func newTestStore(t *testing.T, base string) *Store {
	t.Helper()
	return NewStoreWithCacheDir(base, filepath.Join(base, "cache"))
}

func TestWorkspaceUsesDeviceID(t *testing.T) {
	base := storeFixture(t)
	ws := newTestStore(t, base).Workspace()
	if ws.ID != "device-123" || ws.Source != thinkt.SourceKimi || ws.BasePath != base {
		t.Fatalf("workspace = %+v", ws)
	}
}

func TestListProjectsUsesWorkDirPathAndCountsSubagents(t *testing.T) {
	store := newTestStore(t, storeFixture(t))
	projects, err := store.ListProjects(context.Background())
	if err != nil || len(projects) != 2 {
		t.Fatalf("projects = %+v err = %v", projects, err)
	}
	byID := map[string]thinkt.Project{}
	for _, p := range projects {
		byID[p.ID] = p
	}
	proj, ok := byID["/proj"]
	if !ok || proj.Path != "/proj" || proj.Name != "proj" || proj.SessionCount != 1 || proj.SubagentCount != 1 || proj.Source != thinkt.SourceKimi {
		t.Fatalf("proj = %+v", proj)
	}
	old, ok := byID["/Users/evan/projects/camelkoons"]
	if !ok || old.SessionCount != 1 || old.SubagentCount != 0 {
		t.Fatalf("old = %+v", old)
	}
}

func TestListSessionsHidesSubagentsByDefault(t *testing.T) {
	store := newTestStore(t, storeFixture(t))
	ctx := context.Background()
	sessions, err := store.ListSessions(ctx, "/proj")
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %+v err = %v", sessions, err)
	}
	if s := sessions[0]; s.ID != "session_p1" || s.Title != "we've advanced this project" || s.SubagentCount != 1 || s.LastPrompt != "fix 3." || s.Source != thinkt.SourceKimi {
		t.Fatalf("session = %+v", s)
	}
	all, err := store.ListSessions(ctx, "/proj", thinkt.WithSubagents())
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %+v err = %v", all, err)
	}
	var child *thinkt.SessionMeta
	for i := range all {
		if all[i].ID == "session_p1/agent-0" {
			child = &all[i]
		}
	}
	if child == nil || child.ParentSessionID != "session_p1" || child.Title != "explore agent-0" {
		t.Fatalf("child = %+v", child)
	}
}

func TestListSessionsEnrichSyncFillsScan(t *testing.T) {
	store := newTestStore(t, storeFixture(t))
	ctx := context.Background()
	sessions, err := store.ListSessions(ctx, "/proj", thinkt.WithEnrichSync())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %+v err = %v", sessions, err)
	}
	s := sessions[0]
	if s.EntryCount != 17 || s.Cost == nil || s.Cost.DurationMs != 9000 || s.Cost.InputTokens != 150 || s.Model != "kimi-code/k3" || s.FirstPrompt != "fix the build" || s.LastPrompt != "fix 3." {
		t.Fatalf("session = %+v cost = %+v", s, s.Cost)
	}
}

func TestListSessionsParentIDSurvivesCacheHit(t *testing.T) {
	base := storeFixture(t)
	cacheDir := filepath.Join(base, "cache")
	ctx := context.Background()
	first := NewStoreWithCacheDir(base, cacheDir)
	if _, err := first.ListSessions(ctx, "/proj", thinkt.WithSubagents()); err != nil {
		t.Fatal(err)
	}
	second := NewStoreWithCacheDir(base, cacheDir)
	all, err := second.ListSessions(ctx, "/proj", thinkt.WithSubagents())
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %+v err = %v", all, err)
	}
	for _, s := range all {
		if s.ID == "session_p1/agent-0" && s.ParentSessionID != "session_p1" {
			t.Fatalf("child lost parent on cache hit: %+v", s)
		}
		if s.ID == "session_p1" && (s.SubagentCount != 1 || s.Title == "") {
			t.Fatalf("parent lost count or title on cache hit: %+v", s)
		}
	}
	hidden, err := second.ListSessions(ctx, "/proj")
	if err != nil || len(hidden) != 1 {
		t.Fatalf("hidden = %+v err = %v", hidden, err)
	}
}

func TestGetSessionMetaByIDPathAndChildID(t *testing.T) {
	base := storeFixture(t)
	store := newTestStore(t, base)
	ctx := context.Background()
	mainWire := filepath.Join(base, "sessions", "wd_proj_abc", "session_p1", "agents", "main", "wire.jsonl")
	childWire := filepath.Join(base, "sessions", "wd_proj_abc", "session_p1", "agents", "agent-0", "wire.jsonl")
	for _, q := range []struct{ query, id string }{
		{"session_p1", "session_p1"},
		{mainWire, "session_p1"},
		{"session_p1/agent-0", "session_p1/agent-0"},
		{childWire, "session_p1/agent-0"},
		{"ses_l1", "ses_l1"},
	} {
		meta, err := store.GetSessionMeta(ctx, q.query)
		if err != nil || meta == nil || meta.ID != q.id {
			t.Fatalf("%s → %+v err = %v", q.query, meta, err)
		}
		if q.id == "session_p1" && meta.EntryCount != 17 {
			t.Fatalf("%s not enriched: %+v", q.query, meta)
		}
		if q.id == "session_p1/agent-0" && meta.ParentSessionID != "session_p1" {
			t.Fatalf("child meta = %+v", meta)
		}
	}
	if meta, err := store.GetSessionMeta(ctx, "nobody"); err != nil || meta != nil {
		t.Fatalf("unknown = %+v err = %v", meta, err)
	}
	if meta, err := store.GetSessionMeta(ctx, filepath.Join(t.TempDir(), "wire.jsonl")); err != nil || meta != nil {
		t.Fatalf("outside path = %+v err = %v", meta, err)
	}
}

func TestOpenSessionStampsZeroTimestamps(t *testing.T) {
	store := newTestStore(t, storeFixture(t))
	ctx := context.Background()
	reader, err := store.OpenSession(ctx, "ses_l1")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	meta := reader.Metadata()
	n := 0
	for {
		e, err := reader.ReadNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		n++
		if !e.Timestamp.Equal(meta.CreatedAt) || e.WorkspaceID != meta.WorkspaceID {
			t.Fatalf("entry %s timestamp %v / workspace %q, want %v / %q", e.UUID, e.Timestamp, e.WorkspaceID, meta.CreatedAt, meta.WorkspaceID)
		}
	}
	if n != 5 || meta.CreatedAt.IsZero() {
		t.Fatalf("n = %d created = %v", n, meta.CreatedAt)
	}
}

func TestLoadSessionFixture15(t *testing.T) {
	store := newTestStore(t, storeFixture(t))
	session, err := store.LoadSession(context.Background(), "session_p1")
	if err != nil || session == nil {
		t.Fatalf("session = %+v err = %v", session, err)
	}
	if len(session.Entries) != 17 || session.Meta.ID != "session_p1" || session.Entries[0].Source != thinkt.SourceKimi || session.Entries[0].WorkspaceID != "device-123" {
		t.Fatalf("session = %+v", session.Meta)
	}
	if child, err := store.LoadSession(context.Background(), "session_p1/agent-0"); err != nil || child == nil || len(child.Entries) != 3 {
		t.Fatalf("child = %+v err = %v", child, err)
	}
}

func TestWatchConfigDepth(t *testing.T) {
	wc := newTestStore(t, t.TempDir()).WatchConfig()
	if wc.MaxDepth != 5 || len(wc.IncludeDirs) != 1 || wc.IncludeDirs[0] != "sessions" {
		t.Fatalf("watch = %+v", wc)
	}
}

func TestListProjectsEmptyTree(t *testing.T) {
	projects, err := newTestStore(t, t.TempDir()).ListProjects(context.Background())
	if err != nil || len(projects) != 0 {
		t.Fatalf("projects = %+v err = %v", projects, err)
	}
}
