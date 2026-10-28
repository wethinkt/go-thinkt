# Kimi Code source: reading `~/.kimi-code` with the conversation model

Date: 2026-10-28. Third pass of the "update how we read the agent objects"
work, after Claude (`worktree-conversation-model`) and Codex
(`worktree-codex-source`). Builds on both branches.

## Intent

The `kimi` source reads the legacy Python Kimi CLI tree
(`~/.kimi/sessions/<hash>/<id>/context.jsonl`). Kimi Code, the current CLI,
writes `~/.kimi-code`, migrates the old sessions into it on first run, and
leaves the old tree behind as a copy. thinkt should read Kimi Code's tree
with the same conversation model the Claude and Codex passes established:
every line classified before parsing, agent output and tools from the live
loop stream, user input from the context messages, usage attached to the
assistant entry, session state folded into `SessionMeta`, and subagents as
sessions linked to their parent and hidden by default.

The bias (agreed with Evan 2026-10-28): the loop event stream is the entry
source for agent output and tools; the batched `agent.message.appended`
records and `turn.prompt` are dedupe candidates; protocol 1.0 files, which
have no loop stream, read their context messages directly.

## Evidence that shapes the design

Measured on this machine, 2026-10-28: 44 sessions (31 native `session_`,
13 migrated `ses_`), 66 wire files (44 main, 22 subagents), 21,367 lines.

- Migrated sessions keep their legacy UUID: 13 of 15 legacy session ids
  appear as `ses_<uuid>` directories; the two missing ones were empty
  (`migration-report.json: sessionsSkippedEmpty: 2`). No cross-tree dedupe
  is needed.
- Three protocol generations, from the `metadata` record's
  `protocol_version`:
  - **1.0** (13 files, the migrated ones): only `context.append_message`
    records with roles user 74, assistant 757, tool 801. No `time`, no
    usage, no loop events. Assistant messages carry `content[{think|text}]`
    and `toolCalls[{id, function{name, arguments}}]`; tool messages carry
    `toolCallId`.
  - **1.4** (1 file): `context.append_loop_event` (630), `context.append_message`
    for user messages only (16), `turn.prompt`, `usage.record`, no
    `agent.message.appended`.
  - **1.5** (52 files): loop events (8,059), `context.append_message` for user
    messages only (336), `agent.message.appended` (3,039) written in batches
    (2,801 of them follow another appended record; they carry `meta.source`
    `llm`/`tool`/`input`/`notify`/`salvaged`), `turn.prompt` (131),
    `turn.ended` (131), `interaction.*` (636+636),
    `permission.record_approval_result` (622), `subagent.*`, `task.*`,
    `file_history.*`, `plan.*`.
- Loop events per step: `step.begin{uuid, turnId, step}`,
  `content.part{part{type: think|text}}` (1,323 think, 439 text),
  `tool.call{toolCallId, name, args, display?, description?}` (1,999),
  `tool.result{toolCallId, result{output, durationMs?, note?, isError?,
  truncated?}}` (1,995; `output` is a string 1,905 times and a content array
  90 times, the array form carrying `image_url` parts with `data:` or
  `blobref:` URLs), `step.end{finishReason: tool_use 1,305 | end_turn 108 |
  error 37 | interrupted 16, usage{inputOther, output, inputCacheRead,
  inputCacheCreation}, messageId, llm*Ms}`. Tool results arrive before the
  step's `step.end`.
- Of 1,864 loop tool-call ids in 1.5 files, 1,710 reappear in appended
  assistant messages; the 154 that do not belong to interrupted or failed
  steps. Appended assistant messages carry `meta.messageId` equal to the
  step's `step.end.messageId`, and appended user messages carry
  `meta.userMessageId` equal to the context message's `id`.
- User context messages by `origin.kind` in 1.4/1.5 files: `user` (typed
  prompts), `injection` (every one starts `<system-reminder>`),
  `system_trigger` (`<git-context>` preambles, subagent task prompts,
  `continue`), `task` and `background_task` (`<notification …>`),
  `skill_activation`, `shell_command`. 1.0 files have no `origin`.
- Tool names: Read 702, Bash 669, Grep 154, Edit 151, ReadMediaFile 94,
  Write 48, FetchURL 48, TodoList 43, Agent 20, Glob 16, WebSearch 15,
  TaskStop 13, WaitFor 8, AgentSwarm 5, Enter/ExitPlanMode 4, AskUserQuestion
  3, Skill 2. `tool.call.display` is `{kind: file_io|command, …}`.
- `state.json`: v2 files have `id`, `version: 2`, `cwd`, epoch-ms
  `createdAt`/`updatedAt`, `title`, `titleKind`, `isCustomTitle`,
  `lastPrompt`, `lastTurnReason`, `agents{<id>{type: main|sub,
  parentAgentId, labels{profileName}}}`; older files have ISO
  `createdAt`, `workDir`, no `id`, and migrated ones add
  `custom.imported_from_kimi_cli` with the legacy path and id.
- `session_index.jsonl`: one `{sessionId, sessionDir, workDir}` per session
  (44 lines). `workspaces.json` maps `wd_<name>_<hash>` to `{root, name}`.
- Subagent linkage lives in the parent: `state.json.agents` names each
  `agent-N` with `parentAgentId`, and the parent wire's
  `subagent.spawned{subagentId, subagentName, description, parentToolCallId,
  taskId}` ties the child to the `Agent` tool call. The child's own wire
  starts with `metadata` and a `system_trigger` user prompt; it carries no
  parent reference.
- Plans: `agents/main/plans/<name>.md`, announced by `plan.revision{key,
  version, sha256}`.

## Goals

1. Read `~/.kimi-code` (override `THINKT_KIMI_HOME`) as the Kimi source:
   projects from `wd_*` directories, sessions from their `state.json`, the
   session file `agents/main/wire.jsonl`.
2. Classify every wire line by record key before parsing; emit one assistant
   entry per loop step (thinking, text, tool_use blocks) with usage from
   `step.end`, one tool_result entry per `tool.result`, user entries from
   context messages with origin deciding kind and visibility.
3. Drop the batched `agent.message.appended` twins and `turn.prompt`; read
   protocol 1.0 files from their context messages.
4. Fold session state into `SessionMeta`: title, last prompt, first prompt,
   created time, model, entry count, token totals, duration, approvals.
5. List subagent wire files as sessions with `ParentSessionID`, hidden
   unless `WithSubagents()`, counted on the parent; link the parent's
   `Agent` tool call to the child through `ChildSessionID`.
6. Register `kimi.IsSearchable` so search skips bookkeeping and twins; bump
   `ParserVersion` so caches and the index recompute.

## Non-goals

- Reading the legacy `~/.kimi` tree. Kimi Code migrates it and the copies
  keep their ids; a machine with only `~/.kimi` has no Kimi source until
  Kimi Code runs once. (My call; cost if wrong: keep the old package under a
  `kimi-legacy` name.)
- Resolving `blobref:` image references to bytes (the blob store is not
  surveyed). They render as text references.
- Rendering plan files inline; a plan revision is an entry naming the file.
- New `thinkt` types. The Codex branch already added `KindToolWire`,
  `SessionCost` token totals, `CachedSession.ParentSessionID`,
  `WithSubagents`; this pass uses them.

## Anti-goals

- A user's prompt appearing twice or three times (context message, appended
  record, `turn.prompt`).
- Assistant output split into one entry per streamed `content.part`.
- Subagent sessions crowding the project list or the index.
- An entry with empty text and no blocks.

## Constraints

- Go 1.26, standard library only; the parser stays a streaming `NextEntry`
  reader over `thinkt.NewScannerWithMaxCapacityCustom`.
- `thinkt.ParserVersion` 4 → 5, once.
- Classification lives in one function used by the parser, the session
  scan, and `IsSearchable`, as in the Codex package.
- Tool pair halves never share a UUID; the index upserts on
  `(session_id, uuid)`.
- The `kimi` package is rewritten in place (same import path, same
  `Factory`, same `THINKT_KIMI_HOME`); `resume.go` keeps producing a resume
  command for the new ids.

## Design

### A. Classification (`internal/sources/kimi/classify.go`)

Record key: `"<type>"` for most lines; `loop/<event.type>` for
`context.append_loop_event`; `ctx/<message.role>` for
`context.append_message`; `appended/<message.message.role>` for
`agent.message.appended`. `recordHead` carries `Type`, `Key`, `Role`,
`Origin` (ctx origin kind or appended `meta.origin.kind`, with appended
`meta.source` as `Source`), `ToolCallID`, `TurnID` (event `turnId`,
stringified), `StepUUID`, `Time` (epoch ms, zero when absent), `LeadText`
(joined text parts), `Payload`.

| key / condition | class |
|---|---|
| `metadata` | session state (protocol version) |
| `loop/step.begin` | event (opens the step hold; never an entry) |
| `loop/content.part`, `loop/tool.call` | turn (accumulated into the step's assistant entry) |
| `loop/tool.result` | turn (tool_result entry, buffered until the step closes) |
| `loop/step.end` | event (closes the hold: usage, finish reason, message id) |
| `turn.step.interrupted` | event (arrives after the step's `step.end`; supplies `Error` = `message` to the step entry still held, see C) |
| user message (`appended/user` with `meta.source: input`, or `ctx/user`) origin `user`, or no origin and no harness tag | turn, `KindNone` |
| user message origin `system_trigger` | turn, `KindNone`, `Subtype: system_trigger` (it is the prompt that drove the turn; in a subagent session it is the task) |
| user message origin `injection`, or no origin and `<system-reminder>` lead, or no origin and another `<system>` lead | context, `KindMeta`, secondary |
| user message with no origin and `<system>Previous context has been compacted` lead (1.0 carry-over) | context, `KindCompactSummary`, secondary |
| user message origin `task`, `background_task`; `turn.steer`; `appended/user` with `meta.source: notify` | context, `KindTaskNotification`, secondary |
| user message origin `skill_activation` | context, `KindMeta`, secondary, `Subtype: skill_activation` |
| user message origin `shell_command` | turn, `KindCommandOutput`, secondary |
| `ctx/assistant`, `ctx/tool` | turn (protocol 1.0 content; absent in 1.4/1.5 files) |
| `appended/assistant`, `appended/tool` | turn, dedupe candidate (dropped when the loop already emitted the message or call) |
| `appended/user`, `ctx/user`, `turn.steer` | turn or context per the user rows above, dedupe candidates against each other (see C) |
| `turn.prompt`, `prompt.*`, `agent.turn.*` | bookkeeping (`agent.turn.started.turnId` updates turn state) |
| `turn.ended` | event, `KindTurnDuration`, secondary; `Error` = `error.message` when `reason` is `failed` |
| `subagent.spawned` | event, `KindAgentMessage`, `ChildSessionID` = child id, visible |
| `subagent.completed`, `subagent.failed`, `subagent.cancelled` | event, `KindAgentMessage`, secondary; text from `resultSummary` or `error` |
| `subagent.started` | bookkeeping |
| `task.started`, `task.terminated` | event, `KindCommandOutput`, secondary; `task.terminated` text = `outputTail` |
| `permission.record_approval_result` | event, merged into the tool result (never an entry) |
| `interaction.request`, `interaction.resolved` | bookkeeping (the approval is recorded by `permission.record_approval_result`; a `question` request duplicates the `AskUserQuestion` tool call) |
| `plan.revision` | event, `KindAttachment`, secondary, `Subtype: plan`, text = `key` |
| `usage.record`, `token_counting.*`, `llm.*`, `tools.*`, `file_history.*`, `profile.bind`, `runtime.set_binding`, `permission.set_mode`, `config.update`, `plugin.session_start`, `changeNotifier.snapshot`, `staleGuard.recorded`, `swarm_mode.*`, `plan_mode.*`, `task.waitDelivered`, `turn.step.retrying`, `turn.cancel` | bookkeeping |
| anything else | unknown: secondary system entry with the raw line, logged once per key |

`llm.request` is bookkeeping but updates turn state: `modelAlias` (else
`model`) becomes `Model` on later entries, `thinkingEffort` becomes
`Effort`.

### B. Entry conversion (`internal/sources/kimi/convert.go`, `tools.go`)

- **Step assistant entry**: UUID = `step.begin.uuid`; `TurnID` = step
  `turnId`; blocks in arrival order, consecutive `content.part` of the same
  type merged into one block (`think` → thinking, `text` → text); each
  `tool.call` → `tool_use{ToolUseID: toolCallId, ToolName: name, ToolInput:
  args}`; `Text` = joined text blocks; `Usage` from `step.end.usage`
  (`inputOther → InputTokens`, `output → OutputTokens`, `inputCacheRead →
  CacheReadInputTokens`, `inputCacheCreation → CacheCreationInputTokens`);
  `StopReason` = `finishReason`; `Metadata.message_id`. A step that ends
  with `finishReason: error` or `interrupted` carries no usage and no error
  text of its own; the `turn.step.interrupted{reason, message}` record that
  follows it does. Such a step yields an assistant entry even when it has
  no parts and no calls: `Kind: KindAPIError`, secondary, `StopReason` =
  the finish reason, `Error` = the interrupted record's `message` (else the
  finish reason), so the failure ("[provider.auth_error] 403 …", "Timed
  out") is visible in the conversation. A step that ends `tool_use` or
  `end_turn` with no parts and no calls yields nothing.
- **Model and effort** are stamped when the step closes, from the latest
  `llm.request`, which arrives after `step.begin` inside the step.
- **Mid-step events** (`subagent.spawned`, `task.started`, …) that arrive
  between a call and its result are buffered with the step and queued after
  the assistant entry in arrival order, so a child's spawn never precedes
  the tool call that spawned it.
- **Tool result entry**: user role; UUID = `<toolCallId>:result`;
  `tool_result{ToolUseID, ToolResult}`: string output as is; array output
  joined text parts, `image_url` parts with `data:` URLs as image children
  (`MediaType`, `MediaData`), other URLs (`blobref:`) as text children;
  `IsError` from `result.isError`; `note` as a text child; `Metadata.duration_ms`;
  `Permission = &PermissionDecision{Decision: approved|denied, Source:
  "user"}` when a `permission.record_approval_result` was recorded for the
  call id; `ChildSessionID` when a `subagent.spawned` named this call as
  `parentToolCallId`.
- **Protocol 1.0 messages** (`ctx/assistant`, `ctx/tool`): the assistant
  message becomes one entry (think, text, tool_use blocks from `toolCalls`,
  `function.arguments` parsed as JSON), UUID composed from the line; the
  tool message becomes a tool_result entry with UUID `<toolCallId>:result`,
  `IsError` when its text starts `<system>ERROR` (1.0 results carry no
  `isError` flag).
- **User entries**: from `appended/user` (`meta.source: input`, origin in
  `meta.origin.kind`, id in `meta.userMessageId`), `ctx/user` (origin in
  `origin.kind`, id in `id`), `turn.steer` (origin `background_task`), and
  `appended/user` with `meta.source: notify` (no id, no origin: a task
  notification), per the table; `Text` = joined text parts; UUID = the
  message id when present, else composed from the line.
- **Events**: as the table says; `turn.ended` `Metadata.duration_ms`.
- Child session id: `<sessionID>/<agentId>` (for example
  `session_eb0b625e-…/agent-0`).

### C. Streaming parser (`internal/sources/kimi/parser.go`)

State: the open step (assistant entry under construction, its buffered
results and mid-step events, and whether it is closed but still held for an
error message), turn state (turn id, model, effort), `seenCalls` (tool call
ids emitted by the loop), `seenMessages` (`step.end.messageId` values),
`seenUserIDs` and `turnUserTexts` (user message ids and trimmed texts seen
since the last `agent.turn.started`), approvals by call id, spawned
children by call id, an output queue, and the unknown tally.

Rules:

- `step.begin` flushes any held step and opens a new one. `content.part`
  and `tool.call` append to it; `tool.result` and mid-step events are
  buffered on it (a result with no open step is emitted directly).
  `step.end` with `tool_use` or `end_turn` closes the step: the assistant
  entry is queued first, then the buffered results and events in arrival
  order. `step.end` with `error` or `interrupted` marks the step closed but
  keeps it held through the next line: a `turn.step.interrupted` that
  follows attaches its `message` as `Error`; any other line flushes it
  with `Error` = the finish reason. EOF flushes whatever is held.
- User messages arrive in the order `appended/user (input)` →
  `agent.turn.started` → `turn.prompt` → `ctx/user` with the same id (every
  1.5 prompt), so the appended record is the entry and the context message
  is its twin: a `ctx/user` whose `id` is in `seenUserIDs` is dropped, and
  one with an unseen id (1.0 and 1.4 files, injections, notifications)
  converts. `turn.steer` converts and records its text. An `appended/user`
  with `meta.source: notify` (written at turn end, no id) is dropped when
  its trimmed text is in `turnUserTexts`, else converts as a task
  notification. Every emitted user entry records its id and text;
  `agent.turn.started` clears `turnUserTexts`.
- `appended/assistant` is dropped when its `meta.messageId` is in
  `seenMessages` or any of its tool call ids is in `seenCalls`; otherwise it
  is converted like a 1.0 assistant message (covers steps the loop never
  closed and the `salvaged` records). `appended/tool` is dropped when its
  `toolCallId` is in `seenCalls`.
- Timestamps: `time` (epoch ms) when present; zero otherwise (1.0 files).
  The store's session reader stamps zero timestamps with the session's
  `CreatedAt` so exports and the viewer keep an order; the index orders by
  file position anyway.
- Entries carry `Model` and `Effort` from the latest `llm.request`,
  stamped when they are queued.

### D. Session scan and state (`internal/sources/kimi/sessionstate.go`, `store.go`)

- **Listing**: walk `sessions/wd_*/{ses_,session_}*/state.json`. Session ID =
  the session directory name. `FullPath` = `agents/main/wire.jsonl`.
  `ProjectPath` = `state.cwd`, else `state.workDir`, else the
  `session_index.jsonl` `workDir`, else `workspaces.json` root for the
  `wd_` directory. `CreatedAt`/`ModifiedAt` from `createdAt`/`updatedAt`
  (epoch ms or ISO string). `Title` = `state.title`; `LastPrompt` =
  `state.lastPrompt`. Subagents: for every `agents/<id>/wire.jsonl` with
  `id != main`, a session with ID `<sessionID>/<id>`, `ParentSessionID` =
  session ID, `Title` = `labels.profileName + " " + id` from `state.agents`,
  `ProjectPath` of the parent. `SubagentCount` = number of such children.
  Hidden from `ListSessions` unless `WithSubagents()`; `ListProjects` counts
  them in `SubagentCount`.
- **Scan** (`scanSessionFile`): one pass with the Parser itself (wire files
  are at most a few MB): `EntryCount` = entries emitted, `FirstPrompt` =
  first `KindNone` user entry text, `Model` = last entry model, `Cost` =
  token totals summed over assistant entries' usage, `DurationMs` = Σ
  `turn.ended.durationMs`, `LinesAdded/Removed` left zero (no diffs in the
  wire). Using the parser keeps the count exact by construction.
- Metadata cache, `enrichInto`, `startEnrichment`, `GetSessionMeta` by id
  (subagent ids resolve) and by path (`…/agents/<id>/wire.jsonl` maps back
  to its session directory): same shapes as the Codex store.
- `IsSessionPath`: a path under `sessions/` ending in `/wire.jsonl`.
- `WatchConfig`: `sessions`, depth 5.
- Discovery: `~/.kimi-code` or `THINKT_KIMI_HOME`; the source is present
  when `sessions/` exists.

### E. Search

`kimi.IsSearchable(line)`: false for non-entry classes, `appended/*`, and
`turn.prompt`; true when the line is a `ctx/user` turn, a `loop/content.part`
text or think part, a `loop/tool.call`, a `loop/tool.result`, or a 1.0
`ctx/assistant` / `ctx/tool`. Registered in `internal/index/search`'s
`searchable` map.

### F. Documentation

`internal/sources/kimi/README.md` rewritten for the new tree and class
table; every mention of `~/.kimi` as the Kimi directory changes to
`~/.kimi-code`: `AGENTS.md` (source list, environment table, Docker
example), `README.md`, `DEVELOP.md`, `internal/cmd/sources.go`,
`internal/cmd/help_env.go`; the format report's Kimi column becomes "thinkt
after the Kimi pass"; `etc/reports/KIMI_STRUCTURE.md` gets a note that it
describes the legacy tree. `internal/tui/session_loader.go`, which derives
the old `hash/uuid` id from a `context.jsonl` path, follows the new
`IsSessionPath` and id scheme.

## Rollout order

1. Discovery and listing (`~/.kimi-code`, state.json, subagents, titles).
2. Classifier and fixture (one synthetic 1.5 wire file with a step, two
   results, an approval, an appended batch, a subagent spawn; one 1.0 file).
3. Converter and parser (step hold, dedupe, usage).
4. Session scan, cache, enrichment, `GetSessionMeta`.
5. Search registration, `ParserVersion` 5.
6. Docs, measurements on the real tree (entry counts vs appended twins,
   subagent links, titles), full suite, lint.

## Decisions left to the builder

- Whether `tool.call.display` is kept in the tool_use block's metadata.
- How `resume.go` forms the Kimi Code resume command for a session id.
- The exact text of `subagent.*` and `task.*` entries.
- Whether `subagent.spawned` is visible or secondary (visible here so the
  parent shows where a child began, matching Codex `SubAgentActivity`).

## Calls made after the builder check

A fresh builder read the document against all 66 wire files. Three rules
were contradicted by the data; each had one sensible answer, so they are
settled here rather than asked.

- **Typed prompts come from the appended `input` record**, not the context
  message: in every 1.5 file the appended user record precedes
  `agent.turn.started`, `turn.prompt`, and the context twin (81 of 81
  prompts, 28 of 28 subagent tasks), so dropping "the later twin" is the
  only rule that works without lookahead. The entry is identical either
  way (same text, same origin, same id); the three prompts whose turns were
  aborted before a context message was written now appear as entries,
  which is what the user typed. Cost if wrong: swap which of the two
  records is the twin.
- **`turn.steer` is an entry** (secondary task notification at its real
  time) and `notify` appended records dedupe by text within the turn, since
  seven of them twin a `turn.steer` rather than a context message.
- **Errored and interrupted steps produce a secondary `KindAPIError`
  entry** and the step stays held through the next line so
  `turn.step.interrupted`, which always arrives after `step.end` (23 of
  23), can attach its message. Without this the only human-readable
  failure text in a file ("You've reached your 5-hour limit", "Subagent
  timed out.") never reaches the conversation. Cost if wrong: one hold
  rule and a secondary entry per failed step.
- **`<system>` is a harness tag for 1.0 files**: a compaction carry-over
  (`<system>Previous context has been compacted…`, 3 of 32 migrated first
  prompts) is a secondary `KindCompactSummary`, other `<system>` leads are
  secondary `KindMeta`, and a 1.0 tool message starting `<system>ERROR` is
  an error result. Otherwise three migrated sessions would list a
  compaction notice as their first prompt.
- Verified by the builder and now stated: model and effort are stamped at
  step close (`llm.request` comes after `step.begin`); mid-step events are
  buffered with the step; 1.4 loop records have no `agentId` and their
  `tool.call.uuid` equals the call id; `step.end.turnId` is a string while
  `turn.*.turnId` is an int (both stringified); no nested subagents, no
  duplicate step uuids or call ids, every session has `state.json` and a
  main wire, the largest wire is 4.1 MB; REST routes already unescape ids
  with slashes.

## Calls I made

- Loop stream leads, appended records are twins: the loop is live and
  complete (interrupted steps included) and carries per-step usage; the
  appended batch is written later and skips unfinished turns.
- One entry per step, not per streamed part, because the viewer and exports
  treat an assistant entry as one message with blocks.
- `system_trigger` prompts stay visible: in a subagent session they are the
  only prompt.
- Legacy `~/.kimi` is not read (see Non-goals).
- Child session ids are `<sessionID>/<agentId>` so they are unique, stable,
  and resolvable by `GetSessionMeta` without a lookup table.
