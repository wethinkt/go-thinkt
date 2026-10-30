# kimi

Package `kimi` implements the session source for [Kimi Code](https://kimi.ai),
reading the current CLI's tree at `~/.kimi-code` (override with
`THINKT_KIMI_HOME`).

## Directory Structure

```
kimi/
├── discovery.go          # StoreFactory, IsSessionPath, baseDirForPath, sessionIDForPath
├── classify.go           # recordHead, ClassifyLine (thinkt.RecordClass), userClass, IsSearchable
├── convert.go            # user lines, events, protocol 1.0 messages, tool result blocks, usage
├── parser.go             # streaming Parser: loop-step hold, approvals, spawns, twin dedupe
├── sessionstate.go       # state.json, session_index.jsonl, workspaces.json, scanSessionFile
├── store.go              # thinkt.Store implementation
├── resume.go             # `kimi --session <id>`
├── unknown.go            # once-per-key debug log of unknown record types
└── testdata/wire-1.{0,4,5}.jsonl  # fixtures for the three wire protocols
```

## Layout on disk

```
~/.kimi-code/
├── sessions/wd_<name>_<hash>/<session>/      # <session> is session_<uuid> or ses_<uuid> (migrated)
│   ├── state.json                            # title, lastPrompt, cwd/workDir, createdAt/updatedAt, agents{}
│   ├── agents/main/wire.jsonl                # the session
│   ├── agents/agent-N/wire.jsonl             # one subagent each
│   └── agents/main/{plans,tasks,file-history}/
├── session_index.jsonl                       # {sessionId, sessionDir, workDir}
├── workspaces.json                           # wd_<name>_<hash> → {root, name}
└── device_id
```

Session ids: the session directory name for the main wire, and
`<session>/<agent>` for a subagent wire (for example
`session_eb0b…/agent-0`). Project ids are the work-dir path. Subagent
sessions carry `ParentSessionID`, are hidden from `ListSessions` unless
`thinkt.WithSubagents()` is passed, and count in the parent's and the
project's `SubagentCount`. Every `agents/<id>/wire.jsonl` is listed, even
one `state.json` does not mention.

## Wire protocols

The `metadata` record names the protocol:

- **1.0** — sessions migrated from the Python Kimi CLI: only
  `context.append_message` records (user, assistant with `toolCalls`, tool),
  no times, no usage.
- **1.4** — loop events plus context user messages.
- **1.5** — loop events, context user messages, and batched
  `agent.message.appended` records written at turn end that repeat the
  turn's messages.

## Classification

`ClassifyLine` reads the record key: `"<type>"`, `loop/<event.type>` for
`context.append_loop_event`, `ctx/<role>` for `context.append_message`,
`appended/<role>` for `agent.message.appended`.

| key / condition | class |
|---|---|
| `metadata` | session state |
| `loop/step.begin`, `loop/step.end`, `turn.step.interrupted` | event (step markers, never entries on their own) |
| `loop/content.part`, `loop/tool.call`, `loop/tool.result` | turn |
| `appended/user` with `meta.source: input` | the user entry (the record arrives before its context twin) |
| `ctx/user` | the user entry, unless its `id` was already emitted |
| user origin `injection`, or a `<system-reminder>` / `<system>` lead | context, `KindMeta`, secondary |
| user origin `system_trigger` with a `<git-context`, `Working directory:`, or `continue` lead | context, `KindMeta`, secondary |
| other `system_trigger` (a subagent's task prompt) | turn, visible |
| 1.0 `<system>Previous context has been compacted` | context, `KindCompactSummary`, secondary |
| user origin `task`, `background_task`; `turn.steer`; appended `notify` | context, `KindTaskNotification`, secondary |
| user origin `skill_activation` | context, `KindMeta`, secondary |
| user origin `shell_command` | turn, `KindCommandOutput`, secondary |
| `ctx/assistant`, `ctx/tool` | turn (protocol 1.0) |
| `appended/assistant`, `appended/tool` | turn, dropped when the loop already emitted the message or call |
| `turn.ended` | event, `KindTurnDuration`, secondary |
| `subagent.spawned` | event, `KindAgentMessage`, visible, `ChildSessionID` |
| `subagent.completed/failed/cancelled` | event, `KindAgentMessage`, secondary |
| `task.started`, `task.terminated` | event, `KindCommandOutput`, secondary |
| `plan.revision` | event, `KindAttachment`, secondary |
| `permission.record_approval_result` | merged into the tool result |
| `turn.prompt`, `prompt.*`, `agent.turn.*`, `interaction.*`, `usage.*`, `token_counting.*`, `llm.*`, `tools.*`, `file_history.*`, configuration records | bookkeeping |
| anything else | unknown: a secondary system entry with the raw line, logged once per key |

## Parser

`Parser` assembles each loop step into one assistant entry: thinking and
text blocks (consecutive parts merged), a `tool_use` block per call, usage
and stop reason from `step.end`, model and effort from the step's
`llm.request`. Tool results become their own user-role entries
(`<toolCallId>:result`) carrying the approval recorded for the call and,
for an `Agent` call, the spawned child's `ChildSessionID` (also set on the
`tool_use` and `tool_result` blocks). Everything that arrives while a step
is open is queued after its assistant entry. A step that ends with
`error` or `interrupted` waits one line for `turn.step.interrupted` and
takes its message as `Error`; an empty such step becomes a secondary
`KindAPIError` entry. Batched `appended` records are dropped when the loop
already emitted their message id or tool call id, and a context user
message is dropped when its id was already emitted.

## Session state

| `SessionMeta` field | source |
|---|---|
| `ID`, `ParentSessionID` | directory names |
| `ProjectPath` | `state.cwd`, else `state.workDir`, else `session_index.jsonl`, else `workspaces.json` |
| `Title`, `LastPrompt` | `state.json` (subagents: profile name and agent id) |
| `CreatedAt`, `ModifiedAt` | `state.createdAt`, `state.updatedAt` (epoch ms or RFC3339) |
| `EntryCount`, `FirstPrompt`, `Model`, `Cost` | one parser pass (`scanSessionFile`): token totals from step usage, duration from `turn.ended` |
| `SubagentCount` | subagent wires under the session |

`IsSearchable` applies the same rules line by line so search never hits a
twin the parser drops; the factory exposes `NewSearchFilter` as
`thinkt.SearchLineFilterer`. The wire `turnId` is each entry's `PromptID`. A
loop step is one model response: its entry's `TurnID` is the step uuid (its
own `UUID`), and the results of the calls it made carry the same `TurnID`.
The chat completion id stays in `Metadata["message_id"]`.
`thinkt.ParserVersion` 6 covers this format.

## The legacy `~/.kimi` tree

The Python Kimi CLI wrote `~/.kimi/sessions/<hash>/<uuid>/context.jsonl`.
Kimi Code migrates those sessions into `~/.kimi-code` on first run, keeping
their ids (`ses_<uuid>`), and leaves the old tree as a copy. thinkt does
not read `~/.kimi`.
