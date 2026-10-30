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
- Path resolution that depends on the order sources are registered in.
- Listing, searching or serving a source that is turned off (see the
  source visibility change). Resolvers decide from the path alone. One
  exception, decided with the user: a session file the user names
  explicitly (TUI or `thinkt sessions view <path>`) opens even when its
  source is off, as it does today. The index watcher and relay only use
  registered (enabled) sources.

## 1. TurnID and PromptID

Decided with the user:

- **`PromptID`**: one user prompt and everything the agent does until it
  hands control back. Claude's `promptId` already means this.
- **`TurnID`**: one model response. Claude's `message.id` already means this.

Per source:

| Source | PromptID | TurnID |
|---|---|---|
| Claude | `promptId` (unchanged) | `message.id` (unchanged) |
| Codex | the rollout `turn_id` (moved from TurnID) | empty (see below) |
| Kimi | the wire `turnId` (moved from TurnID) | the loop step uuid of the step the entry came from |
| Antigravity | index of the opening USER_INPUT step, or of the subagent task-prompt step (moved from TurnID) | index of the model-turn step, shared by its text, thinking and tool-call entries (`<idx>` of the `<idx>.<n>` ids) |
| Gemini, Copilot, Qwen | unchanged (none set today) | unchanged |

Codex rollouts have no response id. Item ids from one response share a
prefix, but that is not a documented id, so TurnID stays empty for Codex.
(My call; cheap to revisit if Codex starts writing one.)

Tool results take the ids of the prompt they belong to. They take the
TurnID of the response that made the call where the source pairs them
(Antigravity, Kimi); otherwise TurnID is empty.

Code that changes:

- `internal/thinkt/types.go`: field comments state the two meanings above.
- Codex `convert.go`, Kimi `convert.go`/`parser.go`, Antigravity
  `convert.go`: write the per-prompt value to `PromptID`, and the
  per-response value to `TurnID` where the table says so.
- `internal/conversation`: `Message` gains `PromptID`; `html.go` writes
  `data-prompt-id` next to `data-turn-id`.
- Swagger docs (`internal/server/docs`): field descriptions.
- `thinkt.ParserVersion` 5 → 6 (emitted entries change), so the metadata
  cache and index recompute.
- Source READMEs and `etc/reports/SOURCE_FORMATS_2026-10.md` mappings.

Checked: neither web bundle nor the SQLite index or collector reads
`turn_id` or `prompt_id`, so nothing downstream breaks.

## 2. Session path resolver

A new optional interface in `internal/thinkt`, implemented by each
source's factory:

```go
// SessionPath is what a source can tell about a file from its path alone.
type SessionPath struct {
    Source    Source
    BaseDir   string // the source home the file sits in, read from the path layout
    SessionID string // the id GetSessionMeta and OpenSession accept
    ProjectID string // the project's ID when the path alone gives it; else empty
}

// SessionPathResolver is implemented by a StoreFactory whose source can
// recognise its own session files.
type SessionPathResolver interface {
    // ResolveSessionPath reports whether path is one of this source's
    // session files. It looks at the path only (no file reads, no store).
    ResolveSessionPath(path string) (SessionPath, bool)
    // OpenSessionPath opens path through a store rooted at its BaseDir.
    OpenSessionPath(ctx context.Context, path string) (SessionReader, error)
}
```

`internal/sources` gets `ResolveSessionPath(path) (SessionPath, thinkt.StoreFactory, bool)`,
which asks every factory in `AllFactories()`.

Rules:

- Each resolver claims exactly what that source's `IsSessionPath` claims
  today, so copied sessions keep opening. Kimi, Copilot, Codex and Qwen
  match their folder layout anywhere on disk; Claude, Gemini and
  Antigravity check their home. `BaseDir` comes from the layout, not the
  configured home. (My call, from "same output".)
- `ProjectID` is filled where the project is in the path: Claude (the
  project directory), Gemini and Qwen (the hash directory), Kimi and
  Codex where their ids allow it. Antigravity leaves it empty: its
  project comes from the summaries database. (My call.)

- Resolvers are disjoint: at most one source claims a path. A table test
  in `internal/sources` runs every source's fixture paths through every
  resolver and fails if two claim one. Gemini already excludes the
  Antigravity home. The same check covers Kimi vs Codex.
- The existing per-source `IsSessionPath`, `BaseDirForPath`,
  `SessionIDForPath` and `DefaultDir` become the resolver's
  implementation. They stay exported only where something else in the
  source uses them.

Callers:

- **TUI loader** (`internal/tui/session_loader.go`): `OpenLazySession`
  asks `sources.ResolveSessionPath` and calls `OpenSessionPath`. The
  per-source `open*LazySession` helpers and `find*BaseDir` go away.
  The content sniff (`detectJSONLSessionType`) stays as the last
  fallback for `.jsonl` files outside any known home (copied files).
- **Index watcher** (`discoverSession`): resolve the path, take the store
  for that source from the registry (only registered, so only enabled,
  sources), and call `GetSessionMeta(SessionID)`. With a `ProjectID` the
  entry is built directly. Without one, the existing scan runs over that
  source's projects only. If no resolver claims the path, or the meta
  lookup fails, the current all-projects scan runs as before.
  Sessions whose meta has a `ParentSessionID` are skipped: the index
  does not hold subagent sessions today (sync never lists them), and
  this pass does not change that. (My call.)
- **Relay** (`detectSource`): the string-contains fallback becomes a
  resolver lookup; the deepest-watch-root match stays first.

## 3. Search line filter hook

A second optional factory interface:

```go
// SearchLineFilterer is implemented by a StoreFactory whose session files
// contain lines that are not conversation content.
type SearchLineFilterer interface {
    // NewSearchLineFilter returns a predicate for one file. It sees every
    // line of that file in order and reports whether a hit in the line counts.
    NewSearchLineFilter() func(line []byte) bool
}
```

Claude and Codex wrap their stateless `IsSearchable`. Kimi and
Antigravity return their `NewSearchFilter()`. `search.go` drops the
`searchable` and `searchFilters` maps and looks the factory up by source
name. Sources without the interface keep every matching line, as now.

## 4. Record classes and the unknown tally

- `thinkt.RecordClass` with the seven values the Claude, Codex and Kimi
  copies already share (`ClassTurn`, `ClassEvent`, `ClassContext`,
  `ClassCheckpoint`, `ClassSessionState`, `ClassBookkeeping`,
  `ClassUnknown`) and the same `String()` output. The three packages
  use it directly and delete their copies. Per-value comments that are
  source-specific move to each source's classifier.
- Antigravity's enum is not the same thing: it classifies steps
  (user input, model turn, tool result, harness, ...). It is renamed
  `StepClass` and stays in the package. (My call: forcing it onto the
  shared seven would lose the distinctions its parser branches on.)
- `thinkt.UnknownTally[K ~string]` replaces the four `unknownTally`
  copies, with the same behaviour: count per key, log the first sighting
  of each key at debug level with the source name, `Snapshot()` returns
  a copy or nil. Claude keeps `EntryType` keys through the type parameter.

## Constraints

- Go module `github.com/wethinkt/go-thinkt`; everything stays `internal`.
- `internal/thinkt` must not import `internal/sources` (the reverse
  already holds).
- Existing tests keep passing, apart from the two known iTerm failures in
  `internal/cmd` (`TestAppsList_JSON`, `TestSetTerminalRoundTrip`).

## Order

1. Record classes and tally (mechanical, no behaviour change).
2. Search hook (no behaviour change; existing search tests cover it).
3. Path resolver, with the disjointness test, then the TUI loader,
   index watcher and relay callers.
4. TurnID/PromptID, with the ParserVersion bump and docs.

## Left to the builder

- Exact file names for the new `internal/thinkt` code.
- How each resolver handles symlinked homes (the index watcher already
  resolves symlinks; resolvers should compare against resolved homes).
- Test fixture paths per source for the disjointness test, including
  copied-layout paths outside any home (Copilot's any-`events.jsonl`
  rule is the broadest).
