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
│       └── sessions-index.json       # Optional metadata index
└── statsig/
    └── statsig.stable_id.{x}         # Workspace ID
```

Each line in a `.jsonl` file is an [`Entry`](entry.go) with one of these types:

| Entry Type | Struct | Description |
|------------|--------|-------------|
| `user` | [`UserMessage`](entry.go) | User prompt. Content is polymorphic via [`UserContent`](entry.go) — plain string or array of content blocks. |
| `assistant` | [`AssistantMessage`](entry.go) | Model response containing [`ContentBlock`](entry.go) items (`text`, `thinking`, `tool_use`, `tool_result`, `image`, `document`). Includes [`Usage`](entry.go) token counts. |
| `system` | — | System prompt injection. |
| `summary` | — | Conversation summary (context compression). |
| `progress` | — | Progress/status update. |
| `file-history-snapshot` | — | Snapshot of file state. |
| `queue-operation` | — | Queue management. |

The base directory defaults to `~/.claude` and can be overridden with `THINKT_CLAUDE_HOME`.

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
