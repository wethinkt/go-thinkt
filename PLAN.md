# PLAN: thinkt

Current implementation status and roadmap for `thinkt`.

## Current State

The core CLI is functional with multi-source support, TUI with tree view navigation, first-run setup, local summarization, agent teams, analytics, HTTP/MCP servers, and lite webapp.

### Recently Completed

- [x] **Agent Hub** — Unified active agent following across local + remote
  - `thinkt agents` CLI with list, follow, and filtering
  - `internal/agents/` package: AgentHub, UnifiedAgent, stream providers
  - TUI agents list page with filter toggle (all/local/remote)
  - TUI agent tail page with live streaming
  - WebSocket streaming endpoint on collector (`/v1/sessions/{id}/ws`)
  - Session pub/sub for real-time fan-out
  - Single-use ticket auth for browser WebSocket connections
  - Machine fingerprint integration for local vs. remote detection

- [x] **TUI Tree View** - Collapsible project tree grouped by directory
  - Compacted single-child directory chains (e.g., `~/dev/company/team`)
  - Tree prefix rendering (`├──`, `└──`, `│`)
  - Toggle between tree view and flat list with `t`
  - Sort by date or name within directories
  - Left/Right arrows for collapse/expand

- [x] **TUI Navigation Polish**
  - ESC goes back (pop nav stack), q/ctrl+c quits throughout all screens
  - Fixed back-navigation rendering (only set `quitting` when standalone)
  - Shell sends `WindowSizeMsg` after popping to re-render revealed page
  - Source filter pass-through from project picker to session picker

- [x] **Agent Teams** - Multi-agent team inspection (Claude Code)
  - `TeamStore` interface: `ListTeams`, `GetTeam`, `GetTeamTasks`, `GetTeamMessages`
  - `ClaudeTeamStore` reads from `~/.claude/teams/` and `~/.claude/tasks/`
  - CLI: `thinkt teams [list]` with `--json`, `--active`, `--inactive` flags
  - REST API: `/api/v1/teams`, `/api/v1/teams/{name}`, tasks, messages endpoints

- [x] **StoreCache** - Project and session caching with optional TTL

- [x] **Authentication** - Bearer token auth for REST API and MCP HTTP servers
  - `thinkt server token` generates secure tokens
  - Constant-time comparison, `WWW-Authenticate` header on 401

- [x] **Machine Fingerprint** - `thinkt server fingerprint` for workspace correlation

- [x] **Trace Collector & Exporter** - Push-based trace aggregation
  - `thinkt collect` — HTTP server on port 8785, DuckDB storage, agent registry
  - `thinkt export` — File watcher, HTTP shipper, disk buffer, discovery cascade
  - `thinkt-exporter` / `thinkt-collector` standalone binaries
  - TUI views: collector status page, exporter status page
  - Collector API: `/v1/traces`, `/v1/traces/search`, `/v1/traces/stats`, `/v1/agents`
  - Prometheus metrics on `/metrics` (collector) and `--metrics-port` (exporter)

- [x] **Documentation Updates** - AGENTS.md, README.md, and Hugo docs updated

- [x] **Local Summarization** - On-device generation of summaries, classifications, and shareable tags
  - Separate per-model summaries DuckDB at `~/.thinkt/summaries/<model>.duckdb`
  - `internal/indexer/summarize/` package with prompts, extraction, model registry, and inference
  - `thinkt-indexer summarize` commands: `list`, `enable`, `disable`, `model`, `status`, `sync`, `run`, `tags`, `purge`
  - Summarization wired into the indexer ingester and RPC server
  - README and command docs updated for summarize workflows

- [x] **Setup Wizard** - First-run discovery and configuration flow
  - `thinkt setup` command with interactive Bubble Tea flow
  - `thinkt setup --ok` and automatic first-run setup from `PersistentPreRunE`
  - Source enable/disable persisted in `~/.thinkt/config.json`
  - Indexer preference, language, terminal, and app selections saved in config
  - Setup defaults and command behavior covered by tests

- [x] **GoReleaser Pro with goreleaser-cross** - CGO cross-compilation
  - Builds: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`

- [x] **Multi-platform Docker Images**
  - Published to `ghcr.io/wethinkt/thinkt`
  - Platforms: `linux/amd64`, `linux/arm64`
  - Two Dockerfiles: `Dockerfile` (CI/local), `Dockerfile.goreleaser` (releases)

- [x] **Homebrew Formula** - `brews` section in goreleaser

### Release Workflow

```
Tag push (v*) → GitHub Actions → goreleaser-cross container
  ├── Build binaries (5 platforms via CGO cross-compilation)
  ├── Build Docker images (linux/amd64, linux/arm64)
  ├── Push to GHCR
  ├── Create GitHub Release with archives
  └── Update Homebrew tap
```

## Security TODOs

- [ ] **Tighten `getAllowedBaseDirectories()` in `internal/server/security.go`**
  - Current implementation allows opening any directory under user's home
  - Consider restricting to only known project directories from the registry
  - Add explicit allowlist configuration option in `~/.thinkt/config.json`
  - Review symlink handling for edge cases (symlinks to other symlinks)

## Upcoming

### OpenCode Source Implementation

Add OpenCode (`~/.local/share/opencode/`) as a new source. OpenCode is unique: it uses
**SQLite** (not JSONL) as its primary data store, with a relational schema managed by
Drizzle ORM. This requires a different parsing strategy than all existing sources.

#### Storage Layout

```
~/.local/share/opencode/
├── opencode.db                    # SQLite database (primary data store)
├── opencode.db-shm / .db-wal     # SQLite WAL mode files
├── auth.json                      # API key credentials (DO NOT READ)
├── snapshot/<project_id>/         # Bare git repos for file snapshots
├── log/                           # Application logs
└── storage/
    ├── migration                  # Schema version ("2")
    └── session_diff/              # Per-session file diff JSON files
```

#### Database Schema (from upstream source)

Source: `packages/opencode/src/storage/schema.ts` and referenced `.sql.ts` files.
Timestamps mixin: `{ time_created: integer NOT NULL, time_updated: integer NOT NULL }` (Unix ms).

**`project` table**
```
id              text PK          -- git commit SHA of repo HEAD, or "global"
worktree        text NOT NULL    -- absolute path (e.g., "/Users/evan/wethinkt/go-thinkt")
vcs             text             -- "git" or null
name            text             -- null (auto-derived from worktree)
icon_url        text
icon_color      text
time_created    integer NOT NULL -- Unix ms
time_updated    integer NOT NULL
time_initialized integer
sandboxes       text NOT NULL    -- JSON array, default "[]"
commands        text             -- JSON: { start?: string }
```

**`session` table**
```
id              text PK          -- "ses_" prefix + hex + random
project_id      text NOT NULL FK(project.id CASCADE)
workspace_id    text
parent_id       text             -- FK to session.id (sub-agent tree)
slug            text NOT NULL    -- auto-generated two-word slug ("swift-circuit")
directory       text NOT NULL    -- CWD path
title           text NOT NULL    -- AI-generated session title
version         text NOT NULL    -- OpenCode version (e.g., "1.2.20")
share_url       text
summary_additions integer
summary_deletions integer
summary_files   integer
summary_diffs   text             -- JSON: FileDiff[]
revert          text             -- JSON: { messageID, partID?, snapshot?, diff? }
permission      text             -- JSON: PermissionNext.Ruleset (for sub-agents)
time_created    integer NOT NULL
time_updated    integer NOT NULL
time_compacting integer
time_archived   integer

INDEXES: session_project_idx, session_workspace_idx, session_parent_idx
```

**`message` table**
```
id              text PK          -- "msg_" prefix
session_id      text NOT NULL FK(session.id CASCADE)
time_created    integer NOT NULL
time_updated    integer NOT NULL
data            text NOT NULL    -- JSON: MessageV2.Info (User | Assistant)

INDEX: message_session_idx
```

**`message.data` JSON — User role:**
```typescript
{
  role: "user"
  time: { created: number }                    // Unix ms
  format?: { type: "text" } | { type: "json_schema", schema: JSONSchema, retryCount: number }
  summary?: { title?: string, body?: string, diffs: FileDiff[] }
  agent: string                                // "build" | "explore" | custom
  model: { providerID: string, modelID: string }
  system?: string                              // system prompt override
  tools?: Record<string, boolean>              // tool enable/disable overrides
  variant?: string
}
```

**`message.data` JSON — Assistant role:**
```typescript
{
  role: "assistant"
  time: { created: number, completed?: number }  // Unix ms
  error?: APIError | AuthError | AbortedError | OutputLengthError | ...
  parentID: string                               // msg_ of the user message this responds to
  modelID: string
  providerID: string
  mode: string                                   // deprecated, use agent
  agent: string                                  // "build" | "explore" | custom
  path: { cwd: string, root: string }
  summary?: boolean                              // true if this is a summary/compaction message
  cost: number                                   // monetary cost
  tokens: {
    total?: number
    input: number
    output: number
    reasoning: number
    cache: { read: number, write: number }
  }
  structured?: any                               // structured output
  variant?: string
  finish?: string                                // "stop" | "tool-calls" | ...
}
```

**`part` table**
```
id              text PK          -- "prt_" prefix
message_id      text NOT NULL FK(message.id CASCADE)
session_id      text NOT NULL    -- denormalized
time_created    integer NOT NULL
time_updated    integer NOT NULL
data            text NOT NULL    -- JSON: MessageV2.Part (discriminated union on "type")

INDEXES: part_message_idx, part_session_idx
```

**`part.data` JSON — All 12 part types (discriminated union on `type`):**

| Part Type | Key Fields | Maps To |
|-----------|-----------|---------|
| `text` | `text`, `synthetic?`, `ignored?`, `time?`, `metadata?` | `ContentBlock{Type: "text"}` |
| `reasoning` | `text`, `metadata?`, `time: {start, end?}` | `ContentBlock{Type: "thinking"}` |
| `tool` | `callID`, `tool` (name), `state` (pending\|running\|completed\|error), `metadata?` | `ContentBlock{Type: "tool_use"}` + `ContentBlock{Type: "tool_result"}` |
| `step-start` | `snapshot?` (git tree hash) | Metadata boundary, not a separate entry |
| `step-finish` | `reason`, `snapshot?`, `cost`, `tokens: {total?, input, output, reasoning, cache: {read, write}}` | `TokenUsage` extraction |
| `snapshot` | `snapshot` (git tree hash) | Metadata only |
| `patch` | `hash`, `files: string[]` | Metadata (file changes) |
| `file` | `mime`, `filename?`, `url`, `source?` (file\|symbol\|resource) | `ContentBlock{Type: "image"}` or metadata |
| `subtask` | `prompt`, `description`, `agent`, `model?`, `command?` | Sub-agent invocation metadata |
| `agent` | `name`, `source?` | Agent identification |
| `compaction` | `auto: boolean`, `overflow?: boolean` | Context compaction marker |
| `retry` | `attempt`, `error: APIError`, `time: {created}` | Error retry metadata |

**Tool state variants** (discriminated on `status`):
- `pending`: `{ input, raw }`
- `running`: `{ input, title?, metadata?, time: {start} }`
- `completed`: `{ input, output, title, metadata, time: {start, end, compacted?}, attachments?: FilePart[] }`
- `error`: `{ input, error, metadata?, time: {start, end} }`

**`todo` table** (composite PK)
```
session_id      text NOT NULL FK(session.id CASCADE)
content         text NOT NULL
status          text NOT NULL
priority        text NOT NULL
position        integer NOT NULL
time_created    integer NOT NULL
time_updated    integer NOT NULL

PK: (session_id, position)
INDEX: todo_session_idx
```

**`workspace` table**
```
id              text PK
type            text NOT NULL
branch          text
name            text
directory       text
extra           text             -- JSON
project_id      text NOT NULL FK(project.id CASCADE)
```

**Other tables** (not needed for session parsing):
- `control_account` — OAuth credentials (PK: email+url)
- `session_share` — Share URLs (PK: session_id)
- `permission` — Project-level permission rules (PK: project_id)

#### Data Model Mapping

| OpenCode | thinkt |
|----------|--------|
| `project.id` | `Project.ID` |
| `project.worktree` | `Project.Path` / `Project.Name` |
| `session.id` (ses_...) | `SessionMeta.ID` |
| `session.title` | `SessionMeta.Summary` |
| `session.parent_id` | Sub-agent relationship → `Entry.AgentID` |
| `session.directory` | `Entry.CWD` |
| `workspace.branch` | `SessionMeta.GitBranch` / `Entry.GitBranch` |
| `message` role=user | `Entry{Role: RoleUser}` |
| `message` role=assistant | `Entry{Role: RoleAssistant}` |
| `part` type=text | `ContentBlock{Type: "text"}` |
| `part` type=reasoning | `ContentBlock{Type: "thinking"}` |
| `part` type=tool (completed) | `ContentBlock{Type: "tool_use"}` + `ContentBlock{Type: "tool_result"}` |
| `part` type=tool (error) | `ContentBlock{Type: "tool_use"}` + `ContentBlock{Type: "tool_result", IsError: true}` |
| `part` type=file (image/pdf) | `ContentBlock{Type: "image"}` |
| `part` type=step-finish `.tokens` | `Entry.Usage{InputTokens, OutputTokens, CacheRead, CacheCreation}` |
| `part` type=step-finish `.reason` | Controls whether turn continues ("tool-calls") or ends ("stop") |
| `part` type=compaction | `Entry{Role: RoleCheckpoint, IsCheckpoint: true}` |
| `assistant.modelID` | `Entry.Model` / `SessionMeta.Model` |
| `assistant.providerID` | `Entry.Metadata["providerID"]` |
| `assistant.agent` | `Entry.Metadata["agent"]` |
| `assistant.cost` | `Entry.Metadata["cost"]` |

#### ID Formats

- Session: `ses_<hex><random>` (e.g., `ses_3590bd469ffe879siVZ2BaFmRp`)
- Message: `msg_<hex><random>`
- Part: `prt_<hex><random>`

#### Implementation Plan

**Phase 1: Core Package** (`internal/sources/opencode/`)

- [ ] **1a. `discovery.go`** — `Discoverer` struct implementing `StoreFactory`
  - `Source()` → `thinkt.SourceOpenCode`
  - `basePath()` → `THINKT_OPENCODE_HOME` env or `~/.local/share/opencode/`
  - `IsAvailable()` → check `opencode.db` exists
  - `IsSessionPath()` → not applicable (SQLite, not file-per-session)
  - `Factory()` → constructor

- [ ] **1b. `db.go`** — SQLite read-only access layer
  - Open `opencode.db` in `?mode=ro&_journal_mode=WAL` (read-only, respect WAL)
  - Query helpers for projects, sessions, messages, parts
  - Use `modernc.org/sqlite` (pure Go, no CGO) or `crawshaw.io/sqlite`
    to keep the zero-CGO build constraint
  - **Concurrency**: open/close per operation (like the DuckDB indexer pattern)
    to avoid locking conflicts with the running OpenCode process
  - Consider copy-on-read fallback if SQLite WAL locking is an issue

- [ ] **1c. `types.go`** — OpenCode-specific raw data structures
  - `rawProject`, `rawSession`, `rawMessage`, `rawPart` structs matching DB columns
  - `messageData` struct for the `message.data` JSON blob
  - `partData` struct (union type) for the `part.data` JSON blob
  - `toolState` struct for tool invocation details
  - `tokenInfo` struct for token tracking

- [ ] **1d. `parser.go`** — Conversion from raw DB types to `thinkt.Entry`
  - `convertSession()` → loads messages + parts, groups parts by message,
    converts to `[]thinkt.Entry`
  - Handle the multi-assistant-message-per-user-message pattern:
    when `finish == "tool-calls"`, chain continues
  - Map all 12 part types to content blocks:
    - `text` → `ContentBlock{Type: "text"}` (skip if `ignored == true`)
    - `reasoning` → `ContentBlock{Type: "thinking"}`
    - `tool` (completed) → `ContentBlock{Type: "tool_use"}` + `ContentBlock{Type: "tool_result"}`
    - `tool` (error) → `ContentBlock{Type: "tool_use"}` + `ContentBlock{Type: "tool_result", IsError: true}`
    - `tool` (pending/running) → `ContentBlock{Type: "tool_use"}` (no result yet)
    - `file` (image/pdf) → `ContentBlock{Type: "image"}` with URL
    - `file` (text/plain, directory) → skip (synthetic, already in text parts)
    - `subtask` → metadata for sub-agent spawning (link to child session)
    - `agent` → agent identification metadata
    - `compaction` → `Entry{Role: RoleCheckpoint, IsCheckpoint: true}`
    - `retry` → skip or metadata (error retry attempt)
    - `snapshot` → skip (internal git tracking)
    - `step-start` / `step-finish` → extract `TokenUsage`, not separate entries
    - `patch` → metadata (file change list)
  - Extract `TokenUsage` from `step-finish` parts: `{input, output, reasoning, cache.read, cache.write}`
  - Map tokens: `input` → `InputTokens`, `output` → `OutputTokens`,
    `cache.read` → `CacheReadInputTokens`, `cache.write` → `CacheCreationInputTokens`
  - Map `parent_id` sessions to `AgentID` / `SourceAgentID`
  - Handle `summary == true` assistant messages (compaction summaries)

- [ ] **1e. `store.go`** — `Store` struct implementing `thinkt.Store`
  - Embed `thinkt.StoreCache` for project/session caching
  - `ListProjects()` → query `project` table, map to `thinkt.Project`
  - `GetProject()` → query by project ID or worktree path
  - `ListSessions()` → query `session` table with project filter
  - `GetSessionMeta()` → query session + count messages
  - `LoadSession()` → full load: session + messages + parts → `thinkt.Session`
  - `OpenSession()` → return `SessionReader` that streams entries
  - `WatchConfig()` → watch the `opencode.db` file itself (or `storage/` dir)
  - `Workspace()` → derive from hostname + base path

**Phase 2: Registration** (touch existing files)

- [ ] **2a. Source constant** — `internal/thinkt/types.go`
  - Add `SourceOpenCode Source = "opencode"`
  - Add to `AllSources` slice
  - Add `String()` → `"opencode"`
  - Add `DisplayName()` → `"OpenCode"`

- [ ] **2b. Factory registration** — `internal/sources/sources.go`
  - Import `internal/sources/opencode`
  - Add `opencode.Factory()` to `AllFactories()`

- [ ] **2c. TUI color** — `internal/tui/styles.go`
  - Add `case thinkt.SourceOpenCode: return "#00d4aa"` (teal/green, OpenCode brand)

- [ ] **2d. Environment variable** — `THINKT_OPENCODE_HOME`
  - Document in AGENTS.md and README.md

- [ ] **2e. AGENTS.md updates**
  - Add OpenCode to source list, env vars table, key packages table

**Phase 3: Edge Cases & Polish**

- [ ] **3a. Sub-agent sessions** — Sessions with `parent_id` set
  - These are `task` tool invocations; `session.parent_id` → parent session ID
  - The `subtask` part on the parent message contains `prompt`, `description`, `agent`, `model`
  - The `tool` part with `tool == "task"` has `metadata.sessionId` pointing to child session
  - `session.permission` contains JSON rules restricting sub-agent capabilities
  - **Strategy**: filter sub-agent sessions from top-level `ListSessions()` (only show root).
    Set `Entry.AgentID` on child session entries to link them to the parent.
    Optionally expose via `Entry.SourceAgentID` = child session ID.

- [ ] **3b. Session diff files** — `storage/session_diff/ses_*.json`
  - Parse for file change metadata if useful for session summaries

- [ ] **3c. WatchConfig for exporter**
  - SQLite doesn't produce new files per session
  - Watch `opencode.db-wal` for modifications, or poll the DB
  - May need a different watcher strategy (periodic poll vs fsnotify)

- [ ] **3d. MetadataCache integration**
  - Use `~/.thinkt/cache/sessions-opencode.json` for enriched metadata
  - Key by session ID (not file path, since there are no per-session files)

- [ ] **3e. Tests**
  - Unit tests with a test fixture SQLite DB
  - Test conversion of all part types
  - Test sub-agent session linking
  - Test concurrent read access (WAL mode)

#### Key Design Decisions

1. **SQLite driver**: Use `modernc.org/sqlite` (pure Go) to maintain zero-CGO builds.
   This is the same approach used by many Go projects for portable SQLite. If
   performance is insufficient, can switch to `mattn/go-sqlite3` behind a build tag.

2. **Read strategy**: Open DB read-only per operation batch (list projects, load session, etc.)
   and close immediately. This avoids holding locks against the running OpenCode process.
   WAL mode allows concurrent readers, but we should still be conservative.

3. **No `IsSessionPath()`**: Unlike file-based sources, there's no per-session file path.
   The exporter/watcher will need a different strategy — likely watching the DB file
   for mtime changes and diffing session counts.

4. **Entry grouping**: Each user message + its chain of assistant messages (linked by
   `parentID`) forms a logical turn. Parts within each message become content blocks
   on that entry. `step-start`/`step-finish` are metadata boundaries, not separate entries.

5. **Tool parts are self-contained**: Unlike Claude (where tool_use and tool_result are
   separate entries), OpenCode's `tool` part contains both `input` and `output` in the
   `state` object. We emit both a `tool_use` and `tool_result` content block from a
   single part row.

6. **Config location**: OpenCode's config lives at `~/.config/opencode/opencode.json`,
   data at `~/.local/share/opencode/`. We read from the data directory only. The config
   is irrelevant to session parsing.

### Short Term

- [ ] **Cloud login + push flow** - Finish wiring trace publishing to `wethinkt.com`
  - `internal/cloud/` now has credentials storage, device-flow auth client, push client, and tests
  - `internal/cmd/login.go` and `internal/cmd/push.go` exist, but are not registered on the root command yet
  - Add CLI wiring, flags (`--public`), and help text so generated docs match the actual command tree
  - Verify API paths, error handling, and cross-platform credentials location before release

- [ ] **Setup polish**
  - Add `--reconfigure` flag to re-run setup explicitly
  - Review whether setup should expose summarization options alongside embeddings/indexer
  - Keep environment variable overrides for Docker/CI behavior

- [ ] **Health check endpoint** - For container orchestration

### Medium Term

- [x] **Prometheus metrics** - Collector and exporter expose `/metrics` for Prometheus scraping
- [ ] **Hugo docs site deployment** - Publish to GitHub Pages

### Long Term

- [ ] **Public Go package** - Stabilize and export `thinkt` types and interfaces

## Architecture

```
cmd/thinkt/           CLI entry point (Cobra)
internal/
  thinkt/             Core types, Store/TeamStore interfaces, cache
  sources/            Source implementations (claude, kimi, gemini, copilot, codex)
  tui/                BubbleTea terminal UI (shell, pickers, viewer, tree)
  server/             HTTP REST API, teams API, MCP server, lite webapp
  export/             Trace exporter (watcher, shipper, buffer, discovery)
  collect/            Trace collector (HTTP server, DuckDB store, agent registry)
  analytics/          Analytics
  prompt/             Prompt extraction
  config/             Configuration management
  fingerprint/        Machine fingerprint generation
```

## Docker Usage

```bash
# Run HTTP server with session data
docker run -p 8784:8784 \
  -v ~/.claude:/data/.claude:ro \
  -v ~/.kimi:/data/.kimi:ro \
  ghcr.io/wethinkt/thinkt:latest serve --host 0.0.0.0

# Run any command
docker run -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt:latest projects
```

## Configuration (`~/.thinkt/config.json`)

Planned config structure for `thinkt setup`:

```json
{
  "sources": {
    "claude": { "enabled": true, "path": "~/.claude" },
    "kimi": { "enabled": true, "path": "~/.kimi" },
    "gemini": { "enabled": false },
    "copilot": { "enabled": true, "path": "~/.copilot" }
  }
}
```

- **enabled**: Whether source is active (respects user consent)
- **path**: Custom path override (optional, defaults to standard location)
- Environment variables (`THINKT_*_HOME`) override config for Docker/CI use cases

## Build Targets

We build without CGO so get broad support:

| Platform | Arch | Status |
|----------|------|--------|
| Linux | amd64 | ✅ |
| Linux | arm64 | ✅ |
| FreeBSD | amd64 | ✅ |
| FreeBSD | arm64 | ✅ |
| Darwin | amd64 | ✅ |
| Darwin | arm64 | ✅ |
| Windows | amd64 | ✅ |
| Windows | arm64 | ✅ |
