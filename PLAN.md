# PLAN: thinkt

Current implementation status and roadmap for `thinkt`.

## Current State

The core CLI is functional with multi-source support, TUI with tree view navigation, agent teams, analytics, HTTP/MCP servers, and lite webapp.

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
  - `thinkt collect` — HTTP server on port 4318, DuckDB storage, agent registry
  - `thinkt export` — File watcher, HTTP shipper, disk buffer, discovery cascade
  - `thinkt-exporter` / `thinkt-collector` standalone binaries
  - TUI views: collector status page, exporter status page
  - Collector API: `/v1/traces`, `/v1/traces/search`, `/v1/traces/stats`, `/v1/agents`
  - Prometheus metrics on `/metrics` (collector) and `--metrics-port` (exporter)

- [x] **Documentation Updates** - AGENTS.md, README.md, and Hugo docs updated

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

### Short Term

- [ ] **`thinkt setup` command** - Interactive first-run configuration
  - Create `~/.thinkt/` directory
  - Step through each source, check existence and permissions
  - Prompt user to enable/disable each source
  - Generate `~/.thinkt/config.json` with preferences
  - `--reconfigure` flag to re-run setup
  - Sources respect enabled/disabled in config
  - Environment variables still override for Docker

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
