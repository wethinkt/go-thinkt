# Background-task links

**Why.** An agent starts a build in the background and carries on; minutes
later a notification arrives in an unrelated turn. A reader sees the two as
separate events, and cannot tell which turn started what, or how it ended. The
user skipped the design questions, so the calls below are mine.

## Goals

- `Entry.Task` (`thinkt.TaskLink`) marks entries that start, update or finish a
  background task, with the task id and the tool call behind it.
- `thinkt.TaskTracker` joins them into `BackgroundTask`s, each tied to the
  prompt and model turn that started it.
- `thinkt sessions tasks <session>` and `GET /api/v1/sessions/<path>/tasks`,
  built from entries alone, so they work for any source that sets links.

## Non-goals

- Codex: long `exec` processes are polled in the same turn, and subagents
  already link through `ChildSessionID`.
- Gemini, Copilot, Qwen: no background records.
- Reading task log files.
- Viewers: this pass builds the data.

## What each source has

Tallied from local sessions.

| Source | Started | Finished |
|---|---|---|
| Claude | `toolUseResult.backgroundTaskId` (257), or `isAsync` with `agentId` (357) on the call's result | `<task-notification>` with `<task-id>`, `<status>`, `<output-file>`, `<tool-use-id>` (163 of 334); without a status it is an update |
| Kimi | `task.started` (34) `info.taskId`, `parentToolCallId`, `kind`, `description` | `task.terminated` `info.status` |
| Antigravity | the first tool result naming `<conversation>/task-N` (`RUN_COMMAND` or `GENERIC`) | a system message from sender `<conversation>/task-N` |

## Shape

```go
type TaskLink struct {
	ID, Event, ToolUseID, Status, Kind, Description, OutputPath string
}
type BackgroundTask struct {
	ID, Kind, Description, Status, OutputPath, ToolUseID string
	StartEntryUUID, EndEntryUUID, PromptID, TurnID       string
	StartedAt, EndedAt                                   time.Time
}
```

Events: `started`, `update`, `finished`.

The tracker keys tasks by id, and maps a tool call to its id so a note with a
different id but the same call joins the task. A task's start is the entry of
the tool call when one is known, because that is the turn that started it,
else the entry of the first link. A task with no start record still lists.

`thinkt.ParserVersion` goes to 10.

Known limits, from local data: Claude's `<event>` notes are `Monitor` stream
updates, so they never end a task; and most Claude agents have no closing
record, because their result reaches the parent another way, so they list as
started with no end. Nothing is invented for them.

## Tasks

1. `TaskLink`, `TaskTracker`.
2. Claude. 3. Kimi. 4. Antigravity.
5. CLI and REST, real-data check, docs.

Each task: failing test first. A fresh reviewer reads the branch at the end,
with one fix pass.
