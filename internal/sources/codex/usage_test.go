package codex

import (
	"io"
	"os"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// readFixture parses testdata/rollout-fixture.jsonl to EOF.
func readFixture(t *testing.T) []thinkt.Entry {
	t.Helper()
	f, err := os.Open("testdata/rollout-fixture.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p := NewParser(f, "thr-1")
	var out []thinkt.Entry
	for {
		e, err := p.NextEntry()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("NextEntry: %v", err)
		}
		out = append(out, *e)
	}
}

func TestDecodeUsage(t *testing.T) {
	u := decodeUsage([]byte(`{"turn_id":"t","usage":{"input_tokens":100,"cached_input_tokens":40,"cache_write_input_tokens":5,"output_tokens":20,"reasoning_output_tokens":8,"total_tokens":120}}`))
	if u == nil || u.InputTokens != 100 || u.CacheReadInputTokens != 40 || u.CacheCreationInputTokens != 5 || u.OutputTokens != 20 || u.ThinkingTokens != 8 {
		t.Fatalf("usage = %+v", u)
	}
	if decodeUsage([]byte(`{"turn_id":"t"}`)) != nil {
		t.Fatal("usage without a usage object must be nil")
	}
}

func TestParserAttachesUsageToHeldAssistantEntry(t *testing.T) {
	entries := readFixture(t)
	byUUID := map[string]thinkt.Entry{}
	for _, e := range entries {
		byUUID[e.UUID] = e
	}
	a1 := byUUID["msg-a1"]
	if a1.Usage == nil || a1.Usage.InputTokens != 100 || a1.Usage.OutputTokens != 20 || a1.Usage.CacheReadInputTokens != 40 || a1.Usage.CacheCreationInputTokens != 5 || a1.Usage.ThinkingTokens != 8 {
		t.Fatalf("msg-a1 usage = %+v", a1.Usage)
	}
	if w := byUUID["wire:call-exec-1"]; w.Usage == nil || w.Usage.InputTokens != 200 {
		t.Fatalf("wire exec usage = %+v", w.Usage)
	}
	if a2 := byUUID["msg-a2"]; a2.Usage == nil || a2.Usage.InputTokens != 300 {
		t.Fatalf("msg-a2 usage = %+v", a2.Usage)
	}
}

func TestParserUsageAfterUserEntryIsNotAttached(t *testing.T) {
	entries := parseAll(t,
		`{"timestamp":"2026-02-10T00:00:03Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"call_1","arguments":"{\"cmd\":\"pwd\"}"}}`,
		`{"timestamp":"2026-02-10T00:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_1","output":"ok"}}`,
		`{"timestamp":"2026-02-10T00:00:05Z","type":"token_usage_record","payload":{"turn_id":"t","usage":{"input_tokens":9,"output_tokens":1}}}`,
	)
	if len(entries) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
	for _, e := range entries {
		if e.Usage != nil {
			t.Fatalf("usage attached to %s: %+v", e.UUID, e.Usage)
		}
	}
}

func TestParserDoubleUsageLastWriteWins(t *testing.T) {
	entries := parseAll(t,
		fixAgentItemA2,
		`{"timestamp":"2026-10-01T10:00:14Z","type":"token_usage_record","payload":{"turn_id":"turn-1","usage":{"input_tokens":100,"output_tokens":1}}}`,
		`{"timestamp":"2026-10-01T10:00:14Z","type":"token_usage_record","payload":{"turn_id":"turn-1","usage":{"input_tokens":7,"output_tokens":2}}}`,
	)
	if len(entries) != 1 || entries[0].Usage == nil || entries[0].Usage.InputTokens != 7 {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParserFixtureEntryCounts(t *testing.T) {
	entries := readFixture(t)
	if len(entries) != 39 {
		t.Fatalf("entries = %d", len(entries))
	}
	secondary := 0
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsSecondary() {
			secondary++
		}
		if e.Text == "" && len(e.ContentBlocks) == 0 {
			t.Fatalf("empty entry %s", e.UUID)
		}
		if seen[e.UUID] {
			t.Fatalf("duplicate UUID %s", e.UUID)
		}
		seen[e.UUID] = true
	}
	if secondary != 18 {
		t.Fatalf("secondary = %d", secondary)
	}
}

func TestParserFixtureOrderAndKinds(t *testing.T) {
	type row struct {
		role     thinkt.Role
		kind     thinkt.EntryKind
		sec      bool
		uuid     string // "" = not asserted
		toolName string // "" = not asserted
		subtype  string // "" = not asserted
	}
	s := true
	want := []row{
		{thinkt.RoleSystem, thinkt.KindMeta, s, "", "", "developer"},
		{thinkt.RoleUser, thinkt.KindMeta, s, "", "", "environments.environment_context"},
		{thinkt.RoleUser, thinkt.KindMeta, s, "", "", "# AGENTS.md instructions"},
		{thinkt.RoleUser, thinkt.KindNone, false, "", "", ""},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "msg-a1", "", ""},
		{thinkt.RoleAssistant, thinkt.KindToolWire, s, "wire:call-exec-1", "exec", ""},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "exec-1", "unified_exec_startup", ""},
		{thinkt.RoleUser, thinkt.KindNone, false, "exec-1:result", "", ""},
		{thinkt.RoleUser, thinkt.KindToolWire, s, "wire:call-exec-1:result", "", ""},
		{thinkt.RoleAssistant, thinkt.KindToolWire, s, "wire:call-patch-1", "apply_patch", ""},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "call-patch-1", "apply_patch", ""},
		{thinkt.RoleUser, thinkt.KindNone, false, "call-patch-1:result", "", ""},
		{thinkt.RoleUser, thinkt.KindToolWire, s, "wire:call-patch-1:result", "", ""},
		{thinkt.RoleAssistant, thinkt.KindToolWire, s, "wire:call-mcp-1", "js", ""},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "call-mcp-1", "mcp:cua_repl/js", ""},
		{thinkt.RoleUser, thinkt.KindNone, false, "call-mcp-1:result", "", ""},
		{thinkt.RoleUser, thinkt.KindToolWire, s, "wire:call-mcp-1:result", "", ""},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "ws-1", "web_search", ""},
		{thinkt.RoleUser, thinkt.KindNone, false, "ws-1:result", "", ""},
		{thinkt.RoleAssistant, thinkt.KindToolWire, s, "wire:call-sleep-1", "sleep", ""},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "call-sleep-1", "sleep", ""},
		{thinkt.RoleUser, thinkt.KindNone, false, "call-sleep-1:result", "", ""},
		{thinkt.RoleUser, thinkt.KindToolWire, s, "wire:call-sleep-1:result", "", ""},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "img-1", "view_image", ""},
		{thinkt.RoleUser, thinkt.KindNone, false, "img-1:result", "", ""},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "turn-1-plan", "", "plan"},
		{thinkt.RoleAssistant, thinkt.KindToolWire, s, "wire:call-spawn-1", "spawn_agent", ""},
		{thinkt.RoleSystem, thinkt.KindAgentMessage, false, "call-spawn-1:started", "", "started"},
		{thinkt.RoleUser, thinkt.KindToolWire, s, "wire:call-spawn-1:result", "", ""},
		{thinkt.RoleSystem, thinkt.KindAgentMessage, s, "call-wait-1:collab", "", "wait"},
		{thinkt.RoleUser, thinkt.KindAgentMessage, false, "amsg-1", "", ""},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "msg-a2", "", ""},
		{thinkt.RoleSystem, thinkt.KindTurnDuration, s, "", "", ""},
		{thinkt.RoleSystem, thinkt.KindCompactBoundary, false, "", "", ""},
		{thinkt.RoleUser, thinkt.KindCommandOutput, s, "", "", "shell.user_command"},
		{thinkt.RoleAssistant, thinkt.KindNone, false, "exec-2", "user_shell", ""},
		{thinkt.RoleUser, thinkt.KindNone, false, "exec-2:result", "", ""},
		{thinkt.RoleSystem, thinkt.KindTurnDuration, s, "", "", ""},
		{thinkt.RoleSystem, thinkt.KindUnknown, s, "", "", ""},
	}
	entries := readFixture(t)
	if len(entries) != len(want) {
		t.Fatalf("entries = %d, want %d", len(entries), len(want))
	}
	for i, w := range want {
		e := entries[i]
		if e.Role != w.role || e.Kind != w.kind || e.IsSecondary() != w.sec {
			t.Errorf("row %d: role/kind/secondary = %s/%q/%v, want %s/%q/%v (uuid %s)", i, e.Role, e.Kind, e.IsSecondary(), w.role, w.kind, w.sec, e.UUID)
		}
		if w.uuid != "" && e.UUID != w.uuid {
			t.Errorf("row %d: uuid = %q, want %q", i, e.UUID, w.uuid)
		}
		if w.toolName != "" && (len(e.ContentBlocks) != 1 || e.ContentBlocks[0].ToolName != w.toolName) {
			t.Errorf("row %d: tool name = %+v, want %q", i, e.ContentBlocks, w.toolName)
		}
		if w.subtype != "" && e.Subtype != w.subtype {
			t.Errorf("row %d: subtype = %q, want %q", i, e.Subtype, w.subtype)
		}
	}
	a1 := entries[4]
	if a1.Text != "Checking the build." || a1.Model != "gpt-5.5" || a1.Effort != "medium" || a1.PromptID != "turn-1" {
		t.Errorf("msg-a1 = %+v", a1)
	}
	if tc := entries[32]; tc.Metadata["duration_ms"] != int64(14000) || tc.Metadata["time_to_first_token_ms"] != int64(900) {
		t.Errorf("task_complete = %+v", tc.Metadata)
	}
	if cp := entries[33]; cp.Compaction == nil || cp.Compaction.PreTokens != 660 || cp.Text != "Context compacted" {
		t.Errorf("compacted = %+v", cp)
	}
	if ab := entries[37]; ab.Error != "interrupted" {
		t.Errorf("turn_aborted = %+v", ab)
	}
	if m := entries[38]; m.Text == "" || m.Text[0] != '{' {
		t.Errorf("unknown = %+v", m)
	}
}
