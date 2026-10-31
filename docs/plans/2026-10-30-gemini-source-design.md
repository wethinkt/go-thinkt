# Gemini CLI source: the JSONL session format

Date: 2026-10-30. Branch `worktree-gemini-source`, stacked on
`worktree-shared-model`. Fourth source pass after Codex, Kimi and
Antigravity, and the first built on the shared pieces from the
shared-model cleanup (`thinkt.RecordClass`, `thinkt.UnknownTally`,
`thinkt.SessionPathResolver`, `thinkt.SearchLineFilterer`, the
`TurnID`/`PromptID` meanings).

## Why

Gemini CLI (0.39 and later) writes each session as an append-only `.jsonl`
file. thinkt only reads the older single-document `.json` sessions, so
every session since the switch is invisible: all five on this Mac
(17 May) and the seven newest on darkstar (4–23 May). Projects are also
wrong for the sessions thinkt does show:

- the name is the first prompt ("/rewind", "/model")
- the path is the `~/.gemini/tmp/...` folder, not the repository
- a project whose storage folder was renamed from its hash to its slug
  shows up twice

## Evidence

Two corpora were measured:

- **This Mac:** `~/.gemini/tmp`, Gemini CLI 0.42.0. Five `.jsonl` files
  (four main sessions and one subagent), about 400 lines.
- **darkstar:** a copy of `~/.gemini/tmp` and `projects.json`, kept in
  this session's scratch folder. It holds 7 `.jsonl` files and 35 `.json`
  files.
  - 13 of the `.json` files are under hash folders; 22 are under slug
    folders.
  - No session id appears in both formats.

The `.jsonl` facts below come from these files and from the reconstruction
the current Gemini CLI loader applies; the loader behaviour was supplied in
review. Where the CLI writes a record type that is not in these files,
that is said.

## Goals

- Every `.jsonl` session lists, opens, searches, indexes and relays, main
  and subagent, with the transcript the CLI itself would rebuild.
- Projects carry the real repository path, a readable name, and one
  identity across the hash-to-slug folder rename.
- Notices (cancellations, auth, warnings, errors) are kept as secondary
  entries, visible with "Other".
- `TurnID`/`PromptID` follow the shared meanings.
- Usage carries cached and thinking tokens.

## Non-goals

- The old `.json` reader stays as it is (decided with the user). It
  changes in only two ways:
  - It skips a `.json` file that a `.jsonl` file supersedes (section 5).
  - It maps usage the same way as the new reader, including cached and
    thinking tokens. The user wants thinking and reasoning tracked wherever
    the data has it.
- `logs.json` (the prompt log), `config/projects/*.json` (Antigravity's)
  and `memoryScratchpad` are not read.
- No migration or rewriting of Gemini's files; reads only.

## Anti-goals

- Showing turns the user rewound, or the pre-patch text of an edited
  message.
- A transcript in a different order than the CLI shows.
- Showing a tool result twice, once inline and once as a later
  `functionResponse` message.
- Search hits in lines the session no longer contains.
- Counting a migrated session twice.

## 1. File layout and identity

```
~/.gemini/
├── projects.json                        # {"projects": {"<abs path>": "<slug>"}}
└── tmp/
    ├── <slug>/                          # renamed from <sha256(path)> when the project is opened
    │   ├── .project_root                # the absolute project path
    │   └── chats/
    │       ├── session-<minute>-<8 hex>.jsonl       # main session (a numeric infix when taken)
    │       └── <parent-session-id>/<agentId>.jsonl  # subagent session
    └── <sha256(path)>/                  # a folder the CLI has not migrated yet
```

- **Session ids.** The header's `sessionId` is a UUID for main sessions
  (e.g. `92szal` for a subagent). It is not unique per file. On darkstar,
  two `.json` files in one folder share an id with different content (a
  resume in older CLIs wrote a new file). The CLI's resume and subagent
  links use it, but it cannot be the index key.
  - `SessionMeta.ID` is the filename stem for a main `.jsonl` session, the
    same rule the `.json` reader already uses. It is unique per folder,
    and unique across the project after the folder dedupe below.
  - A subagent's ID is `<parent stem>/<agentId>`, scoped by its parent as
    in Kimi.
  - The header `sessionId` is kept in `Metadata["session_id"]` for resume
    and for subagent links. (My call: the index's `sessions.id` is a
    primary key, and two files with one `sessionId` would overwrite each
    other.)
- **Project identity:** `Project.ID` is the header's `projectHash`
  (`sha256` of the project path). It is the same for an unmigrated hash
  folder and a slug folder, so both are one project.
  - `Project.Path` is the real path. It comes from `.project_root`, or
    else from the `projects.json` key whose `sha256` equals the hash,
    or else from the `.project_root` text hashed the same way. When none
    match (four of darkstar's hash folders), `Path` is empty and the
    project shows as `gemini://<short hash>`, as today.
  - `PathExists` then refers to the repository.
  - `Name` is the base name of the path, else the slug, else the short
    hash. It is never a prompt.
  - The slug is not always the folder name: it is lowercased, punctuation
    becomes hyphens, and a collision gets `-2`. So the slug is only a
    fallback name.
- **Old `.json` sessions** list under the same `projectHash` identity as
  well. Their files carry `projectHash` at the top, so this changes only
  which project they join, not how they are read. (My call: without it, a
  project with both formats would split again.)
- **One session in two folders.** When a hash folder and a slug folder
  both hold a file with the same name (darkstar has byte-identical copies
  of `session-2026-02-06T13-11-dac0f3bc.json` in `933dc8…/` and
  `go-thinkt/`), the project lists it once. The slug folder's copy wins;
  if the copies differ, the newer `lastUpdated` wins. The resolver and
  watcher treat the losing path as not a session. (My call.)

## 2. Reading a `.jsonl` session

Line kinds:

| Line | Meaning |
|---|---|
| header (first line, no `type`): `sessionId`, `projectHash`, `startTime`, `lastUpdated`, `kind` (`main`/`subagent`), `directories` on subagents | session state; never an entry |
| `type: user` | user message; `content` is a part list (`[{text}]` in every file seen; `functionResponse` parts possible in newer CLIs) |
| `type: gemini` | one model response: `content` string, `thoughts[{subject, description, timestamp}]`, `tokens`, `model`, `toolCalls[]` |
| `type: info` / `warning` / `error` | notice |
| `$set` | metadata update: `lastUpdated`, `summary`, `memoryScratchpad`; or, when it carries `messages`, a checkpoint replacing the whole message list |
| `$patch` | in-place edit of one message (content or a tool result); can delete or reorder ids |
| `$rewindTo` | delete that message and everything after it; an id not present deletes every message |

`$patch`, `$rewindTo` and `$set.messages` do not appear in the measured
files. The current CLI loader applies them, so the reader must too.

**Reconstruction.** The reader replays the file into an ordered message
list keyed by id, the way the CLI loader does:

- A message line with a new id is appended. A line with an id already
  present replaces that message in place: the position of the first copy
  is kept. 113 of 122 ids on this Mac appear twice, two lines apart; the
  second copy adds `toolCalls`. Nine appear once.
- `$patch` edits, deletes or reorders as it says.
- `$rewindTo` truncates.
- `$set.messages` replaces the list.
- Entries are produced from the final list.

The whole file is read before anything is emitted. That is the cost of
a format whose later lines can delete earlier ones. These sessions are
small (the largest on darkstar is a few hundred lines). (My call. A
streaming reader would have to retract entries it had already emitted.)

**Entries from the final list:**

- **`user`:** a user entry with the joined text parts.
  - A continuation the CLI injects (text exactly `Please continue.`, and
    the loop-recovery and `[System: …]` nudges) is a secondary
    `KindMeta` entry. It does not open a new `PromptID`. Its
    `displayContent`, which repeats the original turn, is not shown.
  - A user message made only of `functionResponse` parts is not a
    prompt; see tools below.
- **`gemini`:** one assistant entry.
  - It carries text, a `thinking` block per thought ("subject:
    description"), `Model` and `Usage`.
  - A `functionCall` or thought stored inside `content` (newer CLIs) is
    folded into the same entry, not emitted twice.
- **Tool calls:** each becomes a `tool_use` block on the assistant entry
  and a paired `tool_result` entry right after it, carrying the call id.
  Twenty-two messages here have more than one call.
  - The result body is `result[].functionResponse.response.output` on
    success, or `.error` on error (`status: error`, three here); errors
    are marked.
  - `resultDisplay` is a UI value and is not the result.
  - When the same result also appears as a later user message of
    `functionResponse` parts, that message is dropped.
  - `update_topic` calls (21 here, a running topic with `title`,
    `summary` and `strategic_intent`) are secondary. (My call: they are
    the CLI's status line, not work.)
- **`info` / `warning` / `error`:** secondary system entries.
  `error` sets `Entry.Error`.
- **Ids:**
  - `UUID` is the message id. A tool result is `<call id>:result`, as
    in the other sources.
  - `TurnID` is the `gemini` message id; its paired results share it.
  - `PromptID` is the id of the user message that opened the exchange.
  - Notices have no `TurnID`.
- **Usage:** Gemini's `output` excludes thinking
  (`total = input + output + thoughts` in every sample), while thinkt
  treats `ThinkingTokens` as part of `OutputTokens` (Claude, Codex).
  `input` already includes `cached`, as in Codex.
  - `InputTokens` = `input`.
  - `CacheReadInputTokens` = `cached`.
  - `OutputTokens` = `output + thoughts`.
  - `ThinkingTokens` = `thoughts`.
  - `tool` and `total` have no field and are dropped.
  - Otherwise session cost and usage totals would undercount output
    whenever the model thinks.
  - The `.json` reader uses the same mapping (decided with the user: track
    thinking and reasoning wherever the data has it). One shared helper
    serves both readers.
- **Unknown line types** are counted with `thinkt.UnknownTally` and
  emitted as secondary `KindUnknown` entries, as in the other sources.
  The header classifies as session state, so it is never "unknown".

**Classification** uses `thinkt.RecordClass`:

| Lines | Class |
|---|---|
| user, gemini | Turn |
| info, warning, error | Event |
| header, `$set` (metadata) | SessionState |
| `$patch`, `$rewindTo`, `$set.messages` | Bookkeeping (applied, never an entry) |
| anything else | Unknown |

## 3. Session metadata

| `SessionMeta` field | Source |
|---|---|
| `ID` | header `sessionId` |
| `CreatedAt` | header `startTime` |
| `ModifiedAt` | last `lastUpdated` (header or `$set`), else file mtime |
| `Title` | `$set.summary` when it is a single line that is not JSON, else empty |
| `FirstPrompt` | the first user text that the CLI would accept as resumable |
| `Model` | the last `gemini` message's model |
| `EntryCount`, cost/usage totals | from the reconstruction |
| `ParentSessionID` | for `kind: subagent`, the parent folder name (`chats/<parent-id>/`) |
| `ProjectPath` | the project's real path (section 1) |
| `RelatedPaths` | not filled (a later item for all sources) |

- **Title:** two of five files carry a summary; on the subagent it is a
  JSON document of file locations, which is not a title. Titles from
  `update_topic` change through a session and are not used. (My call.)
- **FirstPrompt:** the CLI refuses empty text, slash commands, `?…`,
  `<session_context>` and `<hook_context>` as resumable, so those are
  skipped.
- Metadata goes through the existing metadata cache with the new
  `ParserVersion`.

## 4. Subagents

- An `invoke_agent` tool call carries `agentId` (e.g. `92szal`), the
  child's session id. The link field is `agentId`, not the call id and
  not `args.agent_name`.
  - The child is `chats/<this session id>/<agentId>.jsonl`.
  - `ChildSessionID` is set on the `tool_use` block, which is where
    `conversation.Build` reads it.
- Child sessions are listed only with `thinkt.WithSubagents()`. The
  parent's `SubagentCount` counts them, as in Codex and Antigravity.

## 5. Superseded `.json` files

Gemini CLI converts a `.json` session to `.jsonl` when it is resumed. It
writes `<name>.jsonl` beside `<name>.json` and leaves the old file.

- A `.json` file is skipped when a `.jsonl` file in the same project has
  the same `sessionId` and contains the `.json` file's last message id.
  The conversion carries the old history, so a file whose last message is
  in the `.jsonl` is that file's converted copy. Another `.json` with the
  same `sessionId` but a different branch of the conversation stays
  listed. (My call: the darkstar collisions show that one `sessionId` can
  name several files.)
- Comparing the stem alone is not enough: a new `session-….jsonl` beside
  an older file has a different name.

## 6. Plumbing

- **Resolver** (`gemini/pathresolve.go`):
  - It claims `.jsonl` files under `~/.gemini/tmp/*/chats/` and
    `chats/<id>/`, plus `.json` files under `chats/` only. Today it
    accepts any `.json` under the home, including `logs.json`.
  - It keeps the Antigravity exclusion. Antigravity is still tried first
    in the fixed order.
  - `ProjectID` is the `projectHash`. It is read from the header when
    the resolver may read the first line; otherwise it is left empty and
    the watcher scans this one source. (My call: a header read is one
    short line, and it is the only way to get the hash for a slug
    folder.)
  - `SessionID` is the path, in home spelling.
  - The old "parent named `chats`" rule is replaced: a subagent's parent
    folder is the parent session id, so the project folder is one more
    level up.
- **Watching:** `WatchConfig` reaches `tmp/<p>/chats/<id>/`, which is
  depth 4 from the home (today it is `MaxDepth: 2`, so the relay sees no
  session file). The index watcher must also see subagent folders, which
  `ListSessions` does not return as session directories.
- **Search:** the `SearchLineFilterer` cannot decide on the spot for a
  file whose later lines delete or replace earlier ones, and it is never
  given the file. A second optional interface is added in `thinkt`,
  `SearchFileFilterer { NewSearchFileFilter(path string) func(line []byte,
  matched bool) bool }`. `scanFile` prefers it over `SearchLineFilterer`.
  Claude, Codex, Kimi and Antigravity keep the line interface unchanged.
  Gemini's file filter replays the file once, through the same
  reconstruction, before the first line is scanned.
  - It keeps a hit only in a line whose message survives, using the
    final copy of each id.
  - It drops the earlier copy of a repeated id, even though the later
    copy is the one that carries the tool calls.
  - It drops `$set`, `$patch`, `$rewindTo`, header and `memoryScratchpad`
    lines. A `$set.messages` checkpoint counts as the live transcript.
  - (My call: an additive interface leaves the other sources alone and
    gives search the same transcript the reader shows.)
- **Resume:** add `ResumeCommand`: `gemini --resume <sessionId>`, run in
  the project path. A subagent resumes its parent.
- **`ParserVersion`** goes from 6 to 7.

## Constraints

- Go, `internal` packages, existing store and cache patterns of the other
  sources.
- Fixtures come from the measured files, with content scrubbed. Synthetic
  fixtures cover `$patch`, `$rewindTo` and `$set.messages`, since no
  measured file has them.
- The existing `.json` tests keep passing unchanged apart from project
  identity and naming.

## Left to the builder

- File split inside `internal/sources/gemini/`. Follow the other sources:
  `classify.go`, `replay.go`, `convert.go`, `parser.go`, `store.go`.
- The exact `$patch` operation schema. Read it from the CLI loader
  source, or ask, before writing that part.
- How `.project_root` and `projects.json` are cached per store.
