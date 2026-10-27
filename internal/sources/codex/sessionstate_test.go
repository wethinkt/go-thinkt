package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanSessionFileFixture(t *testing.T) {
	s := scanSessionFile("testdata/rollout-fixture.jsonl")
	if s.FirstPrompt != "fix the build" || s.LastPrompt != "fix the build" || s.Model != "gpt-5.5" || s.EntryCount != 39 {
		t.Fatalf("scan = %+v", s)
	}
	c := s.Cost
	if c == nil || c.DurationMs != 14000 || c.LinesAdded != 1 || c.LinesRemoved != 1 || c.InputTokens != 600 || c.OutputTokens != 60 || c.CachedInputTokens != 140 {
		t.Fatalf("cost = %+v", c)
	}
	if s.ParentSessionID != "" || s.Title != "" {
		t.Fatalf("scan = %+v", s)
	}
}

func TestScanSessionFileSumsEveryUsageRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.jsonl")
	writeLines(t, path,
		`{"timestamp":"2026-10-01T10:00:00Z","type":"session_meta","payload":{"id":"x","session_id":"x","cwd":"/w"}}`,
		`{"timestamp":"2026-10-01T10:00:03Z","type":"token_usage_record","payload":{"turn_id":"t","usage":{"input_tokens":5,"cached_input_tokens":1,"output_tokens":2}}}`,
		`{"timestamp":"2026-10-01T10:00:04Z","type":"token_usage_record","payload":{"turn_id":"t","usage":{"input_tokens":7,"cached_input_tokens":2,"output_tokens":3}}}`,
	)
	s := scanSessionFile(path)
	if s.Cost == nil || s.Cost.InputTokens != 12 || s.Cost.OutputTokens != 5 || s.Cost.CachedInputTokens != 3 || s.EntryCount != 0 {
		t.Fatalf("scan = %+v cost = %+v", s, s.Cost)
	}
}

func TestScanSessionFileCostNilWhenEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.jsonl")
	writeLines(t, path,
		`{"timestamp":"2026-10-01T10:00:00Z","type":"session_meta","payload":{"id":"x","session_id":"x","cwd":"/w","model_provider":"openai"}}`,
		`{"timestamp":"2026-10-01T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"legacy hello"}}`,
	)
	s := scanSessionFile(path)
	if s.Cost != nil || s.FirstPrompt != "legacy hello" || s.Model != "openai" || s.EntryCount != 1 {
		t.Fatalf("scan = %+v", s)
	}
}

func TestScanSessionFileSubagentParentAndTitleFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.jsonl")
	writeLines(t, path,
		`{"timestamp":"2026-10-01T10:00:00Z","type":"session_meta","payload":{"id":"c","session_id":"p","cwd":"/w","source":{"subagent":{"thread_spawn":{"parent_thread_id":"p","depth":1,"agent_path":"/root/x","agent_nickname":"Kierkegaard"}}},"agent_nickname":"Kierkegaard","agent_path":"/root/x"}}`,
		`{"timestamp":"2026-10-01T10:00:01Z","type":"event_msg","payload":{"type":"thread_settings_applied","thread_settings":{"model":"gpt-5.5","reasoning_effort":"high"}}}`,
	)
	s := scanSessionFile(path)
	if s.ParentSessionID != "p" || s.Title != "Kierkegaard /root/x" || s.Model != "gpt-5.5" {
		t.Fatalf("scan = %+v", s)
	}
}

func TestSessionMetaRecordParentThreadID(t *testing.T) {
	var r sessionMetaRecord
	r.Source = []byte(`"cli"`)
	if r.parentThreadID() != "" {
		t.Fatal("string source must have no parent")
	}
	r.Source = []byte(`{"subagent":{"thread_spawn":{"parent_thread_id":"p"}}}`)
	if r.parentThreadID() != "p" {
		t.Fatalf("parent = %q", r.parentThreadID())
	}
}

func TestLoadSessionIndexLastLineWins(t *testing.T) {
	base := t.TempDir()
	writeLines(t, filepath.Join(base, "session_index.jsonl"),
		`{"id":"a","thread_name":"old","updated_at":"2026-10-01T10:00:00Z"}`,
		`{"id":"b","thread_name":"other"}`,
		`{"id":"a","thread_name":"new","updated_at":"2026-10-02T10:00:00Z"}`,
	)
	titles := loadSessionIndex(base)
	if titles["a"] != "new" || titles["b"] != "other" || len(titles) != 2 {
		t.Fatalf("titles = %v", titles)
	}
}

func TestLoadSessionIndexSkipsBadLines(t *testing.T) {
	base := t.TempDir()
	writeLines(t, filepath.Join(base, "session_index.jsonl"),
		`not json`,
		`{"id":"a"}`,
		`{"id":"b","thread_name":""}`,
		`{"id":"c","thread_name":"keep"}`,
	)
	titles := loadSessionIndex(base)
	if len(titles) != 1 || titles["c"] != "keep" {
		t.Fatalf("titles = %v", titles)
	}
	if got := loadSessionIndex(filepath.Join(base, "missing")); len(got) != 0 {
		t.Fatalf("missing file → %v", got)
	}
}
