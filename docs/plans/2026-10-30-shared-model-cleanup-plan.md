# Shared Model Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lift record classes, the unknown tally, search line filters and session path resolution into shared `thinkt` pieces, and give `TurnID`/`PromptID` one meaning across sources.

**Architecture:** Two optional `StoreFactory` interfaces (`SessionPathResolver`, `SearchLineFilterer`) are implemented on each source's `*Discoverer`; `internal/sources` owns the ordered path lookup that replaces the TUI switch. Shared enum and tally live in `internal/thinkt`. The id move touches the Codex, Kimi and Antigravity parsers and bumps `ParserVersion` to 6.

**Tech Stack:** Go, standard library tests.

**Spec:** `docs/plans/2026-10-30-shared-model-cleanup-design.md`

## Global Constraints

- Module `github.com/wethinkt/go-thinkt`; everything stays `internal`.
- `internal/thinkt` must not import `internal/sources`.
- Order: record classes and tally → search hook → path resolver → id move.
- Resolution order is fixed: Kimi, Antigravity, Codex, Copilot, Qwen, Gemini, then the `.jsonl` content sniff (Claude, Codex, Qwen). It does not follow `AllFactories()`. Earlier match wins; nothing errors.
- A file the user names opens even when its source is off. The index watcher and relay use registered (enabled) sources only.
- `thinkt.ParserVersion` 5 → 6, once, in the first task that changes emitted ids.
- Known pre-existing failures: `TestAppsList_JSON`, `TestSetTerminalRoundTrip` in `internal/cmd`. `internal/cmd` tests run through `sh /private/tmp/claude-501/-Users-evan-projects-go-thinkt/9b31cdbd-5d8b-4c00-8fe9-2e0b58718557/scratchpad/agy/cmdtest2.sh '<pattern>'` (the worktree guard refuses the package path).
- Full suite: `go test ./... 2>&1 | tail -40`; expected: only the two known failures.

## Review Focus

1. Search speed: Claude and Codex predicates now run on every line, not only matching ones. A search over a large `~/.claude` should not get noticeably slower (Task 3 times it before/after).
2. A `.jsonl` copied outside every home opens as it did (sniff path, empty `BaseDir`): Codex falls back to `findCodexBaseDir` logic, Qwen to `NewStore("")` (Task 4 overlap table rows).
3. Symlinked homes: watcher paths are symlink-resolved; resolvers must match the resolved home too (Task 4 test with a symlinked temp home).
4. Codex rollouts that end mid-response (file still being written): the held entries come out at EOF without `TurnID`, nothing lost (Task 9 test).
5. Codex rollouts with no `token_usage_record` at all: output identical to today apart from `TurnID`→`PromptID` (Task 9 fixture test).

---

### Task 1: Shared RecordClass and UnknownTally

**Files:**
- Create: `internal/thinkt/recordclass.go`, `internal/thinkt/unknown.go`
- Test: `internal/thinkt/recordclass_test.go`, `internal/thinkt/unknown_test.go`

**Interfaces:**
- Produces:
  - `type RecordClass int` with consts `ClassTurn, ClassEvent, ClassContext, ClassCheckpoint, ClassSessionState, ClassBookkeeping, ClassUnknown` (iota, this order); `func (RecordClass) String() string` (`turn, event, context, checkpoint, session-state, bookkeeping, unknown`, default `invalid`); `func (RecordClass) IsEntry() bool` (true for Turn, Event, Context, Checkpoint, Unknown).
  - `type UnknownTally[K ~string] struct { Source Source; counts map[K]int }`; `func (u *UnknownTally[K]) Note(key K, keyvals ...any)` logs `tuilog.Log.Debug("<source>: unknown record type", "type", key, keyvals...)` on the first sighting of a key; `func (u *UnknownTally[K]) Snapshot() map[K]int` returns a copy or nil.

- [ ] **Step 1: Write the failing tests**

`TestRecordClassStringAndIsEntry`: table over the seven values plus `RecordClass(99)` asserting the strings above and `IsEntry` true exactly for turn, event, context, checkpoint, unknown.
`TestUnknownTallyCountsAndSnapshots`: zero value `Snapshot()` is nil; after `Note("a")`, `Note("a")`, `Note("b")` the snapshot is `{"a":2,"b":1}`; mutating the snapshot does not change a second snapshot.

- [ ] **Step 2: Run** `go test ./internal/thinkt/ -run 'RecordClass|UnknownTally'` — Expected: FAIL, undefined.
- [ ] **Step 3: Implement** both files (copy the Claude enum comments that are source-neutral).
- [ ] **Step 4: Run** the same command — Expected: PASS.
- [ ] **Step 5: Commit** `feat(thinkt): shared RecordClass and UnknownTally`

### Task 2: Sources use the shared class and tally

**Files:**
- Modify: `internal/sources/{claude,codex,kimi}/classify.go` (delete the enum, `String`, `IsEntry`; use `thinkt.RecordClass`/`thinkt.Class*`), their parsers and `claude/lazy_session.go` (field type `thinkt.UnknownTally[EntryType]` / `[string]` with `Source` set where the parser/lazy session is constructed; calls become `Note`/`Snapshot`).
- Modify: `internal/sources/antigravity/classify.go` (rename `RecordClass` → `StepClass`; values keep their names), `antigravity/parser.go`, `antigravity/convert.go`.
- Delete: `internal/sources/{claude,codex,kimi,antigravity}/unknown.go`.
- Test: existing tests in those packages (`classify_test.go` tally tests switch to `thinkt.UnknownTally`).

**Interfaces:**
- Consumes: Task 1.
- Produces: `antigravity.StepClass`; `antigravity.ClassifyLine` returns `(stepHead, StepClass)`. Public `UnknownTypes()` signatures unchanged.

- [ ] **Step 1: Edit the tally tests** to use `thinkt.UnknownTally` and add to codex `parser_hold_test.go` `TestParserUnknownTypesTally` an assertion that a second `NewParser` starts empty (pins that `Source` init didn't make the tally shared).
- [ ] **Step 2: Run** `go test ./internal/sources/...` — Expected: FAIL to compile.
- [ ] **Step 3: Migrate** the four packages as listed. Source-specific per-value comments move next to each classifier's switch.
- [ ] **Step 4: Run** `go test ./internal/sources/... ./internal/index/... && go vet ./internal/...` — Expected: PASS.
- [ ] **Step 5: Commit** `refactor(sources): record classes and unknown tally come from thinkt`

### Task 3: Search line filter hook

**Files:**
- Create: `internal/thinkt/search_filter.go`
- Modify: `internal/sources/{claude,codex,kimi,antigravity}/discovery.go` (method on `*Discoverer`), `internal/index/search/search.go` (drop `searchable`, `searchFilters`; `scanFile` asks the factory)
- Test: `internal/index/search/bookkeeping_test.go`, `internal/sources/sources_test.go`

**Interfaces:**
- Produces: `type SearchLineFilterer interface { NewSearchLineFilter() func(line []byte) bool }` in `thinkt`; `func sources.Factory(s thinkt.Source) thinkt.StoreFactory` (nil when unknown) in `internal/sources/sources.go`.

Claude and Codex wrappers keep today's rule exactly: a line with no top-level `type` is kept; otherwise `IsSearchable(line)`. `scanFile` calls `NewSearchLineFilter()` once per file and the predicate on every line in order.

- [ ] **Step 1: Write the failing test** `TestSearchLineFilterersImplemented` in `internal/sources/sources_test.go`: for claude, codex, kimi, antigravity `sources.Factory(s)` implements `thinkt.SearchLineFilterer`; for gemini, copilot, qwen it does not.
- [ ] **Step 2: Run** `go test ./internal/sources/ -run SearchLineFilterers` — Expected: FAIL.
- [ ] **Step 3: Before changing `search.go`, time a baseline:** `go build -o /private/tmp/claude-501/-Users-evan-projects-go-thinkt/9b31cdbd-5d8b-4c00-8fe9-2e0b58718557/scratchpad/thinkt-base ./cmd/thinkt` and `time <that binary> search --source claude 'func ' >/dev/null` (three runs, note the median).
- [ ] **Step 4: Implement** the interface, the four methods, `sources.Factory`, and the `scanFile` change.
- [ ] **Step 5: Run** `go test ./internal/sources/ ./internal/index/search/` — Expected: PASS, including every `TestScanFile*`.
- [ ] **Step 6: Time again** with a fresh build; Expected: median within ~15% of the baseline. If slower, record the numbers in the ledger as a ruling, don't change the contract.
- [ ] **Step 7: Commit** `refactor(search): line filters are a source hook`

### Task 4: Session path resolver and ordered lookup

**Files:**
- Create: `internal/thinkt/session_path.go`, `internal/sources/resolve.go`, `internal/sources/sniff.go` (moved `detectJSONLSessionType` as `sniffJSONL(path string) (thinkt.Source, error)`)
- Create per source: `internal/sources/<name>/pathresolve.go` for all seven (methods on `*Discoverer`; Qwen on its factory type)
- Test: `internal/sources/resolve_test.go`, `internal/sources/<name>/pathresolve_test.go` where a source has non-trivial layout parsing (codex, kimi, antigravity, gemini, qwen)

**Interfaces:**
- Produces:
  - `thinkt.SessionPath{Source Source; BaseDir, SessionID, ProjectID string}`
  - `thinkt.SessionPathResolver` with `ResolveSessionPath(path string) (SessionPath, bool)` and `OpenSessionPath(ctx context.Context, sp SessionPath, path string) (LazySession, error)`
  - `func sources.ResolveSessionPath(path string) (thinkt.SessionPath, thinkt.SessionPathResolver, bool)`
  - `func sources.OpenSessionPath(ctx context.Context, path string) (thinkt.LazySession, error)` (resolve + open; error `unsupported session format: <path>` on a miss, as today)

Rules from the spec:
- Stage 1 tries the fixed order (Kimi, Antigravity, Codex, Copilot, Qwen, Gemini), each resolver claiming exactly what its `IsSessionPath` claims today. Stage 2, for an unclaimed `.jsonl`: `sniffJSONL` names claude, codex or qwen; that source's resolver is asked; if it declines, `SessionPath{Source: s, SessionID: path}`.
- `BaseDir`/`ProjectID`/opener per the spec's table. Empty `BaseDir` falls back to today's behaviour: `findCodexBaseDir`/`findCopilotBaseDir`/`findGeminiBaseDir` logic (moved from `internal/tui` into the source packages), `DefaultDir` for Kimi/Antigravity, `NewStore("")` for Qwen.
- `ProjectID` set only for Claude (`filepath.Dir(path)`, when under the Claude home), Gemini and Qwen (hash directory).
- `SessionID` is the value the store's `GetSessionMeta` accepts for that file.

- [ ] **Step 1: Write the failing overlap test** `TestResolveSessionPathMatchesOldSwitch` in `resolve_test.go`. Build a temp tree with `THINKT_CLAUDE_HOME`, `THINKT_CODEX_HOME`, `THINKT_QWEN_HOME`, `THINKT_COPILOT_HOME`, `THINKT_GEMINI_HOME`, `THINKT_ANTIGRAVITY_HOME` (inside the Gemini home) and `THINKT_KIMI_HOME` set with `t.Setenv`. Keep the old switch as an unexported oracle `oldSwitchSource(path) string` in the test file (copied from `internal/tui/session_loader.go` before Task 5 deletes it). Rows, each asserting `oracle == resolved.Source`:

| Path | Want |
|---|---|
| `<claude>/projects/-x/events.jsonl` | copilot |
| `<claude>/projects/-x/r.jsonl` with a `session_meta` first line | codex |
| `<claude>/projects/-x/s.jsonl` with a Claude `user` record | claude |
| `<codex>/sessions/2026/10/30/rollout-a.jsonl` | codex |
| `<qwen>/projects/h/chats/s.jsonl` | qwen |
| `<agy>/brain/<id>/.system_generated/logs/transcript.jsonl` | antigravity |
| `/elsewhere/sessions/wd/s1/agents/main/wire.jsonl` | kimi |
| `/elsewhere/.codex/sessions/r.jsonl` | codex |
| `/elsewhere/copy.jsonl` with a Claude record | claude |
| `<gemini>/tmp/h/chats/session-1.json` | gemini |
| `/elsewhere/notes.txt` | no match |

- [ ] **Step 2: Write** `TestResolvedSessionsOpen`: for each claimed row, `sources.OpenSessionPath` returns a `LazySession` without error (fixture contents copied from each source's `testdata`), and `TestProjectIDMatchesStore` for claude, gemini, qwen: `store.ListProjects` contains a project whose `ID == sp.ProjectID`. Add a row resolving through a symlinked home (`os.Symlink` of the Codex home; path given via the link and via the target, both resolve to codex).
- [ ] **Step 3: Run** `go test ./internal/sources/ -run 'ResolveSessionPath|ResolvedSessions|ProjectID'` — Expected: FAIL, undefined.
- [ ] **Step 4: Implement** `session_path.go`, the seven `pathresolve.go` files, `resolve.go`, `sniff.go`. Per-source `IsSessionPath`/`BaseDirForPath`/`SessionIDForPath` become unexported unless still used elsewhere after Task 5–7 (decide at Task 7; leave exported here).
- [ ] **Step 5: Run** the same command plus `go test ./internal/sources/...` — Expected: PASS.
- [ ] **Step 6: Commit** `feat(sources): session path resolvers with the TUI switch order`

### Task 5: TUI loader uses the resolver

**Files:**
- Modify: `internal/tui/session_loader.go` (`OpenLazySession` becomes `sources.OpenSessionPath(context.Background(), path)` with the existing log lines reduced to one `"OpenLazySession: resolved", "source", ...`; delete the switch, `open*LazySession`, `find*BaseDir`, `isJSONLSession`, `detectJSONLSessionType`)
- Test: existing `internal/tui` tests

**Interfaces:**
- Consumes: `sources.OpenSessionPath`, `sources.ResolveSessionPath` (Task 4). `OpenLazySessionWithRegistry` keeps `registry.OpenLazySessionByPath` as fallback, unchanged.

- [ ] **Step 1: Write the failing test** `TestOpenLazySessionOpensDisabledSourceFile` in `internal/tui/session_loader_test.go`: with a config that disables codex (`THINKT_HOME` temp dir, config `{"sources":{"codex":{"enabled":false}}}`) a Codex rollout path under a temp `THINKT_CODEX_HOME` opens via `OpenLazySession`. It passes today; it pins behaviour across the swap — record in the ledger that it passed before the change, and run it again after.
- [ ] **Step 2: Run** `go test ./internal/tui/ -run OpenLazySession` — Expected: PASS (pin).
- [ ] **Step 3: Replace** the loader body and delete the dead helpers.
- [ ] **Step 4: Run** `go test ./internal/tui/ ./internal/sources/... && go build ./...` — Expected: PASS.
- [ ] **Step 5: Commit** `refactor(tui): session loader resolves paths through the sources`

### Task 6: Index watcher discovers through the resolver

**Files:**
- Modify: `internal/index/watcher.go` (`discoverSession`; drop the all-stores scan; keep `underDir` only if still used, `firstUnmatched` stays)
- Test: `internal/index/watcher_discover_test.go`

**Interfaces:**
- Consumes: `sources.ResolveSessionPath` (Task 4); `w.registry.Get(source)`; `store.GetSessionMeta`.
- The six steps from the spec, in order: resolve (miss → stop), registered store (no → stop), `GetSessionMeta(sp.SessionID)` (error → stop), resolved `FullPath` equals the path (no → stop), `ParentSessionID != ""` → stop, `ProjectID` set → entry with `db.ScopedProjectID(source, sp.ProjectID)`, else scan only that store's projects. Each stop logs at debug.

- [ ] **Step 1: Write the failing tests:**
  - `TestDiscoverSessionSkipsDisabledSource`: a Codex rollout under a temp Codex home, registry without codex → `ok == false`, and no store's `ListProjects` is called (use a registry holding a counting fake store for claude; assert count 0).
  - `TestDiscoverSessionSkipsSubagent`: Antigravity child conversation from `testdata/child.jsonl` laid out under a temp home with its summaries row → `ok == false`.
  - `TestDiscoverSessionUsesProjectIDFromPath`: a Claude session under a temp Claude home → entry `projectID == db.ScopedProjectID("claude", <dir>)`.
  - Keep `TestDiscoverSessionOutsideProjectFolder` unchanged (it covers the FullPath rule).
- [ ] **Step 2: Run** `go test ./internal/index/ -run DiscoverSession` — Expected: the three new tests FAIL.
- [ ] **Step 3: Implement** the new `discoverSession`.
- [ ] **Step 4: Run** `go test ./internal/index/...` — Expected: PASS.
- [ ] **Step 5: Commit** `refactor(index): watcher discovers sessions through source resolvers`

### Task 7: Relay fallback uses the resolver; trim exports

**Files:**
- Modify: `internal/relay/watcher.go` (`detectSource` returns `string(sp.Source)` from `sources.ResolveSessionPath`, `"unknown"` on a miss)
- Modify: source packages — unexport `IsSessionPath`, `BaseDirForPath`, `SessionIDForPath`, `DefaultDir` where no caller outside the package remains (`grep -rn` each before changing). Gemini's use of `antigravity.DefaultDir` keeps that one exported.
- Test: `internal/relay/watcher_test.go`

- [ ] **Step 1: Write the failing test** `TestDetectSourceUsesResolver`: an Antigravity transcript path under a temp `THINKT_ANTIGRAVITY_HOME` inside a temp `.gemini` dir returns `"antigravity"` (today's string match returns `"gemini"`); a Qwen path under `.qwen/projects` returns `"qwen"` (today `"unknown"`).
- [ ] **Step 2: Run** `go test ./internal/relay/ -run DetectSource` — Expected: FAIL.
- [ ] **Step 3: Implement**, then unexport.
- [ ] **Step 4: Run** `go build ./... && go test ./internal/relay/ ./internal/sources/...` — Expected: PASS. If `internal/relay` → `internal/sources` creates an import cycle, record a ruling and keep the string fallback.
- [ ] **Step 5: Commit** `refactor(relay): source fallback uses the resolvers`

### Task 8: Kimi and Antigravity ids; ParserVersion 6

**Files:**
- Modify: `internal/thinkt/types.go` (field comments: `TurnID` "One model response (Claude message.id, Codex response_id, Kimi step uuid, Antigravity planner step)"; `PromptID` "One user prompt and everything until the agent hands back"), `internal/thinkt/metadata_cache.go` (or wherever `ParserVersion` is defined: 6)
- Modify: `internal/sources/kimi/convert.go` (`newEntry`: the turn's id goes to `PromptID`), `internal/sources/kimi/parser.go` (step flush sets `TurnID = step.uuid`; parser keeps `callStep map[string]string` toolCallID → step uuid, filled when a step's `tool_use` block is added; `resultEntry` sets `TurnID = p.callStep[head.ToolCallID]`)
- Modify: `internal/sources/antigravity/convert.go` (`converter.turnID` → `promptID`, written to `PromptID`; `toolCall` gains `Turn string`; `modelTurn` sets entry `TurnID = strconv.Itoa(h.Index)` and each call's `Turn`; `resultEntry(h, call toolCall)` sets `TurnID = call.Turn`; orphans pass a zero `toolCall` with only `ID`), `antigravity/parser.go`
- Test: `internal/thinkt/metadata_cache_test.go` (`TestParserVersionIsFive` → `TestParserVersionIsSix`), `kimi/parser_test.go`, `antigravity/convert_test.go`, `antigravity/parser_test.go`

- [ ] **Step 1: Write the failing tests:**
  - `TestParserVersionIsSix`.
  - Kimi `TestStepAndResultsShareTurnID`: over the existing Kimi fixture, every assistant step entry has `TurnID == UUID`, each tool-result entry's `TurnID` equals the `UUID` of the step that holds its `tool_use`, every entry that had a `TurnID` before now has that value in `PromptID`, and `Metadata["message_id"]` is unchanged.
  - Antigravity: rename `TestChildTurnIDBeforeAnyUserInput` → `TestChildPromptIDBeforeAnyUserInput` asserting `e.PromptID == "0"`; add `TestPlannerAndResultShareTurnID` over `main.jsonl`: planner entry `TurnID == strconv.Itoa(index)`, its paired result has the same `TurnID`, USER_INPUT entries have `TurnID == ""`, an orphan result has `TurnID == ""`.
- [ ] **Step 2: Run** `go test ./internal/thinkt/ ./internal/sources/kimi/ ./internal/sources/antigravity/` — Expected: FAIL on the new assertions.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** the same — Expected: PASS.
- [ ] **Step 5: Commit** `feat(kimi,antigravity): PromptID per prompt, TurnID per model response; parser version 6`

### Task 9: Codex response hold and response_id

**Files:**
- Modify: `internal/sources/codex/usage.go` (`usageRecord.ResponseID string \`json:"response_id"\``; `decodeUsage` unchanged; new `responseID(payload json.RawMessage) string`), `codex/parser.go`, `codex/convert.go` (turn id → `PromptID`)
- Test: `internal/sources/codex/parser_hold_test.go`, `codex/usage_test.go`

**Interfaces:** Parser state replaces `pending`/`usageTarget` with `hold []thinkt.Entry`, `usageTarget int` (index of the last assistant entry of the most recent entry-producing line in the hold), `closed bool`, `callResponse map[string]string` (raw call id → response id).

Algorithm (the spec's rules, in parser terms):
- **Result lines** are wire outputs (`response_item/function_call_output`, `custom_tool_call_output`) and harness tool items (`item/CommandExecution`, `FileChange`, `McpToolCall`, `WebSearch`, `ImageView`, `Extension`). Every other entry-producing line is a **response line**.
- Twin checks (`isItemTwin`, legacy `isWireTwin`) run exactly as today, when the line is read, before any hold logic.
- On an entry-producing line: if `closed`, release the hold. Then: a user-role message line, `turn_context`, `task_started` or a legacy event releases the hold unstamped first. A result line with an empty hold is released at once; otherwise it joins the hold. A response line joins the hold.
- Before releasing or holding, each entry whose tool block `ToolUseID` (with any `wire:` prefix removed) is in `callResponse` takes that response id as `TurnID`.
- On `token_usage_record`: if the hold is empty, do nothing (today's behaviour after a user entry). Otherwise attach usage to `usageTarget` (last write wins) and set `TurnID` = this record's `response_id` on held entries whose `TurnID` is empty — a second adjacent record overwrites the first's stamp. Record every held `tool_use` id (prefix removed) → response id in `callResponse`. Set `closed = true`; do not release.
- At EOF: release unstamped.

- [ ] **Step 1: Write the failing tests** (inline fixtures in the `parseAll` style; JSON shapes from `testdata/rollout-fixture.jsonl`):
  - `TestResponseEntriesShareResponseID`: reasoning, assistant message, wire function_call, usage(`resp-1`) → all three entries `TurnID == "resp-1"`; usage on the assistant entry.
  - `TestAdjacentUsageRecordsApplyToOneHold`: response lines, usage(`resp-1`, 10 tokens), usage(`resp-2`, 20 tokens), next response line → held entries `TurnID == "resp-2"`, usage 20 (pins today's last-write-wins in `TestParserDoubleUsageLastWriteWins`, which must still pass).
  - `TestCommandItemInsideHoldTakesResponseID`: wire call, `item/CommandExecution`, usage(`resp-1`) → both command halves `TurnID == "resp-1"`.
  - `TestCommandItemAfterUsageHasNoTurnID`: wire call `call_1`, usage(`resp-1`), `item/CommandExecution` id `exec-1`, wire output `call_1` → command halves `TurnID == ""`, wire output `TurnID == "resp-1"`, and output order equals line order.
  - `TestMcpToolCallAfterUsageJoinsByCallID`: wire call `call_9`, usage(`resp-1`), `item/McpToolCall` id `call_9` → both halves `TurnID == "resp-1"`.
  - `TestHoldReleasedUnstampedAtEOF`: reasoning, assistant message, EOF → both entries present, `TurnID == ""`.
  - `TestUserMessageReleasesHoldUnstamped`: response lines, user message, usage → response entries `TurnID == ""`, usage not attached (today's `TestParserUsageAfterUserEntryIsNotAttached` still passes).
  - `TestTwinDropUnchangedByHold`: `fixAgentItemA2`, `fixWireA2`, usage → one entry, `TurnID` set.
  - `TestRolloutWithoutUsageRecords`: over a copy of `testdata/rollout-fixture.jsonl` with `token_usage_record` lines removed, entries equal today's output except `PromptID` holds what `TurnID` held and `TurnID == ""` (compare UUID, Role, Kind, Text order).
  - Existing tests asserting `TurnID == "turn-1"` change to `PromptID`.
- [ ] **Step 2: Run** `go test ./internal/sources/codex/` — Expected: new tests FAIL.
- [ ] **Step 3: Implement** the hold in `parser.go`; move the turn id to `PromptID` in `convert.go`.
- [ ] **Step 4: Run** `go test ./internal/sources/codex/` — Expected: PASS, including `TestParserFixtureEntryCounts` and `TestParserFixtureOrderAndKinds` unchanged.
- [ ] **Step 5: Real-data check:** `go build -o /private/tmp/claude-501/-Users-evan-projects-go-thinkt/9b31cdbd-5d8b-4c00-8fe9-2e0b58718557/scratchpad/thinkt-ids ./cmd/thinkt`, then for the five newest rollouts compare `thinkt sessions view --json`-style entry counts before (base binary from Task 3) and after. Expected: identical counts; most assistant entries carry `turn_id`.
- [ ] **Step 6: Commit** `feat(codex): TurnID from response_id; PromptID from turn_id`

### Task 10: Presentation, docs

**Files:**
- Modify: `internal/conversation/document.go` (`Message.PromptID`, copied from the entry), `internal/conversation/html.go` (`data-prompt-id` beside `data-turn-id`, same escaping)
- Modify: `internal/sources/{codex,kimi,antigravity}/README.md`, `etc/reports/SOURCE_FORMATS_2026-10.md` (id mapping rows), `AGENTS.md` (Adding a New Source: implement `SessionPathResolver` and, if needed, `SearchLineFilterer` on the factory; add the source to the resolution order in `internal/sources/resolve.go`; drop the "Register in `internal/tui/shell.go`"-style loader step if it no longer applies)
- Run: `task server:swag-v2`; commit generated changes only if any.
- Test: `internal/conversation/children_test.go`

- [ ] **Step 1: Write the failing test** `TestMessageCarriesPromptID` (entry with `PromptID: "p1"` → `Message.PromptID == "p1"` and HTML contains `data-prompt-id="p1"`).
- [ ] **Step 2: Run** `go test ./internal/conversation/ -run PromptID` — Expected: FAIL.
- [ ] **Step 3: Implement**, update docs.
- [ ] **Step 4: Run** the full suite — Expected: only the two known `internal/cmd` failures.
- [ ] **Step 5: Commit** `feat(conversation): carry PromptID; docs for the shared source hooks`
