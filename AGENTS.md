# AGENTS: `go-thinkt`

## Project Overview

`go-thinkt` provides `thinkt`, a CLI tool for exploring and extracting data from AI coding assistant sessions. It is a companion to the [wethinkt](https://github.com/wethinkt/wethinkt) web visualization application.  Right now much is internal, but we will eventually build out a public package.

**Stack**: Go, Cobra (CLI), BubbleTea (TUI), Chi (HTTP), MCP SDK

Released under the MIT license, see [LICENSE.txt](./LICENSE.txt).

## Architecture

### Multi-Source Design

The tool supports multiple AI coding assistants via a `Store` interface:
- **Claude Code** (`~/.claude`) - Primary source
- **Kimi Code** (`~/.kimi`) - Secondary source
- **Gemini CLI** (`~/.gemini`) - Tertiary source
- **GitHub Copilot** (`~/.copilot`) - Quaternary source

Sources are auto-discovered. Use `--source kimi|claude|gemini|copilot` flags to filter.

### Key Packages

| Package | Purpose |
|---------|---------|
| `cmd/thinkt` | CLI entry point (Cobra commands) |
| `internal/thinkt` | Core types, Store/TeamStore interfaces, registry, cache |
| `internal/sources/claude` | Claude Code storage + teams implementation |
| `internal/sources/kimi` | Kimi Code storage implementation |
| `internal/sources/gemini` | Gemini CLI storage implementation |
| `internal/sources/copilot` | Copilot storage implementation |
| `internal/tui` | BubbleTea terminal UI (shell, pickers, viewer, theme builder) |
| `internal/server` | HTTP REST API, teams API, and MCP server |
| `internal/server/web-lite` | Lite webapp submodule ([thinkt-web-lite](https://github.com/wethinkt/thinkt-web-lite)) |
| `internal/analytics` | Analytics |
| `internal/prompt` | Prompt extraction and formatting |
| `internal/config` | Configuration management |
| `internal/fingerprint` | Machine fingerprint generation |

### Command Structure

```
thinkt
├── tui                 # Interactive TUI browser (default)
├── serve               # HTTP/MCP servers
│   ├── mcp             # MCP server (stdio or HTTP)
│   ├── lite            # Lightweight debug webapp
│   ├── token           # Generate secure authentication token
│   └── fingerprint     # Display machine fingerprint
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

Use `--log <file>` to write debug logs (available on `thinkt`, `thinkt tui`, and all `serve` subcommands):

```bash
thinkt tui --log /tmp/thinkt-debug.log
```

### Serve Command Flags

| Flag | Description |
|------|-------------|
| `--port, -p` | Server port (default: 8784 for serve, 8785 for lite, 8786 for mcp, 8787 reserved for VS Code extension) |
| `--host` | Server host (default: localhost) |
| `--no-open` | Don't auto-open browser |
| `--quiet, -q` | Suppress HTTP request logging |
| `--http-log <file>` | Write HTTP access log to file |
| `--log` | Write debug log to file |
| `--token` | Bearer token for authentication (API and MCP HTTP) |

### Authentication

Both the REST API server and MCP HTTP server support Bearer token authentication.

**Token Generation:**
```bash
thinkt serve token  # Generates thinkt_YYYYMMDD_<random> format
```

### Machine Fingerprint

Display the unique machine identifier:
```bash
thinkt serve fingerprint              # Human-readable output
thinkt serve fingerprint --json       # JSON output with source details
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
| `THINKT_API_TOKEN` | Bearer token for API server authentication | (none) |
| `THINKT_MCP_TOKEN` | Bearer token for MCP server authentication | (none) |
| `THINKT_PROFILE` | Write CPU profiling to this file path | (disabled) |

### Teams

Teams represent multi-agent coordination (e.g., Claude Code swarms). The `TeamStore` interface is separate from `Store` because teams are an overlay concept — they reference agents whose sessions exist in an underlying Store.

- **TeamStore** (`internal/thinkt/teams.go`) — Interface: `ListTeams`, `GetTeam`, `GetTeamTasks`, `GetTeamMessages`
- **ClaudeTeamStore** (`internal/sources/claude/teams.go`) — Implementation reading from `~/.claude/teams/` and `~/.claude/tasks/`
- **Teams CLI** (`internal/cmd/teams.go`) — `thinkt teams [list]` with `--json`, `--active`, `--inactive` flags
- **Teams API** (`internal/server/teams_api.go`) — REST endpoints: `/api/v1/teams`, `/api/v1/teams/{name}`, etc.

**Claude Code team file layout:**
- Config: `~/.claude/teams/{name}/config.json`
- Inboxes: `~/.claude/teams/{name}/inboxes/{member}.json`
- Tasks: `~/.claude/tasks/{name}/{id}.json`

### StoreCache

`StoreCache` (`internal/thinkt/cache.go`) provides project and session caching for Store implementations. Stores embed this struct to avoid repeated filesystem scans. Supports optional TTL for cache expiry (default: no expiry).

## Lite Webapp

The lightweight webapp (`thinkt serve lite`) lives in the [thinkt-web-lite](https://github.com/wethinkt/thinkt-web-lite) repo, included as a git submodule at `internal/server/web-lite/`.

### Submodule Setup

After cloning, initialize submodules if not already done:

```bash
git submodule update --init --recursive
```

### Structure

```
internal/server/web-lite/   # git submodule → thinkt-web-lite
├── index.html              # Main HTML file
└── static/
    ├── style.css           # Stylesheet
    └── i18n.js             # Internationalization (EN/ES/ZH)
```

### Static File Serving

Only `index.html` and `static/*` are embedded via `//go:embed` in `internal/server/static.go`. Other files in the submodule (README, AGENTS.md, LICENSE, etc.) are excluded from the binary.

### Updating the Webapp

1. Make changes inside `internal/server/web-lite/`
2. Commit and push from within the submodule
3. Back in the go-thinkt root, stage the updated ref:
   ```bash
   git add internal/server/web-lite
   git commit -m "update web-lite submodule"
   ```

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
  ghcr.io/wethinkt/thinkt:latest serve --host 0.0.0.0
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
