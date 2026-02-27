# AGENTS: `go-thinkt`

## Project Overview

`go-thinkt` provides `thinkt`, a CLI tool for exploring and extracting data from AI coding assistant sessions. It is a companion to the [wethinkt](https://github.com/wethinkt/wethinkt) web visualization application.  Right now much is internal, but we will eventually build out a public package.

**Stack**: Go, Cobra (CLI), BubbleTea (TUI), Chi (HTTP), MCP SDK

Released under the MIT license, see [LICENSE.txt](./LICENSE.txt).

## Architecture

### Multi-Source Design

The tool supports multiple AI coding assistants via a `Store` interface:
- **Claude Code** (`~/.claude`)
- **Kimi Code** (`~/.kimi`)
- **Gemini CLI** (`~/.gemini`)
- **GitHub Copilot** (`~/.copilot`)
- **Codex CLI** (`~/.codex`)
- **Qwen Code** (`~/.qwen`)

Sources are auto-discovered. Use `--source kimi|claude|gemini|copilot|codex|qwen` flags to filter.

### Key Packages

| Package | Purpose |
|---------|---------|
| `cmd/thinkt` | CLI entry point (Cobra commands) |
| `cmd/thinkt-indexer` | DuckDB-powered indexer CLI |
| `internal/thinkt` | Core types, Store/TeamStore interfaces, registry, cache |
| `internal/sources/claude` | Claude Code storage + teams implementation |
| `internal/sources/kimi` | Kimi Code storage implementation |
| `internal/sources/gemini` | Gemini CLI storage implementation |
| `internal/sources/copilot` | Copilot storage implementation |
| `internal/sources/codex` | Codex CLI storage implementation |
| `internal/sources/qwen` | Qwen Code storage implementation |
| `internal/tui` | BubbleTea terminal UI (shell, pickers, viewer, theme builder) |
| `internal/server` | HTTP REST API, teams API, indexer API, and MCP server |
| `internal/server/web` | Full webapp submodule ([thinkt-web](https://github.com/wethinkt/thinkt-web), `dist` branch) |
| `internal/server/web-lite` | Lite webapp submodule ([thinkt-web-lite](https://github.com/wethinkt/thinkt-web-lite), `dist` branch) |
| `internal/indexer` | Indexer ingestion, watching, and search |
| `internal/indexer/db` | DuckDB database layer with copy-on-read support |
| `internal/analytics` | Analytics |
| `internal/prompt` | Prompt extraction and formatting |
| `internal/config` | Configuration management, instance registry |
| `internal/fingerprint` | Machine fingerprint generation |

### Command Structure

```
thinkt
├── tui                 # Interactive TUI browser (default)
├── server              # HTTP/MCP servers
│   ├── start           # Start server in background
│   ├── stop            # Stop background server
│   ├── status          # Show server status
│   ├── logs            # View server logs
│   ├── mcp             # MCP server (stdio or HTTP)
│   ├── token           # Generate secure authentication token
│   └── fingerprint     # Display machine fingerprint
├── web                 # Open web interface in browser
│   └── lite            # Lightweight debug webapp
├── sources             # Source management
│   ├── list
│   └── status
├── projects            # Project management
│   ├── tree
│   ├── summary
│   ├── delete
│   └── copy
├── sessions            # Session management
│   ├── list
│   ├── summary
│   ├── view
│   ├── delete
│   └── copy
├── teams               # Agent team management
│   └── list
├── prompts             # Prompt extraction
│   ├── extract
│   ├── list
│   ├── info
│   └── templates
└── theme               # Theme management
    ├── list
    ├── set
    └── builder

thinkt-indexer          # DuckDB-powered indexer (separate binary)
├── sync                # Full sync of all sessions
├── search              # Search across indexed sessions
├── stats               # Show usage statistics
└── watch               # Watch for changes and auto-index
```

### TUI Architecture

The TUI uses a `Shell` with a `NavStack` that manages page navigation:

- **Shell** (`shell.go`) — Top-level model, manages source discovery and page stack
- **ProjectPickerModel** (`project_picker.go`) — Tree or flat project list with source filtering and sorting
- **SessionPickerModel** (`session_picker.go`) — Session list with source filtering, color-coded source badges
- **MultiViewerModel** (`multi_viewer.go`) — Lazy-loading session viewer with viewport scrolling
- **SourcePickerModel** (`source_picker.go`) — Overlay for filtering by source (used within pickers)
- **ThemeBuilderModel** (`theme_builder.go`) — Standalone theme editor with color picker

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
| `THINKT_KIMI_HOME` | Kimi Code data directory | `~/.kimi` |
| `THINKT_CLAUDE_HOME` | Claude Code data directory | `~/.claude` |
| `THINKT_GEMINI_HOME` | Gemini CLI data directory | `~/.gemini` |
| `THINKT_COPILOT_HOME` | Copilot data directory | `~/.copilot` |
| `THINKT_CODEX_HOME` | Codex CLI data directory | `~/.codex` |
| `THINKT_QWEN_HOME` | Qwen Code data directory | `~/.qwen` |
| `THINKT_API_TOKEN` | Bearer token for API server authentication | (none) |
| `THINKT_MCP_TOKEN` | Bearer token for MCP server authentication | (none) |
| `THINKT_PROFILE` | Write CPU profiling to this file path | (disabled) |

### Instance Registry

The instance registry (`internal/config/instances.go`) provides cross-process discovery to prevent port conflicts:

```go
type InstanceType string

const (
    InstanceServer       InstanceType = "server"
    InstanceServerMCP    InstanceType = "server-mcp"
    InstanceIndexerWatch InstanceType = "indexer-watch"
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
| `GET /api/v1/stats` | `handleGetStats` | Aggregate usage statistics |
| `GET /api/v1/indexer/health` | `handleIndexerHealth` | Indexer binary and DB health |

These endpoints shell out to the `thinkt-indexer` binary (same pattern as MCP tools).

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

Both use `debian:bookworm-slim` runtime.

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
  -v ~/.kimi:/data/.kimi:ro \
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
4. Register in `internal/cmd/registry.go` `CreateSourceRegistry()` and `internal/tui/shell.go` `loadSourcesCmd()`
5. Add `SourceColorHex()` entry in `internal/tui/styles.go`
6. Add environment variable support if needed
7. Optionally implement `TeamStoreFactory` for team support
