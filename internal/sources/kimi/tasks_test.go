package kimi

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestTaskRecordsCarryLinks(t *testing.T) {
	s := convertLine(t, `{"type":"task.started","agentId":"main","time":1,"info":{"taskId":"b1","parentToolCallId":"t1","kind":"process","description":"build","status":"running"}}`)
	if len(s) != 1 || s[0].Task == nil {
		t.Fatalf("%+v", s)
	}
	l := s[0].Task
	if l.ID != "b1" || l.Event != thinkt.TaskStarted || l.ToolUseID != "t1" || l.Kind != "process" || l.Description != "build" {
		t.Fatalf("%+v", l)
	}
	e := convertLine(t, `{"type":"task.terminated","agentId":"main","time":2,"info":{"taskId":"b1","parentToolCallId":"t1","kind":"process","status":"completed"}}`)
	if e[0].Task == nil || e[0].Task.Event != thinkt.TaskFinished || e[0].Task.Status != "completed" {
		t.Fatalf("%+v", e)
	}
}
