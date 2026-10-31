//go:build cgo

package collect

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tracewire"
)

func newModeStore(t *testing.T, mode SupersededMode) *DuckDBStore {
	t.Helper()
	store, err := NewDuckDBStore(filepath.Join(t.TempDir(), "test.duckdb"), 100, 10*time.Millisecond, mode)
	if err != nil {
		t.Fatalf("NewDuckDBStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func entry(uuid, text string) thinkt.Entry {
	return thinkt.Entry{UUID: uuid, Role: thinkt.RoleAssistant, Text: text,
		Timestamp: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC).Add(time.Duration(len(uuid)) * time.Second),
		Usage:     &thinkt.TokenUsage{InputTokens: 10, OutputTokens: 4}}
}

func payload(machine, session string, gen int64, entries []thinkt.Entry, removed ...string) tracewire.Payload {
	return tracewire.Payload{
		Version: tracewire.Version, InstanceID: "inst", MachineID: machine, Source: "gemini",
		Session: tracewire.Session{ID: session, ProjectPath: "/p", Title: "T", Generation: gen},
		Entries: entries, Removed: removed,
	}
}

// ingest writes payloads and waits for the batch writer to flush them.
func ingest(t *testing.T, s *DuckDBStore, ps ...tracewire.Payload) {
	t.Helper()
	for _, p := range ps {
		if err := s.IngestBatch(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	s.Flush()
}

func liveTexts(t *testing.T, s *DuckDBStore, session string) map[string]string {
	t.Helper()
	es, err := s.QueryEntries(context.Background(), session, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range es {
		out[e.UUID] = e.Text
	}
	return out
}

func superseded(t *testing.T, s *DuckDBStore, uuid string) (exists, marked bool) {
	t.Helper()
	var at sql.NullTime
	err := s.db.QueryRow(`SELECT superseded_at FROM entries WHERE uuid = ?`, uuid).Scan(&at)
	if err == sql.ErrNoRows {
		return false, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return true, at.Valid
}

func TestIngestUpsertsByKey(t *testing.T) {
	s := newModeStore(t, SupersededKeep)
	ingest(t, s, payload("m", "s", 1, []thinkt.Entry{entry("u1", "a"), entry("u2", "b")}))
	ingest(t, s, payload("m", "s", 2, []thinkt.Entry{entry("u1", "a2")}))
	got := liveTexts(t, s, "s")
	if len(got) != 2 || got["u1"] != "a2" || got["u2"] != "b" {
		t.Fatalf("%v", got)
	}
}

func TestIngestRemovedKeepMarksSuperseded(t *testing.T) {
	s := newModeStore(t, SupersededKeep)
	ingest(t, s, payload("m", "s", 1, []thinkt.Entry{entry("u1", "a"), entry("u2", "b")}))
	ingest(t, s, payload("m", "s", 2, nil, "u2"))
	if got := liveTexts(t, s, "s"); len(got) != 1 || got["u1"] != "a" {
		t.Fatalf("%v", got)
	}
	if exists, marked := superseded(t, s, "u2"); !exists || !marked {
		t.Fatalf("u2 exists=%v marked=%v", exists, marked)
	}
}

func TestIngestRemovedDelete(t *testing.T) {
	s := newModeStore(t, SupersededDelete)
	ingest(t, s, payload("m", "s", 1, []thinkt.Entry{entry("u1", "a"), entry("u2", "b")}))
	ingest(t, s, payload("m", "s", 2, nil, "u2"))
	if exists, _ := superseded(t, s, "u2"); exists {
		t.Fatal("u2 still stored")
	}
}

func TestIngestReaddClearsSuperseded(t *testing.T) {
	s := newModeStore(t, SupersededKeep)
	ingest(t, s, payload("m", "s", 1, []thinkt.Entry{entry("u1", "a")}))
	ingest(t, s, payload("m", "s", 2, nil, "u1"))
	ingest(t, s, payload("m", "s", 3, []thinkt.Entry{entry("u1", "back")}))
	if got := liveTexts(t, s, "s"); got["u1"] != "back" {
		t.Fatalf("%v", got)
	}
}

func TestIngestStaleGenerationIgnored(t *testing.T) {
	s := newModeStore(t, SupersededKeep)
	ingest(t, s, payload("m", "s", 5, []thinkt.Entry{entry("u1", "five")}))
	ingest(t, s, payload("m", "s", 4, []thinkt.Entry{entry("u1", "four")}))
	if got := liveTexts(t, s, "s"); got["u1"] != "five" {
		t.Fatalf("%v", got)
	}
}

func TestIngestSameGenerationChunksAllApply(t *testing.T) {
	s := newModeStore(t, SupersededKeep)
	ingest(t, s, payload("m", "s", 3, []thinkt.Entry{entry("u1", "a")}), payload("m", "s", 3, []thinkt.Entry{entry("u2", "b")}))
	if got := liveTexts(t, s, "s"); len(got) != 2 {
		t.Fatalf("%v", got)
	}
}

func TestIngestRemovalOnly(t *testing.T) {
	s := newModeStore(t, SupersededKeep)
	ingest(t, s, payload("m", "s", 1, []thinkt.Entry{entry("u1", "a")}))
	ingest(t, s, payload("m", "s", 2, nil, "u1"))
	if _, marked := superseded(t, s, "u1"); !marked {
		t.Fatal("u1 not superseded")
	}
}

func TestIngestKeepsActivity(t *testing.T) {
	s := newModeStore(t, SupersededKeep)
	ctx := context.Background()
	ingest(t, s, payload("m", "s", 1, []thinkt.Entry{entry("u1", "a")}))
	var first time.Time
	if err := s.db.QueryRow(`SELECT first_seen FROM sessions`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 30, 13, 0, 0, 0, time.UTC)
	if err := s.RecordSessionActivity(ctx, SessionActivityEvent{MachineID: "m", Source: "gemini", SessionID: "s", Event: "session_active", Timestamp: at}); err != nil {
		t.Fatal(err)
	}
	ingest(t, s, payload("m", "s", 2, []thinkt.Entry{entry("u2", "b")}))
	var status string
	var last, first2 time.Time
	if err := s.db.QueryRow(`SELECT status, last_activity, first_seen FROM sessions`).Scan(&status, &last, &first2); err != nil {
		t.Fatal(err)
	}
	if status != "active" || !last.Equal(at) || !first2.Equal(first) {
		t.Fatalf("status %q last %v first %v→%v", status, last, first, first2)
	}
}

func TestActiveSessionsByFullKey(t *testing.T) {
	s := newModeStore(t, SupersededKeep)
	ctx := context.Background()
	ingest(t, s, payload("A", "s", 1, []thinkt.Entry{entry("u1", "a")}), payload("B", "s", 1, []thinkt.Entry{entry("u1", "b")}))
	t1 := time.Date(2026, 10, 30, 13, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	_ = s.RecordSessionActivity(ctx, SessionActivityEvent{MachineID: "A", Source: "gemini", SessionID: "s", Event: "session_active", Timestamp: t1})
	_ = s.RecordSessionActivity(ctx, SessionActivityEvent{MachineID: "B", Source: "gemini", SessionID: "s", Event: "session_active", Timestamp: t2})
	active, err := s.QueryActiveSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 {
		t.Fatalf("%+v", active)
	}
	for _, a := range active {
		want := map[string]time.Time{"A": t1, "B": t2}[a.MachineID]
		if !a.LastActivity.Equal(want) {
			t.Errorf("%s: last activity %v, want %v", a.MachineID, a.LastActivity, want)
		}
	}
}

func TestTwoMachinesSameSessionID(t *testing.T) {
	s := newModeStore(t, SupersededKeep)
	ingest(t, s,
		payload("A", "s", 1, []thinkt.Entry{entry("u1", "a"), entry("u2", "a")}),
		payload("B", "s", 1, []thinkt.Entry{entry("u1", "b"), entry("u2", "b")}))
	sessions, err := s.QuerySessions(context.Background(), SessionFilter{})
	if err != nil || len(sessions) != 2 {
		t.Fatalf("%+v %v", sessions, err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("entries %d %v", n, err)
	}
}

func TestDropsOldTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE collected_entries (uuid VARCHAR PRIMARY KEY); CREATE TABLE collected_sessions (id VARCHAR PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := NewDuckDBStore(path, 100, 10*time.Millisecond, SupersededKeep)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name IN ('collected_entries','collected_sessions')`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("old tables left: %d %v", n, err)
	}
}

func TestSearchAndStatsOverLiveEntries(t *testing.T) {
	s := newModeStore(t, SupersededKeep)
	ctx := context.Background()
	ingest(t, s, payload("m", "s", 1, []thinkt.Entry{entry("u1", "needle here"), entry("u2", "other")}))
	ingest(t, s, payload("m", "s", 2, nil, "u2"))
	found, err := s.SearchTraces(ctx, "needle", 10)
	if err != nil || len(found) != 1 || found[0].ID != "s" {
		t.Fatalf("%+v %v", found, err)
	}
	stats, err := s.GetUsageStats(ctx)
	if err != nil || stats.TotalTraces != 1 || stats.TotalSessions != 1 {
		t.Fatalf("%+v %v", stats, err)
	}
}

func TestSummarize(t *testing.T) {
	e := thinkt.Entry{UUID: "u", Role: thinkt.RoleAssistant, Text: "t", Model: "m",
		Usage: &thinkt.TokenUsage{InputTokens: 3, OutputTokens: 2},
		ContentBlocks: []thinkt.ContentBlock{
			{Type: "thinking", Thinking: "abcd"},
			{Type: "tool_use", ToolName: "read_file"},
			{Type: "tool_result", IsError: true},
		}}
	got := summarize(e)
	if got.ToolName != "read_file" || !got.HasToolUse || !got.HasThinking || got.ThinkingLen != 4 ||
		!got.IsError || got.InputTokens != 3 || got.OutputTokens != 2 || len(got.ContentBlocks) != 3 {
		t.Fatalf("%+v", got)
	}
}
