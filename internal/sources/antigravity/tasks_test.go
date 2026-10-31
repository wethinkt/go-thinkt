package antigravity

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestTaskNotificationCarriesLink(t *testing.T) {
	var got *thinkt.TaskLink
	for _, e := range parseFixture(t, "main.jsonl", "11111111-aaaa", "") {
		if e.Kind == thinkt.KindTaskNotification {
			got = e.Task
		}
	}
	if got == nil || got.ID != "task-4" || got.Event != thinkt.TaskFinished {
		t.Fatalf("%+v", got)
	}
}

func TestRunCommandResultStartsTask(t *testing.T) {
	c := converter{conversationID: "11111111-aaaa"}
	h := stepHead{Type: "GENERIC", Content: "Command is running in the background as 11111111-aaaa/task-4.\nLog: /b/.system_generated/tasks/task-4.log"}
	e := c.resultEntry(h, "t1")
	if e.Task == nil || e.Task.ID != "task-4" || e.Task.Event != thinkt.TaskStarted || e.Task.ToolUseID != "t1" || e.Task.Kind != "process" {
		t.Fatalf("%+v", e.Task)
	}
	h.Content = "ok, finished"
	if e := c.resultEntry(h, "t2"); e.Task != nil {
		t.Fatalf("%+v", e.Task)
	}
}

func TestOtherPathsDoNotStartTasks(t *testing.T) {
	c := converter{conversationID: "11111111-aaaa"}
	for _, body := range []string{"* feature/task-123", "/repo/task-4/main.go", "see 22222222-bbbb/task-9"} {
		if e := c.resultEntry(stepHead{Type: "GENERIC", Content: body}, "t1"); e.Task != nil {
			t.Errorf("%q: %+v", body, e.Task)
		}
	}
	mixed := c.resultEntry(stepHead{Type: "GENERIC", Content: "/repo/task-1\n11111111-aaaa/task-4 running"}, "t1")
	if mixed.Task == nil || mixed.Task.ID != "task-4" {
		t.Fatalf("%+v", mixed.Task)
	}
}
