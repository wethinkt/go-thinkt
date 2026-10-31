package thinkt

import (
	"testing"
	"time"
)

func taskCall(uuid, tool, prompt, turn string, at int64) *Entry {
	return &Entry{UUID: uuid, PromptID: prompt, TurnID: turn, Timestamp: time.Unix(at, 0),
		ContentBlocks: []ContentBlock{{Type: "tool_use", ToolUseID: tool, ToolName: "Bash"}}}
}

func taskNote(uuid string, at int64, link TaskLink) *Entry {
	return &Entry{UUID: uuid, Timestamp: time.Unix(at, 0), Task: &link}
}

func TestTaskTrackerTiesTaskToItsTurn(t *testing.T) {
	var tr TaskTracker
	tr.Entry(taskCall("a1", "t1", "p1", "turn1", 10))
	tr.Entry(taskNote("r1", 11, TaskLink{ID: "b1", Event: TaskStarted, ToolUseID: "t1", Kind: "process"}))
	tr.Entry(&Entry{UUID: "x", PromptID: "p2"})
	tr.Entry(taskNote("n1", 50, TaskLink{ID: "b1", Event: TaskFinished, Status: "completed", OutputPath: "/o.log", Description: "build"}))
	got := tr.Tasks()
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	task := got[0]
	if task.StartEntryUUID != "a1" || task.PromptID != "p1" || task.TurnID != "turn1" || task.EndEntryUUID != "n1" {
		t.Fatalf("%+v", task)
	}
	if task.Status != "completed" || task.OutputPath != "/o.log" || task.Description != "build" || task.Kind != "process" {
		t.Fatalf("%+v", task)
	}
	if task.StartedAt.Unix() != 10 || task.EndedAt.Unix() != 50 {
		t.Fatalf("%+v", task)
	}
}

func TestTaskTrackerJoinsByToolCall(t *testing.T) {
	var tr TaskTracker
	tr.Entry(taskCall("a1", "t1", "p1", "turn1", 10))
	tr.Entry(taskNote("r1", 11, TaskLink{ID: "b1", Event: TaskStarted, ToolUseID: "t1"}))
	tr.Entry(taskNote("n1", 50, TaskLink{ID: "other", Event: TaskFinished, ToolUseID: "t1", Status: "failed"}))
	got := tr.Tasks()
	if len(got) != 1 || got[0].ID != "b1" || got[0].Status != "failed" {
		t.Fatalf("%+v", got)
	}
}

func TestTaskTrackerNoteBeforeAnyStart(t *testing.T) {
	var tr TaskTracker
	tr.Entry(taskCall("a1", "t1", "p1", "turn1", 10))
	tr.Entry(taskNote("n1", 50, TaskLink{ID: "b1", Event: TaskFinished, ToolUseID: "t1", Status: "completed"}))
	tr.Entry(taskNote("n2", 60, TaskLink{ID: "orphan", Event: TaskFinished}))
	got := tr.Tasks()
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	if got[0].ID != "b1" || got[0].PromptID != "p1" {
		t.Fatalf("%+v", got[0])
	}
	if got[1].ID != "orphan" || got[1].StartEntryUUID != "" || got[1].EndEntryUUID != "n2" {
		t.Fatalf("%+v", got[1])
	}
}

func TestTaskTrackerUpdateDoesNotFinish(t *testing.T) {
	var tr TaskTracker
	tr.Entry(taskNote("r1", 11, TaskLink{ID: "b1", Event: TaskStarted}))
	tr.Entry(taskNote("u1", 20, TaskLink{ID: "b1", Event: TaskUpdate, Status: "running"}))
	got := tr.Tasks()
	if len(got) != 1 || got[0].EndEntryUUID != "" || got[0].Status != "running" || !got[0].EndedAt.IsZero() {
		t.Fatalf("%+v", got)
	}
}

func TestTaskTrackerOrdersByStart(t *testing.T) {
	var tr TaskTracker
	tr.Entry(taskNote("r1", 20, TaskLink{ID: "late", Event: TaskStarted}))
	tr.Entry(taskNote("r2", 10, TaskLink{ID: "early", Event: TaskStarted}))
	got := tr.Tasks()
	if got[0].ID != "early" {
		t.Fatalf("%+v", got)
	}
}
