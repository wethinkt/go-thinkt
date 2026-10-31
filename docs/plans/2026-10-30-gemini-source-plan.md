# Gemini CLI JSONL Source Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Read Gemini CLI `.jsonl` sessions (main and subagent) with the transcript the CLI itself rebuilds, give Gemini projects their real path and one identity, and keep the `.json` reader as is apart from usage and the supersede skip.

**Architecture:** A replay step (`replay.go`) rebuilds the ordered message list exactly as the CLI's `createJsonlRecordAccumulator` does. Two consumers read its result:
- a converter (`convert.go`) that turns it into `thinkt.Entry` values
- a metadata scan (`sessionstate.go`)

The store groups `tmp/<folder>/` by `projectHash` and lists both formats. Plumbing goes through the shared hooks, plus one new optional `thinkt.SearchFileFilterer`.

**Tech Stack:** Go, standard library tests.

**Spec:** `docs/plans/2026-10-30-gemini-source-design.md`

**Reference:** the CLI loader, fetched from google-gemini/gemini-cli `main`, is at
- `/private/tmp/claude-501/-Users-evan-projects-go-thinkt/9b31cdbd-5d8b-4c00-8fe9-2e0b58718557/scratchpad/chatRecordingService.ts` (see `createJsonlRecordAccumulator`, lines ~256–560)
- `/private/tmp/claude-501/-Users-evan-projects-go-thinkt/9b31cdbd-5d8b-4c00-8fe9-2e0b58718557/scratchpad/chatRecordingTypes.ts`

## Global Constraints

- **Unchanged `.json` reader:** `.json` sessions are read by the existing `session.go` + `convertSession` path. It changes only in three ways:
  - usage goes through `usageFromTokens`
  - superseded files are skipped (spec §5)
  - sessions are grouped by `projectHash`, with the folder dedupe (spec §1)
- **Usage:**
  - `InputTokens` = `input`
  - `CacheReadInputTokens` = `cached`
  - `OutputTokens` = `output + thoughts`
  - `ThinkingTokens` = `thoughts`
  - `tool` and `total` are dropped
- **Ids:**
  - `SessionMeta.ID` is the filename stem for a main session, and `<parent stem>/<agentId>` for a subagent.
  - The header `sessionId` is kept in the store's `headerIDs map[string]string` (`ID` → header id). It is used for resume and the supersede check, so `thinkt.SessionMeta` gains no source-specific field.
- **`Project.ID`** is `projectHash`. `Project.Path` is the real path, or `""` when unknown, with `DisplayPath` `gemini://<first 8 of hash>`.
- **Continuation text**, the hidden `KindMeta` that opens no new `PromptID`:
  - exactly `Please continue.`
  - or a prefix of `System: Potential loop detected`
  - or a prefix of `[System:`
- **FirstPrompt** skips user text that is empty or starts with `/`, `?`, `<session_context>` or `<hook_context>` (the CLI's `isIgnoredUserContent`).
- **`thinkt.ParserVersion`** goes from 6 to 7.
- **`WatchConfig`:** `{IncludeDirs: ["tmp"], MaxDepth: 4}`.
- **Tests:** `internal/cmd` tests run through `sh /private/tmp/claude-501/-Users-evan-projects-go-thinkt/9b31cdbd-5d8b-4c00-8fe9-2e0b58718557/scratchpad/agy/cmdtest2.sh '<pattern>'`. Its cd target needs editing to this worktree. Known failures are `TestAppsList_JSON` and `TestSetTerminalRoundTrip`.
- **Fixtures** must not contain real prompt text, file contents or paths beyond `/Users/dev/proj`.

## Review Focus

1. **A session file being written while it is read** (a partial last line): replay ignores an unparsable line, as the CLI does. The session opens with what is complete. (Task 2 test.)
2. **A `.jsonl` with no header line, or a header without `projectHash`:** the CLI returns null for these, so thinkt skips the file. It must not crash or list an empty session. (Task 2 and Task 6 tests.)
3. **A `$rewindTo` naming an id that is not present:** every message is deleted, and the session lists with 0 entries, not as an error. (Task 2 test.)
4. **A project folder whose hash matches no `projects.json` key and has no `.project_root`:** the project lists with an empty path and `gemini://<hash>`, and its sessions open. (Task 6 test.)
5. **Large real sessions:** darkstar's biggest `.jsonl` must open and list as fast as before for `.json`. (Task 6 real-data check.)

---

### Task 1: Shared usage mapping and parser version

**Files:**
- Create: `internal/sources/gemini/usage.go`
- Modify: `internal/sources/gemini/store.go` (`convertSession` uses it)
- Modify: `internal/thinkt/types.go` (`ParserVersion = 7`, plus a history comment line "7: Gemini JSONL sessions and usage")
- Modify: `internal/thinkt/metadata_cache_test.go` (`TestParserVersionIsSix` → `IsSeven`)
- Test: `internal/sources/gemini/usage_test.go`

**Interfaces:** Produces `func usageFromTokens(t *Tokens) *thinkt.TokenUsage` (nil for nil `t`).

- [ ] **Step 1: Failing tests.**
  - `TestUsageFromTokens`: `{Input:10259, Output:257, Cached:7000, Thoughts:423, Tool:5, Total:10939}` → `{InputTokens:10259, CacheReadInputTokens:7000, OutputTokens:680, ThinkingTokens:423}`.
  - `nil` → `nil`.
  - `TestJSONSessionUsageKeepsThinking`: `convertSession` over a one-message `Session` with those tokens gives the same usage.
  - `TestParserVersionIsSeven`.
- [ ] **Step 2: Run** `go test ./internal/sources/gemini/ ./internal/thinkt/ -run 'Usage|ParserVersion'`. Expected: FAIL.
- [ ] **Step 3: Implement.** Check the `Tokens` struct in `session.go` has `Cached`, `Thoughts` and `Tool`; add any that are missing, with json tags `cached`, `thoughts` and `tool`.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `feat(gemini): usage keeps cached and thinking tokens; parser version 7`.

### Task 2: Replay

**Files:**
- Create: `internal/sources/gemini/replay.go`
- Test: `internal/sources/gemini/replay_test.go`
- Create: `internal/sources/gemini/testdata/replay.jsonl` (synthetic)

**Interfaces:** Produces:

```go
// lineMessage is one message as the CLI keeps it (raw JSON fields decoded on demand).
type lineMessage struct {
    ID        string
    Type      string          // user, gemini, info, warning, error
    Timestamp time.Time
    Content   json.RawMessage // part list or string
    DisplayContent json.RawMessage
    ToolCalls []jsonlToolCall
    Thoughts  []Thought
    Tokens    *Tokens
    Model     string
    Line      int             // line number of the copy that set it last (1-based)
}

type replayed struct {
    Header    replayHeader    // sessionId, projectHash, startTime, lastUpdated, kind, directories
    Meta      replayMeta      // merged $set fields: lastUpdated, summary (raw string only)
    Messages  []lineMessage   // final order
    Live      map[int]bool    // line numbers whose content survives (Task 7 uses it)
    Unknown   thinkt.UnknownTally[string]
}

func replay(r io.Reader) (*replayed, error) // nil, nil when there is no header with sessionId+projectHash
```

`jsonlToolCall` has `ID`, `Name`, `Args map[string]any`, `Result json.RawMessage`, `Status`, `AgentID` (json `agentId`) and `Timestamp`.

**Algorithm:** mirror `createJsonlRecordAccumulator` (non-`metadataOnly` branch), with these rules:
- An ordered map keyed by id. Re-setting an id keeps its position and replaces the value (JS `Map.set`).
- **Record kinds, tested in the CLI's order:**
  1. `$rewindTo` (string). Delete that id and everything after it; if the id is absent, clear everything.
  2. `$patch`:
     - With `id`: apply a single patch, replacing `content` and setting `toolCalls[].result` by call id on a `gemini` message that already has tool calls.
     - `updates[]`: each is a single patch.
     - `removeIds[]`: delete those ids.
     - `orderIds[]`: kept ids that are not listed stay first in their order, then the listed ids in the listed order.
  3. A message: has a string `id` and no `$patch`.
  4. `$set`:
     - `messages[]` clears the map and refills it.
     - Other fields merge into `Meta`.
  5. The header: has string `sessionId` and `projectHash`. It merges into `Header`, and its `messages` (if any) are added.
  6. Anything else: `Unknown.Note(<first key>)`.
- An unparsable line is skipped.
- **`Live`:**
  - The `Line` of every message left at the end.
  - Every `$patch` line that changed `content` or a result of a message still present.
  - Every line of a `$set.messages` that was not superseded by a later `$set.messages`.
  - (The edited message's original line also stays live. Record that imprecision in a comment.)

- [ ] **Step 1: Write `testdata/replay.jsonl`.** Synthetic, eleven lines:
  1. header `{sessionId:"s1",projectHash:"h1",startTime,lastUpdated,kind:"main"}`
  2. user `u1` `[{text:"first"}]`
  3. gemini `g1` `content:"a"`
  4. gemini `g1` `content:"a"`, `toolCalls:[{id:"c1",name:"read_file",status:"success",result:[{functionResponse:{id:"c1",response:{output:"old"}}}]}]`
  5. user `u2` `[{text:"second"}]`
  6. gemini `g2` `content:"b"`
  7. `{"$patch":{"id":"g1","toolCalls":[{"id":"c1","result":[{"functionResponse":{"id":"c1","response":{"output":"new"}}}]}]}}`
  8. `{"$rewindTo":"u2"}`
  9. user `u3` `[{text:"third"}]`
  10. `{"$set":{"summary":"Title","lastUpdated":"2026-05-17T12:00:00Z"}}`
  11. `{"mystery":1}`
- [ ] **Step 2: Failing tests.**
  - `TestReplayFixture`: final ids `[u1 g1 u3]`; `g1` has one tool call whose result output is `new`; `Meta.Summary == "Title"`; `Header.SessionID == "s1"`; `Live` holds lines 2, 4, 7 and 9, and not 3, 5 or 6; `Unknown.Snapshot()["mystery"] == 1`.
  - `TestReplayRepeatKeepsFirstPosition`: `a`, `b`, `a'` → order `[a b]`, with `a` the second copy.
  - `TestReplayRewindUnknownIDClears`: messages, then `$rewindTo:"zz"` → 0 messages.
  - `TestReplaySetMessagesReplaces`: messages `x`, `y`, then `$set:{messages:[z]}` → `[z]`.
  - `TestReplayOrderIDs`: `a`, `b`, `c`, then `$patch:{orderIds:["c","a"]}` → `[b c a]`.
  - `TestReplayRemoveIDs`.
  - `TestReplayNoHeaderIsNil`.
  - `TestReplayPartialLastLine`: valid lines plus a truncated line → the earlier messages, and no error.
  - `TestReplayRealFixtureRepeats`: over the Task 3 fixture, every id appears once and the copy kept is the one with `toolCalls`. This test lands in Task 3.
- [ ] **Step 3: Run** `go test ./internal/sources/gemini/ -run Replay`. Expected: FAIL.
- [ ] **Step 4: Implement `replay`.**
- [ ] **Step 5: Run.** Expected: PASS.
- [ ] **Step 6: Commit** `feat(gemini): replay JSONL sessions the way the CLI loader does`.

### Task 3: Fixtures from real sessions and the converter

**Files:**
- Create: `internal/sources/gemini/convert.go`, `internal/sources/gemini/classify.go`
- Create: `internal/sources/gemini/testdata/main.jsonl`, plus `testdata/chats-sub/<parent>/92szal.jsonl`, scrubbed from this Mac's `tmp/ds4-go/chats/session-2026-05-17T15-05-17503250.jsonl` and its subagent
- Test: `internal/sources/gemini/convert_test.go`

**Scrubbing:**
- Keep structure, ids, timestamps, types, tokens, model, status, tool names, `agentId`, and the array shapes.
- Replace every text value: prompts, content, thoughts, args values, result outputs, `resultDisplay`, `summary`, `directories`, `projectHash` and paths. Use short placeholders and `/Users/dev/proj`.
- Keep one `invoke_agent` call and one `status:"error"` call. If none has an error, edit one to `status:"error"` with `response.error`.
- Add an `info` line `Request cancelled.` and a user `Please continue.` line, so both rules are covered.
- Write the scrubber as a throwaway script in the scratch folder, not in the repo.

**Interfaces:**
- `func classifyLine(line []byte) thinkt.RecordClass`, used by Task 7: header and plain `$set` → SessionState; `$patch`, `$rewindTo` and `$set.messages` → Bookkeeping; user and gemini → Turn; info, warning and error → Event; else Unknown.
- `func convert(r *replayed, sessionID string) []thinkt.Entry`.

**Entries** (spec §2):
- **user:**
  - `Text` is the joined text parts. A string `content` is also accepted.
  - A continuation (Global Constraints) is `KindMeta`, `VisibilitySecondary`. It does not change the current `PromptID`.
  - A message whose parts are all `functionResponse` is dropped when every response id matches a tool call already emitted inline. Otherwise it becomes a `tool_result` entry per part.
  - Any other user message sets the current `PromptID` to its id.
- **gemini:**
  - One assistant entry: `UUID` is the message id, and `TurnID` is the message id.
  - `PromptID` is the current one.
  - It carries `Model` and `Usage` (via `usageFromTokens`).
  - Blocks, in order:
    - a `thinking` block per thought: `"<subject>: <description>"`, or the description alone when there is no subject
    - a `text` block when `content` (string, or text parts) is non-empty
    - a `tool_use` block per call
  - `functionCall` parts inside `content` become `tool_use` blocks unless a `toolCalls` entry has the same id. `thought` parts become `thinking`.
  - Then one entry per tool call:
    - `UUID` is `<call id>:result`, `Role` is user, and `TurnID`/`PromptID` match the assistant entry.
    - It holds a `tool_result` block.
    - The output is `result[0].functionResponse.response.output`, or `.error` when `status == "error"` (then `IsError` is true). Non-string values are JSON-encoded.
  - `invoke_agent`: the `tool_use` block's `ChildSessionID` is `<sessionID>/<agentId>`.
  - `update_topic`: both the `tool_use` and `tool_result` entries are `VisibilitySecondary`. The assistant entry stays visible if it has text or thinking. When it has only `update_topic` calls, the assistant entry is secondary too.
- **info / warning / error:** `RoleSystem`, `VisibilitySecondary`, `Text` is the content, `Subtype` is the type. `error` sets `Error`.

- [ ] **Step 1: Create the fixtures.** Run the scratch scrubber and inspect the output by eye.
- [ ] **Step 2: Failing tests** (`convert_test.go`, using `replay` over the fixtures):
  - `TestConvertFixtureShape`: count of user, assistant, tool-result and system entries, written as literals once the fixture exists. Each assistant entry has `TurnID == UUID`. Every tool result's `TurnID` equals its call's assistant entry `TurnID`.
  - `TestConvertErrorResult`: the error call's result has `IsError` and the error text.
  - `TestConvertContinuationHidden`: the `Please continue.` entry is secondary `KindMeta`, and the next assistant entry keeps the previous `PromptID`.
  - `TestConvertInvokeAgentLinksChild`: `ChildSessionID == "<stem>/92szal"` on the `tool_use` block.
  - `TestConvertUpdateTopicSecondary`.
  - `TestConvertInfoSecondary`.
  - `TestConvertFunctionResponseUserMessageDropped`: synthetic gemini message `g` with call `c`, then a user message `[{functionResponse:{id:"c",...}}]` → no extra entry.
  - `TestConvertUsage`: one assistant entry with `OutputTokens == output + thoughts`.
  - `TestReplayRealFixtureRepeats` (from Task 2).
- [ ] **Step 3: Run** `go test ./internal/sources/gemini/ -run 'Convert|RealFixture'`. Expected: FAIL.
- [ ] **Step 4: Implement** `classify.go` and `convert.go`.
- [ ] **Step 5: Run.** Expected: PASS.
- [ ] **Step 6: Commit** `feat(gemini): convert replayed JSONL sessions to entries`.

### Task 4: Session scan (metadata)

**Files:**
- Create: `internal/sources/gemini/sessionstate.go`
- Test: `internal/sources/gemini/sessionstate_test.go`

**Interfaces:** Produces:

```go
type jsonlScan struct {
    Meta       thinkt.SessionMeta
    HeaderID   string          // the header's sessionId
    MessageIDs map[string]bool // ids in the final message list (the supersede check)
}
func scanJSONL(path, stem, parentStem string) (*jsonlScan, error) // nil, nil for a file with no header
```

`Meta` fills:
- `ID`: `stem`, or `parentStem/agentId-stem` for a subagent
- `ParentSessionID`: the parent stem
- `CreatedAt`: `startTime`
- `ModifiedAt`: the last `lastUpdated`, else file mtime
- `Title`: the summary when it has no newline and does not start with `{` or `[`
- `FirstPrompt`: the first resumable user text, using the CLI rule
- `Model`: the last gemini model
- `EntryCount`: `len(convert(...))`
- `Cost` token totals, summed from usage (the field names Kimi and Antigravity `sessionstate.go` use)
- `FullPath`, `FileSize`, `Source`

- [ ] **Step 1: Failing tests:**
  - `TestScanJSONLFixture`: values written as literals from the fixture.
  - `TestScanJSONLTitleRules`: a JSON-ish summary gives an empty title; a one-line summary gives that title.
  - `TestScanJSONLFirstPromptSkipsSlash`: `/model`, then `fix it` → `fix it`.
  - `TestScanJSONLSubagentIDs`.
  - `TestScanJSONLNoHeader`: nil, nil.
- [ ] **Step 2: Run** `go test ./internal/sources/gemini/ -run ScanJSONL`. Expected: FAIL.
- [ ] **Step 3: Implement.** One `replay`, then `convert`.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `feat(gemini): JSONL session metadata`.

### Task 5: Project identity

**Files:**
- Create: `internal/sources/gemini/projects.go`
- Test: `internal/sources/gemini/projects_test.go`

**Interfaces:** Produces:

```go
type projectFolder struct {
    Dir, Hash, Path string // Dir under tmp/; Hash = projectHash; Path = real path or ""
    IsSlug bool            // folder name is not the hash
}
func scanProjectFolders(baseDir string) ([]projectFolder, error)
```

**Rules:**
- **Hash**, in order:
  1. The first session file header's `projectHash`, or the `.json` top-level `projectHash`.
  2. The folder name, if it is 64 hex characters.
  3. `sha256(.project_root text)`.
  - Folders with no `chats/` are skipped.
- **Path**, in order:
  1. `.project_root`, trimmed.
  2. The `projects.json` key whose lowercase hex `sha256` equals the hash.
  3. Otherwise `""`.
- `projects.json` is read once per scan.

- [ ] **Step 1: Failing tests** (a temp home):
  - `TestProjectFoldersHashFromHeader`.
  - `TestProjectFoldersPathFromProjectsJSON`: a hash folder with no `.project_root` gets its path through `sha256(key)`. Use `sha256("/Users/dev/proj")`.
  - `TestProjectFoldersUnknownPath`.
  - `TestProjectFoldersSkipNoChats`.
- [ ] **Step 2: Run** `go test ./internal/sources/gemini/ -run ProjectFolders`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `feat(gemini): project folders resolve to projectHash and the real path`.

### Task 6: Store over both formats

**Files:**
- Modify: `internal/sources/gemini/store.go` (`ListProjects`, `ListSessions`, `GetSessionMeta`, `LoadSession`, `OpenSession`, `WatchConfig`)
- Create: `internal/sources/gemini/resume.go`
- Test: `internal/sources/gemini/store_test.go` (new tests; existing tests are updated only for `Project.ID`, path and name)

**Interfaces:** Consumes Tasks 2–5.

**Rules:**
- **`ListProjects`:** one `thinkt.Project` per hash.
  - `ID` is the hash.
  - `Path` is the real path (`PathExists` from a stat of it).
  - `Name` is the path's base name, else the slug folder name, else the first 8 characters of the hash.
  - `DisplayPath` is the path, or `gemini://<8>`.
  - `SessionCount` excludes subagents; `SubagentCount` counts them.
- **`ListSessions(hash)`:** every folder of that hash.
  - **Main `.jsonl`:** `scanJSONL`, through the metadata cache (key: path, mtime, size, `ParserVersion`).
  - **`.json`:** the existing `readSessionMeta`.
  - **Subagents:** `chats/<parentStem>/*.jsonl`, only with `thinkt.WithSubagents()`.
  - **Supersede (spec §5):** drop a `.json` when a `.jsonl` in the project has the same header `sessionId` and the `.json`'s last message id is in that `.jsonl`'s `jsonlScan.MessageIDs`. On a metadata-cache hit, the `.jsonl` is replayed only when some `.json` in the project has the same header id.
  - **Folder dedupe:** files with the same name in two folders of one hash are listed once. The slug folder wins; if the contents differ, the newer `ModifiedAt` wins.
- **`GetSessionMeta(id or path)`** and **`OpenSession`:**
  - A `.jsonl` opens through `replay` + `convert` into the existing `memorySessionReader`.
  - A losing duplicate path or a superseded `.json` returns nil, nil.
- **Header `sessionId`:** kept in `Store.headerIDs`, filled by every listing and lookup (from `jsonlScan.HeaderID`, or the `.json` top-level `sessionId`). `ResumeCommand` reads it, and loads the session's meta when the map has no entry. The metadata cache does not store it; a cache hit re-reads only the header line.
- **`ResumeCommand(meta)`:** `gemini --resume <header sessionId>`, with `Dir` set to the project path. A subagent resumes its parent's header id. The command errors when `gemini` is not on `PATH`.
- **`WatchConfig`:** `{IncludeDirs: ["tmp"], MaxDepth: 4}`.

- [ ] **Step 1: Failing tests** (a temp home built from the fixtures):
  - `TestListProjectsMergesHashAndSlugFolders`: a hash folder and a slug folder of one project → one project with the real path and name `proj`.
  - `TestListSessionsBothFormats`: a `.json` and a `.jsonl` in one project → 2 sessions.
  - `TestListSessionsSkipsSupersededJSON`: a `.json` whose messages are a prefix of a `.jsonl` with the same `sessionId` → only the `.jsonl`.
  - `TestListSessionsKeepsJSONBranchWithSameID`: a `.json` with the same `sessionId` but a last message not in the `.jsonl` → both are listed.
  - `TestListSessionsDedupesAcrossFolders`: an identical file in both folders → 1, from the slug folder.
  - `TestSubagentsOnlyWithOption`: without the option the subagent isn't listed, and the parent's `SubagentCount` is 1; with `WithSubagents` it is listed, with `ParentSessionID` set.
  - `TestOpenJSONLSession`: entries equal `convert(replay(file))`.
  - `TestUnknownPathProjectOpens` (Review Focus 4).
  - `TestNoHeaderFileNotListed` (Review Focus 2).
  - `TestResumeCommand`: a main session resumes with its header id; a subagent resumes with its parent's header id.
  - `TestWatchConfigDepth`.
- [ ] **Step 2: Run** `go test ./internal/sources/gemini/`. Expected: the new tests FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/sources/gemini/`. Expected: PASS.
- [ ] **Step 5: Real-data check.** With `THINKT_GEMINI_HOME` set to the darkstar copy (`…/scratchpad/darkstar-gemini`) and to `~/.gemini`, run `thinkt projects list --json` and `export -f json` over every session. Expected:
  - darkstar: 7 `.jsonl` sessions listed and exported.
  - go-thinkt's hash and slug folders appear as one project, and the byte-identical `.json` copies are listed once.
  - Every project with a known path shows it.
  - No session exports empty except those made only of notices, which now export their secondary entries.
  - Time the darkstar `projects list` against the base binary.
- [ ] **Step 6: Commit** `feat(gemini): store lists and opens JSONL sessions under one project per projectHash`.

### Task 7: Resolver, search hook, watching

**Files:**
- Create: `internal/thinkt/search_filter.go` (add `SearchFileFilterer`)
- Modify: `internal/index/search/search.go` (`lineFilter` takes the candidate path and prefers the file filter)
- Create: `internal/sources/gemini/searchfilter.go`
- Modify: `internal/sources/gemini/discovery.go` (`IsSessionPath`), `internal/sources/gemini/pathresolve.go`
- Modify: `internal/index/watcher.go` (`watchProject`)
- Modify: `internal/sources/resolve_test.go` (gemini rows and fixtures with headers)
- Test: `internal/sources/gemini/pathresolve_test.go`, `internal/index/search/bookkeeping_test.go`, `internal/index/watcher_discover_test.go`

**Interfaces:**

```go
// thinkt
type SearchFileFilterer interface {
    NewSearchFileFilter(path string) func(line []byte, matched bool) bool
}
```

**Rules:**
- **`scanFile`:** uses `SearchFileFilterer` when the factory has it, else `SearchLineFilterer`.
- **Gemini's file filter:**
  - For `.jsonl`, it replays the file and keeps a hit only when its 1-based line number is in `Live` and the line is a user or gemini message, or a `$patch` content line.
  - For `.json`, it keeps every hit, as today.
- **`IsSessionPath`:** true for `.jsonl` under `<home>/tmp/*/chats/` or `<home>/tmp/*/chats/*/`, and for `.json` directly under `<home>/tmp/*/chats/`. It keeps the Antigravity exclusion. `logs.json` is not a session.
- **Resolver:**
  - `SessionID` is the path in home spelling.
  - `ProjectID` is the `projectHash` from the file's first line.
  - `BaseDir` is unchanged.
- **`watchProject`:** lists with `thinkt.WithSubagents()` so subagent folders are watched. Only sessions with `ParentSessionID == ""` go into `sessionIndex`. A note in the commit says this also watches Codex, Kimi and Antigravity subagent folders, whose changes `discoverSession` already skips.

- [ ] **Step 1: Failing tests:**
  - `TestSearchGeminiJSONLSkipsDeadLines` (`bookkeeping_test.go`): a `replay.jsonl` copy; search `"second"` → 0 hits (the line was rewound); `"third"` → 1 hit; `"Title"` → 0 hits (`$set`).
  - `TestSearchGeminiRepeatHitOnce`: a word in a repeated gemini message → 1 hit, on the later line.
  - `TestIsSessionPathGemini`:
    - true: `tmp/p/chats/a.jsonl`, `tmp/p/chats/<id>/x.jsonl`, `tmp/p/chats/a.json`
    - false: `tmp/p/logs.json`, `antigravity-cli/...`, `tmp/p/chats/<id>/x.json`
  - `TestResolveGeminiProjectID`: `ProjectID` equals the header hash.
  - `TestWatchProjectWatchesSubagentDir` (`internal/index`): a gemini project with a subagent → the subagent folder is in the watched set, and the subagent is not in `sessionIndex`.
  - Update the `resolve_test.go` gemini fixtures (a `.jsonl` row with a header).
- [ ] **Step 2: Run** `go test ./internal/sources/... ./internal/index/...`. Expected: the new tests FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `feat(gemini): resolver, search file filter and watching for JSONL sessions`.

### Task 8: Docs and suite

**Files:**
- Modify: `internal/sources/gemini/README.md`: the layout, both formats, replay, ids, projects, usage, and that `ParserVersion` 7 covers it.
- Modify: `AGENTS.md`: the Gemini line in Multi-Source Design and the entry-classification paragraph sentence for Gemini.
- Modify: `etc/reports/SOURCE_FORMATS_2026-10.md`: add a Gemini CLI section with the line-kind table from spec §2 and the measured counts.
- Modify: `internal/sources/README.md`: mention `SearchFileFilterer`.

- [ ] **Step 1: Write the docs.**
- [ ] **Step 2: Run the full suite** `go test ./... 2>&1 | tail -40`. Expected: only the two known `internal/cmd` failures.
- [ ] **Step 3: Commit** `docs(gemini): JSONL format, projects and replay`.
