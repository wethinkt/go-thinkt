# Antigravity source: reading `~/.gemini/antigravity-cli` with the conversation model

Date: 2026-10-29. Fourth pass of the "update how we read the agent objects"
work, after Claude (`worktree-conversation-model`), Codex
(`worktree-codex-source`) and Kimi Code (`worktree-kimi-source`). Builds on
all three; the branch is `worktree-antigravity-source`, stacked on the Kimi
branch, pushed, no PR.

## Intent

thinkt does not read Antigravity at all. Antigravity (`agy`) is the agent
Evan uses most right now: 112 conversations, the newest written today. It
lives under `~/.gemini` but is a different product from Gemini CLI and shares
nothing with it except the parent directory.

This pass adds a new `antigravity` source that reads Antigravity's
step transcripts with the same conversation model the earlier passes
established: every line classified before parsing, one assistant entry per
model turn with its usage, tool results paired with their calls, harness
steps kept but hidden, session state folded into `SessionMeta`, and
subagents as sessions linked to their parent and hidden by default.

## Evidence that shapes the design

Measured on this machine, 2026-10-29 (scripts in the session scratchpad, not
committed). Format unchanged from the 2026-10-26 survey in
`etc/reports/SOURCE_FORMATS_2026-10.md`.

- Layout: `brain/<conversation-id>/.system_generated/logs/transcript.jsonl`
  and `transcript_full.jsonl`. 112 brain dirs; 105 have `transcript.jsonl`,
  97 also have `transcript_full.jsonl` with the same line count and
  untruncated content. The 7 without a transcript are empty conversations
  (summary `step_count` 0 or no summary row).
- One JSON object per step: `{step_index, source: USER_EXPLICIT|MODEL|SYSTEM,
  type, status: DONE|ERROR|RUNNING|INVALID, created_at (RFC 3339 UTC),
  content?, error?, media?}`. 42,839 lines, none malformed. `step_index`
  is strictly increasing with 217 gaps (steps the transcript omits).
- Step types (count): `PLANNER_RESPONSE` 19,074; `GENERIC` 10,878;
  `VIEW_FILE` 4,032; `RUN_COMMAND` 2,151; `CODE_ACTION` 1,883;
  `GREP_SEARCH` 1,766; `USER_INPUT` 1,043; `LIST_DIRECTORY` 659;
  `SEARCH_WEB` 541; `SYSTEM_MESSAGE` 274; `ERROR_MESSAGE` 161;
  `CONVERSATION_HISTORY` 149 (content on only 22); `EPHEMERAL_MESSAGE` 92;
  `CHECKPOINT` 76; `READ_URL_CONTENT` 53; `ASK_QUESTION` 7.
- `PLANNER_RESPONSE`: `content` (text, 9,666), `thinking` (5,969),
  `tool_calls[{name, args}]` (17,929), `input_tokens` / `cache_read_tokens`
  / `output_tokens` (7,184 of 19,074). Tool calls have no id.
- Tool results are the steps of the nine result types (`GENERIC`,
  `VIEW_FILE`, `RUN_COMMAND`, `CODE_ACTION`, `GREP_SEARCH`,
  `LIST_DIRECTORY`, `SEARCH_WEB`, `READ_URL_CONTENT`, `ASK_QUESTION`).
  Pairing each result with the oldest unanswered call of the preceding
  `PLANNER_RESPONSE` matches all 21,970 results; 0 results have no call.
  86 calls never get a result: 83 are superseded by the next
  `PLANNER_RESPONSE`, 3 trail the file; the steps between such a call and
  the next response are `ERROR_MESSAGE` 76, `USER_INPUT` 7,
  `SYSTEM_MESSAGE` 3. The result type depends on the tool and is not
  1:1 (`view_file` → `VIEW_FILE` 4,032 or `GENERIC` 2,706), so pairing is
  by order, never by type.
- Result `content` starts with `Created At: …` and `Completed At: …` header
  lines; errors carry `status: ERROR` and an `error` string (360 `GENERIC`).
- `USER_INPUT`: `<USER_REQUEST>…</USER_REQUEST>`, then optional
  `<ADDITIONAL_METADATA>` (local time) and `<USER_SETTINGS_CHANGE>` blocks.
  The model name appears only in a settings change ("changed setting
  `Model Selection` from None to Gemini 4 Argon (High)"); no step records
  the model otherwise. `settings.json` holds the current model.
- `SYSTEM_MESSAGE` content is a `<SYSTEM_MESSAGE>` block carrying
  `[Message] timestamp=… sender=<id> priority=… content=…`. Senders seen:
  `<conversation>/task-N` (a background task finished), a subagent's
  conversation id (a subagent reporting to its parent), and, as the first
  step of a subagent conversation, the parent's conversation id (the task
  prompt).
- `CHECKPOINT` content starts `# Resuming from a compaction` and holds the
  compaction summary. `ERROR_MESSAGE` always has `error` ("The stream was
  interrupted…"), `content` only on 83 of 161.
- `conversation_summaries.db` (SQLite, read with `mode=ro` or
  `immutable=1`; Antigravity holds it open): 110 rows,
  `conversation_id, title (20 non-empty), preview, step_count,
  last_modified_time, workspace_uris (JSON array of file:// URIs: 1 entry
  45, 2 entries 11, empty 54), agent_name, parent_conversation_id,
  nesting_depth, last_user_input_time, …`. Every summary row with a
  transcript has a brain dir; 2 brain dirs have no row (both empty).
- Of the 54 conversations with no workspace (49 from May, 5 from June),
  47 recover one from the most common `run_command` `Cwd` argument, 2 from
  `cache/last_conversations.json` (workspace path → latest conversation
  id), and 5 have none.
- Subagents: 5 rows have `agent_name: research`, a `parent_conversation_id`,
  `nesting_depth: 1`, and their own transcript. The parent's
  `invoke_subagent` result (`GENERIC`) contains
  `Created the following subagents:` followed by JSON objects with
  `conversationId` and `logAbsoluteUri`; one call can create several.
- Resume: `agy --conversation <id>`, run in the workspace directory.
- The gemini source's `IsSessionPath` returns true for any `.json` file
  under `~/.gemini`, which includes Antigravity's `*.metadata.json`,
  `settings.json` and `cache/*.json`.

## Goals

1. A new `antigravity` source (`internal/sources/antigravity`) reading
   `~/.gemini/antigravity-cli` (override `THINKT_ANTIGRAVITY_HOME`).
   Projects are workspaces keyed by path; sessions are conversations keyed
   by conversation id; the session file is `transcript_full.jsonl`, or
   `transcript.jsonl` when there is no full file.
2. Classify every transcript line by `type` before parsing. Emit one user
   entry per typed prompt, one assistant entry per `PLANNER_RESPONSE`
   (thinking, text, tool_use blocks, usage), and one tool_result entry per
   result step paired by order with its call.
3. Keep harness steps (system messages, ephemeral messages, injected
   history, API errors) as secondary entries; show compaction checkpoints
   as compaction summaries.
4. Fold session state into `SessionMeta`: title, first and last prompt,
   created and modified times, model, entry count, token totals, workspace,
   parent.
5. List subagent conversations as sessions with `ParentSessionID`, hidden
   unless `WithSubagents()`, counted on the parent; link the parent's
   `invoke_subagent` call to the child through `ChildSessionID`.
6. Register the source everywhere a source is registered, with a search
   filter, a resume command, active-agent detection for `agy`, and a TUI
   path opener.
7. Fix the gemini source so it no longer claims Antigravity's files.

## Non-goals

- Agent documents in the brain dir (`implementation_plan.md`,
  `walkthrough.md`, `task.md`, … with `*.metadata.json`). Evan wants these
  tracked as they evolve, possibly typeset; that is its own roadmap item in
  `PLAN.md`, not part of this pass. Their content still appears wherever the
  agent wrote it through a tool call.
- `conversations/<id>.db` and `.pb` (protobuf without a published schema),
  `history.jsonl`, `knowledge/`, `annotations/`, `implicit/`, and the
  `steps/<n>/output.txt` files that hold the untruncated output of a
  truncated task result.
- Refreshing the Gemini CLI source's format (7 sessions, all from May).
  Only its path boundary changes here.
- New `thinkt` types or kinds.

## Anti-goals

- A step appearing twice because both transcript files were read.
- A tool result attached to the wrong call.
- Subagent conversations crowding the project list or the index.
- An entry with empty text and no blocks (most `CONVERSATION_HISTORY`
  steps are empty).
- thinkt writing to, locking, or blocking Antigravity's SQLite database.

## Constraints

- Go 1.26; `github.com/mattn/go-sqlite3`, already a dependency, for the
  summaries db. No new dependencies.
- The parser is a streaming `NextEntry` reader over
  `thinkt.NewScannerWithMaxCapacityCustom`, like the Kimi and Codex parsers.
- Classification lives in one function used by the parser, the session
  scan, and the search filter.
- Tool pair halves never share a UUID; the index upserts on
  `(session_id, uuid)`.
- Package layout follows `internal/sources/kimi`: `classify.go`,
  `convert.go`, `parser.go`, `sessionstate.go`, `store.go`, `discovery.go`,
  `resume.go`, `unknown.go`, `README.md`, fixtures in `testdata/`.
- `thinkt.ParserVersion` stays 5. A new source has no cached metadata or
  index rows to invalidate, and no existing source changes what it emits.

## Design

### A. Classification (`classify.go`)

`ClassifyLine(line []byte) (stepHead, RecordClass)` decodes only the step
head (`step_index`, `source`, `type`, `status`, `created_at`, and the
presence of `content`/`error`). Classes:

| class | types |
|---|---|
| user input | `USER_INPUT` |
| model turn | `PLANNER_RESPONSE` |
| tool result | the nine result types |
| system message | `SYSTEM_MESSAGE` (sub-classified by sender, see B) |
| harness | `EPHEMERAL_MESSAGE`, `CONVERSATION_HISTORY` |
| checkpoint | `CHECKPOINT` |
| api error | `ERROR_MESSAGE` |
| unknown | anything else |

### B. Entry conversion (`convert.go`)

Every entry: `UUID = <conversation-id>:<step_index>`, `Timestamp =
created_at`, `TurnID` = the step index of the most recent `USER_INPUT`
(steps before the first one take the first step's index), `Metadata`
carries `step_type` and `status`.

- **`USER_INPUT`** → `RoleUser`, `KindNone`, visible. Text is the inside of
  `<USER_REQUEST>`; without the wrapper, the whole content.
  `<ADDITIONAL_METADATA>` and `<USER_SETTINGS_CHANGE>` text go to
  `Metadata`. A `Model Selection` change sets the model used for later
  assistant entries in the file.
- **`PLANNER_RESPONSE`** → `RoleAssistant` with, in order, a thinking block,
  a text block, and one tool_use block per call. Tool-use ids are
  synthesized as `<step_index>.<n>` (n from 0). `Usage` from
  `input_tokens`, `cache_read_tokens`, `output_tokens` when present.
  `Model` from the latest settings change, empty otherwise. A response with
  no text, thinking or calls emits nothing.
- **Result steps** → `RoleUser` with one tool_result block whose
  `ToolUseID` is the oldest unanswered call; `IsError` when `status` is
  `ERROR` or `error` is set. The `Created At` / `Completed At` header lines
  are stripped from the text and their times kept in `Metadata`. 213 result
  steps stay `RUNNING` (background tasks started, finished later through a
  task notification); they pair like any other result. A result
  with no unanswered call becomes a `KindUnknown` secondary entry carrying
  its text (never observed). A new `PLANNER_RESPONSE` drops the previous
  response's unanswered calls from the queue; they stay unanswered.
- **`invoke_subagent` results**: parse the `conversationId` values after
  `Created the following subagents:`. The first becomes the call's
  `ChildSessionID` (on the tool_use block, as Codex and Kimi do); all go in
  the result's `Metadata["child_session_ids"]`.
- **`SYSTEM_MESSAGE`** by sender: a background task (`<id>/task-N`) →
  `KindTaskNotification`; a subagent's conversation id → `KindAgentMessage`;
  in a subagent conversation, the first message from its parent is the task
  prompt and is presented the way Kimi presents a subagent's task prompt;
  anything else → `KindMeta`. Visibility follows the Kimi pass for each
  kind.
- **`EPHEMERAL_MESSAGE`**, **`CONVERSATION_HISTORY`** with content →
  `RoleSystem`, `KindMeta`, secondary. Empty ones emit nothing.
- **`CHECKPOINT`** → `KindCompactSummary` with the summary text, visibility
  as Claude's compact summaries.
- **`ERROR_MESSAGE`** → `KindAPIError`, secondary, text from `error`.
- **Unknown types** → `KindUnknown`, secondary, logged once per type
  (`unknown.go`, as in Kimi).

### C. Streaming parser (`parser.go`)

Reads one transcript file line by line, holding the queue of unanswered
tool-use ids, the current turn id, and the current model. No look-ahead and
no buffering beyond that queue. A partial last line (the file is written
live) is skipped, not fatal.

### D. Session scan and state (`sessionstate.go`, `store.go`, `discovery.go`)

- **Listing** starts from the brain dirs that have a transcript, joined to
  the summaries db by conversation id. If the db is missing or cannot be
  opened, list from brain dirs alone (titles from the first prompt, times
  from the transcript). The db is opened read-only, never with a write
  lock; the open must not wait on Antigravity.
- **Workspace** (project): one thinkt project per folder, keyed by path,
  like the Claude, Codex and Kimi sources. The folder is the first
  `workspace_uris` entry: the directory `agy` was started in. Further
  entries are folders added to the workspace and go to
  `Metadata`/`SessionMeta` only, never to a second project. Without a
  workspace: the most common `run_command` `Cwd` in the transcript (the
  value is sometimes JSON-quoted; unquote it); else the workspace whose
  `last_conversations.json` entry names this conversation; else one
  project for conversations with no workspace. Antigravity's own
  `project_id` column and `cache/projects.json` are not used for grouping:
  one Antigravity project spans unrelated repos (`ds4dg` and `ds4-go`,
  which Evan confirmed are different projects) and `default-cli-project`
  spans 14.
- **SessionMeta**: `Title` from the summary title, else the summary
  `preview` (a generated title on most rows), else the first prompt;
  `FirstPrompt`/`LastPrompt` from user entries; `CreatedAt` from the first
  step; `ModifiedAt` from `last_modified_time` (a zero time,
  `0001-01-01`, on 5 rows, counts as missing), else the file mtime;
  `Model` from the last settings change; entry count and token totals from
  a parser scan, cached in the metadata cache like Kimi's `enrichInto`.
- **Subagents**: rows with `parent_conversation_id` set `ParentSessionID`
  and are listed only with `WithSubagents()`; the parent's metadata counts
  them. `GetSessionMeta` resolves a child id directly.
- **Discovery**: `DefaultDir`, `IsSessionPath` (exactly
  `brain/<id>/.system_generated/logs/transcript{,_full}.jsonl` under the
  home), `BaseDirForPath`, `SessionIDForPath`. `WatchConfig` covers
  `brain` deep enough to reach `.system_generated/logs`. 26 conversations
  also have `logs/chunks/transcript{,_full}/NNNNNNNN.jsonl` (1,064 files),
  split copies with no step missing from the main files: they are not
  session files and are excluded from `IsSessionPath`, the listing, search
  and the watcher.
- **Resume**: `agy --conversation <id>`, `Dir` = the workspace path.

### E. Registration and the gemini boundary

- `thinkt.SourceAntigravity = "antigravity"`, in `AllSources`, `String`,
  `DisplayName` ("Antigravity CLI"), `Description`
  ("Antigravity CLI conversations (~/.gemini/antigravity-cli)").
- `internal/sources/sources.go` factory list, `internal/tui/styles.go`
  color, `internal/tui/session_loader.go` path opener,
  `internal/cli/projects.go`, `knownProcesses` (`"agy"`), the search
  filter map in `internal/index/search/search.go`, `help_env.go`, and the
  regenerated swagger enum.
- `gemini.IsSessionPath` returns false for paths under the Antigravity home
  (whatever `THINKT_ANTIGRAVITY_HOME` resolves to), and `session_loader`
  checks Antigravity before Gemini.
- While there, fix `SourceKimi.Description()`, which still says `~/.kimi`.

### F. Search

A per-file search filter, as in Kimi: user-input, model-turn and result
lines are searchable; harness, checkpoint, error and unknown lines are not.
Only one of the two transcript files is a session file, so search never
scans both.

### G. Documentation

Package `README.md`; `AGENTS.md` (source list, env var, package table);
`README.md`; `DEVELOP.md` where sources are listed; `help_env.go`;
`etc/reports/SOURCE_FORMATS_2026-10.md` (Antigravity row: "after the
Antigravity pass"); `PLAN.md` gains the agent-documents roadmap item.

## Rollout order

1. Registration of the source constant and the gemini boundary fix, with
   tests.
2. Classification and fixtures (a trimmed real transcript with every step
   type, a subagent parent and child pair, a conversation without a
   workspace).
3. Conversion and the parser.
4. Session state, listing, discovery, resume.
5. Search filter, TUI opener, active detection.
6. Measurement on the real tree (every transcript parses, no empty
   entries, no duplicate UUIDs, every result paired, subagents linked),
   then docs.

## Decisions left to the builder

- Exact names of unexported types and helpers.
- How the metadata cache keys Antigravity entries (path plus size and
  mtime, as Kimi does, is expected).
- How `media` on `GENERIC` and `USER_INPUT` steps renders (49 + 1 steps);
  a text reference is enough.

## Calls I made

- Source id `antigravity`, display name "Antigravity CLI", env
  `THINKT_ANTIGRAVITY_HOME`. Cost if wrong: a rename.
- Agent documents are out of scope and go to the roadmap (Evan's call).
- One project for conversations with no recoverable workspace (5 today).
  Cost if wrong: those 5 show under a placeholder project.
- The first workspace folder is where `agy` was started, so it is the
  project; extra folders do not create projects. Cost if wrong: a
  conversation started in one repo with another added shows under the
  other one.
- No fallback through sibling conversations sharing an Antigravity
  `project_id`: those ids mix unrelated repos. Cost if wrong: a couple
  more conversations in the no-workspace project.

## Calls made after the builder check

- Project grouping is by folder (Evan, 2026-10-29: `ds4dg` and `ds4-go`
  are unrelated projects that Antigravity filed under one project id).
- `chunks/` split copies are not session files (builder finding).
- Title falls back to the summary `preview` before the first prompt;
  zero `last_modified_time` counts as missing; `Cwd` values are unquoted
  (builder findings, my calls).
- No `ParserVersion` bump. Cost if wrong: stale cache rows for a source
  that has none.
- The first child id goes on the tool_use block when one call creates
  several subagents; all are in metadata. Cost if wrong: the other
  children are reachable only through the parent listing.
