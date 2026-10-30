# Shared model cleanup: turn ids, path resolver, search hook, record classes

Date: 2026-10-30. Branch: stacked on `worktree-source-visibility`. To be
done before the Gemini CLI pass, so Gemini is written against the shared
pieces instead of adding a fifth copy of each.

## Why

The Codex, Kimi and Antigravity passes each added the same ideas in their
own package. Gemini would add them a fifth time. Four of these belong in
the shared model now:

1. `Entry.TurnID` means two different things depending on the source.
2. Every source has its own "is this my session file, where is its home,
   what is its id" helpers, wired by hand into a switch in the TUI loader
   and approximated by path prefixes in the index watcher.
3. Search line filters are looked up in two maps in `search.go`.
4. The `RecordClass` enum exists four times and `unknownTally` four times.

Only item 1 changes what a user sees. Items 2–4 are plumbing: the same
output, less per-source wiring.

## Goals

- One meaning for each of `TurnID` and `PromptID`, the same in every source.
- Adding a source means implementing interfaces on its factory, not editing
  the TUI loader, the index watcher, the relay or `search.go`.
- One `RecordClass` and one unknown-record tally in `internal/thinkt`.

## Non-goals

- No new grouping UI in the TUI or web app. The ids are made consistent so
  a later grouping view can rely on them; this pass does not build one.
- No change to which entries a source emits (beyond the ids they carry).
- Copilot, Qwen and Gemini get resolvers (item 2) but no other rework.
- The later items (agent documents, media references, background-task
  links, RelatedPaths for other sources, relay shipping parsed entries)
  are separate designs.

## Anti-goals

- A source silently losing ids it has today. Every entry that carries a
  per-prompt id now still carries it after, in `PromptID`.
- Path resolution that depends on the order of `AllFactories()`; the
  winner order is its own fixed list, matching today's TUI switch.
- Listing, searching or serving a source that is turned off (see the
  source visibility change). Resolvers never open a store to decide. One
  exception, decided with the user: a session file the user names
  explicitly (TUI or `thinkt sessions view <path>`) opens even when its
  source is off, as it does today. The index watcher and relay only use
  registered (enabled) sources.

## 1. TurnID and PromptID

Decided with the user:

- **`PromptID`**: one user prompt and everything the agent does until it
  hands control back. Claude's `promptId` already means this.
- **`TurnID`**: one model response. Claude's `message.id` already means this.

The comments in `internal/thinkt/types.go` already say this; the three
newer sources store their prompt-scoped id in `TurnID` against them.

Per source:

| Source | PromptID | TurnID |
|---|---|---|
| Claude | `promptId` (unchanged) | `message.id` (unchanged) |
| Codex | the rollout `turn_id` (moved from TurnID) | `token_usage_record.payload.response_id` |
| Kimi | the wire `turnId` (moved from TurnID) | the loop step uuid |
| Antigravity | today's value, moved from TurnID: the opening USER_INPUT index, or the first emitted step's index for steps before any USER_INPUT | the PLANNER_RESPONSE step index |
| Gemini, Copilot, Qwen | unchanged (none set today) | unchanged |

**Codex.** `token_usage_record` closes each response and carries
`response_id` (21,649 rows, one per response; see
`etc/reports/SOURCE_FORMATS_2026-10.md`). `usageRecord` in `usage.go`
ignores the field today. The parser holds only the last line's entries
(`pending`), so most entries of a response are already released when the
record arrives. The rules:

- **The hold.** The parser holds every entry produced since the previous
  response closed. A `token_usage_record` closes the hold: it sets
  `TurnID` to its `response_id` on every held entry, and usage goes on the
  assistant entry `attachUsage` picks today.
- **Runs of usage records.** The Codex design measured 13 places where two
  `token_usage_record` lines sit together, and today the last one's usage
  wins. A closed hold is released only when the next entry-producing line
  arrives, so every usage record in a run applies to the same hold. The
  last record in the run wins, for usage (as today) and for `TurnID`.
- **Release without a TurnID** at a user message, a turn boundary
  (`turn_context`, `task_started`), a legacy event, or end of file. Older
  rollouts with no usage records emit what they emit today.
- **Twin drop at read time.** The adjacent-twin check (`isItemTwin`, and
  the legacy-event twin) compares each line with the line before it, as it
  is read. Holding a whole response does not change what that comparison
  sees.
- **End of file.** `NextEntry` parses the bytes on disk. At end of file it
  releases the hold without a `TurnID`, as today's `flushPending` does.
  Otherwise a response still being written would vanish from the output. A
  later parse, after the usage record has landed, stamps it; the metadata
  cache already re-reads a file whose mtime changed. No reader in the tree
  waits at the end of a growing Codex file today (`thinkt agents follow`
  uses `parseJSONLLine`, which skips Codex lines because they have no
  top-level `role`). A reader added later that waits there would wait for
  the usage record instead of flushing. That belongs to that reader's
  design, not this one.
- **What the hold delays.** It only matters to a reader of a growing file.
  Entries wait until the usage line is written. On six recent rollouts:
  - 239 times the usage line came straight after the wire call, so the
    call is released at once.
  - 142 times the command item came first, then the usage line. There the
    completed command item sits in the hold until the usage line, the very
    next record. The wire call in front of it is the secondary
    `KindToolWire` twin.

  Visible output is never held for a tool's runtime.
- The item-id shared-prefix idea stays rejected.

**Kimi.** The loop step uuid is already the assistant entry's `UUID`
(`flush` sets it from `openStep.uuid`). The assistant entry and the tool
results inside that step (`resultEntry`, which today copies only the wire
`turnId`) take the open step's uuid as `TurnID`. The chat completion id
(`messageId`, `chatcmpl-*`) is the closer analogue of Claude's
`message.id`, but tool results cannot share it, so it stays in
`Metadata["message_id"]`.

**Antigravity.** Text, thinking and tool calls are blocks on one planner
entry, whose `TurnID` is its step index (the `<idx>` of the `<idx>.<n>`
call ids). The parser already remembers each pending call; it also keeps
the planner index with it, and the paired result entry takes that index as
`TurnID`. An orphan result has an empty `TurnID`. USER_INPUT, system
message and harness entries have an empty `TurnID`. `PromptID` keeps
exactly today's rule, so `TestChildTurnIDBeforeAnyUserInput` becomes a
`PromptID` test with the same expectations ("0" for every pre-USER_INPUT
step).

**Tool results** take the `PromptID` of the prompt they belong to. They
take the `TurnID` of the response that made the call where it can be found:

- **Kimi, Antigravity:** from the paired call, as above.
- **Codex wire outputs** (`function_call_output`, `custom_tool_call_output`)
  and **`McpToolCall` items:** a result arrives after its response has
  closed, so the hold does not cover it. The parser keeps a map from call
  id to response id, filled when a usage record closes a hold with calls in
  it. A wire output carries the same `call_id`. An `McpToolCall` item id
  matched the call id in all 20 sampled cases. Either kind takes the
  response id of its call. An unpaired result, or one whose response had
  no usage record, has an empty `TurnID`.
- **Codex `CommandExecution` items:** these carry their own ids
  (`exec-...`). They never equal the wire `call_id`: 672 sampled pairs
  differed, and the parser keeps wire ids as `wire:<call_id>` for this
  reason. One command item line becomes both the visible `tool_use` and
  its `tool_result`.
  - A command item that arrives while its response's hold is open takes
    the hold's `TurnID`, both halves.
  - One that arrives after the usage record (239 of the 381 sampled
    shapes) keeps an empty `TurnID` on both halves. No field it carries
    joins it to the response.
  - Pairing pending exec calls by order is rejected: 40 sampled responses
    were followed by two command items.
  - The wire output that follows is the secondary twin; it is stamped
    through the call-id map.

Code that changes:

- Codex `usage.go`, `parser.go`, `convert.go`; Kimi `convert.go`,
  `parser.go`; Antigravity `convert.go`, `parser.go`.
- `internal/conversation`: `Message` gains `PromptID`; `html.go` writes
  `data-prompt-id` next to `data-turn-id`.
- Swagger docs (`internal/server/docs`): field descriptions.
- `thinkt.ParserVersion` 5 → 6 (emitted entries change), so the metadata
  cache and index recompute. `TestParserVersionIsFive` moves with it.
- Source READMEs and `etc/reports/SOURCE_FORMATS_2026-10.md` mappings.

Checked: the SQLite schema, the collector and the embedded web assets do
not read `turn_id` or `prompt_id`. The HTML page reads `data-turn-id`.

## 2. Session path resolver

### The interface

A new optional interface in `internal/thinkt`, implemented by each
source's factory:

```go
// SessionPath is what a source can tell about a file from its path.
type SessionPath struct {
    Source    Source
    BaseDir   string // the source home the file sits in, from the path layout; empty if unknown
    SessionID string // the id GetSessionMeta and OpenSession accept
    ProjectID string // set only when it is the store's Project.ID (see below)
}

// SessionPathResolver is implemented by a StoreFactory whose source can
// recognise its own session files.
type SessionPathResolver interface {
    // ResolveSessionPath reports whether path is one of this source's
    // session files, by its path only.
    ResolveSessionPath(path string) (SessionPath, bool)
    // OpenSessionPath opens path as a lazy session.
    OpenSessionPath(ctx context.Context, sp SessionPath, path string) (LazySession, error)
}
```

### Which source wins

Today's predicates overlap: Copilot claims any `events.jsonl` and anything
under its home; Codex, Claude and Qwen each claim every `.jsonl` under
their home; Codex and Qwen also claim `.codex/sessions/**` and
`.qwen/projects/**` anywhere. The TUI switch order picks the winner.
Resolution keeps that order exactly; it does not follow `AllFactories()`.

`internal/sources` gets

```go
func ResolveSessionPath(path string) (thinkt.SessionPath, thinkt.SessionPathResolver, bool)
```

which tries, in this fixed order, first match wins:

1. Kimi
2. Antigravity
3. Codex
4. Copilot
5. Qwen
6. Gemini
7. For a `.jsonl` no one claimed: the content sniff (today's
   `detectJSONLSessionType`, moved from `internal/tui` into
   `internal/sources`) picks Claude, Codex or Qwen from the first records.
   Claude is only ever reached here, as today.

The order is one ordered slice in `internal/sources`, next to
`AllFactories()`, with a comment that it mirrors the old switch. Narrowing
the predicates so they no longer overlap is out of scope: it would change
which source opens some files.

A table test pins the known overlaps to today's winners, at minimum:

| Path | Winner |
|---|---|
| `events.jsonl` under `~/.claude` | Copilot |
| a Codex rollout (`session_meta` first) in `~/.claude/projects/<dir>/` | Codex (sniff) |
| a Claude transcript in `~/.claude/projects/<dir>/` | Claude (sniff) |
| `.jsonl` under the Codex home | Codex |
| `.jsonl` under the Qwen home | Qwen |
| `transcript.jsonl` under the Antigravity home (inside `~/.gemini`) | Antigravity |
| a Kimi `wire.jsonl` outside any home | Kimi |

Each row is checked against the old `OpenLazySession` switch (kept in the
test as the oracle) and against `ResolveSessionPath`; both must name the
same source. When two resolvers would both match, the earlier one in the
list wins; nothing errors.

### What each resolver fills, and how it opens

Each resolver claims what that source's `IsSessionPath` claims today, so
copied sessions keep opening.

| Source | BaseDir | ProjectID | OpenSessionPath |
|---|---|---|---|
| Claude | empty (sniffed) | the project directory (`DirPath`) | `claude.OpenLazySession(path)`, as today |
| Codex | from the layout (`.codex` above `sessions`), else the home | empty: Project.ID is the workspace cwd, not in the path | `NewLazySession` over `codex.NewStore(BaseDir).OpenSession` |
| Kimi | `BaseDirForPath`, else `DefaultDir` | empty: the `wd_*` folder is not the project id | `NewLazySession` over `kimi.NewStore(BaseDir).OpenSession` |
| Antigravity | `BaseDirForPath`, else `DefaultDir` | empty: comes from the summaries database | `NewLazySession` over `antigravity.NewStore(BaseDir).OpenSession` |
| Copilot | today's `findCopilotBaseDir` | empty | `NewLazySession` over `copilot.NewStore(BaseDir).OpenSession` |
| Gemini | today's `findGeminiBaseDir` | the hash directory | `NewLazySession` over `gemini.NewStore(BaseDir).OpenSession` |
| Qwen | from the layout (`.qwen` above `projects`), else empty | the hash directory | `NewLazySession` over `qwen.NewStore(BaseDir).OpenSession`; an empty BaseDir keeps today's `NewStore("")` |

`ProjectID` is set only where it equals the store's `Project.ID`. A test
confirms the Claude, Gemini and Qwen values against each store's
`ListProjects`.

The per-source `IsSessionPath`, `BaseDirForPath`, `SessionIDForPath` and
`DefaultDir` become the resolver's implementation; they stay exported only
where something else uses them.

### Callers

- **TUI loader** (`internal/tui/session_loader.go`): `OpenLazySession`
  calls `sources.ResolveSessionPath` and then `OpenSessionPath`. The
  switch, the per-source `open*LazySession` helpers, `find*BaseDir` and
  the sniff leave the TUI. `OpenLazySessionWithRegistry` keeps
  `StoreRegistry.OpenLazySessionByPath` as its fallback; the server, MCP
  and `internal/target` keep using that registry scan unchanged. A file
  the user names opens even when its source is off (decided with the
  user).
- **Index watcher** (`discoverSession`):
  1. Resolve the path. No match: stop.
  2. Take that source's store from the registry. Not registered
     (disabled): stop.
  3. `GetSessionMeta(SessionID)`. Error: stop.
  4. If the meta's symlink-resolved `FullPath` is not the path, stop.
     This keeps Antigravity's `transcript.jsonl` sibling undiscovered when
     the store reads `transcript_full.jsonl`
     (`TestDiscoverSessionOutsideProjectFolder`).
  5. If the meta has a `ParentSessionID`, stop: `ListSessions` hides
     subagent sessions, so the index has never held them.
  6. With a `ProjectID`, build the entry. Without one, scan that one
     source's projects for the session, as `discoverSession` does today
     but over one store.

  The scan across every store goes away. Each "stop" is a debug log; the
  existing warn-once for unmatched paths stays.
- **Relay** (`detectSource`): the string-contains fallback becomes a
  `ResolveSessionPath` lookup; the deepest-watch-root match stays first.

## 3. Search line filter hook

A second optional factory interface:

```go
// SearchLineFilterer is implemented by a StoreFactory whose session files
// contain lines that are not conversation content.
type SearchLineFilterer interface {
    // NewSearchLineFilter returns a predicate for one file. scanFile
    // calls it once per file and runs it on every line in order,
    // matching or not; it reports whether a hit in the line counts.
    NewSearchLineFilter() func(line []byte) bool
}
```

Claude and Codex implement it by wrapping their stateless `IsSearchable`.
Kimi and Antigravity return their `NewSearchFilter()`. `search.go` drops
the `searchable` and `searchFilters` maps and looks the factory up by
source name. Gemini, Copilot and Qwen do not implement it and keep every
matching line, as now. A source that loses the interface would start
counting bookkeeping lines; the existing search tests for Claude, Codex,
Kimi and Antigravity catch that.

## 4. Record classes and the unknown tally

- `thinkt.RecordClass` with the seven values the Claude, Codex and Kimi
  copies already share (`ClassTurn`, `ClassEvent`, `ClassContext`,
  `ClassCheckpoint`, `ClassSessionState`, `ClassBookkeeping`,
  `ClassUnknown`), the same `String()` output, and `IsEntry()` (identical
  in all three: checkpoint is an entry, session state is not). The three
  packages use it directly and delete their copies. Per-value comments
  that are source-specific move to each source's classifier.
- Antigravity's enum classifies steps (USER_INPUT, PLANNER_RESPONSE, ...;
  its `ClassCheckpoint` means a CHECKPOINT step). It is renamed
  `StepClass` and stays in the package.
- `thinkt.UnknownTally[K ~string]` replaces the four `unknownTally`
  copies, with the same behaviour: count per key, log the first sighting
  of each key at debug level with the source name, `Snapshot()` returns a
  copy or nil. Claude keeps `EntryType` keys through the type parameter.

## Constraints

- Go module `github.com/wethinkt/go-thinkt`; everything stays `internal`.
- `internal/thinkt` must not import `internal/sources` (the reverse
  already holds).
- Existing tests keep passing, apart from the two known iTerm failures in
  `internal/cmd` (`TestAppsList_JSON`, `TestSetTerminalRoundTrip`).

## Order

1. Record classes and tally (mechanical, no behaviour change).
2. Search hook (no behaviour change; existing search tests cover it).
3. Path resolver, with the overlap table test, then the TUI loader,
   index watcher and relay callers.
4. TurnID/PromptID, with the ParserVersion bump and docs.

## Left to the builder

- Exact file names for the new `internal/thinkt` code.
- How each resolver handles symlinked homes (the index watcher already
  resolves symlinks; resolvers should compare against resolved homes).
- Further rows for the overlap table beyond the minimum listed.
