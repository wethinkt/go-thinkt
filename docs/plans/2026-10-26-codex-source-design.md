# Codex source: classify rollouts, read the item stream, fold session state

Design for bringing `internal/sources/codex` up to the conversation model
introduced by the Claude pass (PR #3). Evidence is in
[etc/reports/SOURCE_FORMATS_2026-10.md](../../etc/reports/SOURCE_FORMATS_2026-10.md);
the Claude design in `docs/plans/2026-10-26-conversation-architecture-design.md`
is the template, and `thinkt.Entry`, `EntryKind`, `Visibility`, `TurnID`,
`CompactionInfo`, `ChildSessionID`, and `SessionMeta.ParentSessionID` already
exist.

## Intent

thinkt should show a Codex session the way it shows a Claude one: the
prompts and replies, each command with its output and exit code, each file
change with its diff, compactions, turn durations, token usage, a title, and
links to the subagents the session spawned. Today the codex source reads the
model wire only, so every tool call is an `exec` JavaScript blob with a JSON
output blob, there are no titles, no usage, no diffs, and 51,000 harness
items and 47,000 usage records per machine are parsed and discarded.

**Bias (decided with Evan):** the harness item stream (`event_msg/item_completed`)
is the entry source for agent output and tools; the model wire
(`response_item`) supplies what items lack. User and developer input is
richer on the wire (`content_item_kinds`, images), so those come from the wire.

## Evidence that shapes the design

- Every rollout on the machine (205, CLI 0.130 to 0.164) carries items. The
  legacy `event_msg/user_message|agent_message|agent_reasoning` records the
  current parser dedupes never appear in them.
- Items and wire records interleave within a turn. An `AgentMessage` item and
  its wire `message` share an id (`msg_…`); a `Reasoning` item and wire
  `reasoning` share an id (`rs_…`). A `UserMessage` item and the wire user
  `message` do not share an id. One `exec` wire call yields one to many
  `CommandExecution` items, and the items sit between the call and its output.
- Reasoning is encrypted: 0 of 15,639 items and 0 of 17,546 wire records
  carry summary text. Codex sessions have no readable thinking.
- Wire user messages carry `internal_chat_message_metadata_passthrough.content_item_kinds`:
  `user.text` is a prompt; `environments.environment_context`,
  `agents_md.instructions`, `plugins.recommendations`,
  `daemon_recovery.internal_context` are harness context; `shell.user_command`
  is a `!` command the user ran; `generic.turn_aborted` on developer messages
  is the interruption notice. 335 older user messages have no kinds and use
  tags (`<environment_context>`, `<turn_aborted>`, `# AGENTS.md instructions`).
- `turn_id` is on every item and on the wire's passthrough metadata;
  `task_started` / `task_complete` / `turn_aborted` bracket each turn with
  `duration_ms`, `time_to_first_token_ms`, `last_agent_message`, and an
  `error` object when the API refused.
- `session_meta.source` is `"cli"`, `"vscode"`, or
  `{subagent:{thread_spawn:{parent_thread_id, depth, agent_path, agent_nickname}}}`.
  Subagent rollouts are ordinary files in the date directories; the parent
  rollout names them only through `SubAgentActivity{agent_thread_id}` items.
  All 24 `parent_thread_id` and all 126 `agent_thread_id` values resolve to
  another rollout's `session_meta.id`.
- `session_meta.id` is the session. On every subagent rollout (24 of 24)
  `session_meta.session_id` differs from `id` and equals the parent's id; on
  the other 181 files the two are equal.
- A wire assistant `message` carries the same text as the `AgentMessage` item
  immediately before it on 7,880 of 7,881 records (1,356 of them have no `id`
  at all, so ids cannot be the join). One reply, on CLI 0.155.1, has no item.
- `Extension` items are `web.search` (463) or `clock.sleep` (210,
  `{id, kind, durationMs}`). `WebSearch` and `web.search` never occur in the
  same file. `ImageView` is `{id, path, type}` (321); nine wire user messages
  carry an `input_image` with `image_url`.
- Titles live in `~/.codex/session_index.jsonl` as `{id, thread_name, updated_at}`;
  the last line per id wins. 144 lines on this machine.
- `FileChange.changes[path] = {type: add|update|delete, unified_diff, move_path}`.
  `unified_diff` is empty on every `add` (822) and `delete` (63) and on 12
  updates; the other 2,873 updates start at `@@`. (The format report says
  `create`; the files say `add`.)
- `CommandExecution` has `command[]`, `cwd`, `exit_code`, `duration`,
  `aggregated_output`, and `source` (`unified_exec_startup` 20,502 or
  `user_shell` 133). `duration` is an object `{secs, nanos}` on all 20,635
  items, never a number.
- `token_usage_record` follows the wire call it bills, with `turn_id`,
  `response_id`, and `usage{input_tokens, cached_input_tokens,
  cache_write_input_tokens, output_tokens, reasoning_output_tokens,
  total_tokens}`. Between two entry-producing lines there is one usage record
  21,506 times and two 13 times; none precedes the first assistant entry.

## Goals

1. Classify every rollout line by `(type, payload.type, item.type)` before
   parsing, with the same `RecordClass` vocabulary the Claude source uses:
   turn, event, context, checkpoint, session state, bookkeeping, unknown.
2. Emit entries with `Kind`, `Visibility`, `TurnID`, `Usage`, `Error`,
   `Compaction`, and `ChildSessionID` filled from the item stream and wire.
3. Render each command, file change, MCP call, web search, and image view as a
   `tool_use` / `tool_result` pair with structured children (diff, stdout,
   stderr), exactly as the HTML page and exports already draw them for Claude.
4. Fold title, last prompt, model, turn durations, token totals, lines added
   and removed, and parent link into `SessionMeta`, cached under
   `ParserVersion`. Effort stays per entry (`Entry.Effort` exists;
   `SessionMeta` has no such field and gets none).
5. List subagent rollouts as sessions with `ParentSessionID`, keep them out of
   a project's `SessionCount`, and count them in `SubagentCount` on both the
   parent session and the project.
6. Keep the legacy `event_msg/user_message|agent_message|agent_reasoning`
   path working for rollouts older than the ones on this machine.

## Non-goals

- Decrypting or inferring reasoning. Encrypted reasoning is dropped, not
  shown as an empty thinking block.
- Reading `history.jsonl`, the SQLite side files (`goals`, `logs`, `queue`,
  `memories`), or `session_index.jsonl` beyond titles.
- Making the `exec` JavaScript wrapper readable. It is kept as a secondary
  entry for the curious, not formatted.
- Changing `thinkt` types beyond three additive items: `KindToolWire`,
  `SessionCost` token totals, and `CachedSession.ParentSessionID`. Anything
  else goes in `Metadata`.

## Anti-goals

- Emitting an entry twice because the item and the wire both describe it.
- An entry with no text and no blocks (the failure the Claude pass fixed).
- Parsing the whole file to list sessions: the fast metadata scan must stay a
  single streaming pass with early exit where possible, and the cache must
  cover it.

## Constraints

- Go 1.26; no new dependencies.
- `thinkt.ParserVersion` bumps once for this change (3 → 4).
- The codex parser stays a streaming `NextEntry` reader; the lazy reader and
  `OpenSession` keep working on multi-hundred-MB rollouts.
- Record classification lives in one function so the search scanner and the
  session scan use the same table.

## Design

### A. Classification (`internal/sources/codex/classify.go`)

`ClassifyLine(line) (recordKey, RecordClass)` decodes `type`,
`payload.type`, `payload.item.type`, `payload.role`, the passthrough
`content_item_kinds`, and, only for a wire user message with no kinds, the
leading text of `content` far enough to see a harness tag (335 older messages
on this machine; the classifier cannot stop at the type fields for those).
It then applies this table. `recordKey` is `"<type>"` for a top-level record
(`session_meta`, `turn_context`, `token_usage_record`, `compacted`,
`world_state`, `inter_agent_communication_metadata`),
`"event_msg/<payload.type>"`, `"response_item/<payload.type>"`, or
`"item/<item.type>"` for `event_msg/item_completed`. The table uses those
keys; the current store looks for `turn_context` inside `event_msg` and so
never sees it.

| record | class | entry |
|---|---|---|
| `response_item/message` role user, kinds contain `user.text`, or no kinds and no leading harness tag | turn | `RoleUser`, `KindNone`; an `input_image` whose `image_url` is a `data:` URI becomes an `image` block (`MediaType`, `MediaData`); any other `image_url` is a text block holding the URL, since the page renders an image block from bytes only |
| `response_item/message` role user, context kinds or harness tag | context | `RoleUser`, `KindMeta`, secondary; `Subtype` = first kind or tag |
| `response_item/message` role user, kinds `shell.user_command` | turn | `RoleUser`, `KindCommandOutput`, secondary (the matching `CommandExecution` item with `source: user_shell` is the visible one) |
| `response_item/message` role developer | context | `RoleSystem`, `KindMeta`, secondary; `Subtype` = first kind |
| `response_item/message` role assistant | turn, dedupe candidate, **not searchable** | emitted only when its text differs from the last `AgentMessage` item's text (the parser keeps that text; ids match only on CLI ≥ 0.146 and 1,356 records have none, so adjacency and text are the rule). Equal on 7,880 of 7,881 local records. The one wire-only reply still renders; search finds the item line, so these are never searchable |
| `item/UserMessage` | bookkeeping | duplicate of the wire user message |
| `item/AgentMessage` | turn | `RoleAssistant`, text; `Metadata.phase`, `Metadata.questions` |
| `item/Reasoning` with summary text | turn | `RoleAssistant`, thinking block |
| `response_item/reasoning` with summary | turn, dedupe candidate, **not searchable** | dropped when its text equals the last `Reasoning` item's (same adjacency rule as messages) |
| `item/Reasoning`, `response_item/reasoning` without summary | bookkeeping | no readable content |
| `item/CommandExecution` | turn | assistant `tool_use` (`ToolName` = the item's `source`, `unified_exec_startup` or `user_shell`, so it never collides with the pre-0.146 wire `exec_command`; input `{command, cwd}`) + user `tool_result` (`aggregated_output`, children `stderr` when present, `Metadata.exit_code`, `duration_ms` = `duration.secs × 1000 + duration.nanos / 1e6`) |
| `item/FileChange` | turn | assistant `tool_use` (`ToolName` = `apply_patch`, input = `{<path>: <type>}` for every change plus `move_path` when set) + user `tool_result` with one `diff` child per change whose `unified_diff` is non-empty (wrapped as `--- a/p` / `+++ b/p`; the body already starts at `@@`). `add` and `delete` carry no diff and are listed as text (`add <path>`, `delete <path>`); line counts come only from non-empty diffs. `IsError` from `status` |
| `item/McpToolCall` | turn | `tool_use` (`ToolName` = `mcp:<server>/<tool>`, input `arguments`) + `tool_result` from `result.content[]`, `IsError` = `result.isError` |
| `item/WebSearch`, `item/Extension` kind `web.search` | turn | `tool_use` `web_search` with queries; results as `tool_result` text. The two never occur in one file, so no search is drawn twice |
| `response_item/web_search_call` | bookkeeping | |
| `item/Extension` kind `clock.sleep` | turn | `tool_use` `sleep` input `{durationMs}` + `tool_result` text `slept <durationMs> ms`; the wire `sleep` call in the orchestration set is its secondary twin |
| `item/Extension` any other kind | unknown | tallied, raw JSON |
| `item/ImageView` | turn | `tool_use` `view_image` input `{path}` + `tool_result` text = the path. No bytes are on disk, so no `image` block |
| `item/Plan` | turn | `RoleAssistant`, `Subtype: plan`, text |
| `item/SubAgentActivity` | event | `RoleSystem`, `KindAgentMessage`, `Subtype` = `kind`, `ChildSessionID` = `agent_thread_id`, text `"<kind> <agent_path>"`; `interacted` is secondary |
| `item/CollabAgentToolCall` | event | `RoleSystem`, `KindAgentMessage`, secondary |
| `item/ContextCompaction` | bookkeeping | the `compacted` record is the entry |
| `response_item/function_call`, `response_item/custom_tool_call` and their `_output` twins, name in the **orchestration set** | event | `tool_use` / `tool_result` as today, `KindToolWire`, secondary. The set is the tools a structured item supersedes: `exec`, `wait` (produce `CommandExecution` items with their own ids), and `js`, `apply_patch`, `sleep`, `spawn_agent`, `send_message`, `wait_agent`, `interrupt_agent`, `followup_task`, `request_user_input_async` (their call id is reused as the item id, 100% on this machine) |
| `response_item/function_call`, `response_item/custom_tool_call` and outputs, any other name | turn | `tool_use` / `tool_result`, normal visibility. Covers CLI 0.130–0.145 where `exec_command` (2,827), `write_stdin`, `update_plan`, `list_agents`, `request_user_input` have no item twin, and any future tool. `exec_command` output text ("Chunk ID… Process exited with code N") yields `Metadata.exit_code` when parseable |
| `response_item/agent_message` | turn | `RoleUser`, `KindAgentMessage`, text from `input_text` parts, `Metadata.author`, `recipient` |
| `response_item/additional_tools` | bookkeeping | |
| `event_msg/task_complete`, `event_msg/turn_aborted` | event | `RoleSystem`, `KindTurnDuration`, secondary; `Metadata.duration_ms`, `time_to_first_token_ms`; `Error` from `error.message` or `reason` |
| `event_msg/task_started` | bookkeeping | `TurnID` comes from each record's own `turn_id` |
| `compacted` | event | `RoleSystem`, `KindCompactBoundary`, text from `message` or `"Context compacted"`, `CompactionInfo{Trigger: "auto", PreTokens: latest token_usage_record.thread_token_usage.total_tokens}` |
| `token_usage_record` | event, merged | attaches `Usage` to the held assistant entry (see C): `input_tokens` → `InputTokens`, `cached_input_tokens` → `CacheReadInputTokens`, `cache_write_input_tokens` → `CacheCreationInputTokens`, `output_tokens` → `OutputTokens`, `reasoning_output_tokens` → `ThinkingTokens` |
| `event_msg/token_count`, `event_msg/thread_settings_applied`, `world_state`, `inter_agent_communication_metadata` | bookkeeping / session state | `thread_settings_applied` feeds model and effort in the scan |
| `turn_context`, `session_meta` | session state | model, effort, cwd, parent; effort stamps `Entry.Effort` on the turn's entries |
| legacy `event_msg/user_message`, `event_msg/agent_message`, `event_msg/agent_reasoning` | turn | as today, with the existing text dedupe against wire twins |
| anything else | unknown | `RoleSystem`, `KindUnknown`, secondary, raw JSON text; tallied per reader |

`KindTurnDuration`, `KindCompactBoundary`, `KindMeta`, `KindCommandOutput`,
`KindAgentMessage`, and `KindUnknown` are the existing constants. `tool_wire`
is a new source-neutral constant `KindToolWire` ("the raw tool call the model
emitted, superseded by a structured item"), added to `thinkt` alongside the
others.

### B. Entry conversion (`internal/sources/codex/convert.go`)

`convertRecord(rec, state) []thinkt.Entry` returns zero, one, or two entries
(tool items yield a `tool_use` and a `tool_result`). Every entry gets:

- `UUID`: the item or wire id when present, otherwise the existing
  `composeUUID(session, line, kind, suffix)`. The two halves of a tool pair
  never share a UUID: the `tool_use` takes the id, the `tool_result` takes
  `<id>:result`. The index keys `entries` by `(session_id, uuid)` with an
  upsert, so equal ids would collapse the pair into one row and move the
  call's tokens onto the result.
- `TurnID`: `payload.turn_id`, or the passthrough `turn_id` for wire records.
- `Timestamp`: the line timestamp; `CommandExecution` uses it for both halves.
- `Source: SourceCodex`, `Model` from the current turn context.

`tool_use` / `tool_result` pairs share `ToolUseID` = item id so the
`conversation` package pairs them as it does for Claude. Wire pairs use
`ToolUseID` = `"wire:" + call_id`, always, because items reuse wire call ids
(`FileChange`, `McpToolCall`, collab tools) and `linkTools` would otherwise
attach a raw wire output to the structured item's call.

### C. Streaming parser (`internal/sources/codex/parser.go`)

`NextEntry` holds the most recent assistant-role entry, of any visibility,
until the next line that itself produces an entry; a `token_usage_record`
seen while it is held fills its `Usage`. Lines that produce nothing
(bookkeeping, `token_count`, dropped duplicates) do not release the hold.
Measured on the local rollouts, the record before a usage line is a wire
`custom_tool_call` 11,786 times, a `CommandExecution` item 3,648, a wire
`function_call` 2,958, and a wire assistant `message` 1,904, so usage lands
on the `KindToolWire` call, the command's `tool_use`, the `exec_command`
call, or (through the dropped wire message) the `AgentMessage` item. Usage
may therefore sit on a secondary entry; the index sums `Usage` regardless of
visibility, and the HTML page shows tokens only where the entry is visible.
This is accepted in exchange for a parser with one hold slot and no
response-id bookkeeping. The session scan also sums `token_usage_record`
`usage` fields directly so `SessionMeta` token totals never depend on where
entries landed.

After a tool item, the held entry is the assistant `tool_use`, not the user
`tool_result` emitted beside it. When two usage records land between
entry-producing lines (13 cases locally), the last write wins; the session
scan still sums every record.

The dedupe state (last `AgentMessage` item text, last `Reasoning` item text)
lives beside the hold. `IsSearchable` cannot see that state, so the
classifier marks wire assistant messages and wire reasoning not searchable:
the item line carries the same text and is searchable, and without this
every reply would match twice. Cost: the one wire-only reply on this machine
is unsearchable.

The parser owns a `turnState` (current `turn_id`, model, effort) updated by
`turn_context` and `task_started`, used to stamp entries that lack a turn id.

### D. Session scan and state (`internal/sources/codex/sessionstate.go`)

One streaming pass, the same shape as Claude's `scanSessionFile`, yields:

| field | from |
|---|---|
| `ID` | `session_meta.id`, never `session_meta.session_id`: on every subagent rollout `session_id` is the parent's id, and "correcting" it would fold each subagent into its parent. The current fast path already uses `id` |
| `Title` | `session_index.jsonl` (loaded once per store, last line per id wins; its ids are `session_meta.id`, 144 of 144); fallback `agent_nickname` + `agent_path` for subagents |
| `FirstPrompt` | first wire user message with `user.text` |
| `LastPrompt` | last such message |
| `Model` | `turn_context` (last seen), else `thread_settings_applied`, else `session_meta.model_provider`. Effort is not a session field |
| `EntryCount` | records whose class emits entries, counting tool items as two |
| `Cost` | `DurationMs` = sum of `task_complete.duration_ms`; `LinesAdded` / `LinesRemoved` from non-empty `FileChange` diffs; `TotalUSD` stays 0; `InputTokens`, `OutputTokens`, `CachedInputTokens` summed from every `token_usage_record` (new additive `SessionCost` fields). Cache-write and reasoning totals are left off the session total on purpose: reasoning is already inside `output_tokens`, and nothing lists cache writes; both stay on the per-entry `Usage` |
| `ParentSessionID` | `session_meta.source.subagent.thread_spawn.parent_thread_id` |
| `CreatedAt` | `session_meta.timestamp` |
| `GitBranch`, `ProjectPath` | `session_meta.git.branch`, `cwd` |

The scan stops early only for the cheap fields (`readSessionMetaFast` keeps
its 200-line cap for `FirstPrompt`, `Model`, cwd); the full pass runs under
the metadata cache keyed by path, mtime, size, and `ParserVersion`, which is
how the Claude store already behaves.

`CachedSession` gains `ParentSessionID` so the cache-hit path can exclude
subagents without re-reading files. Adding the struct field is not enough:
`MetadataCache.MergeInto` and the store's `CachedSession{…}` literal (the
Codex twin of `sessionScan.cached`) both copy it explicitly. Titles from
`session_index.jsonl` are merged at list time, not cached per file, since a
rename does not touch the rollout's mtime.

`SubagentCount` on a parent is computed after the walk from the parent ids
seen; `Project.SessionCount` excludes sessions with a parent and
`Project.SubagentCount` counts them. `ListSessions` returns subagent sessions
only when the caller asks (`thinkt.WithSubagents()`, a new list option, false
by default); `GetSessionMeta`, `LoadSession`, and `OpenSession` accept them by
id or path regardless.

### E. Search

`internal/index/search` calls a per-source `IsSearchable` through a small
registry keyed by `Candidate.Source` instead of the Claude-only branch added
in PR #3. The registry lives in `internal/index/search` and imports `codex`
the way it already imports `claude`; an `init` in the source package that
registered into `search` would import in a cycle. Codex supplies one built
on its classifier and converter, with the two wire dedupe-candidate rows
returning false.

### F. Documentation

`internal/sources/codex/README.md` gets the layout (date directories,
`session_index.jsonl`, subagent rollouts) and the class table above.
`AGENTS.md` gains a `KindToolWire` mention in the entry classification
paragraph. The Codex section of `etc/reports/SOURCE_FORMATS_2026-10.md` is
the field reference.

## Rollout order

1. Classifier and `KindToolWire`; `ParserVersion` 4. Tests: table cases and a
   fixture rollout (synthetic, ~40 lines, covering every row above).
2. Converter for wire user/developer messages and `AgentMessage`, `Plan`,
   `SubAgentActivity`, `agent_message`, turn and compaction events.
3. Tool items: `CommandExecution`, `FileChange` (diff children),
   `McpToolCall`, `WebSearch`, `ImageView`; wire tool calls become
   `KindToolWire`.
4. Usage attachment and `turnState`.
5. Session scan, `session_index.jsonl` titles, subagent listing, project
   counts, cache.
6. Search registry.
7. Docs; measure on a real rollout: entries before and after, zero empty
   entries, no duplicate assistant entries, index rows equal to the
   `NextEntry` count, every non-empty `unified_diff` renders as a diff in
   HTML export, and a subagent's `ParentSessionID` opens the parent.

## Decisions left to the builder

- Exact `ToolName` strings for items other than `CommandExecution`; the
  table gives defaults.
- Whether `duration_ms` goes on the `tool_result` or the `tool_use`
  (default: result). The unit conversion from `{secs, nanos}` is fixed.
- Fixture content, as long as it exercises every row.

## Calls made after the builder check

- Wire tool calls are secondary only for the named orchestration set; every
  other name, including the pre-0.146 `exec_command`, is a normal entry.
  Cost if wrong: a name moves between two lists.
- Wire `ToolUseID`s are namespaced `wire:` so item and wire pairs never
  collide in `linkTools`.
- Usage attaches to the held assistant entry whatever its visibility; the
  scan sums usage records for session totals. Cost if wrong: per-entry
  tokens are shown on a hidden entry for some turns, never lost.
- Wire assistant messages and reasoning are dedupe candidates by adjacent
  text, not unconditional bookkeeping, so a wire-only rollout still renders.
  Cost: one string of parser state.

## Calls made after the second review

Every claim in the review was checked against the 205 rollouts and the
worktree; all held except the wire-only reply count (one, not thirty).

- Tool pairs get distinct UUIDs (`<id>` and `<id>:result`) because the index
  upserts on `(session_id, uuid)`. Cost if wrong: none; `ToolUseID` still
  pairs them.
- `SessionMeta.ID` is pinned to `session_meta.id` in writing.
- `duration_ms` is computed from `{secs, nanos}`; a diff child is emitted
  only for a non-empty `unified_diff`; `clock.sleep` gets a `sleep` pair;
  `ImageView` and non-data image URLs are text, not image blocks.
- Effort stays on entries. Cost if wrong: one `SessionMeta` field later.
- Wire assistant messages and wire reasoning are not searchable. Cost: one
  reply on this machine is reachable only by reading the session.
- `CommandExecution.ToolName` is the item's `source` string, to keep it apart
  from the pre-0.146 wire `exec_command`. Cost if wrong: a display label.
- Per-entry usage maps onto the five `TokenUsage` fields; the session total
  carries input, output, and cached input only.
- The class table is written in `recordKey` form so top-level records are not
  looked for inside `event_msg`.
- The search registry stays in `internal/index/search` to avoid an import
  cycle.

## Calls I made

- User input comes from the wire, agent output and tools from items, because
  each is strictly richer there. Cost if wrong: one classifier row per record.
- Encrypted reasoning is dropped rather than shown as an empty thinking block.
- Subagent sessions are hidden from `ListSessions` by default, matching the
  Claude decision that subagents do not count toward a project's sessions.
  Cost if wrong: flip the default of `WithSubagents`.
- The `exec` wrapper calls stay as secondary `KindToolWire` entries instead of
  being dropped, so `--system` exports and the Other toggle still show what
  the model literally emitted.
