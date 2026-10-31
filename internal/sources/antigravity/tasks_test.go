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
	c := converter{}
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
