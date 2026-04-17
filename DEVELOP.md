# Development Guide

## Organization Overview

`go-thinkt` is part of the [wethinkt](https://github.com/wethinkt) organization. The suite of projects:

| Repository | Language | Description |
|---|---|---|
| [go-thinkt](https://github.com/wethinkt/go-thinkt) | Go | Core CLI, TUI, REST API, and MCP server |
| [ts-thinkt](https://github.com/wethinkt/ts-thinkt) | TypeScript | Shared library — types, JSONL parsers, API client |
| [thinkt-web](https://github.com/wethinkt/thinkt-web) | TypeScript | Full web application, served by `thinkt server` |
| [thinkt-web-lite](https://github.com/wethinkt/thinkt-web-lite) | HTML/CSS/JS | Lightweight dashboard, served by `thinkt web lite` |
| [thinkt-vscode](https://github.com/wethinkt/thinkt-vscode) | TypeScript | VS Code extension for exploring LLM conversations |
| [homebrew-tap](https://github.com/wethinkt/homebrew-tap) | — | Homebrew tap for `brew install` distribution |

### How they fit together

```
ts-thinkt            (shared TypeScript types, parsers, API client)
  └── thinkt-web     (full webapp, depends on ts-thinkt via file: link)

go-thinkt            (Go CLI + server, embeds web apps as submodules)
  ├── internal/server/web/       ← thinkt-web (branch: dist)
  └── internal/server/web-lite/  ← thinkt-web-lite

thinkt-vscode        (VS Code extension, connects to go-thinkt API)
```

`ts-thinkt` provides the object model and API client that `thinkt-web` consumes. The Go server embeds the built web assets via `go:embed` and serves them at runtime.

## Project Structure

```
go-thinkt/
├── cmd/
│   └── thinkt/                  # CLI entry point → internal/cmd
├── internal/
│   ├── cmd/                     # CLI command definitions (cobra)
│   ├── cli/                     # CLI helpers (list, view, copy, delete)
│   ├── config/                  # Configuration, platform app detection
│   ├── fingerprint/             # Machine fingerprint hashing
│   ├── index/                   # SQLite index, ingester, watcher, search
│   │   ├── db/                  # SQLite driver, schema, migrations
│   │   ├── embedding/           # On-device embedding (yzma + sqlite-vec)
│   │   ├── llm/                 # Local LLM runtime management
│   │   ├── search/              # Text, metadata, and semantic search
│   │   └── summarize/           # Local summarization pipeline
│   ├── jsonl/                   # JSONL format parsing
│   ├── prompt/                  # Prompt extraction and templates
│   ├── server/                  # HTTP/REST API and MCP server
│   │   ├── docs/                # Swagger/OpenAPI generated docs
│   │   ├── web/                 # ← thinkt-web submodule (branch: dist)
│   │   └── web-lite/            # ← thinkt-web-lite submodule
│   ├── sources/                 # Per-source implementations
│   │   ├── claude/              # Claude Code (~/.claude)
│   │   ├── kimi/                # Kimi Code (~/.kimi)
│   │   ├── gemini/              # Gemini CLI (~/.gemini)
│   │   ├── copilot/             # Copilot CLI (~/.copilot)
│   │   ├── codex/               # Codex CLI (~/.codex)
│   │   └── qwen/                # Qwen Code (~/.qwen)
│   ├── thinkt/                  # Core abstraction layer (types, discovery, caching)
│   ├── tui/                     # Terminal UI (bubbletea)
│   │   ├── theme/               # Theme definitions and builder
│   │   └── colorpicker/         # Color picker widget
│   ├── tuilog/                  # TUI logging
│   └── version/                 # Version info
├── docs/                        # Hugo documentation site
├── etc/                         # Dockerfiles, analysis reports
├── completions/                 # Shell completions (bash, fish, zsh)
├── manpages/                    # Generated man pages
├── Taskfile.yml                 # Build orchestration
├── .goreleaser.yml              # Release config
├── Dockerfile                   # Docker build
└── go.mod                       # Go 1.25.8
```

## Binary

`thinkt` is the single CLI. It requires CGO (`CGO_ENABLED=1`) for the SQLite index and the `sqlite-vec` extension. Pre-built binaries are distributed via GitHub Releases, Homebrew, and Docker.

Provides: TUI, project/session browsing, HTTP + MCP server, SQLite-backed search and stats, session export, prompt extraction, theme builder.

## Prerequisites

- **Go 1.25.8+**
- **[Task](https://taskfile.dev/)** (build orchestration)
- **swag** (Swagger codegen): `go install github.com/swaggo/swag/cmd/swag@v1.16.6`
- **C compiler** (CGO is required for the SQLite index)
- **Git submodules** must be initialized for the embedded web apps

## Building

```bash
# Clone with submodules
git clone --recurse-submodules https://github.com/wethinkt/go-thinkt.git
cd go-thinkt

# Set up developer dependencies -- needed when you update Golang
task dev-deps

# Build thinkt
task build

# Output goes to ./bin/
```

If you already have a clone, init submodules with:

```bash
task submodules
# or: git submodule update --init --recursive
```

## Common Tasks

```bash
task test               # Run all tests
task lint               # Run golangci-lint
task install            # Install thinkt to GOPATH/bin
task clean              # Remove build artifacts

# Swagger/OpenAPI
task server:swag-v2     # Regenerate docs from api.go annotations

# Documentation
task docs:build         # Generate man pages and hugo markdown
task docs:hugo:serve    # Serve hugo docs locally

# MCP schema inspection
task mcp:stdio-schema   # Dump MCP tool/resource schemas

# Docker
task docker:build-thinkt

# Release testing
task release:test       # goreleaser snapshot build
```

## Web App Development

### thinkt-web (full webapp)

Lives in a separate repo. The `dist` branch is embedded as a git submodule at `internal/server/web/`.

To develop:
1. Run `thinkt server` on port 8784 (the API server)
2. In the `thinkt-web` repo: `npm run dev` (runs on port 7434, proxies `/api` to 8784)
3. Build with `npm run build`, push to the `dist` branch
4. Update the submodule ref in go-thinkt

### thinkt-web-lite (lightweight dashboard)

Vanilla HTML/CSS/JS — no build tools. Submodule at `internal/server/web-lite/`.

Edit files directly, rebuild `thinkt`, and run `thinkt web lite` to test.

### Embedding

Both web apps are embedded into the Go binary via `//go:embed` in `internal/server/static.go`. The SPA handler serves `index.html` for all non-file routes.

## Architecture Notes

### Multi-source abstraction

`internal/thinkt/` defines the common interface (`Source`, `Project`, `Session`, `Entry`). Each source under `internal/sources/` implements discovery and parsing for a specific AI assistant's local file format.

### Lazy loading

Session data is loaded on demand. `internal/thinkt/lazy.go` and `internal/thinkt/cache.go` provide lazy session wrappers and caching to avoid reading all JSONL files upfront.

### Platform support

Platform-specific code uses Go build tags: `_darwin.go`, `_linux.go`, `_windows.go`, `_freebsd.go`. This covers app detection (`internal/config/`), process handling, browser opening, and machine fingerprinting.

### CGO

`thinkt` requires CGO for the SQLite driver (`mattn/go-sqlite3`) and the `sqlite-vec` extension. The local LLM runtime (`yzma`) uses `purego`/libffi to load llama.cpp at runtime, so it does not require CGO at build time.

### Security

- Path validation prevents directory traversal in API/MCP endpoints (`internal/thinkt/security.go`)
- DuckDB (collector) connections disable `enable_external_access` to prevent SQL injection file access
- Optional Bearer token auth for both REST API and MCP server

## Ports

| Command | Default Port |
|---|---|
| `thinkt server` | 8784 |
| `thinkt web lite` | 8784 (served at `/lite/`) |
| `thinkt server mcp --port` | 8786 |
| thinkt-vscode (reserved) | 8787 |
| `thinkt collect` | 8785 (includes `/metrics`) |
| `thinkt relay --metrics-port` | disabled (opt-in) |

## Release

Releases use [GoReleaser](https://goreleaser.com/) with `.goreleaser.yml` — a single `thinkt` binary with `CGO_ENABLED=1` for Linux and macOS.

macOS binaries are code-signed and notarized via `task release:notarize`.

Distribution channels: GitHub Releases, Homebrew (`wethinkt/tap`), Docker (`ghcr.io/wethinkt/thinkt`).
