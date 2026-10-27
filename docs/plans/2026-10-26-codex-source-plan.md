# Codex Source Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring `internal/sources/codex` up to the conversation model: classify every rollout line, read the harness item stream for agent output and tools, fold session state into `SessionMeta`, and list subagent rollouts with a parent link.

**Architecture:** A classifier decides each line's record class from `(type, payload.type, item.type)`; a converter turns entry-class lines into zero, one, or two `thinkt.Entry` values; a streaming parser buffers the entries of the most recent producing line so a trailing `token_usage_record` can attach and so wire twins of harness items can be dropped. A separate single-pass scan builds `SessionMeta`, titles come from `session_index.jsonl` at list time, and subagent rollouts are hidden from `ListSessions` unless asked for.

**Tech Stack:** Go 1.26, standard library only. Tests with `go test`. No new dependencies.

**Spec:** `docs/plans/2026-10-26-codex-source-design.md` (the class table in section A is the binding reference for every row below).

## Global Constraints

- `thinkt.ParserVersion` goes from 3 to 4 once, in Task 1.
- Only three `thinkt` additions beyond the list option: `KindToolWire`, `SessionCost{InputTokens, OutputTokens, CachedInputTokens}`, `CachedSession.ParentSessionID`.
- The parser stays a streaming `NextEntry` reader over `thinkt.NewScannerWithMaxCapacityCustom(r, 64*1024, thinkt.MaxScannerCapacity)`.
- Classification lives in one function (`ClassifyLine`) used by the parser, the session scan, and `IsSearchable`.
- Tool pair halves never share a UUID: `tool_use` takes `<id>`, `tool_result` takes `<id>:result`; wire pairs use `wire:<call_id>` for both the UUID stem and `ToolUseID`.
- No entry is emitted with empty `Text` and no `ContentBlocks`.
- Encrypted reasoning is dropped, never shown as an empty thinking block.
- `SessionMeta.ID` is `session_meta.id`, never `session_meta.session_id`.

## Review Focus

1. A rollout whose `AgentMessage` item text differs from its wire twin by trailing whitespace: both must not render. Pinned in Task 3 (`TestParserDropsWireAssistantTwinIgnoringWhitespace`).
2. A `token_usage_record` arriving right after a wire tool output (user role) with no assistant entry pending: per-entry usage is dropped, nothing panics, and the session scan still sums it. Pinned in Task 5 (`TestParserUsageAfterUserEntryIsNotAttached`) and Task 7 (`TestScanSessionFileSumsEveryUsageRecord`).
3. A `FileChange` with `move_path` and an empty diff: the `tool_use` input names the move, no diff child, no panic. Pinned in Task 4 (`TestFileChangeMoveWithoutDiff`).
4. A `session_index.jsonl` line that is not JSON, or an id that matches no rollout: ignored, listing still works. Pinned in Task 7 (`TestLoadSessionIndexSkipsBadLines`).
5. A subagent rollout whose parent file is missing: the child still lists with `WithSubagents`, `SubagentCount` on nothing, no error. Pinned in Task 7 (`TestListSessionsOrphanSubagent`).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/thinkt/types.go` | `KindToolWire`, `SessionCost` token fields, `ParserVersion = 4` |
| `internal/thinkt/metadata_cache.go` | `CachedSession.ParentSessionID`, copied by `MergeInto` |
| `internal/thinkt/list_options.go` | `WithSubagents()` / `ListSessionsConfig.IncludeSubagents` |
| `internal/sources/codex/classify.go` (new) | `RecordClass`, `recordHead`, `ClassifyLine`, orchestration set, harness tags, `IsSearchable`, `entryWeight` |
| `internal/sources/codex/unknown.go` (new) | `unknownTally` keyed by record key (copy of the Claude one) |
| `internal/sources/codex/convert.go` (new) | `convertRecord` for messages, events, compaction, unknown |
| `internal/sources/codex/tools.go` (new) | tool pairs from items and from wire calls |
| `internal/sources/codex/parser.go` (rewrite) | streaming parser: pending buffer, usage target, dedupe state, turn state, legacy pair rule |
| `internal/sources/codex/sessionstate.go` (new) | `sessionScan`, `scanSessionFile`, `loadSessionIndex`, `applyTo`, `cached` |
| `internal/sources/codex/store.go` | fast scan reads `session_meta` fully; titles merged; subagent filtering and counts; `enrichInto`; background enrichment |
| `internal/index/search/search.go` | per-source `searchable` registry |
| `internal/sources/codex/testdata/rollout-fixture.jsonl` (new) | synthetic rollout exercising every class-table row |
| `internal/sources/codex/README.md`, `AGENTS.md`, `etc/reports/SOURCE_FORMATS_2026-10.md` | docs |

`RecordClass` is declared again in the codex package with the same seven values as `claude.RecordClass`. Lifting it into `thinkt` would touch the Claude source and is out of scope; the builder keeps the names identical so a later lift is mechanical.

---

### Task 1: `thinkt` additions

**Files:**
- Modify: `internal/thinkt/types.go` (kind constants near line 158, `SessionCost` at line 240, `ParserVersion` at line 149)
- Modify: `internal/thinkt/metadata_cache.go` (`CachedSession`, `MergeInto`)
- Modify: `internal/thinkt/list_options.go`
- Test: `internal/thinkt/metadata_cache_test.go`, `internal/thinkt/list_options_test.go`

**Interfaces:**
- Produces: `const KindToolWire EntryKind = "tool_wire"`; `SessionCost.InputTokens, OutputTokens, CachedInputTokens int` (json `input_tokens`, `output_tokens`, `cached_input_tokens`, omitempty); `CachedSession.ParentSessionID string` (json `parent_session_id`, omitempty); `ListSessionsConfig.IncludeSubagents bool`; `func WithSubagents() ListSessionsOption`; `ParserVersion = 4`.

- [ ] **Step 1: Write the failing tests**

```go
func TestMergeIntoCopiesParentSessionID(t *testing.T) {
	mc := &MetadataCache{Sessions: map[string]CachedSession{}}
	mod := time.Unix(100, 0)
	mc.Set("/p/child.jsonl", CachedSession{ModifiedAt: mod, FileSize: 7, ParentSessionID: "parent-1"})
	meta := SessionMeta{FullPath: "/p/child.jsonl", ModifiedAt: mod, FileSize: 7}
	if !mc.MergeInto(&meta) {
		t.Fatal("expected cache hit")
	}
	if meta.ParentSessionID != "parent-1" {
		t.Fatalf("ParentSessionID = %q", meta.ParentSessionID)
	}
}

func TestWithSubagentsSetsIncludeSubagents(t *testing.T) {
	if ResolveListOptions(nil).IncludeSubagents {
		t.Fatal("default must exclude subagents")
	}
	if !ResolveListOptions([]ListSessionsOption{WithSubagents()}).IncludeSubagents {
		t.Fatal("WithSubagents must set IncludeSubagents")
	}
}

func TestParserVersionIsFour(t *testing.T) {
	if ParserVersion != 4 {
		t.Fatalf("ParserVersion = %d", ParserVersion)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/thinkt/ -run 'TestMergeIntoCopiesParentSessionID|TestWithSubagentsSetsIncludeSubagents|TestParserVersionIsFour' -v`
Expected: FAIL to compile on `ParentSessionID`, `WithSubagents`; `TestParserVersionIsFour` fails with `ParserVersion = 3`.

- [ ] **Step 3: Implement**

Add the constant with a comment ("the raw tool call the model emitted, superseded by a structured harness item; secondary"). Add the three `SessionCost` fields after `LinesRemoved`. Add `ParentSessionID` to `CachedSession` after `SubagentCount` and copy it in `MergeInto` when `meta.ParentSessionID == ""`. Add `IncludeSubagents` and `WithSubagents` with a doc comment stating the default hides sessions that have a `ParentSessionID`. Set `ParserVersion = 4` and extend its comment with one line: "4: Codex item stream, tool pairs, usage, subagent rollouts".

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/thinkt/ ./internal/sources/claude/`
Expected: PASS (Claude tests assert `ParserVersion >= 3`).

- [ ] **Step 5: Commit**

```bash
git add internal/thinkt
git commit -m "feat(thinkt): KindToolWire, session token totals, cached parent id, WithSubagents; ParserVersion 4"
```

---

### Task 2: Classifier, unknown tally, and fixture

**Files:**
- Create: `internal/sources/codex/classify.go`, `internal/sources/codex/unknown.go`, `internal/sources/codex/testdata/rollout-fixture.jsonl`
- Test: `internal/sources/codex/classify_test.go`

**Interfaces:**
- Produces:
  - `type RecordClass int` with `ClassTurn, ClassEvent, ClassContext, ClassCheckpoint, ClassSessionState, ClassBookkeeping, ClassUnknown`, `String()`, `IsEntry()` (same semantics as `claude.RecordClass`; `ClassCheckpoint` is declared for parity and never returned).
  - `type recordHead struct { Timestamp, Type, PayloadType, ItemType, ItemKind, Role, Name string; Kinds []string; LeadText string; HasSummary bool; Payload json.RawMessage }` where `Payload` is the raw `payload` object, `LeadText` is the trimmed text of the first `content` element (wire messages only), `HasSummary` is true when `payload.summary` (wire reasoning) or `item.summary_text` (Reasoning item) holds at least one element with non-empty text, or when a wire reasoning payload has a non-empty top-level `text` string (the pre-item form the legacy tests use).
  - `func (h recordHead) Key() string`: `"<type>"` for top-level records, `"event_msg/<payload.type>"`, `"response_item/<payload.type>"`, `"item/<item.type>"` for `event_msg/item_completed`.
  - `func ClassifyLine(line []byte) (recordHead, RecordClass)`; a non-object line is `ClassBookkeeping`.
  - `func isDedupeCandidate(h recordHead) bool`: true for `response_item/message` role assistant and `response_item/reasoning` with summary.
  - `var orchestrationTools = map[string]bool{"exec", "wait", "js", "apply_patch", "sleep", "spawn_agent", "send_message", "wait_agent", "interrupt_agent", "followup_task", "request_user_input_async"}`.
  - `var harnessTags = []string{"<environment_context>", "<turn_aborted>", "# AGENTS.md instructions", "<permissions instructions>", "<collaboration_mode>", "<user_shell_command>"}`; `func hasHarnessTag(text string) bool` (prefix match on trimmed text).
  - `func userMessageClass(h recordHead) (RecordClass, thinkt.EntryKind)`: kinds with a `user.` prefix, or no kinds and no harness tag → `ClassTurn, KindNone`; kinds containing `shell.user_command` → `ClassTurn, KindCommandOutput`; otherwise `ClassContext, KindMeta`.
  - `func entryWeight(h recordHead, class RecordClass) int`: entries one line yields when every dedupe candidate is dropped: 0 for non-entry classes and dedupe candidates, 2 for `item/CommandExecution`, `item/FileChange`, `item/McpToolCall`, `item/WebSearch`, `item/ImageView`, `item/Extension` kind `web.search` or `clock.sleep`, 1 otherwise.
  - `type unknownTally struct{ counts map[string]int }` with `note(key string, keyvals ...any)` (logs `"codex: unknown record type"` once per key via `tuilog.Log.Debug`) and `snapshot() map[string]int`.

Class table, by `Key()` (the spec's section A, restated as code decides it):

| key / condition | class |
|---|---|
| `response_item/message` role user | `userMessageClass` |
| `response_item/message` role developer | context |
| `response_item/message` role assistant | turn (dedupe candidate) |
| `item/UserMessage`, `item/ContextCompaction` | bookkeeping |
| `item/AgentMessage`, `item/Plan`, `item/CommandExecution`, `item/FileChange`, `item/McpToolCall`, `item/WebSearch`, `item/ImageView` | turn |
| `item/Reasoning` with summary / without | turn / bookkeeping |
| `response_item/reasoning` with summary / without | turn (dedupe candidate) / bookkeeping |
| `item/Extension` kind `web.search` or `clock.sleep` | turn |
| `item/Extension` other kind, `item/<anything else>` | unknown |
| `item/SubAgentActivity`, `item/CollabAgentToolCall` | event |
| `response_item/function_call`, `response_item/custom_tool_call`, `response_item/function_call_output`, `response_item/custom_tool_call_output` | turn when `Name` (for outputs: the name recorded for that `call_id`, see Task 4) is not in `orchestrationTools`, else event |
| `response_item/agent_message` | turn |
| `response_item/web_search_call`, `response_item/additional_tools` | bookkeeping |
| `event_msg/task_complete`, `event_msg/turn_aborted`, `compacted` | event |
| `event_msg/task_started`, `event_msg/token_count`, `event_msg/thread_settings_applied`, `world_state`, `inter_agent_communication_metadata` | bookkeeping |
| `token_usage_record` | event (merged by the parser, never an entry) |
| `turn_context`, `session_meta` | session state |
| `event_msg/user_message`, `event_msg/agent_message`, `event_msg/agent_reasoning` | turn (legacy) |
| anything else | unknown |

Outputs do not carry a tool name, so `ClassifyLine` returns `ClassTurn` for every output line; the parser (Task 4) reclassifies an output as event when it remembers the call as orchestration. `entryWeight` counts outputs as 1 either way.

- [ ] **Step 1: Write the fixture**

Write `testdata/rollout-fixture.jsonl` with exactly these 49 lines, in this order (session `thr-1`, turn `turn-1` then `turn-2`):

```jsonl
{"timestamp":"2026-10-01T10:00:00Z","type":"session_meta","payload":{"session_id":"thr-1","id":"thr-1","timestamp":"2026-10-01T10:00:00Z","cwd":"/work/proj","cli_version":"0.160.0","source":"cli","model_provider":"openai","git":{"branch":"main"}}}
{"timestamp":"2026-10-01T10:00:00Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<permissions instructions>\nsandbox"}]}}
{"timestamp":"2026-10-01T10:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n  <cwd>/work/proj</cwd>\n</environment_context>"}],"internal_chat_message_metadata_passthrough":{"turn_id":"turn-1","content_item_kinds":["environments.environment_context"]}}}
{"timestamp":"2026-10-01T10:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions for /work/proj\nbe nice"}]}}
{"timestamp":"2026-10-01T10:00:01Z","type":"turn_context","payload":{"turn_id":"turn-1","cwd":"/work/proj","model":"gpt-5.5","effort":"medium"}}
{"timestamp":"2026-10-01T10:00:01Z","type":"event_msg","payload":{"type":"task_started","turn_id":"turn-1"}}
{"timestamp":"2026-10-01T10:00:01Z","type":"response_item","payload":{"type":"message","id":"msg-u1","role":"user","content":[{"type":"input_text","text":"fix the build"}],"internal_chat_message_metadata_passthrough":{"turn_id":"turn-1","content_item_kinds":["user.text"]}}}
{"timestamp":"2026-10-01T10:00:01Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"UserMessage","id":"item-1","content":[{"type":"text","text":"fix the build"}]}}}
{"timestamp":"2026-10-01T10:00:02Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"Reasoning","id":"rs-1","summary_text":[],"raw_content":[]}}}
{"timestamp":"2026-10-01T10:00:02Z","type":"response_item","payload":{"type":"reasoning","id":"rs-1","summary":[],"content":null,"encrypted_content":"xxx"}}
{"timestamp":"2026-10-01T10:00:03Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"AgentMessage","id":"msg-a1","content":[{"type":"Text","text":"Checking the build."}],"phase":"commentary"}}}
{"timestamp":"2026-10-01T10:00:03Z","type":"response_item","payload":{"type":"message","id":"msg-a1","role":"assistant","content":[{"type":"output_text","text":"Checking the build."}],"phase":"commentary"}}
{"timestamp":"2026-10-01T10:00:03Z","type":"token_usage_record","payload":{"turn_id":"turn-1","usage":{"input_tokens":100,"cached_input_tokens":40,"cache_write_input_tokens":5,"output_tokens":20,"reasoning_output_tokens":8,"total_tokens":120}}}
{"timestamp":"2026-10-01T10:00:04Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"call-exec-1","name":"exec","input":"await run('go build ./...')"}}
{"timestamp":"2026-10-01T10:00:04Z","type":"token_usage_record","payload":{"turn_id":"turn-1","usage":{"input_tokens":200,"cached_input_tokens":0,"cache_write_input_tokens":0,"output_tokens":30,"reasoning_output_tokens":0,"total_tokens":230}}}
{"timestamp":"2026-10-01T10:00:05Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"CommandExecution","id":"exec-1","command":["/bin/zsh","-lc","go build ./..."],"cwd":"file:///work/proj","source":"unified_exec_startup","status":"failed","stdout":"","stderr":"undefined: foo","aggregated_output":"undefined: foo","exit_code":1,"duration":{"secs":1,"nanos":500000000}}}}
{"timestamp":"2026-10-01T10:00:05Z","type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"call-exec-1","output":"{\"output\":\"undefined: foo\"}"}}
{"timestamp":"2026-10-01T10:00:06Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"call-patch-1","name":"apply_patch","input":"*** Begin Patch\n*** Update File: main.go\n*** End Patch"}}
{"timestamp":"2026-10-01T10:00:06Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"FileChange","id":"call-patch-1","changes":{"/work/proj/main.go":{"type":"update","unified_diff":"@@ -1,2 +1,2 @@\n-foo()\n+bar()\n ok\n"},"/work/proj/new.go":{"type":"add","content":"package main\n"},"/work/proj/old.go":{"type":"delete","unified_diff":""}},"status":"completed","stdout":"Success."}}}
{"timestamp":"2026-10-01T10:00:06Z","type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"call-patch-1","output":"{\"output\":\"Success.\"}"}}
{"timestamp":"2026-10-01T10:00:07Z","type":"response_item","payload":{"type":"function_call","call_id":"call-mcp-1","name":"js","arguments":"{\"code\":\"1+1\"}"}}
{"timestamp":"2026-10-01T10:00:07Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"McpToolCall","id":"call-mcp-1","server":"cua_repl","tool":"js","arguments":{"code":"1+1"},"status":"completed","result":{"content":[{"type":"text","text":"2"}],"isError":false},"duration":{"secs":0,"nanos":1000000}}}}
{"timestamp":"2026-10-01T10:00:07Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-mcp-1","output":"2"}}
{"timestamp":"2026-10-01T10:00:08Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"Extension","kind":"web.search","id":"ws-1","query":"go build undefined","action":{"type":"search"},"results":[{"type":"text_result","title":"Go FAQ","url":"https://go.dev/doc/faq","snippet":"faq"}]}}}
{"timestamp":"2026-10-01T10:00:08Z","type":"response_item","payload":{"type":"web_search_call","status":"completed","action":{"type":"search","query":"go build undefined"}}}
{"timestamp":"2026-10-01T10:00:09Z","type":"response_item","payload":{"type":"function_call","call_id":"call-sleep-1","name":"sleep","arguments":"{\"duration_ms\":10}"}}
{"timestamp":"2026-10-01T10:00:09Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"Extension","kind":"clock.sleep","id":"call-sleep-1","durationMs":10}}}
{"timestamp":"2026-10-01T10:00:09Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-sleep-1","output":"slept"}}
{"timestamp":"2026-10-01T10:00:10Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"ImageView","id":"img-1","path":"file:///tmp/shot.png"}}}
{"timestamp":"2026-10-01T10:00:11Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"Plan","id":"turn-1-plan","text":"# Plan\n1. fix"}}}
{"timestamp":"2026-10-01T10:00:12Z","type":"response_item","payload":{"type":"function_call","call_id":"call-spawn-1","name":"spawn_agent","arguments":"{\"agent_path\":\"/root/helper\"}"}}
{"timestamp":"2026-10-01T10:00:12Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"SubAgentActivity","id":"call-spawn-1","kind":"started","agent_thread_id":"thr-child","agent_path":"/root/helper"}}}
{"timestamp":"2026-10-01T10:00:12Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-spawn-1","output":"spawned"}}
{"timestamp":"2026-10-01T10:00:13Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"CollabAgentToolCall","id":"call-wait-1","tool":"wait","status":"completed","sender_thread_id":"thr-1","receiver_thread_ids":["thr-child"]}}}
{"timestamp":"2026-10-01T10:00:13Z","type":"inter_agent_communication_metadata","payload":{"trigger_turn":false}}
{"timestamp":"2026-10-01T10:00:13Z","type":"response_item","payload":{"type":"agent_message","id":"amsg-1","author":"/root/helper","recipient":"/root","content":[{"type":"input_text","text":"done"}],"internal_chat_message_metadata_passthrough":{"turn_id":"turn-1"}}}
{"timestamp":"2026-10-01T10:00:14Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"AgentMessage","id":"msg-a2","content":[{"type":"Text","text":"Build fixed."}],"phase":"final_answer"}}}
{"timestamp":"2026-10-01T10:00:14Z","type":"response_item","payload":{"type":"message","id":"msg-a2","role":"assistant","content":[{"type":"output_text","text":"Build fixed."}],"phase":"final_answer"}}
{"timestamp":"2026-10-01T10:00:14Z","type":"token_usage_record","payload":{"turn_id":"turn-1","usage":{"input_tokens":300,"cached_input_tokens":100,"cache_write_input_tokens":0,"output_tokens":10,"reasoning_output_tokens":0,"total_tokens":310}}}
{"timestamp":"2026-10-01T10:00:15Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"turn-1","last_agent_message":"Build fixed.","duration_ms":14000,"time_to_first_token_ms":900}}
{"timestamp":"2026-10-01T10:00:15Z","type":"event_msg","payload":{"type":"token_count","info":null}}
{"timestamp":"2026-10-01T10:00:16Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-2","item":{"type":"ContextCompaction","id":"item-9"}}}
{"timestamp":"2026-10-01T10:00:16Z","type":"compacted","payload":{"message":"","replacement_history":[],"latest_token_usage_record":{"thread_token_usage":{"total_tokens":660}}}}
{"timestamp":"2026-10-01T10:00:17Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"!git status"}],"internal_chat_message_metadata_passthrough":{"turn_id":"turn-2","content_item_kinds":["shell.user_command"]}}}
{"timestamp":"2026-10-01T10:00:17Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-2","item":{"type":"CommandExecution","id":"exec-2","command":["/bin/zsh","-lc","git status"],"cwd":"file:///work/proj","source":"user_shell","status":"completed","stdout":"clean","stderr":"","aggregated_output":"clean","exit_code":0,"duration":{"secs":0,"nanos":2000000}}}}
{"timestamp":"2026-10-01T10:00:18Z","type":"event_msg","payload":{"type":"turn_aborted","turn_id":"turn-2","reason":"interrupted","duration_ms":500}}
{"timestamp":"2026-10-01T10:00:19Z","type":"world_state","payload":{"full":true}}
{"timestamp":"2026-10-01T10:00:19Z","type":"mystery_record","payload":{"x":1}}
{"timestamp":"2026-10-01T10:00:20Z","type":"response_item","payload":{"type":"additional_tools","id":"at-1","tools":[]}}
```

Expected entries from this fixture once Tasks 3–5 are done, in order (S = secondary):

| # | line | entry |
|---|---|---|
| 1 | developer | system, KindMeta, S |
| 2 | env context | user, KindMeta, S, Subtype `environments.environment_context` |
| 3 | AGENTS.md (no kinds) | user, KindMeta, S, Subtype `# AGENTS.md instructions` |
| 4 | "fix the build" | user, KindNone |
| 5 | AgentMessage msg-a1 | assistant "Checking the build.", Usage 100/20, Model gpt-5.5, Effort medium |
| 6 | wire exec call | assistant tool_use, KindToolWire, S, UUID `wire:call-exec-1`, Usage 200/30 |
| 7, 8 | CommandExecution exec-1 | assistant tool_use `unified_exec_startup` (UUID `exec-1`); user tool_result (UUID `exec-1:result`) text "undefined: foo", exit_code 1, duration_ms 1500, IsError true |
| 9 | wire exec output | user tool_result, KindToolWire, S, UUID `wire:call-exec-1:result` |
| 10 | wire apply_patch call | S, KindToolWire |
| 11, 12 | FileChange call-patch-1 | tool_use `apply_patch` (input `{"/work/proj/main.go":"update","/work/proj/new.go":"add","/work/proj/old.go":"delete"}`); tool_result text has lines `update /work/proj/main.go`, `add /work/proj/new.go`, `delete /work/proj/old.go`, exactly one `diff` child whose text starts `--- a//work/proj/main.go` |
| 13 | wire patch output | S |
| 14 | wire js call | S |
| 15, 16 | McpToolCall | tool_use `mcp:cua_repl/js` input `{"code":"1+1"}`; tool_result "2", IsError false |
| 17 | wire js output | S |
| 18, 19 | Extension web.search | tool_use `web_search` input `{"query":"go build undefined"}`; tool_result text contains "Go FAQ" and the URL |
| 20 | wire sleep call | S |
| 21, 22 | Extension clock.sleep | tool_use `sleep` input `{"durationMs":10}`; tool_result "slept 10 ms" |
| 23 | wire sleep output | S |
| 24, 25 | ImageView | tool_use `view_image` input `{"path":"/tmp/shot.png"}`; tool_result "/tmp/shot.png" |
| 26 | Plan | assistant, Subtype `plan`, text "# Plan\n1. fix" |
| 27 | wire spawn_agent call | S |
| 28 | SubAgentActivity | system, KindAgentMessage, Subtype `started`, ChildSessionID `thr-child`, text "started /root/helper", UUID `call-spawn-1:started` |
| 29 | wire spawn output | S |
| 30 | CollabAgentToolCall | system, KindAgentMessage, S, UUID `call-wait-1:collab` |
| 31 | agent_message | user, KindAgentMessage, text "done", Metadata author `/root/helper`, recipient `/root` |
| 32 | AgentMessage msg-a2 | assistant "Build fixed.", Usage 300/10 |
| 33 | task_complete | system, KindTurnDuration, S, duration_ms 14000, time_to_first_token_ms 900 |
| 34 | compacted | system, KindCompactBoundary, text "Context compacted", Compaction.PreTokens 660, Trigger `auto` |
| 35 | `!git status` | user, KindCommandOutput, S |
| 36, 37 | CommandExecution exec-2 | tool_use `user_shell`; tool_result "clean", exit_code 0 |
| 38 | turn_aborted | system, KindTurnDuration, S, Error "interrupted" |
| 39 | mystery_record | system, KindUnknown, S, Text = raw line |

39 entries, 18 secondary, 21 visible. The wire message, reasoning, UserMessage, ContextCompaction, web_search_call, additional_tools, task_started, token_count, world_state, inter_agent_communication_metadata, turn_context, and session_meta lines produce nothing. `SubAgentActivity` and `CollabAgentToolCall` get UUID suffixes because their ids are reused wire call ids that `wire:` already namespaces on the wire side but that other item kinds (`McpToolCall`, `FileChange`) use bare.

- [ ] **Step 2: Write the failing classifier tests**

```go
func TestClassifyLineTable(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		key   string
		class RecordClass
	}{
		{"user text", `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["user.text"]}}}`, "response_item/message", ClassTurn},
		{"user env context by kind", `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"x"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["environments.environment_context"]}}}`, "response_item/message", ClassContext},
		{"user env context by tag", `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n</environment_context>"}]}}`, "response_item/message", ClassContext},
		{"user no kinds plain", `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"plain prompt"}]}}`, "response_item/message", ClassTurn},
		{"developer", `{"type":"response_item","payload":{"type":"message","role":"developer","content":[]}}`, "response_item/message", ClassContext},
		{"assistant wire", `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"a"}]}}`, "response_item/message", ClassTurn},
		{"item user message", `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"i1"}}}`, "item/UserMessage", ClassBookkeeping},
		{"item reasoning empty", `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"Reasoning","id":"r1","summary_text":[]}}}`, "item/Reasoning", ClassBookkeeping},
		{"item reasoning text", `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"Reasoning","id":"r1","summary_text":[{"type":"summary_text","text":"why"}]}}}`, "item/Reasoning", ClassTurn},
		{"wire reasoning encrypted", `{"type":"response_item","payload":{"type":"reasoning","summary":[],"encrypted_content":"x"}}`, "response_item/reasoning", ClassBookkeeping},
		{"extension sleep", `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"Extension","kind":"clock.sleep","id":"c1","durationMs":5}}}`, "item/Extension", ClassTurn},
		{"extension other", `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"Extension","kind":"future.thing","id":"c2"}}}`, "item/Extension", ClassUnknown},
		{"subagent activity", `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"SubAgentActivity","id":"s1","kind":"started"}}}`, "item/SubAgentActivity", ClassEvent},
		{"wire exec call", `{"type":"response_item","payload":{"type":"custom_tool_call","call_id":"c","name":"exec","input":"x"}}`, "response_item/custom_tool_call", ClassEvent},
		{"wire exec_command call", `{"type":"response_item","payload":{"type":"function_call","call_id":"c","name":"exec_command","arguments":"{}"}}`, "response_item/function_call", ClassTurn},
		{"task complete", `{"type":"event_msg","payload":{"type":"task_complete","turn_id":"t"}}`, "event_msg/task_complete", ClassEvent},
		{"task started", `{"type":"event_msg","payload":{"type":"task_started","turn_id":"t"}}`, "event_msg/task_started", ClassBookkeeping},
		{"compacted", `{"type":"compacted","payload":{}}`, "compacted", ClassEvent},
		{"usage", `{"type":"token_usage_record","payload":{}}`, "token_usage_record", ClassEvent},
		{"turn context", `{"type":"turn_context","payload":{"turn_id":"t"}}`, "turn_context", ClassSessionState},
		{"session meta", `{"type":"session_meta","payload":{"id":"s"}}`, "session_meta", ClassSessionState},
		{"legacy user", `{"type":"event_msg","payload":{"type":"user_message","message":"hi"}}`, "event_msg/user_message", ClassTurn},
		{"unknown top", `{"type":"mystery","payload":{}}`, "mystery", ClassUnknown},
		{"not json", `nope`, "", ClassBookkeeping},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			head, class := ClassifyLine([]byte(tc.line))
			if head.Key() != tc.key || class != tc.class {
				t.Fatalf("got (%q, %s), want (%q, %s)", head.Key(), class, tc.key, tc.class)
			}
		})
	}
}

func TestEntryWeightCountsFixture(t *testing.T) {
	// Sum of entryWeight over testdata/rollout-fixture.jsonl must be 39.
}

func TestUnknownTallyLogsOnce(t *testing.T) {
	var u unknownTally
	u.note("mystery", "line", 1)
	u.note("mystery", "line", 2)
	if got := u.snapshot(); got["mystery"] != 2 {
		t.Fatalf("snapshot = %v", got)
	}
}
```

Fill in `TestEntryWeightCountsFixture` by reading the fixture line by line with `ClassifyLine` and summing `entryWeight`; assert 39.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/sources/codex/ -run 'TestClassifyLineTable|TestEntryWeightCountsFixture|TestUnknownTallyLogsOnce' -v`
Expected: FAIL to compile (`ClassifyLine`, `RecordClass`, `unknownTally` undefined).

- [ ] **Step 4: Implement `classify.go` and `unknown.go`**

`ClassifyLine` unmarshals one head struct: `timestamp`, `type`, and `payload` as `json.RawMessage`, then unmarshals the payload into a second struct with `type`, `role`, `name`, `content json.RawMessage`, `summary json.RawMessage`, `item struct{ Type, Kind string; SummaryText json.RawMessage }`, `internal_chat_message_metadata_passthrough struct{ ContentItemKinds []string }`. `LeadText` is decoded only for `response_item/message` with role user and no kinds: unmarshal `content` into `[]struct{ Text string }` and take the first non-empty text, trimmed. `HasSummary` decodes `summary` / `summary_text` into `[]struct{ Text string }` and reports any non-empty text. Keep `Payload` so the converter does not re-split the line.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/sources/codex/ -run 'TestClassifyLineTable|TestEntryWeightCountsFixture|TestUnknownTallyLogsOnce' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/sources/codex/classify.go internal/sources/codex/unknown.go internal/sources/codex/classify_test.go internal/sources/codex/testdata/rollout-fixture.jsonl
git commit -m "feat(codex): classify rollout lines by record key; unknown tally; fixture rollout"
```

---

### Task 3: Converter for messages and events; parser rewrite

**Files:**
- Create: `internal/sources/codex/convert.go`
- Rewrite: `internal/sources/codex/parser.go` (keep `NewParser`, `composeUUID`, `parseTimestamp`, `readString`, `extractMessageText`, `extractReasoningText`, `parseToolInput`, `normalizeToolOutput`, `mapMessageRole`)
- Test: `internal/sources/codex/convert_test.go`, `internal/sources/codex/parser_test.go`

**Interfaces:**
- Consumes: `ClassifyLine`, `recordHead`, `RecordClass`, `isDedupeCandidate`, `userMessageClass`, `unknownTally` (Task 2); `thinkt.KindToolWire` (Task 1).
- Produces:
  - `type turnState struct { TurnID, Model, Effort string }`; `func (t *turnState) apply(h recordHead)` reads `turn_context` (`turn_id`, `model`, `effort`) and `task_started` (`turn_id`).
  - `type converter struct { sessionID string; lineNo int; turn *turnState; calls map[string]string }` where `calls` remembers wire `call_id → name` (used in Task 4).
  - `func (c *converter) convert(h recordHead, class RecordClass, line []byte) []thinkt.Entry` — returns the entries for one line; nil for non-entry classes, dedupe candidates (the parser decides those), and `token_usage_record`.
  - `func (c *converter) newEntry(role thinkt.Role, h recordHead, uuid string) thinkt.Entry` sets `UUID` (given, else `composeUUID(sessionID, lineNo, h.Key(), "")`), `Role`, `Timestamp`, `Source: thinkt.SourceCodex`, `TurnID` (payload `turn_id`, else passthrough `turn_id`, else `turn.TurnID`), `Model: turn.Model`, `Effort: turn.Effort`, `Metadata: map[string]any{}`.
  - Parser: `type Parser struct { scanner; sessionID; lineNo; conv converter; out []thinkt.Entry; pending []thinkt.Entry; usageTarget int; lastAgentText, lastReasoningText string; legacy *thinkt.Entry; unknown unknownTally; errors []error }`; `NextEntry() (*thinkt.Entry, error)` unchanged signature (returns `io.EOF` at end); `UnknownTypes() map[string]int`.

Converter rows for this task (tool rows are Task 4):

| key | entry |
|---|---|
| `response_item/message` user, `userMessageClass` → turn/KindNone | user; `Text` = joined `input_text` parts; each `input_image` with `image_url` starting `data:` becomes `ContentBlocks` `{Type:"image", MediaType: <between "data:" and ";">, MediaData: <after "base64,">}`; any other `image_url` becomes `{Type:"text", Text: url}`; when blocks exist, also add a leading text block when `Text` is non-empty |
| … → turn/KindCommandOutput | user, `Kind: KindCommandOutput`, secondary, `Subtype: "shell.user_command"` |
| … → context/KindMeta | user, `Kind: KindMeta`, secondary, `Subtype` = first kind, else the matched harness tag |
| `response_item/message` developer | system, `KindMeta`, secondary, `Subtype` = first kind or `"developer"` |
| `response_item/message` assistant | assistant, `Text`; `Metadata.phase` when present; UUID = payload `id` when present (the parser drops it when it twins an item) |
| `item/AgentMessage` | assistant; `Text` = joined `content[].text`; UUID = item id; `Metadata.phase`; `Metadata.questions` = raw `questions` when present |
| `item/Reasoning` with summary | assistant; `ContentBlocks` = one thinking block of the joined `summary_text[].text`; UUID = item id |
| `response_item/reasoning` with summary | same shape (parser decides) |
| `item/Plan` | assistant, `Subtype: "plan"`, `Text` = item `text`, UUID = item id |
| `item/SubAgentActivity` | system, `Kind: KindAgentMessage`, `Subtype` = `kind`, `ChildSessionID` = `agent_thread_id`, `Text` = `"<kind> <agent_path>"`, secondary when `kind == "interacted"`, UUID `<id>:<kind>` |
| `item/CollabAgentToolCall` | system, `KindAgentMessage`, secondary, `Subtype` = `tool`, `Text` = `"<tool> <receiver_thread_ids joined by space>"` (or `tool` alone), UUID `<id>:collab` |
| `response_item/agent_message` | user, `KindAgentMessage`, `Text` = joined `input_text` parts, `Metadata.author`, `Metadata.recipient`, UUID = payload `id` |
| `event_msg/task_complete` | system, `KindTurnDuration`, secondary, `Text` = `"turn complete"`; `Metadata.duration_ms`, `Metadata.time_to_first_token_ms`; `Error` = `error.message` when present |
| `event_msg/turn_aborted` | system, `KindTurnDuration`, secondary, `Text` = `"turn aborted"`; `Metadata.duration_ms`; `Error` = `reason` |
| `compacted` | system, `KindCompactBoundary`, `Text` = `message` or `"Context compacted"`, `Compaction = &CompactionInfo{Trigger:"auto", PreTokens: latest_token_usage_record.thread_token_usage.total_tokens}` (0 when null) |
| legacy `event_msg/user_message` / `agent_message` / `agent_reasoning` | as today: user text, assistant text, assistant thinking block; UUID `composeUUID` |
| `ClassUnknown` | system, `KindUnknown`, secondary, `Text` = raw line |

Parser algorithm (the hold, the two dedupe rules, and usage):

```
NextEntry:
  loop:
    if len(out) > 0: pop front, return it
    read next non-empty line; on EOF: flush pending and legacy into out, then return io.EOF when out is empty
    head, class := ClassifyLine(line); lineNo++
    turn.apply(head) on session-state/task_started lines
    if class == ClassUnknown: unknown.note(head.Key(), "line", lineNo)
    if head.Key() == "token_usage_record":
        if usageTarget >= 0: pending[usageTarget].Usage = usage(payload)   # last write wins
        continue
    if isDedupeCandidate(head):
        text := message or summary text of the line
        if (assistant message and text == lastAgentText) or (reasoning and text == lastReasoningText): continue
        # otherwise fall through and convert it like any turn line
    entries := conv.convert(head, class, line)
    if len(entries) == 0: continue                      # bookkeeping, session state: hold stays
    if legacy != nil:                                    # legacy pair rule, as today
        if isWireTwin(legacy, head, entries[0]): legacy = nil            # wire wins
        else: out = append(out, *legacy); legacy = nil
    if head is a legacy event_msg user/agent/reasoning:
        out = append(out, pending...); pending = nil; usageTarget = -1   # file order: pending precedes the event
        legacy = &entries[0]; continue
    out = append(out, pending...); pending = entries
    usageTarget = index of the last assistant-role entry in pending, or -1
    if head.Key() == "item/AgentMessage": lastAgentText = entries[0].Text
    if head.Key() == "item/Reasoning": lastReasoningText = thinking text
```

Text comparison for dedupe uses `strings.TrimSpace` on both sides. `isWireTwin` is the existing `isDuplicateEventResponsePair` logic (same role, equal trimmed text, event kind `user_message`/`agent_message` ↔ wire `message`, `agent_reasoning` ↔ wire `reasoning`).

- [ ] **Step 1: Update the existing parser tests**

In `parser_test.go`, change `TestParser_NextEntry` so `e4.Role` is `thinkt.RoleUser` (tool results are user-role entries, matching Claude; see Rulings). Keep the two legacy dedupe tests unchanged.

- [ ] **Step 2: Write the failing converter and parser tests**

```go
func TestConvertUserMessageWithDataImage(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"look"},{"type":"input_image","image_url":"data:image/png;base64,AAAA"},{"type":"input_image","image_url":"https://x/y.png"}],"internal_chat_message_metadata_passthrough":{"turn_id":"t1","content_item_kinds":["user.text","user.image"]}}}`
	entries := convertLine(t, line)
	if len(entries) != 1 || entries[0].Role != thinkt.RoleUser || entries[0].TurnID != "t1" {
		t.Fatalf("entries = %+v", entries)
	}
	blocks := entries[0].ContentBlocks
	if len(blocks) != 3 || blocks[0].Type != "text" || blocks[1].Type != "image" || blocks[1].MediaType != "image/png" || blocks[1].MediaData != "AAAA" || blocks[2].Type != "text" || blocks[2].Text != "https://x/y.png" {
		t.Fatalf("blocks = %+v", blocks)
	}
}

func TestConvertDeveloperMessageIsSecondaryMeta(t *testing.T) { /* Kind KindMeta, Visibility secondary, Role system */ }

func TestConvertTaskCompleteWithError(t *testing.T) {
	line := `{"timestamp":"2026-10-01T10:00:00Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"t1","error":{"message":"flagged"},"duration_ms":10,"time_to_first_token_ms":3}}`
	e := convertLine(t, line)[0]
	if e.Kind != thinkt.KindTurnDuration || !e.IsSecondary() || e.Error != "flagged" || e.Metadata["duration_ms"] != int64(10) {
		t.Fatalf("entry = %+v", e)
	}
}

func TestConvertCompactedPreTokens(t *testing.T) { /* Compaction.PreTokens == 660, Kind KindCompactBoundary, Text "Context compacted" */ }

func TestConvertSubAgentActivity(t *testing.T) { /* ChildSessionID "thr-child", Subtype "started", UUID "call-spawn-1:started", Text "started /root/helper" */ }

func TestConvertUnknownKeepsRawLine(t *testing.T) { /* Kind KindUnknown, secondary, Text == line */ }

func TestParserDropsWireAssistantTwin(t *testing.T) {
	// AgentMessage item "Build fixed." then wire message "Build fixed." → one assistant entry, UUID "msg-a2".
}

func TestParserDropsWireAssistantTwinIgnoringWhitespace(t *testing.T) {
	// wire text "Build fixed.\n" → still one entry.
}

func TestParserKeepsWireOnlyAssistantReply(t *testing.T) {
	// wire assistant message with no preceding item → one assistant entry with that text.
}

func TestParserStampsTurnModelAndEffort(t *testing.T) {
	// turn_context {model gpt-5.5, effort medium, turn_id t1} then AgentMessage item without turn_id → Model, Effort, TurnID stamped.
}

func TestParserEncryptedReasoningProducesNothing(t *testing.T) {
	// Reasoning item with empty summary_text + wire reasoning with empty summary → zero entries.
}
```

`convertLine(t, line)` is a test helper: `ClassifyLine` then `(&converter{sessionID: "s", turn: &turnState{}, calls: map[string]string{}}).convert(...)`.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/sources/codex/ -run 'TestConvert|TestParserDrops|TestParserKeeps|TestParserStamps|TestParserEncrypted' -v`
Expected: FAIL to compile (`converter` undefined).

- [ ] **Step 4: Implement `convert.go` and rewrite `parser.go`**

Follow the row table and the algorithm block. Delete `convertEventMsg`, `convertResponseItem`, `pendingEvent`, `queued`, `parsedEntry` from the old parser; keep the helpers listed under Files. Wire tool calls and tool items return nil from `convert` until Task 4 (so the fixture is not yet complete).

- [ ] **Step 5: Run the package tests**

Run: `go test ./internal/sources/codex/`
Expected: PASS, including the three pre-existing parser tests and `TestStore_LoadSession` (3 entries: user, tool_use, tool_result).

- [ ] **Step 6: Commit**

```bash
git add internal/sources/codex
git commit -m "feat(codex): convert wire and item messages and events; streaming parser with hold, dedupe, and turn state"
```

---

### Task 4: Tool pairs from items and wire calls

**Files:**
- Create: `internal/sources/codex/tools.go`
- Modify: `internal/sources/codex/convert.go` (dispatch tool keys)
- Test: `internal/sources/codex/tools_test.go`

**Interfaces:**
- Consumes: `converter`, `newEntry`, `orchestrationTools`, `thinkt.BlockTypeDiff`.
- Produces: `func (c *converter) toolPair(h recordHead, uuid string, call thinkt.ContentBlock, result thinkt.ContentBlock, visibility thinkt.Visibility, kind thinkt.EntryKind) []thinkt.Entry` returning `[assistant tool_use entry (UUID uuid), user tool_result entry (UUID uuid+":result")]` with `ToolUseID` set on both blocks; `func durationMs(d struct{ Secs int64; Nanos int64 }) int64` = `Secs*1000 + Nanos/1e6`; `func stripFileURI(s string) string` removes a leading `file://`; `func parseExitCode(output string) (int, bool)` matching `Process exited with code (\d+)`; `func wrapDiff(path, body string) string` = `"--- a/" + path + "\n+++ b/" + path + "\n" + body`; `func diffLineCounts(diff string) (added, removed int)` counting `+`/`-` lines that are not `+++`/`---`.

Rows:

| key | tool_use | tool_result |
|---|---|---|
| `item/CommandExecution` | `ToolName` = item `source`; `ToolInput` = `{"command": <joined command[] by space>, "cwd": stripFileURI(cwd)}`; UUID = id | `ToolResult` = `aggregated_output` (else `stdout`); `Children` = one text child with `stderr` when non-empty and different from `aggregated_output`; `IsError` = `exit_code != 0`; `Metadata.exit_code` (int), `Metadata.duration_ms` (int64), `Metadata.status` |
| `item/FileChange` | `ToolName: "apply_patch"`; `ToolInput` = map path → type, plus `"<path> -> <move_path>"` as the value when `move_path` set; UUID = id | `ToolResult` = one line per change `"<type> <path>"` joined by `\n`; one `BlockTypeDiff` child per change with non-empty `unified_diff`, text `wrapDiff(path, unified_diff)`; `IsError` = `status == "failed"`; `Metadata.status` |
| `item/McpToolCall` | `ToolName` = `"mcp:" + server + "/" + tool`; `ToolInput` = `arguments`; UUID = id | `ToolResult` = joined `result.content[].text`; non-text content elements become `Children` with `Type` = their type and `Text` = raw JSON; `IsError` = `result.isError`; `Metadata.duration_ms` |
| `item/WebSearch`, `item/Extension` kind `web.search` | `ToolName: "web_search"`; `ToolInput` = `{"query": query}` plus `"url"` from `action.url` when set; UUID = id | `ToolResult` = one line per result `"<title> <url>"` (empty string when none → use `action.url` or the query so the block is never empty) |
| `item/Extension` kind `clock.sleep` | `ToolName: "sleep"`; `ToolInput` = `{"durationMs": n}`; UUID = id | `ToolResult` = `"slept <n> ms"` |
| `item/ImageView` | `ToolName: "view_image"`; `ToolInput` = `{"path": stripFileURI(path)}`; UUID = id | `ToolResult` = the stripped path |
| `response_item/function_call`, `custom_tool_call` | `ToolName` = name; `ToolInput` = `parseToolInput(payload)`; UUID `"wire:" + call_id`; `ToolUseID` `"wire:" + call_id`; remember `calls[call_id] = name` | (single entry: only the call) |
| `response_item/function_call_output`, `custom_tool_call_output` | (single entry) | `ToolResult` = `normalizeToolOutput(output)`; UUID `"wire:" + call_id + ":result"`; `ToolUseID` `"wire:" + call_id`; `Metadata.exit_code` when `parseExitCode` matches |

Wire call and output entries get `Kind: KindToolWire` and `VisibilitySecondary` when `calls[call_id]` (or the call's own name) is in `orchestrationTools`; otherwise `KindNone`, normal visibility. Wire tool entries are single entries, not pairs, because the call and its output are separate lines.

- [ ] **Step 1: Write the failing tests**

```go
func TestCommandExecutionPair(t *testing.T) {
	// fixture line 16 → 2 entries: tool_use ToolName "unified_exec_startup", ToolInput cwd "/work/proj", UUID "exec-1";
	// tool_result UUID "exec-1:result", ToolResult "undefined: foo", IsError true, Metadata exit_code 1, duration_ms int64(1500); both ToolUseID "exec-1".
}

func TestFileChangeDiffChildrenOnlyForNonEmptyDiffs(t *testing.T) {
	// fixture line 19 → tool_result has exactly 1 child of Type thinkt.BlockTypeDiff whose Text has prefix "--- a//work/proj/main.go\n+++ b//work/proj/main.go\n@@";
	// ToolResult contains "add /work/proj/new.go" and "delete /work/proj/old.go"; ToolInput["/work/proj/new.go"] == "add".
}

func TestFileChangeMoveWithoutDiff(t *testing.T) {
	// changes {"/a.go": {"type":"update","unified_diff":"","move_path":"/b.go"}} → no children, ToolInput["/a.go"] == "update -> /b.go".
}

func TestMcpToolCallPair(t *testing.T) { /* ToolName "mcp:cua_repl/js", ToolResult "2", IsError false */ }

func TestWebSearchExtensionPair(t *testing.T) { /* ToolName "web_search", ToolInput query, ToolResult contains "Go FAQ https://go.dev/doc/faq" */ }

func TestClockSleepPair(t *testing.T) { /* ToolName "sleep", ToolResult "slept 10 ms" */ }

func TestImageViewPair(t *testing.T) { /* ToolInput path "/tmp/shot.png", ToolResult "/tmp/shot.png", no image block */ }

func TestWireOrchestrationCallIsSecondaryToolWire(t *testing.T) {
	// exec call → 1 entry, Kind KindToolWire, secondary, UUID "wire:call-exec-1", block ToolUseID "wire:call-exec-1".
	// its output → Kind KindToolWire, secondary, UUID "wire:call-exec-1:result".
}

func TestWireExecCommandStaysVisibleWithExitCode(t *testing.T) {
	// function_call exec_command + output "Chunk ID: x\nWall time: 0.1 seconds\nProcess exited with code 2\n..." → both normal visibility, output Metadata exit_code == 2.
}

func TestDurationMs(t *testing.T) { /* {1, 500000000} → 1500; {0, 2000000} → 2 */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sources/codex/ -run 'Pair|TestFileChange|TestWire|TestDurationMs' -v`
Expected: FAIL (`toolPair` undefined / zero entries).

- [ ] **Step 3: Implement `tools.go` and dispatch from `convert`**

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/sources/codex/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/codex
git commit -m "feat(codex): tool pairs from CommandExecution, FileChange, MCP, web search, sleep, image items; wire calls as KindToolWire"
```

---

### Task 5: Usage attachment and the fixture end to end

**Files:**
- Modify: `internal/sources/codex/parser.go` (usage decode), `internal/sources/codex/convert.go`
- Test: `internal/sources/codex/parser_test.go`

**Interfaces:**
- Produces: `func decodeUsage(payload json.RawMessage) *thinkt.TokenUsage` mapping `input_tokens → InputTokens`, `cached_input_tokens → CacheReadInputTokens`, `cache_write_input_tokens → CacheCreationInputTokens`, `output_tokens → OutputTokens`, `reasoning_output_tokens → ThinkingTokens`; nil when `usage` is absent.
- `func readFixture(t *testing.T) []thinkt.Entry` test helper parsing `testdata/rollout-fixture.jsonl` to EOF.

- [ ] **Step 1: Write the failing tests**

```go
func TestParserAttachesUsageToHeldAssistantEntry(t *testing.T) {
	entries := readFixture(t)
	byUUID := map[string]thinkt.Entry{}
	for _, e := range entries { byUUID[e.UUID] = e }
	a1 := byUUID["msg-a1"]
	if a1.Usage == nil || a1.Usage.InputTokens != 100 || a1.Usage.OutputTokens != 20 || a1.Usage.CacheReadInputTokens != 40 || a1.Usage.CacheCreationInputTokens != 5 || a1.Usage.ThinkingTokens != 8 {
		t.Fatalf("msg-a1 usage = %+v", a1.Usage)
	}
	if w := byUUID["wire:call-exec-1"]; w.Usage == nil || w.Usage.InputTokens != 200 {
		t.Fatalf("wire exec usage = %+v", w.Usage)
	}
	if a2 := byUUID["msg-a2"]; a2.Usage == nil || a2.Usage.InputTokens != 300 {
		t.Fatalf("msg-a2 usage = %+v", a2.Usage)
	}
}

func TestParserUsageAfterUserEntryIsNotAttached(t *testing.T) {
	// wire exec_command call, its output, then a usage record → no entry has Usage; no panic.
}

func TestParserDoubleUsageLastWriteWins(t *testing.T) {
	// AgentMessage item, usage 100, usage 7 → Usage.InputTokens == 7.
}

func TestParserFixtureEntryCounts(t *testing.T) {
	entries := readFixture(t)
	if len(entries) != 39 { t.Fatalf("entries = %d", len(entries)) }
	secondary := 0
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsSecondary() { secondary++ }
		if e.Text == "" && len(e.ContentBlocks) == 0 { t.Fatalf("empty entry %s", e.UUID) }
		if seen[e.UUID] { t.Fatalf("duplicate UUID %s", e.UUID) }
		seen[e.UUID] = true
	}
	if secondary != 18 { t.Fatalf("secondary = %d", secondary) }
}

func TestParserFixtureOrderAndKinds(t *testing.T) {
	// Assert the 39-row table from Task 2 Step 1: for each index, Role, Kind, Visibility, UUID, and ToolName where given.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sources/codex/ -run 'Usage|Fixture' -v`
Expected: `TestParserAttachesUsageToHeldAssistantEntry` fails on nil Usage; count tests fail if any row is still unimplemented.

- [ ] **Step 3: Implement `decodeUsage` and wire it into the parser loop**

- [ ] **Step 4: Run the package tests with the race detector**

Run: `go test -race ./internal/sources/codex/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/codex
git commit -m "feat(codex): attach token usage to the held assistant entry; fixture rollout parses end to end"
```

---

### Task 6: `IsSearchable` and the per-source search registry

**Files:**
- Modify: `internal/sources/codex/classify.go` (add `IsSearchable`)
- Modify: `internal/index/search/search.go` (`searchable` registry; `scanFile`)
- Test: `internal/sources/codex/classify_test.go`, `internal/index/search/bookkeeping_test.go`

**Interfaces:**
- Produces: `func IsSearchable(line []byte) bool` in `codex`: false for non-entry classes and dedupe candidates; otherwise true when `convert` with a fresh `converter` yields at least one entry that is not secondary. A wire tool output line cannot name its tool on its own, so outputs are searchable; a hit in an `exec` output may also hit the `CommandExecution` item that duplicates it, bounded by the per-session match limit. `var searchable = map[string]func([]byte) bool{string(thinkt.SourceClaude): claude.IsSearchable, string(thinkt.SourceCodex): codex.IsSearchable}` in `search`; `scanFile` does `if fn := searchable[c.Source]; fn != nil && entry.Type != "" && !fn([]byte(text)) { continue }`.

- [ ] **Step 1: Write the failing tests**

```go
func TestIsSearchableCodex(t *testing.T) {
	cases := map[string]bool{
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"a","content":[{"type":"Text","text":"needle"}]}}}`:                       true,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"needle"}]}}`:                                        false,
		`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"needle"}]}}`:                                         false,
		`{"type":"response_item","payload":{"type":"custom_tool_call","call_id":"c","name":"exec","input":"needle"}}`:                                                          false,
		`{"type":"response_item","payload":{"type":"function_call","call_id":"c","name":"exec_command","arguments":"{\"cmd\":\"needle\"}"}}`:                                   true,
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"e","command":["needle"],"source":"unified_exec_startup","exit_code":0,"duration":{"secs":0,"nanos":0}}}}`: true,
		`{"type":"token_usage_record","payload":{"usage":{"input_tokens":1}}}`:                                                                                                  false,
	}
	for line, want := range cases {
		if got := IsSearchable([]byte(line)); got != want {
			t.Errorf("IsSearchable(%s) = %v, want %v", line, got, want)
		}
	}
}
```

In `internal/index/search/bookkeeping_test.go` add `TestScanFileCodexSkipsWireAssistantTwin`: a candidate with `Source: "codex"` whose file holds an `AgentMessage` item line and its wire twin, both containing the query → exactly one `RawMatch`, at the item's line number. Check `TestScanFileKeepsOtherSourcesWithTypedLines`: if its codex line is a wire assistant message, change it to an `AgentMessage` item line so the test still asserts that typed lines from other sources are kept.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sources/codex/ ./internal/index/search/ -run 'TestIsSearchableCodex|TestScanFileCodex' -v`
Expected: FAIL (`IsSearchable` undefined; search test finds two matches).

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run the two packages**

Run: `go test ./internal/sources/codex/ ./internal/index/search/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/codex/classify.go internal/sources/codex/classify_test.go internal/index/search
git commit -m "feat(search): per-source searchable registry; codex skips wire twins and secondary lines"
```

---

### Task 7: Session scan, titles, subagent listing, counts, cache

**Files:**
- Create: `internal/sources/codex/sessionstate.go`
- Modify: `internal/sources/codex/store.go`
- Test: `internal/sources/codex/sessionstate_test.go`, `internal/sources/codex/store_test.go`

**Interfaces:**
- Consumes: `ClassifyLine`, `entryWeight`, `decodeUsage`, `diffLineCounts`, `wrapDiff`; `thinkt.WithSubagents`, `CachedSession.ParentSessionID`, `SessionCost` token fields (Task 1).
- Produces:
  - `type sessionMetaRecord struct { ID, SessionID, Timestamp, CWD, Model, ModelProvider, CLIVersion string; Git struct{ Branch string }; Source json.RawMessage; AgentNickname, AgentPath string }` with `func (r sessionMetaRecord) parentThreadID() string` decoding `source.subagent.thread_spawn.parent_thread_id` when `Source` is an object.
  - `type sessionScan struct { FirstPrompt, LastPrompt, Model, Title string; EntryCount int; Cost *thinkt.SessionCost; ParentSessionID string }`.
  - `func scanSessionFile(path string) sessionScan`: one pass; `FirstPrompt`/`LastPrompt` from `response_item/message` user lines whose `userMessageClass` is turn/KindNone (joined `input_text`); `Model` = last `turn_context.model` that `thinkt.IsRealModel`, else `thread_settings_applied.thread_settings.model`, else `session_meta.model_provider`; `EntryCount` = Σ `entryWeight`; `Cost.DurationMs` = Σ `task_complete.duration_ms`; `Cost.LinesAdded/LinesRemoved` from `diffLineCounts` over every non-empty `FileChange.unified_diff`; `Cost.InputTokens/OutputTokens/CachedInputTokens` = Σ `usage.input_tokens/output_tokens/cached_input_tokens` over every `token_usage_record`; `Title` fallback for subagents = `agent_nickname + " " + agent_path` (trimmed); `Cost` is nil when every field is zero.
  - `func (s sessionScan) applyTo(meta *thinkt.SessionMeta)` fills empty fields (same shape as Claude's; `EntryCount` always overwritten).
  - `func (s sessionScan) cached(meta thinkt.SessionMeta) thinkt.CachedSession` including `ParentSessionID`.
  - `func loadSessionIndex(baseDir string) map[string]string`: reads `<baseDir>/session_index.jsonl`, `{id, thread_name}`, last non-empty `thread_name` per id wins, bad lines skipped, missing file → empty map.
  - Store: `readSessionMetaFast` reads the whole `session_meta` payload into `sessionMetaRecord` (`ID` → `meta.ID`, `parentThreadID()` → `meta.ParentSessionID`, `CWD`, `Git.Branch`, `Timestamp`, model fallbacks) and keeps the 200-line cap for `FirstPrompt` (first wire user message classified turn/KindNone; the legacy `event_msg/user_message` still counts) and `Model` (top-level `turn_context`, not `event_msg`). `readSessionMeta` is deleted; `GetSessionMeta` returns the listed meta after `enrichInto`.
  - `func (s *Store) enrichInto(mc *thinkt.MetadataCache, meta *thinkt.SessionMeta) bool` (same contract as Claude's).
  - `scanSessions`: cache-hit literal and `mc.Set` both carry `ParentSessionID`; after the walk, `SubagentCount` on each parent = number of sessions whose `ParentSessionID` equals its `ID`; titles from `loadSessionIndex` are merged into every session whose `Title` is empty (hit and miss paths alike).
  - `ListProjects`: `SessionCount++` only when `ParentSessionID == ""`, else `SubagentCount++`.
  - `ListSessions`: after cache merge, drop sessions with `ParentSessionID != ""` unless `cfg.IncludeSubagents`; `EnrichSync` runs `enrichInto` over the kept sessions and saves once; `EnrichCallback` runs the same enrichment in a goroutine (copy Claude's `startEnrichment` shape: stale = `FirstPrompt == "" || !IsRealModel(Model) || EntryCount == 0`, `mc.Set` + `Save` per session, `SetSessions`, callback with a copy).
  - `GetSessionMeta` by id searches `loadAllSessions()` (which includes subagents), so a child id resolves.

- [ ] **Step 1: Write the failing tests**

```go
func TestScanSessionFileFixture(t *testing.T) {
	s := scanSessionFile("testdata/rollout-fixture.jsonl")
	if s.FirstPrompt != "fix the build" || s.LastPrompt != "fix the build" || s.Model != "gpt-5.5" || s.EntryCount != 39 {
		t.Fatalf("scan = %+v", s)
	}
	c := s.Cost
	if c == nil || c.DurationMs != 14000 || c.LinesAdded != 1 || c.LinesRemoved != 1 || c.InputTokens != 600 || c.OutputTokens != 60 || c.CachedInputTokens != 140 {
		t.Fatalf("cost = %+v", c)
	}
}

func TestScanSessionFileSumsEveryUsageRecord(t *testing.T) {
	// two usage records with no assistant entry between them → InputTokens is their sum.
}

func TestScanSessionFileSubagentParentAndTitleFallback(t *testing.T) {
	// session_meta with source.subagent.thread_spawn.parent_thread_id "p", agent_nickname "Kierkegaard", agent_path "/root/x"
	// → ParentSessionID "p", Title "Kierkegaard /root/x".
}

func TestLoadSessionIndexLastLineWins(t *testing.T) { /* two lines for id "a": "old", "new" → "new" */ }

func TestLoadSessionIndexSkipsBadLines(t *testing.T) { /* a non-JSON line and a line without thread_name are ignored; missing file → empty map, no error */ }
```

In `store_test.go` (write rollouts under `<base>/sessions/2026/10/01/`; parent `rollout-p.jsonl` with `session_meta{id:"p", session_id:"p", cwd:<proj>}` and one `user.text` message; child `rollout-c.jsonl` with `session_meta{id:"c", session_id:"p", source:{subagent:{thread_spawn:{parent_thread_id:"p",agent_path:"/root/x",agent_nickname:"Kierkegaard"}}}, cwd:<proj>}`; `session_index.jsonl` with `{"id":"p","thread_name":"Fix build"}`):

```go
func TestListSessionsHidesSubagentsByDefault(t *testing.T) {
	// ListProjects → 1 project, SessionCount 1, SubagentCount 1.
	// ListSessions(proj) → 1 session: ID "p", Title "Fix build", SubagentCount 1.
	// ListSessions(proj, thinkt.WithSubagents()) → 2 sessions; the one with ID "c" has ParentSessionID "p" and Title "Kierkegaard /root/x".
	// GetSessionMeta(ctx, "c") → non-nil, ParentSessionID "p".
}

func TestListSessionsParentIDSurvivesCacheHit(t *testing.T) {
	// NewStoreWithCacheDir(base, cacheDir); ListProjects + ListSessions(WithSubagents) once; build a second store on the same dirs;
	// second ListSessions(WithSubagents) → child still has ParentSessionID "p" and the cache file holds "parent_session_id":"p".
}

func TestListSessionsOrphanSubagent(t *testing.T) {
	// only the child rollout exists → ListSessions default: 0; WithSubagents: 1 with ParentSessionID "p"; no error.
}

func TestListSessionsEnrichSyncFillsEntryCountAndCost(t *testing.T) {
	// copy testdata/rollout-fixture.jsonl under sessions/; ListSessions(proj, thinkt.WithEnrichSync()) → EntryCount 39, Cost.DurationMs 14000, LastPrompt "fix the build".
}

func TestGetSessionMetaByPathUsesSessionMetaID(t *testing.T) {
	// file named rollout-x.jsonl with session_meta id "thr-9" → GetSessionMeta(path).ID == "thr-9".
}
```

Update `TestStore_ListProjectsAndSessions` and `TestStore_LoadSession` only if their fixtures no longer parse (they use legacy events and should still pass).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sources/codex/ -run 'TestScanSessionFile|TestLoadSessionIndex|TestListSessions|TestGetSessionMetaByPath' -v`
Expected: FAIL to compile (`scanSessionFile`, `loadSessionIndex` undefined), then listing assertions fail.

- [ ] **Step 3: Implement `sessionstate.go` and the store changes**

- [ ] **Step 4: Run the package and dependents**

Run: `go test ./internal/sources/codex/ ./internal/thinkt/ ./internal/index/... ./internal/cmd/`
Expected: PASS except the two pre-existing `internal/cmd` apps tests that need iTerm.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/codex
git commit -m "feat(codex): single-pass session scan with titles, token totals, and subagent rollouts hidden by default"
```

---

### Task 8: Documentation and real-rollout measurement

**Files:**
- Modify: `internal/sources/codex/README.md`, `AGENTS.md` (entry classification paragraph and the Codex row of the Multi-Source list), `etc/reports/SOURCE_FORMATS_2026-10.md` (Codex "thinkt today" column → "thinkt after the Codex pass" with one line per row), `docs/plans/2026-10-26-codex-source-design.md` (no edits; referenced)

- [ ] **Step 1: Rewrite the README**

Sections: layout (`sessions/YYYY/MM/DD/rollout-*.jsonl`, `session_index.jsonl`, subagent rollouts as ordinary files with `session_meta.source.subagent`), the class table from Task 2 in record-key form, the parser's hold/dedupe/usage rules in three sentences, session-state fields and where each comes from, `THINKT_CODEX_HOME`, key types and functions (`ClassifyLine`, `IsSearchable`, `Parser`, `scanSessionFile`, `Store`), and `ParserVersion` 4.

- [ ] **Step 2: Update AGENTS.md and the format report**

Add one sentence to the entry classification paragraph: "Codex wire tool calls superseded by a harness item carry `KindToolWire` and are secondary; subagent rollouts are listed only with `thinkt.WithSubagents()`." Change the Codex summary row and the record-type table's last column in the report.

- [ ] **Step 3: Measure on a real rollout**

Build `go build -o bin/thinkt ./cmd/thinkt` and, against a real `~/.codex` rollout that has `FileChange` items (pick with `grep -l '"FileChange"' ~/.codex/sessions/*/*/*/*.jsonl | head -1`), record in the ledger:

- `bin/thinkt sessions view --source codex --json <id> | jq '.entries | length'` and the same count with `--system` or the equivalent "include secondary" flag; the two numbers and the entry count the old binary printed (build it from `git stash`-free `git worktree` of `main`, or note "old count not measured").
- No entry with empty text and no blocks: `jq '[.entries[] | select(.text=="" and (.content_blocks|length)==0)] | length'` → 0.
- No duplicate assistant text in adjacent entries: `jq -r '.entries[] | select(.role=="assistant") | .text' | uniq -d | wc -l` → 0.
- `bin/thinkt sync --source codex` then `sqlite3 ~/.thinkt/dbs/index.db "select count(*) from entries where session_id='<id>'"` equals the `--json` entry count including secondary.
- `bin/thinkt export --format html <id> > /tmp/x.html` and `grep -c 'class="diff"' /tmp/x.html` equals the number of non-empty `unified_diff` values in the file.
- For a subagent rollout: `bin/thinkt sessions view --json <child-id> | jq .meta.parent_session_id` names a parent that `bin/thinkt sessions view --json <parent-id>` opens.

Expected: every check holds; any that does not is a finding to fix before the final review, not a note.

- [ ] **Step 4: Full suite and lint**

Run: `go test ./... && task lint`
Expected: PASS except the two iTerm-dependent `internal/cmd` tests; lint clean.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/codex/README.md AGENTS.md etc/reports/SOURCE_FORMATS_2026-10.md
git commit -m "docs(codex): README for the classified rollout format; AGENTS and format report updates"
```

---

## Rulings made while planning

- Tool results are `RoleUser` entries (Claude's convention), not `RoleTool`; the one existing test that asserted `RoleTool` changes. Cost if wrong: one constant.
- `RecordClass` is re-declared in the codex package rather than lifted to `thinkt`. Cost if wrong: a mechanical lift later.
- Wire tool outputs cannot know their tool's name, so the parser's `calls` map decides their visibility; an output whose call was never seen is a normal entry.
- `SubAgentActivity` and `CollabAgentToolCall` UUIDs get `:<kind>` / `:collab` suffixes because several activities can share one spawn call id across kinds in future rollouts, even though the local corpus never repeats an id.
- Background enrichment copies Claude's `startEnrichment` shape instead of the former synchronous callback, because a full scan of 200 rollouts at TUI open is seconds of work.
- The index ingests through `ListSessions` without `WithSubagents`, so Codex subagent rollouts are not indexed, matching Claude subagents today.
