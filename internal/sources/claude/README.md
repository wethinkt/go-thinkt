# claude

Package `claude` implements the session source for [Claude Code](https://docs.anthropic.com/en/docs/claude-code).

## Directory Structure

```
claude/
├── discovery.go          # StoreFactory implementation, session path detection
├── entry.go              # JSONL entry types and content block parsing
├── lazy_session.go       # Lazy-loading session reader with windowed access
├── parser.go             # JSONL trace file parser
├── projects.go           # Project directory discovery and session metadata
├── resume.go             # Session resume command generation
├── session.go            # Session type and file discovery utilities
├── store.go              # thinkt.Store implementation
├── teams.go              # thinkt.TeamStore implementation
├── *_test.go             # Tests
```

## Session File Format

Sessions are stored as JSONL trace files, read via the [`internal/jsonl`](../../jsonl/) streaming reader.

```
~/.claude/
├── projects/
│   └── {project-path}/
│       ├── {session-uuid}.jsonl      # Session trace file
│       ├── {session-uuid}/
│       │   └── subagents/
│       │       ├── agent-{id}.jsonl       # Subagent transcript
│       │       └── agent-{id}.meta.json   # agentType, description, toolUseId
│       └── sessions-index.json       # Legacy index (pre-2.1); read when present
└── statsig/
    └── statsig.stable_id.{x}         # Workspace ID
```

Title, last prompt, PR links, worktree, and cost live inside the transcript as
session-state records (`ai-title`, `last-prompt`, `pr-link`, `worktree-state`,
`cost-state`); `scanSessionFile` folds them into `SessionMeta`.

Each line in a `.jsonl` file is an [`Entry`](entry.go) with one of these types:

| Entry Type | Struct | Description |
|------------|--------|-------------|
| `user` | [`UserMessage`](entry.go) | User prompt. Content is polymorphic via [`UserContent`](entry.go) — plain string or array of content blocks. |
| `assistant` | [`AssistantMessage`](entry.go) | Model response containing [`ContentBlock`](entry.go) items (`text`, `thinking`, `tool_use`, `tool_result`, `image`, `document`). Includes [`Usage`](entry.go) token counts. |
| `system` | — | System prompt injection. |
| `summary` | — | Conversation summary (context compression). |
| `progress` | — | Progress/status update. |
| `file-history-snapshot` | — | Snapshot of file state. |
| `queue-operation` | — | Prompt queue. Only records with `content` (enqueue) become entries; dequeue/remove/popAll are bookkeeping. |

The base directory defaults to `~/.claude` and can be overridden with `THINKT_CLAUDE_HOME`.

## Record Classes

Claude Code writes many record types per transcript, most of them harness
bookkeeping. `classify.go` decides from the `type` field (and `attachment.type`)
before any other parsing:

| class | types | result |
|---|---|---|
| turn | `user`, `assistant` | entry |
| event | `system`, `queue-operation` with `content`, `progress` | entry; `Text` from `content`, `Kind` from subtype |
| context | `attachment` in `ContextAttachmentTypes` | entry, `Kind = attachment`, `Visibility = secondary` |
| checkpoint | `file-history-snapshot`, `file-history-delta`, `summary` | entry (checkpoint role) |
| session state | `ai-title`, `last-prompt`, `pr-link`, `relocated`, `worktree-state`, `cost-state` | folded into `SessionMeta` by `scanSessionFile`, never an entry |
| bookkeeping | `api-request*`, `mode`, `permission-mode`, `atis-latch`, content-less `queue-operation`, other attachments | dropped |
| unknown | anything else | entry, `Kind = unknown`, `Visibility = secondary`, raw JSON as text |

Every reader (`Parser.NextEntry`, `LazySession`, `LoadSessionWindow`) and the
counter (`countConversationRecords`) apply the same classification, so
`EntryCount` means "entries emitted". `thinkt.ParserVersion` is bumped when the
classification changes; the metadata cache and the index re-scan on mismatch.
Tool results decode array content into `ContentBlock.Children` and the typed
`toolUseResult` into a `diff` child, stdout fallback, and `ChildSessionID`.
See [etc/reports/CLAUDE_TRANSCRIPT_SCHEMA.md](../../../etc/reports/CLAUDE_TRANSCRIPT_SCHEMA.md).

## Key Types

### Discovery & Store

- [`Discoverer`](discovery.go) — implements [`thinkt.StoreFactory`](../../thinkt/). Detects Claude Code installations and creates stores.
- [`Store`](store.go) — implements [`thinkt.Store`](../../thinkt/). Manages project listing, session loading, caching, and file watching.
- [`TeamStore`](teams.go) — implements [`thinkt.TeamStore`](../../thinkt/). Discovers teams, members, shared tasks, and messages.

### Session & Parsing

- [`Session`](session.go) — represents a complete Claude Code conversation session.
- [`LazySession`](lazy_session.go) — lazy-loading session reader. Preloads metadata and defers full content loading for efficient streaming.
- [`SessionWindow`](parser.go) — holds a window of session content with byte-offset position info.
- [`Parser`](parser.go) — reads Claude Code JSONL trace files entry by entry.

### Entry Types

- [`Entry`](entry.go) — a single line in a JSONL trace file. Type is one of: `user`, `assistant`, `system`, `progress`, `file-history-snapshot`, `summary`, `queue-operation`.
- [`UserMessage`](entry.go) / [`UserContent`](entry.go) — polymorphic user message content (plain text or content blocks).
- [`AssistantMessage`](entry.go) — assistant response with model info, content blocks, and token usage.
- [`ContentBlock`](entry.go) — individual content block: `text`, `thinking`, `tool_use`, `tool_result`, `image`, or `document`.
- [`Prompt`](entry.go) — extracted user prompt with text, timestamp, and UUID.
- [`Usage`](entry.go) / [`CacheCreation`](entry.go) — token usage and cache statistics.

### Project Discovery

- [`Project`](projects.go) — a Claude Code project directory with decoded display name.
- [`SessionMeta`](projects.go) — lightweight session metadata without loading full JSONL content.

## Key Functions

- [`Factory()`](discovery.go) — returns a [`thinkt.StoreFactory`](../../thinkt/) for Claude Code.
- [`IsSessionPath(path)`](discovery.go) — reports whether a path looks like a Claude session file.
- [`LoadSession(path)`](parser.go) — loads a complete session from a JSONL trace file.
- [`OpenLazySession(path)`](lazy_session.go) — opens a session with lazy content loading.
- [`DefaultDir()`](session.go) — returns the Claude Code base directory (`~/.claude`).
- [`DecodeDirName(dirName)`](projects.go) — converts hashed directory names to readable project paths.
- [`ListProjects(baseDir)`](projects.go) — discovers all Claude Code project directories.

## Related folders

`RelatedPaths` lists the folders outside the project a session touched,
folded to repo roots by `thinkt.RelatedTracker`. Shell command text is not
parsed.

| reason | from |
|---|---|
| `read` | `Read`, `NotebookRead`, `Grep`, `Glob`, `LS` |
| `edited` | `Edit`, `MultiEdit`, `Write`, `NotebookEdit` |
| `command` | `Bash`: the record's `cwd` |

The project is the session's first recorded `cwd`, which is exact where a
decoded folder name is not.
