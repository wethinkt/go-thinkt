# AGENTS: `go-thinkt`

## Project Overview

`go-thinkt` provides `thinkt`, a CLI tool for exploring and extracting data from AI coding assistant sessions. It is a companion to the [wethinkt](https://github.com/wethinkt/wethinkt) web visualization application.  Right now much is internal, but we will eventually build out a public package.

**Stack**: Go, Cobra (CLI), BubbleTea (TUI), Chi (HTTP), MCP SDK

Released under the MIT license, see [LICENSE.txt](./LICENSE.txt).

## Architecture

### Multi-Source Design

The tool supports multiple AI coding assistants via a `Store` interface:
- **Claude Code** (`~/.claude`)
- **Kimi Code** (`~/.kimi-code`; the legacy `~/.kimi` tree is not read)
- **Gemini CLI** (`~/.gemini`; `.jsonl` sessions replayed the way the CLI loads them, older `.json` sessions as before; one project per `projectHash` across hash and slug folders; subagents in `chats/<parent>/`)
- **GitHub Copilot** (`~/.copilot`)
- **Codex CLI** (`~/.codex`; rollouts under `sessions/`, titles in `session_index.jsonl`, subagent rollouts linked by `ParentSessionID`)
- **Qwen Code** (`~/.qwen`)
- **Antigravity CLI** (`~/.gemini/antigravity-cli`; step transcripts under `brain/<id>/.system_generated/logs`, titles and workspaces in `conversation_summaries.db`; the gemini source skips this directory)

Sources are auto-discovered. Use `--source claude|kimi|gemini|copilot|codex|qwen|antigravity` flags to filter.

**Source Enable/Disable:** Sources can be enabled/disabled via `thinkt config sources enable/disable` (stored in `config.Sources` map). A source with no entry in an existing map (installed after setup) stays off: `config sources list` shows it as `new` with the enable command, and the first other command announces it once on stderr (recorded in `config.NotifiedSources`). Sources that are off are only checked for availability, never opened. Disabled sources are invisible at the API boundary — the MCP/HTTP server validates against the `StoreRegistry` (which only registers enabled sources), and the indexer RPC server independently filters via `enabledSources` (derived from `config.EnabledSources()`, refreshed on `config_reload`). Both layers enforce filtering so neither can leak disabled source data.

**Entry classification:** `thinkt.Entry` carries a source-neutral `Kind` (an open string type with constants) and a `Visibility` hint. Sources mark bookkeeping-adjacent entries (`turn_duration`, attachments, meta prompts) as `secondary`; the export filter, TUI "Other" toggle, and `conversation.WritePage` hide those by default while the REST API and MCP return every emitted entry. `thinkt.ParserVersion` must be bumped whenever a source changes which entries it emits; the metadata cache and the SQLite index recompute lazily on mismatch. In every source `Entry.PromptID` is one user prompt and everything until the agent hands back, and `Entry.TurnID` is one model response (Claude `message.id`, Codex `response_id`, Kimi step uuid, Antigravity planner step); a source that cannot tell responses apart leaves `TurnID` empty. The Claude parser drops request dumps and session-state records before parsing (see `internal/sources/claude/README.md`). Codex wire tool calls superseded by a harness item carry `KindToolWire` and are secondary; subagent rollouts are listed only with `thinkt.WithSubagents()` (see `internal/sources/codex/README.md`). Antigravity pairs tool results to calls by order, hides harness steps, and lists subagent conversations only with `thinkt.WithSubagents()` (see `internal/sources/antigravity/README.md`). Gemini CLI `.jsonl` files are rebuilt before they are read (later lines can rewind or patch earlier ones); notices and `update_topic` calls are secondary, and search uses the optional `thinkt.SearchFileFilterer` so hits count only in lines the rebuilt session keeps (see `internal/sources/gemini/README.md`). `SessionMeta.RelatedPaths` lists the folders outside the project a session touched (repo roots, with reasons `workspace`, `edited`, `command`, `read`). Claude, Codex, Kimi, Gemini and Antigravity fill it through `thinkt.RelatedTracker`, each with a table of its path-bearing tools; shell command text is not parsed. Optional `thinkt.DocumentStore` returns the documents an agent kept (Antigravity brain files, Kimi plan snapshots, Claude `plans/` and `ExitPlanMode`, Codex plans), each with versions; `thinkt.DocumentTracker` rebuilds versions from tool calls on request, never in listings, and marks a version `Partial` when history is incomplete. Every attachment is an `image` or `document` block carrying `ContentBlock.Media` (`thinkt.MediaRef`: where the bytes are, their SHA-256 and size); nothing opens a referenced file, and `MediaData` stays for inline bytes. `WatchConfig.HiddenDirs` names dot-directories the relay watcher may enter, and nested watch roots (`~/.gemini` and `~/.gemini/antigravity-cli`) resolve to the deepest root. The relay ships every enabled source through its store, so it carries what the readers produce, not raw lines.

### Key Packages

| Package | Purpose |
|---------|---------|
| `cmd/thinkt` | CLI entry point (Cobra commands) |
| `internal/thinkt` | Core types, Store/TeamStore interfaces, registry, cache |
| [`internal/sources`](./internal/sources/) | [Source factory registry](./internal/sources/README.md) |
| [`internal/sources/claude`](./internal/sources/claude/) | Claude Code storage + teams implementation |
| [`internal/sources/kimi`](./internal/sources/kimi/) | Kimi Code storage implementation |
| [`internal/sources/gemini`](./internal/sources/gemini/) | Gemini CLI storage implementation |
| [`internal/sources/copilot`](./internal/sources/copilot/) | Copilot storage implementation |
| [`internal/sources/codex`](./internal/sources/codex/) | Codex CLI storage implementation |
| [`internal/sources/qwen`](./internal/sources/qwen/) | Qwen Code storage implementation |
| [`internal/sources/antigravity`](./internal/sources/antigravity/) | Antigravity CLI storage implementation |
| `internal/tui` | BubbleTea terminal UI (shell, pickers, viewer, theme builder) |
| `internal/server` | HTTP REST API, teams API, indexer API, and MCP server |
| `internal/server/web` | Full webapp submodule ([thinkt-web](https://github.com/wethinkt/thinkt-web), `dist` branch) |
| `internal/server/web-lite` | Lite webapp submodule ([thinkt-web-lite](https://github.com/wethinkt/thinkt-web-lite), `dist` branch) |
| `internal/indexer` | Indexer ingestion, watching, and search |
| `internal/indexer/db` | DuckDB database layer with copy-on-read support |
| `internal/relay` | Trace relay: file watcher, HTTP shipper, disk buffer, discovery |
| `internal/agents` | Agent hub: unified local+remote detection, streaming, filter |
| `internal/collect` | Trace collector: HTTP server, DuckDB store, agent registry, normalizer |
| `internal/analytics` | Analytics |
| `internal/export` | Session export (Markdown, HTML, JSON) with content filtering |
| [`internal/conversation`](./internal/conversation/) | Semantic presentation adapter (kinds, visibility, nested tool-result blocks, diffs) and shared terminal/web HTML/CSS; used by HTML export |
| `internal/prompt` | Prompt extraction and formatting |
| `internal/config` | Configuration management, instance registry |
| `internal/fingerprint` | Machine fingerprint generation |
| `cmd/thinkt-relay` | Standalone relay binary (flag-only, no cobra) |
| `cmd/thinkt-collector` | Standalone collector binary (flag-only, no cobra) |

### Command Structure

```
thinkt
├── tui                 # Interactive TUI browser (default)
├── setup               # Scan for AI session sources and configure thinkt
├── server              # HTTP/MCP servers
│   ├── run             # Run server in foreground (default)
│   ├── start           # Start server in background
│   ├── stop            # Stop background server
│   ├── status          # Show server status
│   ├── logs            # View server logs
│   ├── http-logs       # View HTTP access logs
│   ├── metrics         # View server metrics
│   ├── mcp             # MCP server (stdio or HTTP)
│   ├── token           # Generate secure authentication token
│   └── fingerprint     # Display machine fingerprint
├── web                 # Open web interface in browser
│   └── lite            # Lightweight debug webapp
├── sources             # Source management
│   ├── list
│   ├── status
│   ├── enable
│   └── disable
├── projects            # Project management
│   ├── list
│   ├── tree
│   ├── summary
│   ├── view
│   ├── delete
│   └── copy
├── sessions            # Session management
│   ├── list
│   ├── summary
│   ├── view
│   ├── resolve
│   ├── resume
│   ├── docs            # Agent documents and their versions
│   ├── delete
│   └── copy
├── agents              # Active agent detection
│   └── follow          # Live-tail an agent's conversation
├── search              # Full-text search across indexed sessions
├── semantic            # Semantic search by meaning
├── embeddings          # Manage embedding models
├── relay               # Relay traces to remote collector
├── collect             # Start trace collector server
│   └── export-parquet  # Export collector data to Parquet
├── teams               # Agent team management
│   └── list
├── export              # Session export
│   └── template        # Extract user prompts with Go templates
├── indexer             # DuckDB-powered indexer management
│   ├── start
│   ├── stop
│   ├── status
│   ├── sync
│   ├── search
│   ├── semantic
│   ├── sessions
│   ├── stats
│   ├── metrics
│   ├── logs
│   ├── embeddings
│   └── version
├── theme               # Theme management
│   ├── browse
│   ├── list
│   ├── show
│   ├── set
│   ├── import
│   └── builder
├── language            # Display language management
│   ├── get
│   ├── set
│   └── list
├── apps                # Open-in app management
│   ├── list
│   ├── enable
│   ├── disable
│   ├── get-terminal
│   └── set-terminal
├── completion          # Shell completion scripts
├── docs                # Documentation generation
│   ├── markdown
│   └── man
└── version             # Print version information
```

The index (SQLite, under `~/.thinkt/dbs/index.db`) is populated by `thinkt sync` and kept current by the background watcher started by `thinkt` (TUI) and `thinkt server`. Trace relay and collector functionality live under `thinkt relay` and `thinkt collect`.

### TUI Architecture

The TUI uses a `Shell` with a `NavStack` that manages page navigation:

- **Shell** (`shell.go`) — Top-level model, manages source discovery and page stack
- **ProjectPickerModel** (`project_picker.go`) — Tree or flat project list with source filtering and sorting
- **SessionPickerModel** (`session_picker.go`) — Session list with source filtering, color-coded source badges
- **MultiViewerModel** (`multi_viewer.go`) — Lazy-loading session viewer with viewport scrolling
- **ConversationViewerModel** (`conversation_viewer.go`) — Opt-in HTML/nthtml viewer (`THINKT_VIEWER=html`) with bounded message groups, source-anchor navigation, and lazy readers; exercise via `sh etc/try-thinkt.sh` while renderer commits are unpublished
- **SourcePickerModel** (`source_picker.go`) — Overlay for filtering by source (used within pickers)
- **ThemeBuilderModel** (`theme_builder.go`) — Standalone theme editor with color picker
- **CollectorPageModel** (`collector_page.go`) — Live collector status view (fetches REST API, auto-refreshes)
- **ExporterPageModel** (`exporter_page.go`) — Relay status view (config, buffer, stats)

**Navigation pattern:**
- Each page sends a result message (e.g., `ProjectPickerResult`, `SessionPickerResult`) back to Shell
- Shell pushes/pops pages on the `NavStack` based on the result
- ESC sends a cancelled result (Shell pops back to previous page)
- q/ctrl+c sends `tea.Quit` (exits the app entirely)
- After popping, Shell sends `WindowSizeMsg` to the revealed page for re-rendering

**Key conventions:**
- Pickers set `quitting = true` only when standalone (not when embedded in Shell), so they remain renderable when popped back to
- Tree view uses `treeItem` as the `list.Item` type (wraps `treeNode`), rendered by `treeProjectDelegate` or `flatProjectDelegate`
- Source colors are defined in `styles.go` via `SourceColorHex()`

### Debug Logging

Use `--log <file>` to write debug logs (available on `thinkt`, `thinkt tui`, and all `server` subcommands):

```bash
thinkt tui --log /tmp/thinkt-debug.log
```

### Server Command Flags

| Flag | Description |
|------|-------------|
| `--port, -p` | Server port (default: 8784 for server, 8786 for mcp, 8787 reserved for VS Code extension) |
| `--host` | Server host (default: localhost) |
| `--no-open` | Don't auto-open browser |
| `--quiet, -q` | Suppress HTTP request logging |
| `--http-log <file>` | Write HTTP access log to file |
| `--log` | Write debug log to file |
| `--token` | Bearer token for authentication (API and MCP HTTP) |
| `--dev <url>` | Dev mode: proxy non-API routes to a frontend dev server (e.g. `http://localhost:8784`) |

### Authentication

Both the REST API server and MCP HTTP server use a unified `BearerAuthenticator` (defined in `auth.go`) with a single `AuthConfig` type supporting three modes: `AuthModeNone`, `AuthModeToken`, and `AuthModeEnvToken`. Each server has its own defaults via `DefaultAPIAuthConfig()` and `DefaultMCPAuthConfig()`, which embed the appropriate realm (`thinkt-api` or `thinkt-mcp`).

**Token Generation:**
```bash
thinkt server token                  # Generates thinkt_YYYYMMDD_<random> format
thinkt server token | pbcopy         # Copy to clipboard (macOS)
thinkt server token | xclip -sel c   # Copy to clipboard (Linux)
thinkt server token | clip           # Copy to clipboard (Windows)
```

### Machine Fingerprint

Display the unique machine identifier:
```bash
thinkt server fingerprint              # Human-readable output
thinkt server fingerprint --json       # JSON output with source details
```

**Fingerprint Sources (in order of preference):**
| Platform | Source | Location |
|----------|--------|----------|
| macOS | IOPlatformUUID | `ioreg -rd1 -c IOPlatformExpertDevice` |
| macOS | Hardware UUID | `system_profiler SPHardwareDataType` |
| Linux | machine-id | `/etc/machine-id` |
| Linux | dbus-machine-id | `/var/lib/dbus/machine-id` |
| Windows | MachineGuid | Registry: `HKLM\SOFTWARE\Microsoft\Cryptography` |
| Fallback | Generated | `~/.thinkt/machine_id` |

The fingerprint is normalized to a consistent UUID format (lowercase, 8-4-4-4-12) for cross-platform correlation.

**API Server:**
- Environment: `THINKT_API_TOKEN`
- Flag: `--token`
- Header: `Authorization: Bearer <token>`

**MCP Server:**
- Stdio: Uses `THINKT_MCP_TOKEN` environment variable
- HTTP: Uses `THINKT_MCP_TOKEN` env var or `--token` flag
- Header: `Authorization: Bearer <token>`

**Security Features:**
- 256-bit random tokens (32 bytes hex-encoded)
- Constant-time comparison to prevent timing attacks
- `WWW-Authenticate` header on 401 responses
- No authentication by default (local development)

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `THINKT_HOME` | thinkt config and data directory | `~/.thinkt` |
| `THINKT_CLAUDE_HOME` | Claude Code data directory | `~/.claude` |
| `THINKT_KIMI_HOME` | Kimi Code data directory | `~/.kimi-code` |
| `THINKT_GEMINI_HOME` | Gemini CLI data directory | `~/.gemini` |
| `THINKT_COPILOT_HOME` | Copilot data directory | `~/.copilot` |
| `THINKT_CODEX_HOME` | Codex CLI data directory | `~/.codex` |
| `THINKT_QWEN_HOME` | Qwen Code data directory | `~/.qwen` |
| `THINKT_ANTIGRAVITY_HOME` | Antigravity CLI data directory | `~/.gemini/antigravity-cli` |
| `THINKT_LANG` | Override UI language (BCP 47 tag) | auto-detected |
| `THINKT_LOG_FILE` | Write debug logs to this file path | (disabled) |
| `THINKT_VIEWER` | Set `html` for the opt-in shared HTML/nthtml session viewer | existing viewer |
| `THINKT_API_TOKEN` | Bearer token for API server authentication | (none) |
| `THINKT_MCP_TOKEN` | Bearer token for MCP server authentication | (none) |
| `THINKT_MCP_ALLOW_TOOLS` | Comma-separated list of allowed MCP tools | (all) |
| `THINKT_MCP_DENY_TOOLS` | Comma-separated list of denied MCP tools | (none) |
| `THINKT_CORS_ORIGIN` | CORS `Access-Control-Allow-Origin` header | `*` (unauthenticated) |
| `THINKT_COLLECTOR_URL` | Collector endpoint URL for relay | (auto-discover) |
| `THINKT_API_KEY` | Bearer token for collector authentication | (none) |
| `THINKT_PROFILE` | Write CPU profiling to this file path | (disabled) |

### Instance Registry

The instance registry (`internal/config/instances.go`) provides cross-process discovery to prevent port conflicts:

```go
type InstanceType string

const (
    InstanceServer       InstanceType = "server"
    InstanceServerMCP    InstanceType = "server-mcp"
    InstanceIndexerWatch InstanceType = "indexer-watch"
    InstanceCollect      InstanceType = "collect"
    InstanceRelay        InstanceType = "relay"
)
```

- **Storage**: `~/.thinkt/instances.json`
- **Cleanup**: Stale PIDs removed via `syscall.Kill(pid, 0)` check
- **Usage**: Servers check `FindInstanceByPort()` before binding; clear error if port in use

### DuckDB Concurrency (Copy-on-Read)

DuckDB does not support concurrent READ_ONLY connections when a READ_WRITE connection is active. The `internal/indexer/db` package implements a copy-on-read fallback:

**`OpenReadOnly()` strategy:**
1. Retry direct read-only open (5 attempts, 100ms delay)
2. If locked, copy the main DB file to a temp directory
3. Validate the copy by querying actual table data
4. Clean up temp files on `Close()`

**`Watcher` pattern:**
- Opens/closes DB per file change (not long-lived connection)
- Uses mutex to serialize ingestion
- Triggered by `fsnotify` events with 2-second debounce

### Teams

Teams represent multi-agent coordination (e.g., Claude Code swarms). The `TeamStore` interface is separate from `Store` because teams are an overlay concept — they reference agents whose sessions exist in an underlying Store.

- **TeamStore** (`internal/thinkt/teams.go`) — Interface: `ListTeams`, `GetTeam`, `GetTeamTasks`, `GetTeamMessages`
- **ClaudeTeamStore** (`internal/sources/claude/teams.go`) — Implementation reading from `~/.claude/teams/` and `~/.claude/tasks/`
- **Teams CLI** (`internal/cmd/teams.go`) — `thinkt teams [list]` with `--json`, `--active`, `--inactive` flags
- **Teams API** (`internal/server/teams_api.go`) — REST endpoints under `/api/v1/teams` route group, guarded by `requireTeamStore` middleware

**Claude Code team file layout:**
- Config: `~/.claude/teams/{name}/config.json`
- Inboxes: `~/.claude/teams/{name}/inboxes/{member}.json`
- Tasks: `~/.claude/tasks/{name}/{id}.json`

### StoreCache

`StoreCache` (`internal/thinkt/cache.go`) provides project and session caching for Store implementations. Stores embed this struct to avoid repeated filesystem scans. Supports optional TTL for cache expiry (default: no expiry).

### Indexer API

The REST API (`thinkt server`) exposes indexer functionality via `internal/server/indexer_api.go`:

| Endpoint | Handler | Description |
|----------|---------|-------------|
| `GET /api/v1/search` | `handleSearchSessions` | Search indexed sessions with filters |
| `GET /api/v1/semantic-search` | `handleSemanticSearch` | Semantic search using embeddings |
| `GET /api/v1/stats` | `handleGetStats` | Aggregate usage statistics |
| `GET /api/v1/indexer/health` | `handleIndexerHealth` | Indexer binary and DB health |
| `GET /api/v1/indexer/status` | `handleIndexerStatus` | Live indexer status (sync, embedding progress) |

These endpoints query the built-in SQLite index directly (no subprocess).

### Trace Collector & Relay

The collector/relay system enables push-based trace aggregation from multiple machines:

**Relay** (`internal/relay/`) reads the enabled sources' sessions through their stores and ships full `thinkt.Entry` values to a remote collector. The payload is `tracewire.Payload` version 2 (`internal/tracewire`): one session's changed entries, the uuids it no longer has (`removed`), and a full session snapshot.
- **Archive keys:** a session is `(machine_id, source, session_id)`, an entry is `(machine_id, source, session_id, uuid)`. Entries without a uuid get a stable composed one (`relay-noid:…`).
- **State:** `~/.thinkt/relay/state.json` keeps, per session, the entry hashes last shipped, the file's mtime and size, and a generation. Only changes ship, and a restart re-reads only sessions whose files changed. It is saved when dirty: every 30 s, at the end of a reconcile (also an interrupted one) and at shutdown.
- **Generations:** a sync's generation is its start time in nanoseconds (never below the session's last), so a relay that lost its state still ships above what the collector holds.
- **Reconcile:** `thinkt relay` (one-shot) and the start of `--forward` reconcile every session; forward mode starts the watcher first, so writes during the reconcile still ship. `--backfill all|none` (`THINKT_RELAY_BACKFILL`, default `all`) decides whether history already on disk ships on first sight.
- **Vanished sessions:** a session file that disappears sends nothing, and the archive keeps it.
- `FileWatcher` uses fsnotify with 2-second debounce (only `.jsonl` files); each event re-reads that session through its store.
- `Shipper` POSTs with retry (3 attempts, exponential backoff: 1s/2s/4s). After one failed retry cycle the relay buffers without trying until a buffer drain succeeds. A payload the collector rejects permanently (4xx other than 401/403/408/429) is moved to `quarantine/` beside the state file.
- `DiskBuffer` stores payloads as JSON files when the collector is unreachable. While it holds any payload, new ones queue behind it, so a newer sync never overtakes an older one. All payloads of one sync share a generation.
- **One relay per machine:** the relay registers as `InstanceRelay`, and a second one refuses to start. Buffered payloads of sources the config turned off are dropped at start, with their state; a `--source` filter does not drop other sources' payloads.
- `DiscoverCollector()` cascade: `THINKT_COLLECTOR_URL` env → `.thinkt/collector.json` → well-known endpoint → local buffer

**Collector** (`internal/collect/`) receives, normalizes, and stores traces:
- HTTP server with chi router, bearer auth (constant-time comparison), CORS
- `TraceStore` interface backed by DuckDB (`~/.thinkt/collector.duckdb`)
- Single-writer batch pattern: incoming payloads queue to a channel, one goroutine drains and writes each in its own transaction. `IngestBatch` returns after the commit, so a 200 means the payload is stored; a write error answers 500 and the relay retries
- `POST /v1/traces` accepts version 2 only (older bodies get 400 `unsupported_version`), up to 64 MB. A payload with no entries still applies its `removed` and session metadata.
- **Generations:** a payload whose generation is lower than the session's stored one is acknowledged and ignored.
- **Removed entries:** `--superseded keep|delete` (`THINKT_COLLECTOR_SUPERSEDED`, default `keep`). They are kept with `superseded_at` set, or deleted; readers skip superseded rows.
- `AgentRegistry` with heartbeat tracking, stale cleanup after 5 minutes
- `NormalizePayload()` requires machine, source and session ids, and drops entries without a uuid

**Collector API endpoints:**
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/traces` | POST | Ingest trace entries from relays |
| `/v1/traces/search` | GET | Search collected traces by text query |
| `/v1/traces/stats` | GET | Aggregate usage statistics |
| `/v1/agents/register` | POST | Register an exporter agent |
| `/v1/agents` | GET | List registered agents |
| `/v1/collector/health` | GET | Health check (no auth required) |

**DuckDB Schema (distinct from indexer):**
- `sessions` — one row per `(machine_id, source, session_id)`: the latest snapshot, generation, `first_seen`/`last_seen`, activity status
- `entries` — one row per `(machine_id, source, session_id, uuid)`: `entry_json` (the full entry), derived role/kind/text/tool/token columns, `content_hash`, `superseded_at`
- `session_activity` — relay lifecycle events, keyed by the full session key
- `collected_agents` — registered relay agents
- The pre-archive `collected_sessions`/`collected_entries` tables are dropped at startup.

**Standalone Binaries:**
- `thinkt-relay` — Flag-only (no cobra), repeatable `--source`, `--backfill`, env var fallbacks; ships the sources the config enables
- `thinkt-collector` — Flag-only (no cobra), `--port`, `--host`, `--storage`, `--token`, `--superseded`

## Webapps

Two web interfaces are embedded into the binary via git submodules:

| Submodule | Path | Command | Port | Description |
|-----------|------|---------|------|-------------|
| [thinkt-web](https://github.com/wethinkt/thinkt-web) (`dist` branch) | `internal/server/web/` | `thinkt server` / `thinkt web` | 8784 | Full webapp for trace exploration |
| [thinkt-web-lite](https://github.com/wethinkt/thinkt-web-lite) (`dist` branch) | `internal/server/web-lite/` | `thinkt web lite` | 8784 (`/lite/`) | Lightweight debug interface |

### Submodule Setup

After cloning, initialize submodules if not already done:

```bash
git submodule update --init --recursive
```

### Static File Serving

Both webapps are embedded via `//go:embed` directives in `internal/server/static.go`:
- `StaticWebAppHandler()` — serves `web/index.html` + `web/assets/*` (full webapp)
- `StaticLiteWebAppHandler()` — serves `web-lite/index.html` + `web-lite/static/*` (lite)

The `Config.StaticHandler` field selects which handler to use. `thinkt server` sets it to `StaticWebAppHandler()`. Both use SPA routing — non-file paths fall back to `index.html`.

### Co-developing thinkt-web

Use `--dev` to proxy non-API routes to a local frontend dev server (e.g. Vite). This gives you hot module reload and source maps while the Go backend serves the API:

```bash
# Terminal 1: run the frontend dev server
cd ../thinkt-web && npm run dev     # e.g. starts on localhost:8784

# Terminal 2: run the Go backend with dev proxy
thinkt server --dev http://localhost:8784
```

All API routes (`/api/*`, `/swagger/*`) are served by Go. Everything else (SPA, assets, HMR websocket) is reverse-proxied to the frontend dev server.

### Updating the Webapps

1. Make changes inside the submodule directory
2. Commit and push from within the submodule
3. Back in the go-thinkt root, stage the updated ref:
   ```bash
   git add internal/server/web       # or internal/server/web-lite
   git commit -m "update web submodule"
   ```

## Cross-Platform Support

thinkt builds and runs on macOS, Linux, and Windows. Platform-specific code uses Go build tags and `runtime.GOOS` checks:

| Area | Approach | Files |
|------|----------|-------|
| Default apps (Finder/Explorer/xdg-open, terminals, editors) | Build-tagged `DefaultApps()` per platform | `internal/config/apps_darwin.go`, `apps_linux.go`, `apps_windows.go` |
| Machine fingerprint | Build-tagged implementations | `internal/fingerprint/fingerprint_{darwin,linux,windows,fallback}.go` |
| Directory name decoding | `runtime.GOOS` check for Windows drive letters | `internal/sources/claude/projects.go` `DecodeDirName()` |
| Signal handling | `os.Interrupt` only (not `syscall.SIGTERM`) | `internal/cmd/serve.go`, `internal/indexer/cmd/watch.go` |
| Browser opening | `runtime.GOOS` switch: `open` / `xdg-open` / `rundll32` | `internal/cmd/serve.go` `openBrowser()` |

Shared code (editor apps, `BuildCommand`, `Launch`, `checkCommandExists`) lives in `internal/config/apps.go`.

## Documentation Map

| File | Purpose | Editable |
|------|---------|----------|
| README.md | Human-readable project overview | Yes |
| AGENTS.md | Agent instructions (this file) | Yes |
| MEMORY.md | Long-term concepts and decisions | Yes |
| PLAN.md | Implementation planning | Yes |
| PROMPTS.md | Generated by tool or hook | **READ ONLY** |

### Research Reports

Persisted research reports are available in `etc/reports`.

| File | Topic |
|------|-------|
| [etc/reports/thinking-tracer.md](./etc/reports/thinking-tracer.md) | Architecture of the thinking-tracer visualization app |
| [etc/reports/go-patterns.md](./etc/reports/go-patterns.md) | Go project conventions |
| [etc/reports/CLAUDE_STRUCTURE.md](./etc/reports/CLAUDE_STRUCTURE.md) | Claude Code storage structure |
| [etc/reports/CLAUDE_TRANSCRIPT_SCHEMA.md](./etc/reports/CLAUDE_TRANSCRIPT_SCHEMA.md) | Claude Code transcript record types and fields as observed in Oct 2026 (CLI 2.1.3xx) |
| [etc/reports/SOURCE_FORMATS_2026-10.md](./etc/reports/SOURCE_FORMATS_2026-10.md) | Codex, Antigravity, and Kimi Code on-disk formats vs what thinkt reads (Oct 2026), with mappings onto the Entry model |
| [etc/reports/KIMI_STRUCTURE.md](./etc/reports/KIMI_STRUCTURE.md) | Kimi Code storage structure |
| [etc/reports/SESSION_STORAGE.md](./etc/reports/SESSION_STORAGE.md) | Session storage comparison |
| [etc/reports/ONTOLOGY_ANALYSIS.md](./etc/reports/ONTOLOGY_ANALYSIS.md) | Data model ontology |
| [etc/reports/COMPONENT_MODEL.md](./etc/reports/COMPONENT_MODEL.md) | Component architecture |

## Docker

Multi-platform Docker images (`linux/amd64`, `linux/arm64`) are published to `ghcr.io/wethinkt/thinkt`.

### Dockerfiles

Two Dockerfiles exist:

| File | Purpose |
|------|---------|
| `Dockerfile` | Multi-stage build for CI and local use. Builds binary from source. |
| `Dockerfile.goreleaser` | Simple runtime image for GoReleaser. Uses pre-built cross-compiled binary. |

Both use `debian:trixie-slim` runtime.

- Runs as non-root user `thinkt` (uid 5454)
- Home directory: `/data` (so `~/.claude` → `/data/.claude`)
- Entrypoint: `thinkt` (requires subcommand)

### Building

- **CI/Local**: Uses `Dockerfile` with multi-stage build (golang → debian-slim)
- **Release**: Uses `Dockerfile.goreleaser` with `goreleaser-cross` for CGO cross-compilation. See `.goreleaser.yml`.

### Usage

```bash
# Bind-mount session directories (paths resolve automatically via $HOME=/data)
docker run -p 8784:8784 \
  -v ~/.claude:/data/.claude:ro \
  -v ~/.kimi-code:/data/.kimi-code:ro \
  -v ~/.codex:/data/.codex:ro \
  ghcr.io/wethinkt/thinkt:latest server --host 0.0.0.0
```

## Development

```bash
task build      # Build binary to ./bin/thinkt
task test       # Run tests
task lint       # Run linter
task install    # Install to GOPATH/bin
```

### Adding a New Source

1. Create package in `internal/sources/<name>/`
2. Implement `thinkt.Store` interface (embed `StoreCache` for caching)
3. Add `Factory()` function returning `thinkt.StoreFactory`
4. Register the factory in `internal/sources/sources.go` `AllFactories()`
   - Implement `thinkt.SessionPathResolver` on the factory and add the source to `resolveOrder` in `internal/sources/resolve.go`; the TUI loader, index watcher and relay find sources through it
   - Implement `thinkt.SearchLineFilterer` if some lines of a session file are not conversation content, or `thinkt.SearchFileFilterer` if a line can only be judged with the whole file (later lines delete or replace earlier ones)
   - Classify lines with `thinkt.RecordClass` and count unknown types with `thinkt.UnknownTally`
5. Add `SourceColorHex()` entry in `internal/tui/styles.go`
6. Add environment variable support if needed
7. Optionally implement `TeamStoreFactory` for team support
