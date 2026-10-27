# codex

Package `codex` implements the session source for [Codex CLI](https://github.com/openai/codex).

## Directory Structure

```
codex/
├── discovery.go          # StoreFactory implementation, session path detection
├── classify.go           # RecordClass, recordHead, ClassifyLine, IsSearchable, entryWeight
├── convert.go            # converter: messages, events, compaction, legacy events, unknown
├── tools.go              # tool pairs from harness items; wire calls as single entries
├── usage.go              # decodeUsage for token_usage_record
├── parser.go             # streaming Parser: hold, dedupe, usage attach, turn state
├── sessionstate.go       # scanSessionFile, loadSessionIndex, sessionMetaRecord
├── unknown.go            # once-per-key debug log of unknown record keys
├── store.go              # thinkt.Store implementation
├── testdata/rollout-fixture.jsonl  # synthetic rollout exercising every class-table row
└── *_test.go
```

## Layout on disk

```
~/.codex/
├── session_index.jsonl                      # {id, thread_name, updated_at}; last line per id wins
└── sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl
```

Subagent rollouts are ordinary files in the same tree. Their `session_meta.source`
is an object (`{subagent:{thread_spawn:{parent_thread_id, depth, agent_path,
agent_nickname}}}`) instead of the string `"cli"` / `"vscode"`, and their
`session_id` is the parent's id while `id` is their own. `SessionMeta.ID` is
always `session_meta.id`.

The base directory defaults to `~/.codex` and can be overridden with `THINKT_CODEX_HOME`.

## Rollout lines and their classes

Every line is `{timestamp, type, payload}`. `ClassifyLine` reads the record key
(`"<type>"`, `event_msg/<payload.type>`, `response_item/<payload.type>`, or
`item/<item.type>` for `event_msg/item_completed`) and decides a `RecordClass`.
Harness items (`item/*`) are the entry source for agent output and tools; the
model wire (`response_item/*`) supplies what items lack: user and developer
input, raw tool calls, inter-agent messages.

| key / condition | class |
|---|---|
| `response_item/message` role user | turn (`user.*` kinds, or no kinds and no harness tag); turn + `KindCommandOutput` (`shell.user_command`); otherwise context + `KindMeta` |
| `response_item/message` role developer | context (`KindMeta`, secondary) |
| `response_item/message` role assistant, `response_item/reasoning` with summary | turn, dedupe candidate (dropped when it twins the preceding item) |
| `item/AgentMessage`, `item/Plan`, `item/CommandExecution`, `item/FileChange`, `item/McpToolCall`, `item/WebSearch`, `item/ImageView`, `item/Extension` kind `web.search` / `clock.sleep` | turn |
| `item/Reasoning` with summary / without | turn / bookkeeping |
| `response_item/reasoning` without summary (encrypted only) | bookkeeping |
| `item/SubAgentActivity`, `item/CollabAgentToolCall` | event (`KindAgentMessage`) |
| `response_item/function_call`, `custom_tool_call` and their outputs | turn, or event when the name is an orchestration tool (`exec`, `wait`, `js`, `apply_patch`, `sleep`, `spawn_agent`, `send_message`, `wait_agent`, `interrupt_agent`, `followup_task`, `request_user_input_async`): those become secondary `KindToolWire` entries because a harness item carries the same call |
| `response_item/agent_message` | turn (`KindAgentMessage`, user role) |
| `event_msg/task_complete`, `event_msg/turn_aborted` | event (`KindTurnDuration`, secondary) |
| `compacted` | event (`KindCompactBoundary`) |
| `token_usage_record` | event, merged into the held assistant entry, never an entry |
| `item/UserMessage`, `item/ContextCompaction`, `response_item/web_search_call`, `response_item/additional_tools`, `event_msg/task_started`, `token_count`, `thread_settings_applied`, `world_state`, `inter_agent_communication_metadata` | bookkeeping |
| `turn_context`, `session_meta` | session state |
| `event_msg/user_message`, `agent_message`, `agent_reasoning` | turn (legacy pre-item rollouts) |
| anything else | unknown: a secondary system entry carrying the raw line, logged once per key |

Tool items become a `tool_use` entry (assistant, UUID `<item id>`) and a
`tool_result` entry (user, UUID `<item id>:result`) sharing `ToolUseID`.
`CommandExecution` names its tool by the item's `source` (`unified_exec_startup`,
`user_shell`), carries `exit_code`, `duration_ms` (from `{secs, nanos}`), and
`status` in the result's metadata, and marks a non-zero exit as an error.
`FileChange` lists one `"<type> <path>"` line per change and adds a `diff` child
per change: the item's `unified_diff` for updates, or a diff synthesized from
`content` for adds (all lines added) and deletes (all lines removed), so a
created file is visible without opening the secondary wire `apply_patch`
call. Wire calls use `wire:<call_id>` for both the
UUID stem and `ToolUseID`, so they never collide with item pairs.

## Parser

`Parser` is a streaming `NextEntry` reader. It holds the entries of the most
recent producing line so a trailing `token_usage_record` can attach to the
assistant entry among them (a second record overwrites the first; a record with
no assistant entry held is dropped). A wire assistant message or reasoning
record whose trimmed text equals the preceding `AgentMessage` or `Reasoning`
item is dropped; a wire-only reply is kept. Legacy rollouts pair each
`event_msg` user/agent/reasoning event with the wire record that follows, and
the wire record wins. `turn_context` and `task_started` stamp `TurnID`, `Model`,
and `Effort` on every later entry.

## Session state

| `SessionMeta` field | source |
|---|---|
| `ID`, `ProjectPath`, `GitBranch`, `CreatedAt`, `ParentSessionID` | `session_meta` (`id`, `cwd`, `git.branch`, `timestamp`, `source.subagent.thread_spawn.parent_thread_id`) |
| `Title` | `session_index.jsonl` thread name; subagents fall back to `agent_nickname agent_path` |
| `FirstPrompt`, `LastPrompt` | user wire messages classified as turns (legacy `user_message` too) |
| `Model` | last real `turn_context.model`, else `thread_settings_applied.thread_settings.model`, else `session_meta.model_provider` |
| `EntryCount` | sum of `entryWeight` over the file (one full scan) |
| `Cost` | `DurationMs` from `task_complete`, lines added/removed from `FileChange` diffs, token totals from every `token_usage_record` |
| `SubagentCount` | rollouts whose `ParentSessionID` is this id |

The listing reads the first 200 lines of each rollout (`readSessionMetaFast`)
and caches the result; `scanSessionFile` fills the rest on enrichment
(`WithEnrichSync`, `WithEnrich`, or `GetSessionMeta`). Subagent rollouts are
hidden from `ListSessions` unless `thinkt.WithSubagents()` is passed; projects
count them in `SubagentCount`, not `SessionCount`.

`IsSearchable` lets the search scanner skip bookkeeping lines, wire twins, and
secondary entries. `thinkt.ParserVersion` 4 covers this classification.

## Key Types

- [`Discoverer`](discovery.go) — implements [`thinkt.StoreFactory`](../../thinkt/).
- [`RecordClass`](classify.go), [`recordHead`](classify.go) — the class table's inputs and output.
- [`Parser`](parser.go) — streaming reader producing [`thinkt.Entry`](../../thinkt/) values.
- [`sessionScan`](sessionstate.go) — one full pass over a rollout for `SessionMeta`.
- [`Store`](store.go) — implements [`thinkt.Store`](../../thinkt/).

## Key Functions

- [`Factory()`](discovery.go), [`IsSessionPath(path)`](discovery.go)
- [`ClassifyLine(line)`](classify.go), [`IsSearchable(line)`](classify.go)
- [`NewParser(r, sessionID)`](parser.go)
- [`scanSessionFile(path)`](sessionstate.go), [`loadSessionIndex(baseDir)`](sessionstate.go)
- [`NewStore(baseDir)`](store.go)
