# Kimi Code Source Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite `internal/sources/kimi` to read Kimi Code's `~/.kimi-code` tree with the conversation model: classified wire lines, one assistant entry per loop step with usage, tool results with approvals, user input from the record that arrives first, subagent wire files as hidden linked sessions, session state from `state.json`.

**Architecture:** A classifier decides each wire line's record key and class from `(type, event.type, message role, origin)`; a converter turns user lines, events, and protocol 1.0 messages into entries; a streaming parser assembles loop steps (parts, calls, buffered results and events) and flushes them at `step.end`, dropping the batched `agent.message.appended` twins by id. The store lists sessions from `state.json` files, treats every `agents/<id>/wire.jsonl` other than `main` as a subagent session, and computes `SessionMeta` by running the parser once. Search, the TUI loader, resume, and the index prune follow the new ids.

**Tech Stack:** Go 1.26, standard library only. Tests with `go test`. No new dependencies.

**Spec:** `docs/plans/2026-10-28-kimi-source-design.md` (sections A–E are binding; "Calls made after the second review" overrides anything older in the document).

## Global Constraints

- `thinkt.ParserVersion` goes from 4 to 5 once, in Task 1. No other `thinkt` type changes.
- The package keeps its import path, `Factory()`, `NewStore`, `NewStoreWithCacheDir`, `DefaultDir`, `IsSessionPath`, and `THINKT_KIMI_HOME`. Every legacy-format function, type, and test is deleted, not kept.
- Classification lives in one function (`ClassifyLine`) used by the parser and `IsSearchable`; the session scan runs the parser, so it needs no weight table.
- Tool pair halves never share a UUID: the step entry takes `step.begin.uuid`, each result takes `<toolCallId>:result`.
- No entry is emitted with empty `Text` and no `ContentBlocks`.
- Session ids: the session directory name for the main wire; `<sessionDir>/<agentId>` for other wires. Project ids are the work-dir path.
- User prompts appear once: the `appended` input record is the entry, the context message with a seen id is dropped.

## Review Focus

1. A 1.5 wire whose last line is a `step.end` with `finishReason: error` and no `turn.step.interrupted` after it (file cut mid-write): the step flushes at EOF with `Error` = `"error"`, nothing hangs. Pinned in Task 4 (`TestParserFlushesErrorStepAtEOF`).
2. A `permission.record_approval_result` whose tool call never arrives (rejected before the call): no entry, no panic, later calls unaffected. Pinned in Task 4 (`TestParserApprovalWithoutCallIsIgnored`).
3. A `state.json` of the older shape (ISO `createdAt`, `workDir`, no `id`, no `version`): `CreatedAt` parsed, `ProjectPath` from `workDir`, id from the directory name. Pinned in Task 5 (`TestReadStateOlderShape`).
4. A session directory with `agents/agent-4/wire.jsonl` that `state.json.agents` does not mention: listed as a subagent from the filesystem with the agent id as title. Pinned in Task 5 (`TestSessionsFromDirListsUnregisteredAgent`).
5. A tool result whose output array carries a `blobref:` image URL: a text child, no image block, no panic. Pinned in Task 3 (`TestToolResultBlockBlobrefIsText`).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/thinkt/types.go` | `ParserVersion = 5` |
| `internal/index/ingester.go` | `PruneUnlistedProjects` |
| `internal/cmd/sync.go`, `internal/cmd/tui_bgsync.go` | call the project prune after listing |
| `internal/sources/kimi/classify.go` (new) | `RecordClass`, `recordHead`, `ClassifyLine`, `userClass`, `IsSearchable` |
| `internal/sources/kimi/unknown.go` (new) | `unknownTally` (copy of the Codex one, "kimi:" prefix) |
| `internal/sources/kimi/convert.go` (new) | user entries, events, protocol 1.0 messages, unknown, `decodeUsage`, `toolResultBlock` |
| `internal/sources/kimi/parser.go` (new) | step hold, buffered results and events, approvals, spawns, dedupe sets, EOF flush |
| `internal/sources/kimi/sessionstate.go` (new) | `stateFile`, `readState`, `parseStateTime`, `sessionScan`, `scanSessionFile`, `loadSessionIndex`, `loadWorkspaces` |
| `internal/sources/kimi/store.go` (rewrite) | listing from `state.json`, subagents, cache, enrichment, `GetSessionMeta` by id or path, readers |
| `internal/sources/kimi/discovery.go` (rewrite) | `~/.kimi-code`, `IsSessionPath`, `BaseDirForPath`, `SessionIDForPath` |
| `internal/sources/kimi/resume.go` (rewrite) | `kimi --session <id>` from `ProjectPath` |
| `internal/sources/kimi/testdata/wire-1.5.jsonl`, `wire-1.0.jsonl`, `wire-1.4.jsonl` (new) | fixtures |
| `internal/index/search/search.go` | register `kimi.IsSearchable` |
| `internal/tui/session_loader.go` | open Kimi sessions by wire path; delete `findKimiBaseDir` |
| docs: `internal/sources/kimi/README.md`, `AGENTS.md`, `README.md`, `DEVELOP.md`, `internal/cmd/help_env.go`, `internal/cmd/sources.go`, `etc/reports/SOURCE_FORMATS_2026-10.md`, `etc/reports/KIMI_STRUCTURE.md` | `~/.kimi-code`, new class table, legacy note |

`store.go` is rewritten from scratch (the old file is 1,150 lines of legacy layout); `store_test.go`, `store_enrich_test.go`, and `discovery_test.go` are deleted and replaced by the tests below.

---

### Task 1: `ParserVersion` 5 and the project-level index prune

**Files:**
- Modify: `internal/thinkt/types.go` (`ParserVersion` and its comment)
- Modify: `internal/thinkt/metadata_cache_test.go` (`TestParserVersionIsFour` → `TestParserVersionIsFive`)
- Modify: `internal/index/ingester.go`, `internal/cmd/sync.go:44-47`, `internal/cmd/tui_bgsync.go:43-47`
- Test: `internal/index/prune_test.go`

**Interfaces:**
- Produces: `ParserVersion = 5`; `func (i *Ingester) PruneUnlistedProjects(ctx context.Context, listed []thinkt.Project) error` which, for every source the ingester's registry has (`i.registry.All()`), deletes the `projects` rows of that source whose scoped id (`db.ScopedProjectID(source, id)`) is not among `listed`, together with their `sessions`, `entries`, `entry_attachments`, and `sync_state` rows. Sources absent from the registry (disabled) are left alone. A nil registry prunes nothing.

- [ ] **Step 1: Write the failing tests**

In `internal/thinkt/metadata_cache_test.go` rename `TestParserVersionIsFour` to `TestParserVersionIsFive` asserting `ParserVersion == 5`.

In `internal/index/prune_test.go` (reuse `listOnlyStore`):

```go
func TestPruneUnlistedProjectsRemovesStaleProjectsOfRegisteredSources(t *testing.T) {
	// db with projects: codex::/old (stale, with one session, two entries, one sync_state row),
	// codex::/proj (listed), kimi::oldhash (source not in registry).
	// registry holds only a listOnlyStore (SourceCodex) whose ListProjects returns /proj.
	// PruneUnlistedProjects(ctx, []thinkt.Project{{ID: "/proj", Source: thinkt.SourceCodex}})
	// → codex::/old and its rows are gone; codex::/proj and kimi::oldhash remain.
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/thinkt/ -run TestParserVersionIsFive; go test ./internal/index/ -run TestPruneUnlistedProjects`
Expected: `ParserVersion = 4`; `PruneUnlistedProjects undefined`.

- [ ] **Step 3: Implement**

Bump the constant and add the comment line "5: Kimi Code tree, loop steps, subagent wires". Implement `PruneUnlistedProjects` next to `pruneUnlisted`, reusing its per-session deletes; delete the `projects` row last. In both sync loops call `ingester.PruneUnlistedProjects(ctx, projects)` right after `ListAllProjects` succeeds, logging (not returning) its error.

- [ ] **Step 4: Run the packages**

Run: `go test ./internal/thinkt/ ./internal/index/ ./internal/sources/claude/ ./internal/sources/codex/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/thinkt internal/index internal/cmd/sync.go internal/cmd/tui_bgsync.go
git commit -m "feat(index): prune projects a source no longer lists; ParserVersion 5"
```

---

### Task 2: Fixtures and classifier

**Files:**
- Create: `internal/sources/kimi/classify.go`, `internal/sources/kimi/unknown.go`, `internal/sources/kimi/testdata/wire-1.5.jsonl`, `wire-1.0.jsonl`, `wire-1.4.jsonl`
- Delete: `internal/sources/kimi/store_test.go`, `store_enrich_test.go`, `discovery_test.go` (their subjects are gone; the package will not compile until Task 5 replaces `store.go`, so Tasks 2–4 run their tests with the old `store.go`, `discovery.go`, and `resume.go` temporarily moved aside: `git mv internal/sources/kimi/store.go internal/sources/kimi/store.go.old` etc. in this task, restored by the rewrite in Task 5 — see the Ruling at the end of the plan)
- Test: `internal/sources/kimi/classify_test.go`

**Interfaces:**
- Produces:
  - `type RecordClass int` with the seven Codex values, `String()`, `IsEntry()`.
  - `type recordHead struct { Type, Key, Role, Origin, Source, ID, ToolCallID, TurnID, StepUUID, LeadText string; Time int64; Payload json.RawMessage }` where `Key` is `"<type>"`, `"loop/<event.type>"`, `"ctx/<message.role>"`, or `"appended/<message.message.role>"`; `Role` is the message role (ctx, appended, or `"user"` for `turn.steer`); `Origin` is `message.origin.kind` (ctx), `message.meta.origin.kind` (appended), or `origin.kind` (`turn.steer`); `Source` is appended `meta.source`; `ID` is ctx `message.id` or appended `meta.userMessageId` (users) / `meta.messageId` (assistant); `ToolCallID` is the loop event's, the ctx/appended tool message's, or the approval's call id; `TurnID` is the stringified `turnId` of loop events and `turn.*`/`agent.turn.*` records; `StepUUID` is the loop event's `uuid` (`step.begin`/`step.end`) or `stepUuid`; `LeadText` is the joined trimmed text parts of a user message or `turn.steer` input; `Time` is the record's `time`.
  - `func ClassifyLine(line []byte) (recordHead, RecordClass)`: a non-object line is `ClassBookkeeping`.
  - `func userClass(h recordHead) (RecordClass, thinkt.EntryKind, string)` returning class, kind, and subtype per the spec's user rows: origin `user`/none without harness lead → turn/KindNone/""; `system_trigger` with lead `<git-context`, `Working directory:`, or text `continue` → context/KindMeta/"system_trigger"; other `system_trigger` → turn/KindNone/"system_trigger"; `injection`, or no origin with lead `<system-reminder>` or `<system>` → context/KindMeta/"injection"; no origin with lead `<system>Previous context has been compacted` → context/KindCompactSummary/"compaction"; `task`, `background_task`, or appended `notify` → context/KindTaskNotification/origin-or-"notify"; `skill_activation` → context/KindMeta/"skill_activation"; `shell_command` → turn/KindCommandOutput/"shell_command".
  - `var harnessLeads = []string{"<system-reminder>", "<system>"}`.
  - `func IsSearchable(line []byte) bool` per spec section E.
  - `unknownTally` as in Codex, logging `"kimi: unknown record type"`.

Class table by `Key` (spec section A restated; `userClass` decides the user rows):

| key / condition | class |
|---|---|
| `metadata` | session state |
| `loop/step.begin`, `loop/step.end`, `turn.step.interrupted` | event (never entries on their own) |
| `loop/content.part`, `loop/tool.call`, `loop/tool.result` | turn |
| `appended/user` with `Source == "input"` | `userClass` |
| `ctx/user` | `userClass` (the parser drops one whose `ID` was already emitted) |
| `turn.steer`; `appended/user` with `Source == "notify"` | context (`KindTaskNotification`) |
| `ctx/assistant`, `ctx/tool` | turn |
| `appended/assistant`, `appended/tool` | turn, dedupe candidate |
| `turn.ended` | event |
| `subagent.spawned`, `subagent.completed`, `subagent.failed`, `subagent.cancelled` | event |
| `task.started`, `task.terminated` | event |
| `plan.revision` | event |
| `permission.record_approval_result` | event (merged into the result, never an entry) |
| `turn.prompt`, `prompt.*`, `agent.turn.*`, `subagent.started`, `interaction.*`, `usage.record`, `token_counting.*`, `llm.*`, `tools.*`, `file_history.*`, `profile.bind`, `runtime.set_binding`, `permission.set_mode`, `config.update`, `plugin.session_start`, `changeNotifier.snapshot`, `staleGuard.recorded`, `swarm_mode.*`, `plan_mode.*`, `task.waitDelivered`, `turn.step.retrying`, `turn.cancel` | bookkeeping |
| anything else | unknown |

`llm.request` and `agent.turn.started` are bookkeeping but update turn state (Task 4).

- [ ] **Step 1: Write the fixtures**

`testdata/wire-1.5.jsonl`, exactly these 45 lines (session `session_fix1`):

```jsonl
{"type":"metadata","protocol_version":"1.5","created_at":1790000000000}
{"type":"runtime.set_binding","agentId":"main","time":1790000000001}
{"message":{"message":{"role":"user","content":[{"type":"text","text":"fix the build"}]},"meta":{"source":"input","promptId":"msg_u1","origin":{"kind":"user"},"tracked":true,"createdAt":"2026-10-01T10:00:00.000Z","userMessageId":"msg_u1"}},"type":"agent.message.appended","time":1790000001000,"kind":"event"}
{"turnId":0,"queueItemId":"msg_u1","type":"agent.turn.started","time":1790000001001,"kind":"event"}
{"type":"turn.prompt","agentId":"main","input":[{"type":"text","text":"fix the build"}],"origin":{"kind":"user"},"time":1790000001002}
{"type":"context.append_message","agentId":"main","message":{"role":"user","content":[{"type":"text","text":"fix the build"}],"toolCalls":[],"origin":{"kind":"user"},"id":"msg_u1"},"time":1790000001003}
{"type":"context.append_message","agentId":"main","message":{"role":"user","content":[{"type":"text","text":"<system-reminder>\nToday's date is 2026-10-01.\n</system-reminder>"}],"toolCalls":[],"origin":{"kind":"injection"}},"time":1790000001004}
{"type":"permission.record_approval_result","turnId":0,"toolCallId":"tool_b1","toolName":"Bash","action":"Running: go build ./...","result":{"decision":"approved","scope":"session"},"agentId":"main","time":1790000001005}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"step.begin","uuid":"step-1","turnId":"0","step":1},"time":1790000001006}
{"type":"llm.request","agentId":"main","kind":"loop","provider":"openai","model":"k3","modelAlias":"kimi-code/k3","thinkingEffort":"high","messageCount":2,"turnStep":"0.1","time":1790000001007}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"content.part","uuid":"p1","turnId":"0","step":1,"stepUuid":"step-1","part":{"type":"think","think":"Need to build."}},"time":1790000001008}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"content.part","uuid":"p2","turnId":"0","step":1,"stepUuid":"step-1","part":{"type":"text","text":"Running the build."}},"time":1790000001009}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"tool.call","uuid":"c1","turnId":"0","step":1,"stepUuid":"step-1","toolCallId":"tool_b1","name":"Bash","args":{"command":"go build ./..."},"display":{"kind":"command","command":"go build ./...","cwd":"/w"}},"time":1790000001010}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"tool.call","uuid":"c2","turnId":"0","step":1,"stepUuid":"step-1","toolCallId":"tool_a1","name":"Agent","args":{"description":"Explore","subagent_type":"explore"}},"time":1790000001011}
{"type":"subagent.spawned","subagentId":"agent-0","subagentName":"explore","parentToolCallId":"tool_a1","parentAgentId":"main","callerAgentId":"main","description":"Explore","runInBackground":false,"model":"kimi-code/k3","thinkingEffort":"high","taskId":"agent-x1","time":1790000001012}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"tool.result","parentUuid":"c1","toolCallId":"tool_b1","result":{"output":"ok","durationMs":12}},"time":1790000001013}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"tool.result","parentUuid":"c2","toolCallId":"tool_a1","result":{"output":[{"type":"text","text":"<image path=\"/x.png\">"},{"type":"image_url","imageUrl":{"url":"data:image/png;base64,AAAA"}},{"type":"text","text":"done"}],"isError":false}},"time":1790000001014}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"step.end","uuid":"step-1","turnId":"0","step":1,"finishReason":"tool_use","usage":{"inputOther":100,"output":20,"inputCacheRead":40,"inputCacheCreation":5},"messageId":"chatcmpl-1","providerFinishReason":"tool_calls"},"time":1790000001015}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"step.begin","uuid":"step-2","turnId":"0","step":2},"time":1790000001016}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"content.part","uuid":"p3","turnId":"0","step":2,"stepUuid":"step-2","part":{"type":"text","text":"Build is fixed."}},"time":1790000001017}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"step.end","uuid":"step-2","turnId":"0","step":2,"finishReason":"end_turn","usage":{"inputOther":50,"output":10,"inputCacheRead":0,"inputCacheCreation":0},"messageId":"chatcmpl-2"},"time":1790000001018}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"step.begin","uuid":"step-3","turnId":"0","step":3},"time":1790000001019}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"step.end","uuid":"step-3","turnId":"0","step":3,"finishReason":"error"},"time":1790000001020}
{"type":"turn.step.interrupted","agentId":"main","turnId":0,"step":3,"reason":"error","message":"[provider.auth_error] 403","time":1790000001021}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"step.begin","uuid":"step-4","turnId":"0","step":4},"time":1790000001022}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"content.part","uuid":"p4","turnId":"0","step":4,"stepUuid":"step-4","part":{"type":"text","text":"Partial answer"}},"time":1790000001023}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"step.end","uuid":"step-4","turnId":"0","step":4,"finishReason":"interrupted"},"time":1790000001024}
{"type":"turn.step.interrupted","agentId":"main","turnId":0,"step":4,"reason":"aborted","message":"Timed out","time":1790000001025}
{"type":"turn.ended","agentId":"main","turnId":0,"reason":"completed","durationMs":9000,"time":1790000001026}
{"message":{"message":{"role":"assistant","content":[{"type":"think","think":"Need to build."},{"type":"text","text":"Running the build."}],"toolCalls":[{"type":"function","id":"tool_b1","name":"Bash","arguments":"{\"command\":\"go build ./...\"}"},{"type":"function","id":"tool_a1","name":"Agent","arguments":"{\"description\":\"Explore\"}"}]},"meta":{"model":{"provider":"agent-loop","model":"agent-loop"},"source":"llm","usage":{"inputOther":100,"output":20,"inputCacheRead":40,"inputCacheCreation":5},"finish":{"finishReason":"tool_calls"},"messageId":"chatcmpl-1"}},"type":"agent.message.appended","time":1790000001027,"kind":"event"}
{"message":{"message":{"role":"tool","content":[{"type":"text","text":"ok"}],"toolCallId":"tool_b1"},"meta":{"source":"tool"}},"type":"agent.message.appended","time":1790000001027,"kind":"event"}
{"message":{"message":{"role":"tool","content":[{"type":"text","text":"done"}],"toolCallId":"tool_a1"},"meta":{"source":"tool"}},"type":"agent.message.appended","time":1790000001027,"kind":"event"}
{"message":{"message":{"role":"assistant","content":[{"type":"text","text":"Build is fixed."}],"toolCalls":[]},"meta":{"source":"llm","usage":{"inputOther":50,"output":10,"inputCacheRead":0,"inputCacheCreation":0},"finish":{"finishReason":"completed"},"messageId":"chatcmpl-2"}},"type":"agent.message.appended","time":1790000001027,"kind":"event"}
{"message":{"message":{"role":"assistant","content":[{"type":"think","think":"leftover"}],"toolCalls":[]},"meta":{"source":"salvaged","usage":{"inputOther":0,"output":0,"inputCacheRead":0,"inputCacheCreation":0}}},"type":"agent.message.appended","time":1790000001027,"kind":"event"}
{"type":"turn.steer","input":[{"type":"text","text":"<notification id=\"task:bash-1:failed\" category=\"task\">failed</notification>"}],"origin":{"kind":"background_task","taskId":"bash-1","status":"failed"},"time":1790000001028}
{"message":{"message":{"role":"user","content":[{"type":"text","text":"<notification id=\"task:bash-1:failed\" category=\"task\">failed</notification>"}]},"meta":{"source":"notify"}},"type":"agent.message.appended","time":1790000001029,"kind":"event"}
{"turnId":1,"queueItemId":"msg_u2","type":"agent.turn.started","time":1790000001030,"kind":"event"}
{"message":{"message":{"role":"user","content":[{"type":"text","text":"<notification id=\"task:bash-2:done\" category=\"task\">done</notification>"}]},"meta":{"source":"notify"}},"type":"agent.message.appended","time":1790000001031,"kind":"event"}
{"message":{"message":{"role":"user","content":[{"type":"text","text":"<git-context>\nWorking directory: /w\n</git-context>"}]},"meta":{"source":"input","promptId":"msg_u2","origin":{"kind":"system_trigger","name":"subagent"},"tracked":true,"userMessageId":"msg_u2"}},"type":"agent.message.appended","time":1790000001032,"kind":"event"}
{"type":"context.append_message","agentId":"main","message":{"role":"user","content":[{"type":"text","text":"<git-context>\nWorking directory: /w\n</git-context>"}],"toolCalls":[],"origin":{"kind":"system_trigger"},"id":"msg_u2"},"time":1790000001033}
{"message":{"message":{"role":"user","content":[{"type":"text","text":"Review the code structure"}]},"meta":{"source":"input","promptId":"msg_u3","origin":{"kind":"system_trigger","name":"subagent"},"tracked":true,"userMessageId":"msg_u3"}},"type":"agent.message.appended","time":1790000001034,"kind":"event"}
{"type":"context.append_message","agentId":"main","message":{"role":"user","content":[{"type":"text","text":"Review the code structure"}],"toolCalls":[],"origin":{"kind":"system_trigger"},"id":"msg_u3"},"time":1790000001035}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"step.begin","uuid":"step-5","turnId":"1","step":1},"time":1790000001036}
{"type":"context.append_loop_event","agentId":"main","event":{"type":"content.part","uuid":"p5","turnId":"1","step":1,"stepUuid":"step-5","part":{"type":"text","text":"Unclosed"}},"time":1790000001037}
{"type":"mystery.record","x":1,"time":1790000001038}
```

Expected entries from this fixture once Task 4 is done, in order (S = secondary):

| # | line | entry |
|---|---|---|
| 1 | appended input msg_u1 | user, KindNone, UUID `msg_u1`, Text "fix the build", TurnID "" (before any turn) |
| 2 | ctx injection | user, KindMeta, S, Subtype `injection`, TurnID "0" |
| 3 | step-1 | assistant, UUID `step-1`, TurnID "0", Model `kimi-code/k3`, Effort `high`, blocks [thinking "Need to build.", text "Running the build.", tool_use `tool_b1` Bash input `{"command":"go build ./..."}`, tool_use `tool_a1` Agent with ChildSessionID `session_fix1/agent-0`], Text "Running the build.", Usage 100/20/40/5, StopReason `tool_use`, Metadata.message_id `chatcmpl-1` |
| 4 | subagent.spawned | system, KindAgentMessage, visible, ChildSessionID `session_fix1/agent-0`, Subtype `spawned`, Text "spawned explore: Explore" |
| 5 | tool.result tool_b1 | user, UUID `tool_b1:result`, ToolResult "ok", Permission{Decision "approved", Source "user"}, Metadata scope `session`, duration_ms 12 |
| 6 | tool.result tool_a1 | user, UUID `tool_a1:result`, ToolResult "<image path=\"/x.png\">\ndone", one image child (image/png, AAAA), ChildSessionID `session_fix1/agent-0` on entry and block |
| 7 | step-2 | assistant "Build is fixed.", Usage 50/10, StopReason `end_turn` |
| 8 | step-3 + interrupted | assistant, KindAPIError, S, no blocks, Text "[provider.auth_error] 403", Error "[provider.auth_error] 403", StopReason `error`, Usage nil |
| 9 | step-4 + interrupted | assistant "Partial answer", visible, Error "Timed out", StopReason `interrupted` |
| 10 | turn.ended | system, KindTurnDuration, S, Metadata.duration_ms 9000 |
| 11 | appended salvaged | assistant, one thinking block "leftover", UUID composed from the line |
| 12 | turn.steer | user, KindTaskNotification, S, Subtype `background_task` |
| 13 | appended notify (bash-2) | user, KindTaskNotification, S, Subtype `notify`, TurnID "1" |
| 14 | appended input msg_u2 | user, KindMeta, S, Subtype `system_trigger`, UUID `msg_u2` |
| 15 | appended input msg_u3 | user, KindNone, Subtype `system_trigger`, UUID `msg_u3`, Text "Review the code structure" |
| 16 | step-5 (EOF) | assistant "Unclosed", TurnID "1", Usage nil, StopReason "" |
| 17 | mystery.record | system, KindUnknown, S, Text = raw line (buffered with step-5, queued after it) |

17 entries, 7 secondary (rows 2, 8, 10, 12, 13, 14, 17), 10 visible. The `turn.prompt`, both ctx twins (`msg_u1`, `msg_u2`, `msg_u3`), the four batched appended llm/tool records, the first notify record, `agent.turn.started`, `llm.request`, `runtime.set_binding`, `metadata`, and all `step.begin`/`step.end`/`turn.step.interrupted` lines produce nothing of their own.

`testdata/wire-1.0.jsonl`, 6 lines (session `ses_fix0`):

```jsonl
{"type":"metadata","protocol_version":"1.0","created_at":1750000000000}
{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"<system>Previous context has been compacted. Summary: we were fixing tests.</system>"}],"toolCalls":[]}}
{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"hello legacy"}],"toolCalls":[]}}
{"type":"context.append_message","message":{"role":"assistant","content":[{"type":"think","think":"t"},{"type":"text","text":"Running"}],"toolCalls":[{"type":"function","id":"tool_l1","function":{"name":"Bash","arguments":"{\"command\":\"ls\"}"}}]}}
{"type":"context.append_message","message":{"role":"tool","content":[{"type":"text","text":"<system>ERROR: Command failed</system>"}],"toolCalls":[],"toolCallId":"tool_l1"}}
{"type":"context.append_message","message":{"role":"assistant","content":[{"type":"text","text":"Done"}],"toolCalls":[]}}
```

Expected: 5 entries: user KindCompactSummary S; user "hello legacy" KindNone; assistant [thinking "t", text "Running", tool_use tool_l1 Bash input `{"command":"ls"}`]; tool_result `tool_l1:result` IsError true; assistant "Done". All timestamps zero.

`testdata/wire-1.4.jsonl`, 9 lines (session `session_fix4`):

```jsonl
{"type":"metadata","protocol_version":"1.4","created_at":1780000000000}
{"type":"turn.prompt","input":[{"type":"text","text":"hi 1.4"}],"origin":{"kind":"user"},"time":1780000001000}
{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"hi 1.4"}],"toolCalls":[],"origin":{"kind":"user"}},"time":1780000001001}
{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s14-1","turnId":"0","step":1},"time":1780000001002}
{"type":"llm.request","kind":"loop","provider":"openai","model":"k2","thinkingEffort":"medium","messageCount":1,"turnStep":"0.1","time":1780000001003}
{"type":"context.append_loop_event","event":{"type":"content.part","uuid":"q1","turnId":"0","step":1,"stepUuid":"s14-1","part":{"type":"text","text":"hello"}},"time":1780000001004}
{"type":"context.append_loop_event","event":{"type":"tool.call","uuid":"tool_x","turnId":"0","step":1,"stepUuid":"s14-1","toolCallId":"tool_x","name":"Read","args":{"path":"a"}},"time":1780000001005}
{"type":"context.append_loop_event","event":{"type":"tool.result","parentUuid":"tool_x","toolCallId":"tool_x","result":{"output":"contents"}},"time":1780000001006}
{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"s14-1","turnId":"0","step":1,"finishReason":"end_turn","usage":{"inputOther":7,"output":3,"inputCacheRead":0,"inputCacheCreation":0},"messageId":"chatcmpl-14"},"time":1780000001007}
```

Expected: 3 entries: user "hi 1.4" (UUID composed, no id); assistant `s14-1` Model `k2` Effort `medium` [text "hello", tool_use tool_x Read]; tool_result `tool_x:result` "contents".

- [ ] **Step 2: Write the failing classifier tests**

```go
func TestClassifyLineTable(t *testing.T) {
	cases := []struct{ name, line, key string; class RecordClass }{
		{"metadata", `{"type":"metadata","protocol_version":"1.5"}`, "metadata", ClassSessionState},
		{"step begin", `{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s","turnId":"0"}}`, "loop/step.begin", ClassEvent},
		{"part", `{"type":"context.append_loop_event","event":{"type":"content.part","stepUuid":"s","part":{"type":"text","text":"x"}}}`, "loop/content.part", ClassTurn},
		{"call", `{"type":"context.append_loop_event","event":{"type":"tool.call","toolCallId":"t","name":"Bash","args":{}}}`, "loop/tool.call", ClassTurn},
		{"result", `{"type":"context.append_loop_event","event":{"type":"tool.result","toolCallId":"t","result":{"output":"o"}}}`, "loop/tool.result", ClassTurn},
		{"appended input user", `{"type":"agent.message.appended","message":{"message":{"role":"user","content":[{"type":"text","text":"hi"}]},"meta":{"source":"input","origin":{"kind":"user"},"userMessageId":"m"}}}`, "appended/user", ClassTurn},
		{"appended notify", `{"type":"agent.message.appended","message":{"message":{"role":"user","content":[{"type":"text","text":"<notification/>"}]},"meta":{"source":"notify"}}}`, "appended/user", ClassContext},
		{"appended assistant", `{"type":"agent.message.appended","message":{"message":{"role":"assistant","content":[{"type":"text","text":"a"}]},"meta":{"source":"llm","messageId":"c"}}}`, "appended/assistant", ClassTurn},
		{"ctx user", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"hi"}],"origin":{"kind":"user"},"id":"m"}}`, "ctx/user", ClassTurn},
		{"ctx injection", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"<system-reminder>x"}],"origin":{"kind":"injection"}}}`, "ctx/user", ClassContext},
		{"ctx git context trigger", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"<git-context>\nx"}],"origin":{"kind":"system_trigger"},"id":"m"}}`, "ctx/user", ClassContext},
		{"ctx task trigger", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"Review this"}],"origin":{"kind":"system_trigger"},"id":"m"}}`, "ctx/user", ClassTurn},
		{"ctx 1.0 compaction", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"<system>Previous context has been compacted."}]}}`, "ctx/user", ClassContext},
		{"ctx 1.0 plain", `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"plain"}]}}`, "ctx/user", ClassTurn},
		{"ctx assistant", `{"type":"context.append_message","message":{"role":"assistant","content":[{"type":"text","text":"a"}],"toolCalls":[]}}`, "ctx/assistant", ClassTurn},
		{"steer", `{"type":"turn.steer","input":[{"type":"text","text":"<notification/>"}],"origin":{"kind":"background_task"}}`, "turn.steer", ClassContext},
		{"turn prompt", `{"type":"turn.prompt","input":[{"type":"text","text":"hi"}]}`, "turn.prompt", ClassBookkeeping},
		{"turn ended", `{"type":"turn.ended","turnId":0,"reason":"completed","durationMs":1}`, "turn.ended", ClassEvent},
		{"approval", `{"type":"permission.record_approval_result","toolCallId":"t","result":{"decision":"approved"}}`, "permission.record_approval_result", ClassEvent},
		{"spawned", `{"type":"subagent.spawned","subagentId":"agent-0","parentToolCallId":"t"}`, "subagent.spawned", ClassEvent},
		{"llm request", `{"type":"llm.request","modelAlias":"kimi-code/k3"}`, "llm.request", ClassBookkeeping},
		{"usage record", `{"type":"usage.record","usage":{}}`, "usage.record", ClassBookkeeping},
		{"unknown", `{"type":"mystery.record","x":1}`, "mystery.record", ClassUnknown},
		{"not json", `nope`, "", ClassBookkeeping},
	}
	// assert head.Key and class per case
}

func TestClassifyLineHeadFields(t *testing.T) {
	// appended input user: Role "user", Origin "user", Source "input", ID "m", LeadText "hi".
	// loop tool.call: ToolCallID "t", StepUUID from stepUuid, TurnID "0" (from "turnId":"0").
	// turn.ended with "turnId":0 → TurnID "0".
	// appended assistant: ID = meta.messageId.
}

func TestUserClass(t *testing.T) {
	// table of (origin, source, lead) → (class, kind, subtype) covering every spec row,
	// including "continue" system_trigger → context/KindMeta and "Working directory:" lead.
}

func TestIsSearchableKimi(t *testing.T) {
	cases := map[string]bool{
		`{"type":"agent.message.appended","message":{"message":{"role":"user","content":[{"type":"text","text":"needle"}]},"meta":{"source":"input","origin":{"kind":"user"},"userMessageId":"m"}}}`: true,
		`{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"needle"}],"origin":{"kind":"user"},"id":"m"}}`:                                                  false,
		`{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"needle"}]}}`:                                                                                    true,
		`{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"<system-reminder>needle"}],"origin":{"kind":"injection"}}}`:                                   false,
		`{"type":"context.append_loop_event","event":{"type":"content.part","part":{"type":"text","text":"needle"}}}`:                                                                                true,
		`{"type":"context.append_loop_event","event":{"type":"tool.result","toolCallId":"t","result":{"output":"needle"}}}`:                                                                          true,
		`{"type":"agent.message.appended","message":{"message":{"role":"assistant","content":[{"type":"text","text":"needle"}]},"meta":{"source":"llm","messageId":"c"}}}`:                           false,
		`{"type":"turn.prompt","input":[{"type":"text","text":"needle"}]}`:                                                                                                                             false,
		`{"type":"context.append_message","message":{"role":"assistant","content":[{"type":"text","text":"needle"}],"toolCalls":[]}}`:                                                                  true,
		`{"type":"usage.record","usage":{"inputOther":1}}`:                                                                                                                                             false,
	}
	// assert each
}

func TestUnknownTallyLogsOnce(t *testing.T) { /* as in Codex */ }
```

- [ ] **Step 3: Move the legacy files aside and run the tests to verify they fail**

Run:
```bash
git mv internal/sources/kimi/store.go internal/sources/kimi/store.go.old
git mv internal/sources/kimi/discovery.go internal/sources/kimi/discovery.go.old
git mv internal/sources/kimi/resume.go internal/sources/kimi/resume.go.old
git rm -q internal/sources/kimi/store_test.go internal/sources/kimi/store_enrich_test.go internal/sources/kimi/discovery_test.go
go test ./internal/sources/kimi/ -run 'TestClassifyLine|TestUserClass|TestIsSearchableKimi|TestUnknownTally' -v
```
Expected: FAIL to compile (`ClassifyLine` undefined). The repository will not build as a whole until Task 5 restores the package's exported surface; `go build ./internal/sources/kimi/` is the check until then.

- [ ] **Step 4: Implement `classify.go` and `unknown.go`**

`ClassifyLine` decodes one head struct: `type`, `time`, `event{type, uuid, stepUuid, turnId (json.RawMessage), toolCallId}`, `message` as `json.RawMessage`, `turnId (json.RawMessage)`, `toolCallId`, `input`, `origin{kind}`, `subagentId`; then, by `type`, decodes `message` as a ctx message (`role, id, origin{kind}, toolCallId, content`) or an appended envelope (`message{role, content, toolCallId}, meta{source, origin{kind}, userMessageId, messageId}`). `TurnID` stringifies a number or a string. `LeadText` joins the trimmed text parts (`text`, or `think` parts are not included) with "\n". `IsSearchable` classifies, applies `userClass` for user rows, and returns true only for the keys the spec names whose entry would not be secondary.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/sources/kimi/ -run 'TestClassifyLine|TestUserClass|TestIsSearchableKimi|TestUnknownTally' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/sources/kimi
git commit -m "feat(kimi): classify Kimi Code wire lines by record key; fixtures for protocols 1.0, 1.4, 1.5"
```

---

### Task 3: Converter for user lines, events, and protocol 1.0 messages

**Files:**
- Create: `internal/sources/kimi/convert.go`
- Test: `internal/sources/kimi/convert_test.go`

**Interfaces:**
- Consumes: `ClassifyLine`, `recordHead`, `RecordClass`, `userClass` (Task 2).
- Produces:
  - `type turnState struct { TurnID, Model, Effort string }`; `func (t *turnState) apply(h recordHead)`: `agent.turn.started` and `turn.*`/loop `step.begin` set `TurnID` when non-empty; `llm.request` sets `Model` (`modelAlias`, else `model`) and `Effort` (`thinkingEffort`).
  - `type converter struct { sessionID string; lineNo int; turn *turnState }`.
  - `func (c *converter) newEntry(role thinkt.Role, h recordHead, uuid string) thinkt.Entry`: UUID given else `composeUUID(sessionID, lineNo, h.Key)` (`"%s:%06d:%s"`), `Timestamp` from `h.Time` (ms) or zero, `Source: thinkt.SourceKimi`, `TurnID` = `h.TurnID` else `turn.TurnID`, `Model`, `Effort`, `Metadata: map[string]any{}`.
  - `func (c *converter) convertUser(h recordHead) []thinkt.Entry` for `appended/user`, `ctx/user`, `turn.steer`: role user, Kind/Visibility/Subtype from `userClass` (secondary for every context class), `Text` = LeadText, UUID = `h.ID` when set; nil when the text is empty.
  - `func (c *converter) convertEvent(h recordHead, line []byte) []thinkt.Entry` for `turn.ended` (system, KindTurnDuration, secondary, Text "turn ended: <reason>", `Metadata.duration_ms` int64, `Error` = `error.message` when reason is `failed`), `subagent.spawned` (system, KindAgentMessage, visible, Subtype `spawned`, `ChildSessionID` = `childSessionID(c.sessionID, subagentId)`, Text `"spawned <subagentName>: <description>"`), `subagent.completed`/`failed`/`cancelled` (system, KindAgentMessage, secondary, Subtype = the verb, Text = `resultSummary` or `error` or the verb, ChildSessionID), `task.started`/`task.terminated` (system, KindCommandOutput, secondary, Subtype = the verb, Text = `info.description` plus `outputTail` for terminated), `plan.revision` (system, KindAttachment, secondary, Subtype `plan`, Text = `key`, `Metadata.version`), unknown (system, KindUnknown, secondary, Text = raw line). UUIDs composed from the line, with `:<subagentId>` suffix for subagent events.
  - `func childSessionID(sessionID, agentID string) string`: the part of `sessionID` before any `/`, plus `"/" + agentID`.
  - `func (c *converter) convertLegacyAssistant(h recordHead) []thinkt.Entry` (`ctx/assistant` and the appended fallback): blocks from `content` (`think` → thinking, `text` → text) then `toolCalls` (`function.name`/`function.arguments` or flat `name`/`arguments`, arguments parsed as JSON else kept as string) as tool_use blocks; `Text` = joined text parts; UUID given (`h.ID` for appended) else composed; nil when there are no blocks.
  - `func (c *converter) convertLegacyTool(h recordHead) []thinkt.Entry` (`ctx/tool` and the appended fallback): user role, UUID `<toolCallId>:result`, `toolResultBlock` over the content array, `IsError` when the joined text starts `<system>ERROR`.
  - `func toolResultBlock(toolCallID string, output any, note string, isError bool) thinkt.ContentBlock`: string output as `ToolResult`; array output → joined text parts as `ToolResult`, `image_url` parts with `data:<type>;base64,<data>` as image children, other `image_url` as text children holding the URL; `note` as a trailing text child; `IsError`.
  - `func decodeUsage(raw json.RawMessage) *thinkt.TokenUsage`: `inputOther → InputTokens`, `output → OutputTokens`, `inputCacheRead → CacheReadInputTokens`, `inputCacheCreation → CacheCreationInputTokens`; nil for empty input.

- [ ] **Step 1: Write the failing tests**

```go
func TestConvertAppendedInputUser(t *testing.T) { /* fixture line 3 → user KindNone UUID msg_u1 Text "fix the build" */ }
func TestConvertCtxInjectionIsSecondaryMeta(t *testing.T) { /* fixture line 7 */ }
func TestConvertSystemTriggerVariants(t *testing.T) { /* git-context → KindMeta S; "continue" → KindMeta S; "Review the code structure" → KindNone visible, Subtype system_trigger */ }
func TestConvertTurnSteerIsTaskNotification(t *testing.T) { /* fixture line 35 */ }
func TestConvertTurnEndedFailed(t *testing.T) { /* reason failed, error.message "boom" → Error "boom", duration_ms int64 */ }
func TestConvertSubagentSpawned(t *testing.T) { /* fixture line 15 with sessionID "session_fix1" → ChildSessionID "session_fix1/agent-0", visible, Subtype "spawned"; the same line with sessionID "session_fix1/agent-0" and subagentId "agent-9" → "session_fix1/agent-9" (children hang off the root session) */ }
func TestConvertLegacyAssistantAndTool(t *testing.T) { /* 1.0 fixture lines 4 and 5 → blocks and IsError */ }
func TestConvertCompactionCarryOver(t *testing.T) { /* 1.0 fixture line 2 → KindCompactSummary S */ }
func TestToolResultBlockArrayOutput(t *testing.T) { /* fixture line 17 output → text "<image path=\"/x.png\">\ndone", one image child image/png AAAA */ }
func TestToolResultBlockBlobrefIsText(t *testing.T) { /* image_url blobref:image/png;abc → text child "blobref:image/png;abc", no image block */ }
func TestDecodeUsage(t *testing.T) { /* {"inputOther":100,"output":20,"inputCacheRead":40,"inputCacheCreation":5} → 100/20/40/5 */ }
func TestTurnStateApply(t *testing.T) { /* llm.request modelAlias+thinkingEffort; agent.turn.started turnId 1 → "1" */ }
```

`convertLine(t, line)` helper: `ClassifyLine` then `(&converter{sessionID: "session_fix1", lineNo: 1, turn: &turnState{}})` dispatching by key to `convertUser`/`convertEvent`/`convertLegacyAssistant`/`convertLegacyTool`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sources/kimi/ -run 'TestConvert|TestToolResultBlock|TestDecodeUsage|TestTurnState' -v`
Expected: FAIL to compile (`converter` undefined).

- [ ] **Step 3: Implement `convert.go`**

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/sources/kimi/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/kimi
git commit -m "feat(kimi): convert user lines, events, and protocol 1.0 messages"
```

---

### Task 4: Streaming parser with the step hold

**Files:**
- Create: `internal/sources/kimi/parser.go`
- Test: `internal/sources/kimi/parser_test.go`

**Interfaces:**
- Consumes: Task 2 and Task 3 names.
- Produces: `type Parser struct` with `func NewParser(r io.Reader, sessionID string) *Parser`, `func (p *Parser) NextEntry() (*thinkt.Entry, error)` (io.EOF at end), `func (p *Parser) UnknownTypes() map[string]int`. Internals: `step *openStep` where `type openStep struct { uuid, turnID string; blocks []thinkt.ContentBlock; buffered []thinkt.Entry; closed bool; finish string; usage *thinkt.TokenUsage; messageID string }`; `approvals map[string]approval` (`approval{decision, scope, selectedLabel, feedback string}`); `spawns map[string]string` (parentToolCallId → child session id); `seenCalls`, `seenMessages`, `seenUserIDs`, `turnTexts map[string]bool`; `out []thinkt.Entry`; `conv converter`; `turn turnState`; `unknown unknownTally`.

Algorithm (spec section C):

```
step(line):
  head, class := ClassifyLine(line); turn.apply(head); note unknown
  switch head.Key:
  case "permission.record_approval_result": approvals[toolCallID] = decode; return
  case "subagent.spawned": spawns[parentToolCallId] = childSessionID(...); entries := convertEvent → buffer-or-queue; return
  case "loop/step.begin": flush(); open step{uuid, turnID}; return
  case "loop/content.part": append/merge block (same type as last block → concatenate); return
  case "loop/tool.call": append tool_use block (ChildSessionID from spawns if known later: set at flush); seenCalls[id]=true; return
  case "loop/tool.result": entry := result entry (Permission from approvals, ChildSessionID from spawns on entry and block); if step open: buffer, else queue; return
  case "loop/step.end": if no step open: return; step.closed=true; finish, usage, messageID; seenMessages[messageID]=true; if finish is tool_use/end_turn: flush(); return   # error/interrupted stay held
  case "turn.step.interrupted": if step open && closed: step.error = message; flush(); return
  case "appended/assistant": if seenMessages[ID] or any toolCall id in seenCalls: return; entries := convertLegacyAssistant; (mark its call ids seen); buffer-or-queue
  case "appended/tool": if seenCalls[ToolCallID]: return; convertLegacyTool; buffer-or-queue
  case "appended/user": if Source == "input": emit via convertUser; seenUserIDs[ID]=true; turnTexts[text]=true
                        if Source == "notify": if turnTexts[text] return; emit as task notification; turnTexts[text]=true
  case "ctx/user": if ID != "" && seenUserIDs[ID]: return; emit via convertUser; record id and text
  case "turn.steer": emit via convertUser; record text
  case "agent.turn.started": turnTexts = {}; return
  default: entries := convertEvent/convertLegacy*/unknown as class says; buffer-or-queue
  buffer-or-queue: if step open → step.buffered = append(...), else out = append(...)
  any line other than step.end/turn.step.interrupted while a closed step is held → flush() first (Error = finish reason), then handle the line
flush(): if step nil return; build assistant entry (UUID step.uuid, TurnID, blocks, Text, Usage, StopReason, Model/Effort from turn, Error); empty blocks + finish error/interrupted → KindAPIError secondary with Text = Error; empty blocks otherwise → no entry; queue entry then step.buffered; step = nil
EOF: flush(); then drain out
```

- [ ] **Step 1: Write the failing tests**

```go
func readFixture(t *testing.T, name, sessionID string) []thinkt.Entry  // parses testdata/<name> to EOF
func TestParserFixture15Table(t *testing.T)      // the 17-row table from Task 2, every column named there
func TestParserFixture15Counts(t *testing.T)     // 17 entries, 7 secondary, no empty entry, no duplicate UUID, every tool_use ToolUseID has a tool_result with the same ToolUseID
func TestParserFixture10(t *testing.T)           // 5 entries as listed in Task 2
func TestParserFixture14(t *testing.T)           // 3 entries as listed in Task 2; Model "k2", Effort "medium"
func TestParserFlushesErrorStepAtEOF(t *testing.T)        // step.begin + step.end{error} as the last lines → one KindAPIError entry, Error "error"
func TestParserApprovalWithoutCallIsIgnored(t *testing.T) // approval for tool_z, then a normal step with tool_b → tool_b result has no Permission; no entry for tool_z
func TestParserMergesConsecutiveParts(t *testing.T)       // two think parts then two text parts → exactly one thinking block and one text block
func TestParserResultWithoutStepEmitsDirectly(t *testing.T) // tool.result with no open step → a tool_result entry
func TestParserPromptAppearsOnce(t *testing.T)            // fixture 1.5: entries whose Text == "fix the build" == 1
func TestParserTimestamps(t *testing.T)                   // 1.5 entry 1 Timestamp == 2026-09-... from time 1790000001000 (ms); 1.0 entries zero
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sources/kimi/ -run 'TestParser' -v`
Expected: FAIL to compile (`NewParser` undefined).

- [ ] **Step 3: Implement `parser.go`**

- [ ] **Step 4: Run the package with the race detector**

Run: `go test -race ./internal/sources/kimi/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/kimi
git commit -m "feat(kimi): streaming parser assembling loop steps with usage, approvals, spawns, and twin dedupe"
```

---

### Task 5: Session state, listing, store, discovery, resume

**Files:**
- Create: `internal/sources/kimi/sessionstate.go`, `internal/sources/kimi/store.go`, `internal/sources/kimi/discovery.go`, `internal/sources/kimi/resume.go` (the `.old` files are deleted in this task: `git rm internal/sources/kimi/*.old`)
- Test: `internal/sources/kimi/sessionstate_test.go`, `internal/sources/kimi/store_test.go`, `internal/sources/kimi/discovery_test.go`, `internal/sources/kimi/resume_test.go`

**Interfaces:**
- Consumes: `NewParser` (Task 4); `thinkt.WithSubagents`, `CachedSession.ParentSessionID`, `SessionCost` token fields, `MetadataCache`.
- Produces:
  - `type stateAgent struct { Type, ParentAgentID string; Labels struct{ ProfileName string } }`; `type stateFile struct { ID string; Version int; CWD, WorkDir string; CreatedAt, UpdatedAt time.Time; Title string; IsCustomTitle bool; LastPrompt, LastTurnReason string; Agents map[string]stateAgent }`; `func readState(path string) (stateFile, error)` decoding `createdAt`/`updatedAt` through `parseStateTime(json.RawMessage) time.Time` (number → epoch ms; string → RFC3339; else zero).
  - `type sessionScan struct { FirstPrompt, LastPrompt, Model string; EntryCount int; Cost *thinkt.SessionCost }`; `func scanSessionFile(path, sessionID string) sessionScan`: runs `NewParser` to EOF; `EntryCount` = entries; `FirstPrompt`/`LastPrompt` = first/last user entry with `KindNone`; `Model` = last non-empty entry model; `Cost.InputTokens/OutputTokens/CachedInputTokens` = Σ assistant `Usage.InputTokens/OutputTokens/CacheReadInputTokens`; `Cost.DurationMs` = Σ `turn.ended` `Metadata.duration_ms`; nil when all zero. `applyTo(meta *thinkt.SessionMeta)` fills empty fields (EntryCount always, LastPrompt only when state had none); `cached(meta) thinkt.CachedSession` including `ParentSessionID`.
  - `func loadSessionIndex(baseDir string) map[string]string` (sessionId → workDir); `func loadWorkspaces(baseDir string) map[string]string` (`wd_` dir name → root). Both tolerate missing files and bad lines.
  - `func sessionsFromDir(baseDir, sessionDir, wdName, workspaceID string, index, workspaces map[string]string) []thinkt.SessionMeta`: reads `state.json`; main session (ID = `filepath.Base(sessionDir)`, FullPath `agents/main/wire.jsonl`, ProjectPath = `cwd` else `workDir` else index else workspaces root else "unknown", Title, LastPrompt, CreatedAt, ModifiedAt = `updatedAt` else the wire's mtime, FileSize of the wire, Source kimi, ChunkCount 1); then one session per `agents/<id>/wire.jsonl` with `id != "main"` (glob the filesystem, not only `state.agents`): ID `<sessionID>/<id>`, ParentSessionID, Title = `labels.profileName + " " + id` trimmed, else `id`, ProjectPath of the parent, times from the wire's mtime. The main session's `SubagentCount` = number of children. Nothing when `state.json` or the main wire is missing.
  - Store: `scanSessions` walks `sessions/wd_*/*/state.json`, serves cache hits by wire path (`ID` from `SessionIDForPath`), caches misses with `ParentSessionID` and `Title`; `ListProjects` groups by `ProjectPath` (Project.ID = path; subagents count in `SubagentCount`); `ListSessions` filters subagents unless `IncludeSubagents`, supports `EnrichSync` and background enrichment (copy the Codex shapes, `EntryCount == 0` means unscanned); `GetSessionMeta(ctx, id)` accepts a session id, a `<session>/<agent>` id, or an absolute wire path (validated under the base dir); `LoadSession`/`OpenSession` through it; the session reader stamps zero entry timestamps with `meta.CreatedAt`; `WatchConfig{IncludeDirs: ["sessions"], MaxDepth: 5}`; `Workspace()` unchanged in shape (`kimi-code-<hostname>`).
  - Discovery: `basePath()` and `DefaultDir()` use `THINKT_KIMI_HOME` else `~/.kimi-code`; `IsAvailable` true when `<base>/sessions` exists; `func IsSessionPath(path string) bool`: base name `wire.jsonl`, grandparent named `agents`, and a `sessions` component above; `func BaseDirForPath(path string) string`: the parent of the `sessions` component, "" when absent; `func SessionIDForPath(path string) string`: for `<base>/sessions/<wd>/<session>/agents/<id>/wire.jsonl` returns `<session>` when id is `main`, else `<session>/<id>`.
  - Resume: `ResumeCommand(session)`: `kimi --session <parent id>` where the parent id is the part before `/`; `Dir` = `session.ProjectPath`; error when `kimi` is not on PATH.

- [ ] **Step 1: Write the failing tests**

`sessionstate_test.go`:

```go
func TestReadStateV2(t *testing.T)            // the fixture state from the design (id, version 2, cwd, epoch ms times, title, lastPrompt, agents main+agent-0 sub with profileName explore)
func TestReadStateOlderShape(t *testing.T)    // ISO createdAt, workDir, no id → CreatedAt 2026-07-30T00:09:55.213Z, WorkDir set, ID ""
func TestParseStateTime(t *testing.T)         // number ms, RFC3339 string, garbage → zero
func TestScanSessionFileFixture15(t *testing.T) // EntryCount 17, FirstPrompt "fix the build", LastPrompt "Review the code structure", Model "kimi-code/k3", Cost 150/30/40 tokens, DurationMs 9000
func TestScanSessionFileFixture10(t *testing.T) // EntryCount 5, FirstPrompt "hello legacy", Cost nil, Model ""
func TestLoadSessionIndexAndWorkspaces(t *testing.T) // index maps id→workDir, bad line skipped; workspaces maps wd name→root; missing files → empty maps
func TestSessionsFromDirMainAndSubagents(t *testing.T)  // temp session dir with state (agents main, agent-0 explore) and wires main, agent-0 → 2 sessions: main ID = dir name, SubagentCount 1, Title from state; child ID "<dir>/agent-0", ParentSessionID, Title "explore agent-0"
func TestSessionsFromDirListsUnregisteredAgent(t *testing.T) // agents/agent-4/wire.jsonl exists, state.agents lacks it → child listed with Title "agent-4"
func TestSessionsFromDirProjectPathFallbacks(t *testing.T) // state without cwd/workDir → index workDir; without index → workspaces root; without both → "unknown"
```

`store_test.go` (fixture tree under `t.TempDir()`: `sessions/wd_proj_abc/session_p1/{state.json, agents/main/wire.jsonl (copy of wire-1.5.jsonl), agents/agent-0/wire.jsonl (copy of wire-1.4.jsonl)}`, `session_index.jsonl`, `workspaces.json`; a second dir `sessions/wd_old_def/ses_l1/{state.json older shape, agents/main/wire.jsonl (copy of wire-1.0.jsonl)}`):

```go
func TestListProjectsUsesWorkDirPathAndCountsSubagents(t *testing.T) // 2 projects with ID = path; proj has SessionCount 1, SubagentCount 1
func TestListSessionsHidesSubagentsByDefault(t *testing.T)           // 1 session (session_p1, Title from state, SubagentCount 1); WithSubagents → 2, child ParentSessionID session_p1
func TestListSessionsEnrichSyncFillsScan(t *testing.T)               // WithEnrichSync → EntryCount 17, Cost.DurationMs 9000, Model kimi-code/k3
func TestListSessionsParentIDSurvivesCacheHit(t *testing.T)          // second store on the same cache dir → child still has ParentSessionID; parent SubagentCount 1
func TestGetSessionMetaByIDPathAndChildID(t *testing.T)              // "session_p1", the absolute main wire path, "session_p1/agent-0", the child's wire path → all resolve; unknown → nil
func TestOpenSessionStampsZeroTimestamps(t *testing.T)               // ses_l1 (1.0) entries get Timestamp == meta.CreatedAt
func TestLoadSessionFixture15(t *testing.T)                          // 17 entries, Source kimi, WorkspaceID set
func TestWatchConfigDepth(t *testing.T)                              // MaxDepth 5, IncludeDirs ["sessions"]
```

`discovery_test.go`:

```go
func TestIsSessionPath(t *testing.T)      // ".../sessions/wd_x/session_y/agents/main/wire.jsonl" true; ".../agents/agent-0/wire.jsonl" true; ".../session_y/state.json" false; ".../sessions/x/context.jsonl" false
func TestBaseDirForPath(t *testing.T)     // "/h/.kimi-code/sessions/wd/s/agents/main/wire.jsonl" → "/h/.kimi-code"; no sessions component → ""
func TestSessionIDForPath(t *testing.T)   // main → "session_y"; agent-0 → "session_y/agent-0"
func TestDefaultDirUsesKimiCode(t *testing.T) // with THINKT_KIMI_HOME unset → ends in ".kimi-code"; set → that value
```

`resume_test.go`:

```go
func TestResumeCommandUsesParentIDAndProjectPath(t *testing.T) // t.Setenv PATH to a temp dir holding an executable "kimi" → Args ["kimi","--session","session_p1"] for both "session_p1" and "session_p1/agent-0", Dir "/proj"
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sources/kimi/ -run 'TestReadState|TestParseStateTime|TestScanSessionFile|TestLoadSessionIndex|TestSessionsFromDir|TestList|TestGetSessionMeta|TestOpenSession|TestLoadSession|TestWatchConfig|TestIsSessionPath|TestBaseDirForPath|TestSessionIDForPath|TestDefaultDir|TestResumeCommand' -v`
Expected: FAIL to compile.

- [ ] **Step 3: Implement `sessionstate.go`, `store.go`, `discovery.go`, `resume.go`; delete the `.old` files**

- [ ] **Step 4: Build and test the repository**

Run: `go build ./... && go test ./internal/sources/kimi/ ./internal/sources/... ./internal/thinkt/ ./internal/cmd/ ./internal/tui/`
Expected: PASS except the two pre-existing iTerm tests in `internal/cmd`. (`internal/tui/session_loader.go` still compiles because `kimi.IsSessionPath`, `kimi.DefaultDir`, and `kimi.NewStore` keep their signatures; Task 6 fixes its behaviour.)

- [ ] **Step 5: Commit**

```bash
git add internal/sources/kimi
git commit -m "feat(kimi): read ~/.kimi-code: state.json listing, subagent wires as linked sessions, parser-backed scan, resume by session id"
```

---

### Task 6: Search registration and the TUI loader

**Files:**
- Modify: `internal/index/search/search.go` (`searchable` map), `internal/index/search/bookkeeping_test.go`
- Modify: `internal/tui/session_loader.go` (`openKimiLazySession`; delete `findKimiBaseDir`)
- Test: `internal/tui/session_loader_test.go` (new or extended)

**Interfaces:**
- Consumes: `kimi.IsSearchable`, `kimi.BaseDirForPath`, `kimi.NewStore(...).OpenSession(ctx, path)`.

- [ ] **Step 1: Write the failing tests**

In `bookkeeping_test.go`: `TestScanFileKimiSkipsContextTwinAndBatch`: a `Source: "kimi"` file with the appended input line, its ctx twin, and a batched appended assistant line, all containing the query → exactly one match, on line 1.

In `internal/tui/session_loader_test.go`: `TestOpenLazySessionKimiWirePath`: build the Task 5 fixture tree under a temp dir, set `THINKT_KIMI_HOME` to it, call `OpenLazySession(<main wire path>)` → non-nil session whose first entry text is "fix the build"; the child wire path opens too.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/index/search/ -run TestScanFileKimi -v; go test ./internal/tui/ -run TestOpenLazySessionKimi -v`
Expected: search finds three matches; the loader fails to open (legacy id).

- [ ] **Step 3: Implement**

Add `string(thinkt.SourceKimi): kimi.IsSearchable` to `searchable`. Rewrite `openKimiLazySession` as: `base := kimi.BaseDirForPath(path)`, default dir when empty, `kimi.NewStore(base).OpenSession(ctx, path)`, wrapped in `thinkt.NewLazySession`. Delete `findKimiBaseDir`.

- [ ] **Step 4: Run the packages**

Run: `go test ./internal/index/search/ ./internal/tui/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/index/search internal/tui
git commit -m "feat(kimi): searchable registry entry; TUI opens Kimi Code wire paths"
```

---

### Task 7: Documentation and real-tree measurement

**Files:**
- Modify: `internal/sources/kimi/README.md` (rewrite), `AGENTS.md:17,251,491`, `README.md:50,102,551`, `DEVELOP.md:56`, `internal/cmd/help_env.go:52`, `internal/cmd/sources.go:48,136`, `etc/reports/SOURCE_FORMATS_2026-10.md` (Kimi summary row, wire-record table's notes, mapping table incl. the child-id sentence), `etc/reports/KIMI_STRUCTURE.md` (legacy note at the top)

- [ ] **Step 1: Rewrite the README**

Sections: layout (`~/.kimi-code/sessions/wd_<name>_<hash>/<session>/`, `state.json`, `agents/<id>/wire.jsonl`, `session_index.jsonl`, `workspaces.json`), protocol generations, the class table from Task 2, the parser's step hold and twin rules in four sentences, session-state fields and their sources, ids (`<session>`, `<session>/<agent>`), `THINKT_KIMI_HOME`, key types and functions, `ParserVersion` 5, and one paragraph on the legacy `~/.kimi` tree not being read.

- [ ] **Step 2: Update the other docs and strings**

Every `~/.kimi` becomes `~/.kimi-code` (Docker mounts included); `sources.go` and `help_env.go` strings likewise; the format report's Kimi rows describe what thinkt reads after this pass and the child id form `<sessionID>/<agentId>`; `KIMI_STRUCTURE.md` opens with "This describes the legacy `~/.kimi` tree of the Python Kimi CLI; thinkt reads `~/.kimi-code` (see `internal/sources/kimi/README.md`)."

- [ ] **Step 3: Measure on the real tree**

Build `go build -o bin/thinkt ./cmd/thinkt` and record in the ledger:

- `bin/thinkt sessions list --source kimi -p /Users/evan/projects/gloss` lists `session_…` ids only (no `/agent-` ids); `bin/thinkt projects list --source kimi` shows paths, not hashes.
- `bin/thinkt export -f json --source kimi -p /Users/evan/projects/gloss --system -s session_eb0b625e-26c0-495c-9f7a-8c2a11178945`: entry count; `jq '[.[] | select((.text // "")=="" and ((.content_blocks // [])|length)==0)] | length'` → 0; duplicate UUIDs → 0; entries whose text starts "we've advanced this project" → 1; every tool_use id has a tool_result (`jq` set difference) → 0 missing; entries with `usage` ≥ the number of assistant step entries with a `tool_use` or `end_turn` stop reason.
- `bin/thinkt sessions view --source kimi -p /Users/evan/projects/gloss --raw session_eb0b625e-26c0-495c-9f7a-8c2a11178945/agent-0 | head -3` opens the child; the parent's `SubagentCount` is 3 (via a throwaway test or `sessions summary`).
- `bin/thinkt sync` then `sqlite3 ~/.thinkt/dbs/index.db "select count(*) from projects where id like 'kimi::%' and path not like '/%'"` → 0 (old hash projects pruned) and the entry count for the session equals the export count.
- A throwaway test comparing `scanSessionFile` to the parser on all 66 wires is unnecessary (the scan is the parser); instead run a throwaway test that parses all 66 wires and asserts no empty entry, no duplicate UUID per file, and every prompt text (`appended input`) appears once.

Expected: every check holds; a failing check is a finding to fix before the final review.

- [ ] **Step 4: Full suite and lint**

Run: `go test ./... && task lint`
Expected: PASS except the two iTerm tests; lint adds no findings in touched files (the repo has pre-existing ones).

- [ ] **Step 5: Commit**

```bash
git add internal/sources/kimi/README.md AGENTS.md README.md DEVELOP.md internal/cmd/help_env.go internal/cmd/sources.go etc/reports
git commit -m "docs(kimi): README for the Kimi Code tree; ~/.kimi-code everywhere; report updates"
```

---

## Rulings made while planning

- Tasks 2–4 compile against a package whose legacy `store.go`, `discovery.go`, and `resume.go` are moved aside (`.old`) rather than deleted, so `git` keeps their history visible until Task 5 replaces them; the repository does not build as a whole between Task 2 and Task 5. Cost if wrong: squash the moves into Task 5.
- The session scan runs the parser instead of a weight table; the files are small (≤ 4.1 MB) and exactness matters more than the second pass.
- Mid-step lines of every kind (events, unknown lines, appended fallbacks) are buffered on the open step and queued after its assistant entry, so a step's output always precedes what happened during it.
- `appended/user` input records set `turnTexts` too, so a `notify` twin of a typed prompt (none observed) is also dropped.
- `ChildSessionID` on a tool_use block is set at flush time from `spawns`, because `subagent.spawned` arrives after the `tool.call` that spawned it.
- The 1.0 compaction carry-over uses `KindCompactSummary` with `Subtype: compaction`, the Claude kind for the same thing.
