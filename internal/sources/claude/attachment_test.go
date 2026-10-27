package claude

import (
	"encoding/json"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestAttachmentText(t *testing.T) {
	cases := []struct{ typ, payload, want string }{
		{"hook_success", `{"hookName":"SessionStart:startup","stdout":"loaded\n","stderr":""}`, "SessionStart:startup: loaded\n"},
		{"hook_success", `{"hookName":"PreToolUse:Bash","stdout":"ok","stderr":"warn"}`, "PreToolUse:Bash: ok\n\nwarn"},
		{"hook_additional_context", `{"hookName":"h","content":["a","b"]}`, "a\nb"},
		{"queued_command", `{"prompt":"do it","origin":"human"}`, "do it"},
		{"task_status", `{"description":"Implement Task 1","status":"completed"}`, "Implement Task 1 — completed"},
		{"instructions", `{"files":[{"path":"/p/AGENTS.md"},{"path":"/p/CLAUDE.md"}]}`, "/p/AGENTS.md\n/p/CLAUDE.md"},
		{"file", `{"filename":"/p/memory/x.md","content":{}}`, "/p/memory/x.md"},
		{"invoked_skills", `{"skills":[{"name":"a"},{"name":"b"}]}`, "a, b"},
		{"skill_listing", `{"content":"- gloss: x"}`, "- gloss: x"},
		{"diagnostics", `{"files":[{"uri":"/a.go","diagnostics":[{"message":"bad import"}]}]}`, "1 files with diagnostics\n/a.go: bad import"},
		{"edited_text_file", `{"filename":"/p/a.go","snippet":""}`, "/p/a.go"},
		{"compact_file_reference", `{"filename":"/p/docs/x.md","displayPath":"docs/x.md"}`, "docs/x.md"},
		{"read_truncation_notice", `{"banner":"[Truncated]"}`, "[Truncated]"},
	}
	for _, c := range cases {
		if got := attachmentText(c.typ, json.RawMessage(c.payload), nil); got != c.want {
			t.Errorf("%s: got %q want %q", c.typ, got, c.want)
		}
	}
	if got := attachmentText("never_seen", json.RawMessage(`{"k":1}`), []string{"rendered one", "rendered two"}); got != "rendered one\nrendered two" {
		t.Errorf("fallback to rendered: %q", got)
	}
	if got := attachmentText("never_seen", json.RawMessage(`{"k":1}`), nil); got != "{\n  \"k\": 1\n}" {
		t.Errorf("fallback to raw payload: %q", got)
	}
}

func TestConvertAttachmentEntry(t *testing.T) {
	line := `{"type":"attachment","uuid":"at1","parentUuid":"p","timestamp":"2026-10-19T14:47:32Z","attachment":{"type":"hook_success","hookName":"PreToolUse:Bash","toolUseID":"toolu_1","stdout":"ok"},"renderedRole":"system"}`
	e := convertLine(t, line)
	if e.Role != thinkt.RoleSystem || e.Kind != thinkt.KindAttachment || e.Subtype != "hook_success" || e.Visibility != thinkt.VisibilitySecondary {
		t.Fatalf("attachment entry: %+v", e)
	}
	if e.Text != "PreToolUse:Bash: ok" || e.ParentToolUseID != "toolu_1" || e.Metadata["attachment_type"] != "hook_success" {
		t.Fatalf("attachment text/links: %+v", e)
	}
	line2 := `{"type":"attachment","uuid":"at2","attachment":{"type":"session_context","context":{}},"rendered":[{"content":"ctx"}],"renderedRole":"user"}`
	e2 := convertLine(t, line2)
	if e2.Role != thinkt.RoleUser || e2.Text != "ctx" || e2.Kind != thinkt.KindAttachment {
		t.Fatalf("rendered fallback: %+v", e2)
	}
}
