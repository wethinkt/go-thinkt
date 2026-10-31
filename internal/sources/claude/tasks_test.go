package claude

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestParseTaskNotification(t *testing.T) {
	l := parseTaskNotification("<task-notification>\n<task-id>b1</task-id>\n<tool-use-id>t1</tool-use-id>\n<output-file>/o.log</output-file>\n<status>completed</status>\n<summary>Built &amp; done</summary>\n</task-notification>")
	if l == nil || l.ID != "b1" || l.ToolUseID != "t1" || l.OutputPath != "/o.log" || l.Status != "completed" || l.Event != thinkt.TaskFinished || l.Description != "Built & done" {
		t.Fatalf("%+v", l)
	}
	u := parseTaskNotification("<task-notification><task-id>b1</task-id><event>progress</event></task-notification>")
	if u == nil || u.Event != thinkt.TaskUpdate {
		t.Fatalf("%+v", u)
	}
	if parseTaskNotification("<task-notification>no id</task-notification>") != nil {
		t.Fatal("no id")
	}
}

func TestSessionLinksBackgroundBash(t *testing.T) {
	t.Setenv("THINKT_HOME", t.TempDir())
	home := filepath.Join(t.TempDir(), "claude")
	id := "11111111-2222-4333-8444-555555555555"
	dir := filepath.Join(home, "projects", "-p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"type":"user","uuid":"u1","promptId":"prompt-1","timestamp":"2026-10-01T00:00:00Z","cwd":"/p","message":{"role":"user","content":"build it"}}`,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-10-01T00:00:01Z","cwd":"/p","message":{"id":"msg1","role":"assistant","model":"claude-opus-4","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"make","run_in_background":true}}]}}`,
		`{"type":"user","uuid":"r1","timestamp":"2026-10-01T00:00:02Z","cwd":"/p","toolUseResult":{"stdout":"","backgroundTaskId":"b1"},"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"started"}]}}`,
		`{"type":"user","uuid":"n1","timestamp":"2026-10-01T00:05:00Z","cwd":"/p","message":{"role":"user","content":"<task-notification><task-id>b1</task-id><tool-use-id>t1</tool-use-id><status>failed</status><summary>make failed</summary></task-notification>"}}`,
	}
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	writeTempSession(t, dir, id+".jsonl", body)
	var tr thinkt.TaskTracker
	if err := thinkt.VisitEntries(context.Background(), NewStore(home), id, tr.Entry); err != nil {
		t.Fatal(err)
	}
	got := tr.Tasks()
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	task := got[0]
	if task.ID != "b1" || task.StartEntryUUID != "a1" || task.EndEntryUUID != "n1" || task.Status != "failed" || task.Kind != "process" {
		t.Fatal(fmt.Sprintf("%+v", task))
	}
}

// A notification's free-text result can quote anything.
func TestParseTaskNotificationIgnoresTheResult(t *testing.T) {
	l := parseTaskNotification("<task-notification><task-id>b1</task-id><summary>Fix <b>x</b> and more</summary><result>saw <task-id>zzz</task-id><status>failed</status></result></task-notification>")
	if l == nil || l.ID != "b1" || l.Event != thinkt.TaskUpdate || l.Status != "" || l.Description != "Fix <b>x</b> and more" {
		t.Fatalf("%+v", l)
	}
	s := parseTaskNotification("<task-notification><task-id>b2</task-id><status>completed</status><status>failed</status></task-notification>")
	if s == nil || s.Status != "completed" {
		t.Fatalf("%+v", s)
	}
}

func TestQueuedNotificationCarriesLink(t *testing.T) {
	e := convertLine(t, `{"type":"attachment","uuid":"at1","timestamp":"2026-10-19T14:47:32Z","attachment":{"type":"queued_command","prompt":"<task-notification><task-id>b1</task-id><tool-use-id>t1</tool-use-id><status>completed</status></task-notification>"}}`)
	if e.Task == nil || e.Task.ID != "b1" || e.Task.Event != thinkt.TaskFinished {
		t.Fatalf("%+v", e)
	}
}
