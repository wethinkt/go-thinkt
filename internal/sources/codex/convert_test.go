package codex

import (
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// convertLine classifies and converts one rollout line with a fresh converter.
func convertLine(t *testing.T, line string) []thinkt.Entry {
	t.Helper()
	head, class := ClassifyLine([]byte(line))
	c := &converter{sessionID: "s", lineNo: 1, turn: &turnState{}, calls: map[string]string{}}
	return c.convert(head, class, []byte(line))
}

func TestConvertUserMessageWithDataImage(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"look"},{"type":"input_image","image_url":"data:image/png;base64,AAAA"},{"type":"input_image","image_url":"https://x/y.png"}],"internal_chat_message_metadata_passthrough":{"turn_id":"t1","content_item_kinds":["user.text","user.image"]}}}`
	entries := convertLine(t, line)
	if len(entries) != 1 || entries[0].Role != thinkt.RoleUser || entries[0].TurnID != "t1" {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Text != "look" || entries[0].Kind != thinkt.KindNone || entries[0].IsSecondary() {
		t.Fatalf("entry = %+v", entries[0])
	}
	blocks := entries[0].ContentBlocks
	if len(blocks) != 3 || blocks[0].Type != "text" || blocks[1].Type != "image" || blocks[1].MediaType != "image/png" || blocks[1].MediaData != "AAAA" || blocks[2].Type != "text" || blocks[2].Text != "https://x/y.png" {
		t.Fatalf("blocks = %+v", blocks)
	}
}

func TestConvertUserShellCommandIsSecondary(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:17Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"!git status"}],"internal_chat_message_metadata_passthrough":{"turn_id":"turn-2","content_item_kinds":["shell.user_command"]}}}`
	e := convertLine(t, line)[0]
	if e.Role != thinkt.RoleUser || e.Kind != thinkt.KindCommandOutput || !e.IsSecondary() || e.Subtype != "shell.user_command" || e.Text != "!git status" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertUserHarnessTagIsMeta(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions for /work/proj\nbe nice"}]}}`
	e := convertLine(t, line)[0]
	if e.Role != thinkt.RoleUser || e.Kind != thinkt.KindMeta || !e.IsSecondary() || e.Subtype != "# AGENTS.md instructions" {
		t.Fatalf("entry = %+v", e)
	}
	byKind := `{"timestamp":"2026-10-01T10:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n</environment_context>"}],"internal_chat_message_metadata_passthrough":{"turn_id":"turn-1","content_item_kinds":["environments.environment_context"]}}}`
	e = convertLine(t, byKind)[0]
	if e.Kind != thinkt.KindMeta || e.Subtype != "environments.environment_context" || e.TurnID != "turn-1" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertDeveloperMessageIsSecondaryMeta(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:00Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<permissions instructions>\nsandbox"}]}}`
	e := convertLine(t, line)[0]
	if e.Role != thinkt.RoleSystem || e.Kind != thinkt.KindMeta || !e.IsSecondary() || e.Subtype != "developer" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertAgentMessageItem(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:03Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"AgentMessage","id":"msg-a1","content":[{"type":"Text","text":"Checking the build."}],"phase":"commentary","questions":[{"id":"q1"}]}}}`
	e := convertLine(t, line)[0]
	if e.Role != thinkt.RoleAssistant || e.Text != "Checking the build." || e.UUID != "msg-a1" || e.TurnID != "turn-1" || e.Metadata["phase"] != "commentary" || e.Metadata["questions"] == nil {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertReasoningItem(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:02Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"Reasoning","id":"rs-2","summary_text":[{"type":"summary_text","text":"**Why**"},{"type":"summary_text","text":"because"}],"raw_content":[]}}}`
	e := convertLine(t, line)[0]
	if e.Role != thinkt.RoleAssistant || e.UUID != "rs-2" || len(e.ContentBlocks) != 1 || e.ContentBlocks[0].Type != "thinking" || e.ContentBlocks[0].Thinking != "**Why**\nbecause" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertPlanItem(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:11Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"Plan","id":"turn-1-plan","text":"# Plan\n1. fix"}}}`
	e := convertLine(t, line)[0]
	if e.Role != thinkt.RoleAssistant || e.Subtype != "plan" || e.Text != "# Plan\n1. fix" || e.UUID != "turn-1-plan" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertTaskCompleteWithError(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:00Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"t1","error":{"message":"flagged"},"duration_ms":10,"time_to_first_token_ms":3}}`
	e := convertLine(t, line)[0]
	if e.Kind != thinkt.KindTurnDuration || !e.IsSecondary() || e.Error != "flagged" || e.Metadata["duration_ms"] != int64(10) {
		t.Fatalf("entry = %+v", e)
	}
	if e.Role != thinkt.RoleSystem || e.Text != "turn complete" || e.Metadata["time_to_first_token_ms"] != int64(3) || e.TurnID != "t1" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertTurnAborted(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:18Z","type":"event_msg","payload":{"type":"turn_aborted","turn_id":"turn-2","reason":"interrupted","duration_ms":500}}`
	e := convertLine(t, line)[0]
	if e.Kind != thinkt.KindTurnDuration || !e.IsSecondary() || e.Error != "interrupted" || e.Text != "turn aborted" || e.Metadata["duration_ms"] != int64(500) {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertCompactedPreTokens(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:16Z","type":"compacted","payload":{"message":"","replacement_history":[],"latest_token_usage_record":{"thread_token_usage":{"total_tokens":660}}}}`
	e := convertLine(t, line)[0]
	if e.Role != thinkt.RoleSystem || e.Kind != thinkt.KindCompactBoundary || e.Text != "Context compacted" || e.Compaction == nil || e.Compaction.PreTokens != 660 || e.Compaction.Trigger != "auto" {
		t.Fatalf("entry = %+v", e)
	}
	nullUsage := `{"timestamp":"2026-10-01T10:00:16Z","type":"compacted","payload":{"message":"squashed","latest_token_usage_record":null}}`
	e = convertLine(t, nullUsage)[0]
	if e.Text != "squashed" || e.Compaction == nil || e.Compaction.PreTokens != 0 {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertSubAgentActivity(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:12Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"SubAgentActivity","id":"call-spawn-1","kind":"started","agent_thread_id":"thr-child","agent_path":"/root/helper"}}}`
	e := convertLine(t, line)[0]
	if e.Role != thinkt.RoleSystem || e.Kind != thinkt.KindAgentMessage || e.ChildSessionID != "thr-child" || e.Subtype != "started" || e.UUID != "call-spawn-1:started" || e.Text != "started /root/helper" || e.IsSecondary() {
		t.Fatalf("entry = %+v", e)
	}
	interacted := `{"timestamp":"2026-10-01T10:00:12Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"SubAgentActivity","id":"call-spawn-1","kind":"interacted","agent_thread_id":"thr-child","agent_path":"/root/helper"}}}`
	e = convertLine(t, interacted)[0]
	if !e.IsSecondary() || e.UUID != "call-spawn-1:interacted" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertCollabAgentToolCall(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:13Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"CollabAgentToolCall","id":"call-wait-1","tool":"wait","status":"completed","sender_thread_id":"thr-1","receiver_thread_ids":["thr-child"]}}}`
	e := convertLine(t, line)[0]
	if e.Role != thinkt.RoleSystem || e.Kind != thinkt.KindAgentMessage || !e.IsSecondary() || e.UUID != "call-wait-1:collab" || e.Subtype != "wait" || e.Text != "wait thr-child" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertWireAgentMessage(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:13Z","type":"response_item","payload":{"type":"agent_message","id":"amsg-1","author":"/root/helper","recipient":"/root","content":[{"type":"input_text","text":"done"}],"internal_chat_message_metadata_passthrough":{"turn_id":"turn-1"}}}`
	e := convertLine(t, line)[0]
	if e.Role != thinkt.RoleUser || e.Kind != thinkt.KindAgentMessage || e.Text != "done" || e.UUID != "amsg-1" || e.Metadata["author"] != "/root/helper" || e.Metadata["recipient"] != "/root" || e.TurnID != "turn-1" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertUnknownKeepsRawLine(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:19Z","type":"mystery_record","payload":{"x":1}}`
	e := convertLine(t, line)[0]
	if e.Role != thinkt.RoleSystem || e.Kind != thinkt.KindUnknown || !e.IsSecondary() || e.Text != line {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertBookkeepingProducesNothing(t *testing.T) {
	for _, line := range []string{
		`{"timestamp":"2026-10-01T10:00:01Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"UserMessage","id":"item-1","content":[{"type":"text","text":"fix the build"}]}}}`,
		`{"timestamp":"2026-10-01T10:00:02Z","type":"response_item","payload":{"type":"reasoning","id":"rs-1","summary":[],"content":null,"encrypted_content":"xxx"}}`,
		`{"timestamp":"2026-10-01T10:00:03Z","type":"token_usage_record","payload":{"turn_id":"turn-1","usage":{"input_tokens":100}}}`,
		`{"timestamp":"2026-10-01T10:00:01Z","type":"turn_context","payload":{"turn_id":"turn-1","model":"gpt-5.5"}}`,
	} {
		if got := convertLine(t, line); len(got) != 0 {
			t.Fatalf("%s → %+v", line, got)
		}
	}
}

func TestConvertEmptyUserMessageProducesNothing(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"   "}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["user.text"]}}}`
	if got := convertLine(t, line); len(got) != 0 {
		t.Fatalf("entries = %+v", got)
	}
}

func TestTurnStateApply(t *testing.T) {
	var ts turnState
	h, _ := ClassifyLine([]byte(`{"type":"turn_context","payload":{"turn_id":"t1","model":"gpt-5.5","effort":"medium"}}`))
	ts.apply(h)
	if ts.TurnID != "t1" || ts.Model != "gpt-5.5" || ts.Effort != "medium" {
		t.Fatalf("state = %+v", ts)
	}
	h, _ = ClassifyLine([]byte(`{"type":"event_msg","payload":{"type":"task_started","turn_id":"t2"}}`))
	ts.apply(h)
	if ts.TurnID != "t2" || ts.Model != "gpt-5.5" {
		t.Fatalf("state = %+v", ts)
	}
}

func TestNewEntryDefaultsUUIDFromLine(t *testing.T) {
	c := &converter{sessionID: "sess", lineNo: 7, turn: &turnState{TurnID: "t9", Model: "m", Effort: "high"}, calls: map[string]string{}}
	h, _ := ClassifyLine([]byte(`{"timestamp":"2026-10-01T10:00:19Z","type":"compacted","payload":{}}`))
	e := c.newEntry(thinkt.RoleSystem, h, "")
	if e.UUID != "sess:000007:compacted" || e.Source != thinkt.SourceCodex || e.TurnID != "t9" || e.Model != "m" || e.Effort != "high" || e.Timestamp.IsZero() {
		t.Fatalf("entry = %+v", e)
	}
	if !strings.HasPrefix(e.UUID, "sess:") {
		t.Fatalf("uuid = %q", e.UUID)
	}
}
