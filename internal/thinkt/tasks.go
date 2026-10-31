package thinkt

import (
	"slices"
	"time"
)

// TaskLink events.
const (
	TaskStarted  = "started"
	TaskUpdate   = "update"
	TaskFinished = "finished"
)

// TaskLink ties an entry to a background task. ToolUseID names the tool call
// behind the task, when the source records it.
type TaskLink struct {
	ID          string `json:"id"`
	Event       string `json:"event"`
	ToolUseID   string `json:"tool_use_id,omitempty"`
	Status      string `json:"status,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Description string `json:"description,omitempty"`
	OutputPath  string `json:"output_path,omitempty"`
}

// BackgroundTask is a task a session started, tied to the prompt and model
// turn that started it. A task whose start was not recorded has no
// StartEntryUUID.
type BackgroundTask struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind,omitempty"`
	Description    string    `json:"description,omitempty"`
	Status         string    `json:"status,omitempty"`
	OutputPath     string    `json:"output_path,omitempty"`
	ToolUseID      string    `json:"tool_use_id,omitempty"`
	StartEntryUUID string    `json:"start_entry_uuid,omitempty"`
	EndEntryUUID   string    `json:"end_entry_uuid,omitempty"`
	PromptID       string    `json:"prompt_id,omitempty"`
	TurnID         string    `json:"turn_id,omitempty"`
	StartedAt      time.Time `json:"started_at,omitzero"`
	EndedAt        time.Time `json:"ended_at,omitzero"`
}

// taskCallSite is the entry that made a tool call.
type taskCallSite struct {
	uuid, prompt, turn string
	at                 time.Time
}

// TaskTracker joins a session's task links into tasks. Feed it every entry in
// order. The zero value is ready to use.
type TaskTracker struct {
	calls  map[string]taskCallSite    // by tool use id
	tasks  map[string]*BackgroundTask // by task id
	byTool map[string]string          // tool use id → task id
	order  []string
}

// Entry feeds one entry to the tracker.
func (t *TaskTracker) Entry(e *Entry) {
	if t.calls == nil {
		t.calls, t.tasks, t.byTool = map[string]taskCallSite{}, map[string]*BackgroundTask{}, map[string]string{}
	}
	for _, b := range e.ContentBlocks {
		if b.Type == "tool_use" && b.ToolUseID != "" {
			t.calls[b.ToolUseID] = taskCallSite{e.UUID, e.PromptID, e.TurnID, e.Timestamp}
		}
	}
	if e.Task != nil {
		t.link(e, e.Task)
	}
}

func (t *TaskTracker) link(e *Entry, l *TaskLink) {
	id := l.ID
	if known, ok := t.byTool[l.ToolUseID]; ok && l.ToolUseID != "" {
		id = known
	}
	if id == "" {
		id = l.ToolUseID
	}
	if id == "" {
		return
	}
	task := t.tasks[id]
	if task == nil {
		task = &BackgroundTask{ID: id}
		t.tasks[id] = task
		t.order = append(t.order, id)
	}
	if l.ToolUseID != "" {
		t.byTool[l.ToolUseID] = id
		if task.ToolUseID == "" {
			task.ToolUseID = l.ToolUseID
		}
		if call, ok := t.calls[l.ToolUseID]; ok && task.StartEntryUUID == "" {
			task.StartEntryUUID, task.PromptID, task.TurnID, task.StartedAt = call.uuid, call.prompt, call.turn, call.at
		}
	}
	if l.Event == TaskStarted && task.StartEntryUUID == "" {
		task.StartEntryUUID, task.PromptID, task.TurnID, task.StartedAt = e.UUID, e.PromptID, e.TurnID, e.Timestamp
	}
	if l.Event == TaskFinished {
		task.EndEntryUUID, task.EndedAt = e.UUID, e.Timestamp
	}
	for dst, src := range map[*string]string{&task.Kind: l.Kind, &task.Description: l.Description, &task.Status: l.Status, &task.OutputPath: l.OutputPath} {
		if src != "" {
			*dst = src
		}
	}
}

// Tasks returns the session's tasks by start time, in first-seen order for
// ties and unrecorded starts, which sort last.
func (t *TaskTracker) Tasks() []BackgroundTask {
	out := make([]BackgroundTask, 0, len(t.order))
	for _, id := range t.order {
		out = append(out, *t.tasks[id])
	}
	slices.SortStableFunc(out, func(a, b BackgroundTask) int {
		switch {
		case a.StartedAt.IsZero() != b.StartedAt.IsZero():
			if a.StartedAt.IsZero() {
				return 1
			}
			return -1
		}
		return a.StartedAt.Compare(b.StartedAt)
	})
	return out
}
