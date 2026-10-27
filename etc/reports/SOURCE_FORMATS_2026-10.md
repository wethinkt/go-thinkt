# Source formats as of October 2026: Codex, Antigravity, Kimi Code

Survey of what each harness writes on one machine, compared with what the
matching thinkt source reads. Companion to
[CLAUDE_TRANSCRIPT_SCHEMA.md](./CLAUDE_TRANSCRIPT_SCHEMA.md); the Claude pass
(record classes, `Kind`/`Visibility`, `TurnID`, session-state folding) is the
template each of these should follow.

Counts are from `~/.codex`, `~/.gemini/antigravity-cli`, and `~/.kimi-code`
on 2026-10-26. Survey scripts: `codex_survey.py`, `codex_items.py`,
`agy_survey.py`, `kimi_survey.py` (session scratchpad; not committed).

## Summary

| source | files | what thinkt reads | biggest gaps |
|---|---|---|---|
| Codex | 205 rollouts, 24 of them subagents | after the Codex pass: `item_completed` items for agent output and tools, wire messages for user/developer input, `token_usage_record` per assistant entry, `compacted`, `task_complete`/`turn_aborted`, titles from `session_index.jsonl`, subagent rollouts linked by `ParentSessionID` and hidden unless `WithSubagents()` | `token_count` rate limits, `thread_settings_applied`, `turn_context` policy fields, inter-agent `encrypted_content`; subagent rollouts are not indexed |
| Antigravity | 111 brain dirs, 104 with `transcript.jsonl`, 109 summary rows | nothing: the gemini source reads `~/.gemini/tmp/*/chats` | an entirely new source: step-indexed transcript, titles and workspaces in a SQLite summary db, nested conversations |
| Kimi Code | 42 sessions, 64 wire files (22 subagents) | nothing: the kimi source reads `~/.kimi/sessions/*/*/context.jsonl` | new layout under `~/.kimi-code`, `state.json` with title and last prompt, `session_index.jsonl`, per-agent `wire.jsonl` with turn and usage events |

## Codex (`~/.codex`)

### Layout

```
~/.codex/
├── sessions/YYYY/MM/DD/rollout-<timestamp>-<thread-id>.jsonl
├── session_index.jsonl        # {id, thread_name, updated_at}; last line per id wins
├── history.jsonl              # {session_id, ts, text}: every user prompt
├── goals_1.sqlite, logs_2.sqlite, queue_1.sqlite, memories_1.sqlite
└── config.toml, rules/, plugins/, skills in packages/
```

Every rollout line is `{timestamp, ordinal, type, payload}`; `response_item`
lines sometimes add `metadata`. CLI versions seen: 0.130 to 0.164.

### Record types (205 files, 185k lines)

| type / payload.type | count | meaning | thinkt after the Codex pass |
|---|---|---|---|
| `event_msg/item_completed` | 51,082 | semantic item stream: `item.type` is `CommandExecution` 20,635, `Reasoning` 15,639, `AgentMessage` 7,976, `UserMessage` 2,461, `FileChange` 2,090, `McpToolCall` 906, `Extension` 673, `ImageView` 321, `SubAgentActivity` 126, `ContextCompaction` 115, `WebSearch` 92, `CollabAgentToolCall` 65, `Plan` 1 | the entry source for agent output and tools: `AgentMessage`/`Reasoning`/`Plan` are assistant entries, tool items become tool_use + tool_result pairs, `SubAgentActivity`/`CollabAgentToolCall` are `KindAgentMessage` events, `UserMessage`/`ContextCompaction` are bookkeeping |
| `event_msg/token_count` | 25,320 | cumulative usage + rate limits after each response | bookkeeping |
| `token_usage_record` | 21,649 | per-response usage with `turn_id`, `response_id` | `Usage` on the held assistant entry; summed into `SessionCost` token totals |
| `response_item/custom_tool_call` + `_output` | 17,609 / 17,608 | `exec` JavaScript orchestration calls and chunked output | secondary `KindToolWire` entries (`wire:<call_id>`) when the name is an orchestration tool; normal tool entries otherwise |
| `response_item/reasoning` | 17,540 | `summary[]` plus `encrypted_content`; `content` on 1,907 | dropped when it twins the preceding `Reasoning` item; thinking block when wire-only; bookkeeping when encrypted only |
| `response_item/message` | 11,607 | roles user 2,924 / assistant 7,881 / developer 804; `phase` commentary 5,726 or final_answer 2,155 | user: turn, `KindCommandOutput`, or `KindMeta` by content item kinds; developer: secondary `KindMeta`; assistant: dropped when it twins the preceding `AgentMessage` item |
| `response_item/function_call` + `_output` | 6,326 each | `wait`, MCP, and legacy tools | as `custom_tool_call` |
| `event_msg/task_started` / `task_complete` / `turn_aborted` | 2,417 / 2,296 / 120 | turn brackets: `turn_id`, `root_turn_id`, `duration_ms`, `time_to_first_token_ms`, `last_agent_message`, `turn_attribution{turn_trigger, parent_turn_id, initiating_agent_path}` | `task_started` stamps `TurnID`; `task_complete`/`turn_aborted` are secondary `KindTurnDuration` entries with `duration_ms` and `Error` |
| `turn_context` | 2,388 | per-turn model, effort, cwd, approval and sandbox policy, collaboration mode | session state: `TurnID`, `Model`, `Effort` on every later entry; `SessionMeta.Model` |
| `event_msg/thread_settings_applied` | 2,166 | model, service tier, permission profile, reasoning effort | bookkeeping; model fallback for `SessionMeta.Model` |
| `world_state` | 472 | hashes of instructions, skills, environment | bookkeeping |
| `session_meta` | 205 | `session_id`, `cwd`, `cli_version`, `source` (`"vscode"`, `"cli"`, or `{subagent:{thread_spawn:{parent_thread_id, depth, agent_path, agent_nickname}}}`), `base_instructions` | `ID` (from `id`), cwd, branch, created, `ParentSessionID`, subagent title fallback |
| `compacted` | 114 | compaction with `replacement_history`, `retained_context`, window ids, `latest_token_usage_record` | `KindCompactBoundary` with `PreTokens` |
| `inter_agent_communication_metadata` | 122 | `{trigger_turn}` marker before an `agent_message` | bookkeeping |
| `response_item/agent_message` | 122 | inter-agent message with `author`, `recipient`, `encrypted_content` | user `KindAgentMessage` with author/recipient metadata |
| `response_item/web_search_call` | 86 | search action with queries | bookkeeping (the `WebSearch`/`Extension` item carries the query and results) |
| `response_item/additional_tools` | 12 | tool namespace catalog (large) | bookkeeping |

### What `item_completed` adds

The `response_item` stream is the model wire; `item_completed` is the
harness's own view and carries what the wire lacks:

- `FileChange.changes[path] = {type: add|update|delete, unified_diff, move_path}` with apply `stdout`/`stderr`. This is the Codex equivalent of Claude's `structuredPatch` and maps onto the `diff` child block.
- `CommandExecution` has the parsed command, `cwd`, `exit_code`, `duration` (`{secs, nanos}`), `aggregated_output`, and separate `stdout`/`stderr` on 6,778.
- `McpToolCall` has `server`, `tool`, `arguments`, `result{content[], isError}`, `duration`.
- `AgentMessage.phase` separates commentary from the final answer; `questions` on 96 marks `ask_user` prompts.
- `SubAgentActivity{kind, agent_thread_id, agent_path, model, reasoning_effort}` and `CollabAgentToolCall{sender_thread_id, receiver_thread_ids}` are the parent-side subagent events.
- `ContextCompaction` marks where a `compacted` record applies.

Items duplicate `response_item` content, so the plan should pick one as the
entry source and fold the other in. Items are richer; the wire has the
`call_id` linkage and the reasoning summary.

### Session state

| field | source |
|---|---|
| title | `session_index.jsonl` `thread_name`, last entry per id |
| first prompt | first `response_item/message` role user (or `history.jsonl`) |
| last prompt | `history.jsonl` or last `UserMessage` item |
| model, effort | `turn_context` (per turn) or `thread_settings_applied` |
| cost / tokens | sum of `token_usage_record`; `token_count.info.total_token_usage` is the running total |
| parent session | `session_meta.source.subagent.thread_spawn.parent_thread_id`, with `agent_path` and `agent_nickname` |
| cwd | `session_meta.cwd`, `turn_context.cwd` |

Subagent rollouts are ordinary files in the same date directory; nothing in
the parent rollout names the child file, so the link is child to parent only.
24 subagent rollouts across 3 parents on this machine.

### Mapping to the thinkt model

| Codex | thinkt |
|---|---|
| `message` role user, `UserMessage` item | `RoleUser`, `KindNone` |
| `message` role developer | `RoleSystem`, `KindMeta`, secondary |
| `message` role assistant, `AgentMessage` | `RoleAssistant`; `phase` in metadata |
| `reasoning.summary` | thinking block (summary); encrypted content dropped |
| `function_call`, `custom_tool_call`, `McpToolCall`, `CommandExecution` | tool_use block, `TurnID` from `turn_id` |
| `*_output`, `CommandExecution.aggregated_output` | tool_result block with stdout/stderr children; exit code in metadata |
| `FileChange` | tool_result with `diff` child per file |
| `task_complete`, `turn_aborted` | `RoleSystem`, `KindTurnDuration` (secondary), `Error` on abort |
| `compacted` | `RoleSystem`, `KindCompactBoundary`, `CompactionInfo{PreTokens from latest_token_usage_record}` |
| `agent_message`, `SubAgentActivity` | `KindAgentMessage`, `ChildSessionID` = agent thread id |
| `web_search_call`, `WebSearch`, `Extension` | tool_use `web_search` |
| `token_count`, `token_usage_record`, `turn_context`, `world_state`, `thread_settings_applied`, `additional_tools`, `inter_agent_communication_metadata` | bookkeeping or session state, never entries |

## Antigravity (`~/.gemini/antigravity-cli`)

Antigravity is a different product from Gemini CLI and shares only the
`~/.gemini` parent. The gemini source does not see it.

### Layout

```
~/.gemini/antigravity-cli/
├── brain/<conversation-id>/
│   ├── .system_generated/
│   │   ├── logs/transcript.jsonl        # one line per step (104 of 111 dirs)
│   │   ├── logs/transcript_full.jsonl   # same lines, untruncated content
│   │   ├── messages/, steps/, tasks/    # per-step text and task files
│   ├── <artifact>.md + .metadata.json   # agent-authored artifacts (plans, roadmaps) with summary and updatedAt
│   ├── .user_uploaded/, scratch/
├── conversations/<id>.db                # SQLite: steps (protobuf blobs), trajectory_meta
├── conversations/<id>.pb                # older protobuf snapshot (54 files, to June)
├── conversation_summaries.db            # SQLite: title, preview, step_count, workspace_uris, parent_conversation_id, nesting_depth, agent_name
├── cache/last_conversations.json        # workspace path -> latest conversation id
├── history.jsonl                        # user prompts
└── knowledge/, annotations/, implicit/
```

### Transcript steps (104 files, 40,129 lines)

Each line: `{step_index, source: MODEL|USER_EXPLICIT|SYSTEM, type, status, created_at, content, truncated_fields?, error?, media?}`.

| type | count | notes |
|---|---|---|
| `PLANNER_RESPONSE` | 17,940 | the model turn: `content` (text), `thinking`, `tool_calls[{name, args}]`, `input_tokens`, `cache_read_tokens`, `output_tokens` |
| `GENERIC` | 9,419 | tool result for tools without a dedicated type; `error` on 331, `media` on 49 |
| `VIEW_FILE` | 4,032 | file read result with path, line range |
| `RUN_COMMAND` | 2,151 | command output, truncated in `transcript.jsonl` |
| `CODE_ACTION` | 1,883 | file create/edit result |
| `GREP_SEARCH`, `LIST_DIRECTORY`, `SEARCH_WEB`, `READ_URL_CONTENT` | 1,766 / 659 / 541 / 53 | tool results |
| `USER_INPUT` | 985 | `<USER_REQUEST>` wrapped prompt plus `<ADDITIONAL_METADATA>` and settings changes |
| `SYSTEM_MESSAGE`, `EPHEMERAL_MESSAGE` | 261 / 92 | harness reminders (secondary) |
| `CONVERSATION_HISTORY` | 149 | injected summaries of earlier conversations |
| `ERROR_MESSAGE` | 128 | API errors |
| `CHECKPOINT` | 63 | compaction resume summary |
| `ASK_QUESTION` | 7 | clarifying question step |

Tool results carry `Created At` / `Completed At` header lines inside
`content`; tool calls live only in the preceding `PLANNER_RESPONSE.tool_calls`
with no call id, so call and result pair by order.

### Session state

`conversation_summaries.db` gives title (19 of 109 rows have one), preview,
step count, workspace URIs, last user input time, and
`parent_conversation_id` / `nesting_depth` for 5 nested conversations.
`cache/last_conversations.json` maps workspace path to the latest
conversation. Brain artifacts (`*.md.metadata.json`) are user-facing outputs
worth listing as attachments.

### Mapping

| Antigravity | thinkt |
|---|---|
| `USER_INPUT` | `RoleUser`; strip `<USER_REQUEST>` wrapper, keep metadata in `Metadata` |
| `PLANNER_RESPONSE` | `RoleAssistant` with text, thinking, tool_use blocks; `Usage` from token fields |
| tool result types | `RoleUser` tool_result paired by order with the preceding tool call; `GENERIC.error` sets `IsError` |
| `SYSTEM_MESSAGE`, `EPHEMERAL_MESSAGE`, `CONVERSATION_HISTORY` | `RoleSystem`, `KindMeta`, secondary |
| `CHECKPOINT` | `KindCompactBoundary` |
| `ERROR_MESSAGE` | `KindAPIError`, secondary |
| summary row | `SessionMeta.Title`, `ParentSessionID`, project from `workspace_uris` |

The `.db` step blobs are protobuf without a published schema; the JSONL
transcript is the practical source. `transcript_full.jsonl` should be
preferred when present since `transcript.jsonl` truncates content.

## Kimi Code (`~/.kimi-code`)

Kimi CLI migrated to Kimi Code in July 2026 (`~/.kimi/.migrated-to-kimi-code`).
The kimi source still reads the old `~/.kimi/sessions/<hash>/<id>/context.jsonl`
layout; 13 of the 42 new sessions are imports of those, the other 29 exist
only in the new layout.

### Layout

```
~/.kimi-code/
├── sessions/wd_<project>_<hash>/{ses_|session_}<uuid>/
│   ├── state.json              # title, isCustomTitle, lastPrompt, createdAt, agents{main, sub...}, custom.imported_from_kimi_cli
│   ├── agents/<agentId>/wire.jsonl
│   ├── agents/<agentId>/plans/*.md
│   └── logs/
├── session_index.jsonl         # {sessionId, sessionDir, workDir}
├── file-history/               # content-addressed file versions (<hash>@v<n>)
├── user-history/<hash>.jsonl
└── config.toml, skills/, workspaces.json
```

### Wire records (64 files, 22 of them subagents)

| type | count | notes |
|---|---|---|
| `context.append_loop_event` | 7,922 | step begin/end markers |
| `agent.message.appended` | 2,716 | newer form: `{message{role, content[]}, meta{source, promptId, origin, createdAt}}` |
| `context.append_message` | 1,951 | older form: `{message{role, content[{text|think}], toolCalls[]}, origin, id}` |
| `llm.request` | 1,342 | model, alias, thinking effort, prompt and tool hashes |
| `usage.record`, `token_counting.*` | 1,301 / 1,305 | per-request usage |
| `permission.record_approval_result`, `interaction.request/resolved` | 624 / 591 / 590 | approvals with tool call id and action |
| `turn.prompt`, `turn.ended`, `agent.turn.started/ended` | 130 / 122 / 101 / 100 | turn brackets with `turnId`, `outcome` |
| `file_history.tracked/checkpoint` | 94 / 32 | file versions per turn |
| `subagent.spawned/started/completed/failed/cancelled` | 21 / 21 / 12 / 4 / 5 | subagent lifecycle; each subagent has its own `agents/<id>/wire.jsonl` |
| `plan_mode.enter/exit`, `plan.revision` | 4 / 3 / 3 | plan files under `agents/main/plans` |
| `profile.bind`, `config.update`, `llm.tools_snapshot`, `permission.set_mode`, `metadata` | 50 / 11 / 51 / 50 / 64 | session configuration |

Message roles: user 393, assistant 757, tool 801; content parts `text`,
`think`, and `toolCalls` on the message.

### Mapping

| Kimi Code | thinkt |
|---|---|
| `context.append_message` / `agent.message.appended` | user, assistant, tool entries; `think` to thinking block, `toolCalls` to tool_use, role tool to tool_result |
| `turn.*`, `agent.turn.*` | `TurnID`; `outcome: aborted` sets `Error` |
| `usage.record` | `Usage` on the preceding assistant entry |
| `interaction.*`, `permission.record_approval_result` | `PermissionDecision` on the tool result |
| `subagent.*` | `ChildSessionID` = `<session>/agents/<agentId>` |
| `plan.revision` | attachment entry pointing at the plan file |
| `state.json` | `Title`, `LastPrompt`, `CreatedAt`, `ParentSessionID` for sub agents |
| everything else | bookkeeping |

## Shared work

All three need the same three pieces the Claude pass built: a classifier
that decides from the record type before parsing, session-state folding
into `SessionMeta` (title, last prompt, parent, cost), and `Kind` /
`Visibility` / `TurnID` on emitted entries. Two of them (Antigravity, Kimi
Code) are new source packages rather than updates. All three have subagents
as separate files with a child-to-parent link, which is the shape the Claude
design's step 3 (subagents as addressable sessions) already assumes.
