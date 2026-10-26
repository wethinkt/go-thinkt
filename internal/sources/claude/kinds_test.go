package claude

import (
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func convertLine(t *testing.T, line string) *thinkt.Entry {
	t.Helper()
	return convertEntry(parseFixtureLine(t, line), thinkt.SourceClaude, "")
}

func TestConvertSystemEvents(t *testing.T) {
	cases := []struct {
		line, text, subtype string
		kind                thinkt.EntryKind
		vis                 thinkt.Visibility
	}{
		{`{"type":"system","subtype":"turn_duration","uuid":"s1","durationMs":46913,"messageCount":60}`, "turn_duration", "turn_duration", thinkt.KindTurnDuration, thinkt.VisibilitySecondary},
		{`{"type":"system","subtype":"away_summary","uuid":"s2","content":"Done. Next: merge."}`, "Done. Next: merge.", "away_summary", thinkt.KindAwaySummary, thinkt.VisibilityNormal},
		{`{"type":"system","subtype":"informational","uuid":"s4","content":"Compacted while idle","level":"notice"}`, "Compacted while idle", "informational", thinkt.KindInformational, thinkt.VisibilitySecondary},
		{`{"type":"system","subtype":"local_command","uuid":"s5","content":"<command-name>/model</command-name>"}`, "<command-name>/model</command-name>", "local_command", thinkt.KindLocalCommand, thinkt.VisibilitySecondary},
		{`{"type":"system","subtype":"something_new","uuid":"s3"}`, "something_new", "something_new", thinkt.KindUnknown, thinkt.VisibilitySecondary},
		{`{"type":"queue-operation","operation":"enqueue","content":"<cross-session-message>hi</cross-session-message>"}`, "<cross-session-message>hi</cross-session-message>", "enqueue", thinkt.KindQueueOperation, thinkt.VisibilitySecondary},
		{`{"type":"queue-operation","operation":"dequeue","timestamp":"2026-10-26T13:35:08.790Z"}`, "dequeue", "dequeue", thinkt.KindQueueOperation, thinkt.VisibilitySecondary},
	}
	for _, c := range cases {
		e := convertLine(t, c.line)
		if e.Role != thinkt.RoleSystem || e.Text != c.text || e.Kind != c.kind || e.Visibility != c.vis || e.Subtype != c.subtype {
			t.Errorf("%s: got role=%s text=%q kind=%s vis=%q subtype=%q", c.subtype, e.Role, e.Text, e.Kind, e.Visibility, e.Subtype)
		}
	}
	td := convertLine(t, cases[0].line)
	if td.Metadata["duration_ms"] != 46913 || td.Metadata["message_count"] != 60 {
		t.Fatalf("turn_duration metadata: %v", td.Metadata)
	}
}

func TestConvertCompactBoundary(t *testing.T) {
	line := `{"type":"system","subtype":"compact_boundary","uuid":"c1","parentUuid":null,"logicalParentUuid":"p9","content":"Conversation compacted","compactMetadata":{"trigger":"auto","preTokens":230993,"durationMs":90130,"preservedSegment":{"headUuid":"h","anchorUuid":"a","tailUuid":"t"}}}`
	e := convertLine(t, line)
	if e.Kind != thinkt.KindCompactBoundary || e.Visibility != thinkt.VisibilityNormal || e.Text != "Conversation compacted" {
		t.Fatalf("boundary: %+v", e)
	}
	want := thinkt.CompactionInfo{Trigger: "auto", PreTokens: 230993, DurationMs: 90130, HeadUUID: "h", AnchorUUID: "a", TailUUID: "t", LogicalParentUUID: "p9"}
	if e.Compaction == nil || *e.Compaction != want {
		t.Fatalf("compaction: got %+v want %+v", e.Compaction, want)
	}
}

func TestConvertUserKinds(t *testing.T) {
	cases := []struct {
		content       string
		meta, compact bool
		kind          thinkt.EntryKind
		vis           thinkt.Visibility
	}{
		{`"plain prompt"`, false, false, thinkt.KindNone, thinkt.VisibilityNormal},
		{`"<system-reminder>\nx\n</system-reminder>"`, false, false, thinkt.KindMeta, thinkt.VisibilitySecondary},
		{`[{"type":"text","text":"<task-notification>\n<task-id>abc</task-id>\n</task-notification>"}]`, false, false, thinkt.KindTaskNotification, thinkt.VisibilityNormal},
		{`"<agent-message from=\"x\">report</agent-message>"`, true, false, thinkt.KindAgentMessage, thinkt.VisibilityNormal},
		{`"<local-command-stdout>ok</local-command-stdout>"`, false, false, thinkt.KindCommandOutput, thinkt.VisibilitySecondary},
		{`"<local-command-caveat>c</local-command-caveat>\n<command-name>/x</command-name>"`, false, false, thinkt.KindCommandOutput, thinkt.VisibilitySecondary},
		{`"This session is being continued..."`, false, true, thinkt.KindCompactSummary, thinkt.VisibilityNormal},
		{`"anything"`, true, false, thinkt.KindMeta, thinkt.VisibilitySecondary},
		{`[{"type":"tool_result","tool_use_id":"t","content":"out"}]`, false, false, thinkt.KindNone, thinkt.VisibilityNormal},
	}
	for i, c := range cases {
		line := `{"type":"user","uuid":"u","isMeta":` + boolJSON(c.meta) + `,"isCompactSummary":` + boolJSON(c.compact) + `,"message":{"role":"user","content":` + c.content + `}}`
		e := convertLine(t, line)
		if e.Kind != c.kind || e.Visibility != c.vis {
			t.Errorf("case %d (%s): got kind=%q vis=%q want kind=%q vis=%q", i, c.content, e.Kind, e.Visibility, c.kind, c.vis)
		}
		if e.Role != thinkt.RoleUser {
			t.Errorf("case %d: role %s", i, e.Role)
		}
	}
}

func boolJSON(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestConvertUserPermissionAndPrompt(t *testing.T) {
	line := `{"type":"user","uuid":"u","promptId":"p1","permissionDecision":{"decision":"reject","source":"config","reasonType":"safetyCheck","reasonCode":"safetyCheck"},"toolDenialKind":"permission-rule","turnOrigin":"human","promptSource":"typed","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":"denied","is_error":true}]}}`
	e := convertLine(t, line)
	if e.PromptID != "p1" {
		t.Fatalf("prompt id: %q", e.PromptID)
	}
	want := thinkt.PermissionDecision{Decision: "reject", Source: "config", ReasonType: "safetyCheck", ReasonCode: "safetyCheck", DenialKind: "permission-rule"}
	if e.Permission == nil || *e.Permission != want {
		t.Fatalf("permission: got %+v want %+v", e.Permission, want)
	}
	if e.Metadata["turn_origin"] != "human" || e.Metadata["prompt_source"] != "typed" {
		t.Fatalf("metadata: %v", e.Metadata)
	}
	plain := convertLine(t, `{"type":"user","uuid":"u2","message":{"role":"user","content":"hi"}}`)
	if plain.Permission != nil || plain.PromptID != "" {
		t.Fatalf("plain prompt must not invent permission: %+v", plain)
	}
}

func TestConvertAssistantMetadata(t *testing.T) {
	line := `{"type":"assistant","uuid":"a","effort":"high","requestedModel":"claude-fable-5-1","thinkingDurationMs":1200,"attributionAgent":"Explore","attributionSkill":"ntrecord","message":{"id":"msg_9","role":"assistant","model":"claude-fable-5-1","stop_reason":"refusal","content":[{"type":"text","text":"no"}],"usage":{"input_tokens":1,"output_tokens":2,"service_tier":"standard","speed":"standard","output_tokens_details":{"thinking_tokens":7}}}}`
	e := convertLine(t, line)
	if e.TurnID != "msg_9" || e.Effort != "high" || e.RequestedModel != "claude-fable-5-1" || e.StopReason != "refusal" {
		t.Fatalf("turn fields: %+v", e)
	}
	if e.Attribution == nil || *e.Attribution != (thinkt.Attribution{Agent: "Explore", Skill: "ntrecord"}) {
		t.Fatalf("attribution: %+v", e.Attribution)
	}
	if e.Usage == nil || e.Usage.ThinkingTokens != 7 || e.Usage.ServiceTier != "standard" || e.Usage.Speed != "standard" {
		t.Fatalf("usage: %+v", e.Usage)
	}
	if e.Metadata["thinking_duration_ms"] != 1200 {
		t.Fatalf("thinking duration: %v", e.Metadata)
	}
	if e.Kind != thinkt.KindNone || e.Visibility != thinkt.VisibilityNormal || e.Error != "" {
		t.Fatalf("normal assistant must stay normal: %+v", e)
	}
	bare := convertLine(t, `{"type":"assistant","uuid":"b","message":{"id":"m","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"x"}]}}`)
	if bare.Attribution != nil {
		t.Fatal("no attribution fields must leave Attribution nil")
	}
}

func TestConvertAssistantAPIError(t *testing.T) {
	line := `{"type":"assistant","uuid":"e","isApiErrorMessage":true,"error":"server_error","message":{"id":"x","role":"assistant","model":"<synthetic>","content":[{"type":"text","text":"API Error: Can't reach the API server"}]}}`
	e := convertLine(t, line)
	if e.Kind != thinkt.KindAPIError || e.Visibility != thinkt.VisibilitySecondary || e.Error != "server_error" {
		t.Fatalf("api error: %+v", e)
	}
	if !strings.HasPrefix(e.Text, "API Error:") {
		t.Fatalf("text lost: %q", e.Text)
	}
}

func TestConvertUnknownType(t *testing.T) {
	e := parseFixtureLine(t, `{"type":"brand-new","uuid":"n","payload":1}`)
	e.Raw = []byte(`{"type":"brand-new","uuid":"n","payload":1}`)
	out := convertEntry(e, thinkt.SourceClaude, "")
	if out.Role != thinkt.RoleSystem || out.Kind != thinkt.KindUnknown || out.Subtype != "brand-new" || out.Visibility != thinkt.VisibilitySecondary {
		t.Fatalf("unknown: %+v", out)
	}
	if !strings.Contains(out.Text, `"payload": 1`) {
		t.Fatalf("raw text: %q", out.Text)
	}
}

func TestConvertFileHistoryDelta(t *testing.T) {
	e := convertLine(t, `{"type":"file-history-delta","messageId":"m","trackingPath":"/p/a.md","backup":{"version":1}}`)
	if e.Role != thinkt.RoleCheckpoint || e.Visibility != thinkt.VisibilitySecondary || e.Text != "File History Delta: /p/a.md" {
		t.Fatalf("delta: %+v", e)
	}
}
