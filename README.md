# go-thinkt

[![CI](https://github.com/wethinkt/go-thinkt/actions/workflows/ci.yml/badge.svg)](https://github.com/wethinkt/go-thinkt/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/wethinkt/go-thinkt.svg)](https://pkg.go.dev/github.com/wethinkt/go-thinkt)
[![User Guide](https://img.shields.io/badge/User%20Guide-6B2DAD)](https://wethinkt.github.io/go-thinkt/)


**This tool is still in alpha stages and is for educational purposes only in its current state. Consider running it [in a container](#docker).**

`thinkt` is a CLI tool for exploring conversation traces from AI coding assistants.   You can learn more about us at [wethinkt.com](https://wethinkt.com).

There are many local agentic coding environments such as Claude Code, Kimi Code, and Gemini CLI.  As you use them, session data is written locally.  `thinkt` unlocks those "thinking traces" for you.

You can use `thinkt` to...

 * *Explore* the conversation traces with a CLI, TUI, or webapp
 * *Analyze* and index these traces for understanding and governance
 * *Share* these with your tooling and LLMs via an OpenAPI HTTP server and MCP server

All of these LLM Assistant `Source`s have similar structures and use common machinery such as JSONL file: 
 * `Project`s located in *local folders*, which hold many:
 * `Session`s that has many conversation
 * `Turn`s, which each have:
    * one `User Input`
    * multiple `Tool Call`s and `Tool Result`
    * multiple `Thinking` blocks
    * one `LLM Output`

We have a common `thinkt` interface to enable uniform access to various `Sources`.  We maintain a library of implementations and currently support:
  - [*Claude Code*](https://claude.com/product/claude-code) from Anthropic
  - [*Kimi Code*](https://www.kimi.com/code) from Moonshot
  - [*Gemini CLI*](https://geminicli.com) from Google
  - [*Copilot CLI*](https://github.com/features/copilot/cli) from GitHub
  - [*Codex CLI*](https://github.com/openai/codex) from OpenAI

Right now much of the implementation is in package `internal`, but we will eventually build out a public package as it stabilizes.

## [User Guide](https://wethinkt.github.io/go-thinkt/)

## Features

- **Interactive TUI**: Navigate projects, sessions, and conversation content with a keyboard-driven terminal interface
- **Multi-Source Support**: Works with Claude Code (`~/.claude`), Kimi Code (`~/.kimi`), Gemini CLI (`~/.gemini`), Copilot CLI (`~/.copilot`), and Codex CLI (`~/.codex`) — sessions from all sources are shown together
- **Tree View**: Browse projects in a collapsible tree grouped by directory, or switch to a flat list
- **Agent Teams**: Inspect multi-agent teams (Claude Code), including members, tasks, and messages
- **Analytics**: Token usage, tool frequency, word analysis, activity timelines via `thinkt-indexer`
- **Prompt Extraction**: Generate timestamped logs of user prompts in markdown, JSON, or plain text
- **MCP Server**: Model Context Protocol integration for use with AI assistants
- **REST API**: HTTP server for programmatic access
- **Web Interface**: Full webapp for visual trace exploration via `thinkt web`
- **Lite Webapp**: Lightweight debug interface with i18n (EN/ES/中文), connection status, and "open-in" buttons
- **Themes**: Customizable color themes with interactive theme builder
- **App Management**: Configure open-in apps and default terminal via `thinkt apps`

## Installation

### Homebrew

```bash
brew install --cask wethinkt/tap/thinkt
```

### Go

```bash
go install github.com/wethinkt/go-thinkt/cmd/thinkt@latest
```

### From Source

```bash
git clone --recurse-submodules https://github.com/wethinkt/go-thinkt.git
cd go-thinkt
task build
```

### Docker

Multi-platform Docker images are available for `linux/amd64` and `linux/arm64`:

```bash
docker pull ghcr.io/wethinkt/thinkt:latest
```

The container user's home directory is `/data`, so default paths like `~/.claude` resolve to `/data/.claude`. Simply bind-mount your session directories:

```bash
# Run the HTTP server
docker run -p 8784:8784 \
  -v ~/.claude:/data/.claude:ro \
  -v ~/.kimi:/data/.kimi:ro \
  -v ~/.codex:/data/.codex:ro \
  ghcr.io/wethinkt/thinkt:latest server --host 0.0.0.0

# Run any command
docker run \
  -v ~/.claude:/data/.claude:ro \
  -v ~/.codex:/data/.codex:ro \
  ghcr.io/wethinkt/thinkt:latest projects

# Show help
docker run ghcr.io/wethinkt/thinkt:latest --help
```

## Quick Start

```bash
# Launch interactive TUI (default)
thinkt

# List available sources
thinkt sources list

# Browse projects
thinkt projects
thinkt projects --short
thinkt projects tree

# View sessions
thinkt sessions list
thinkt sessions view
thinkt sessions resolve -p ./myproject <session-id>

# List agent teams
thinkt teams

# Open the web interface
thinkt web

# Start HTTP server without opening browser
thinkt server --no-open

# Debug logging
thinkt tui --log /tmp/thinkt-debug.log
```

## Commands

| Command | Description |
|---------|-------------|
| `thinkt` | Launch interactive TUI (default) |
| `thinkt tui` | Launch interactive TUI |
| `thinkt sources` | List available sources (claude, kimi, gemini, copilot, codex) |
| `thinkt sources status` | Show detailed source status |
| `thinkt projects` | List all projects (detailed columns) |
| `thinkt projects --short` | List project paths only |
| `thinkt projects tree` | Tree view grouped by parent directory |
| `thinkt projects summary` | Detailed project info |
| `thinkt sessions list` | List sessions in a project |
| `thinkt sessions view` | View session in terminal |
| `thinkt sessions resolve` | Resolve a session query to its canonical path |
| `thinkt sessions copy` | Copy a known session to another path |
| `thinkt sessions delete` | Delete a known session (with confirmation) |
| `thinkt teams` | List agent teams (Claude Code) |
| `thinkt teams list` | Same as above |
| `thinkt prompts extract` | Extract prompts to markdown/JSON |
| `thinkt server` | Start web interface and REST API (port 8784) |
| `thinkt server start/stop/status` | Manage background server |
| `thinkt server logs` | View server logs |
| `thinkt server mcp` | Start MCP server |
| `thinkt server token` | Generate secure authentication token |
| `thinkt server fingerprint` | Display machine fingerprint for workspace correlation |
| `thinkt web` | Open web interface in browser |
| `thinkt web lite` | Open lightweight debug webapp |
| `thinkt apps` | List configured open-in apps |
| `thinkt apps enable/disable` | Enable or disable an app |
| `thinkt apps set-terminal` | Set the default terminal app |
| `thinkt theme` | Display current theme |
| `thinkt theme builder` | Interactive theme editor |

## Indexer (DuckDB-Powered)

The `thinkt-indexer` provides fast, searchable storage for your conversation traces:

```bash
# Sync all sessions to the index
thinkt-indexer sync

# Search across indexed sessions (case-insensitive by default)
thinkt-indexer search "authentication"

# Case-sensitive or regex search
thinkt-indexer search "AuthManager" --case-sensitive
thinkt-indexer search --regex "func\s+Test\w+"

# Show usage statistics
thinkt-indexer stats

# Watch for changes and auto-index
thinkt-indexer watch
```

Indexer data is stored in `~/.thinkt/index.duckdb`.

## TUI Keyboard Shortcuts

The interactive TUI uses a navigation stack. ESC goes back to the previous screen; q or ctrl+c exits the app.

**Project Picker:**

| Key | Action |
|-----|--------|
| `enter` | Select project / toggle directory |
| `/` | Search/filter |
| `t` | Toggle tree view / flat list |
| `space` | Toggle directory expand/collapse |
| `left` / `right` | Collapse / expand directory |
| `d` | Sort by date |
| `n` | Sort by name |
| `s` | Filter by source |
| `esc` | Back |
| `q` / `ctrl+c` | Quit |

**Session Picker:**

| Key | Action |
|-----|--------|
| `enter` | Select session |
| `/` | Search/filter |
| `s` | Filter by source |
| `esc` | Back to project picker |
| `q` / `ctrl+c` | Quit |

**Session Viewer:**

| Key | Action |
|-----|--------|
| `up` / `down` / `j` / `k` | Scroll |
| `pgup` / `pgdn` | Page up/down |
| `g` / `G` | Go to top / bottom |
| `esc` | Back to session picker |
| `q` / `ctrl+c` | Quit |

## Server Options

```bash
# Control HTTP logging
thinkt server --quiet              # Suppress HTTP logs
thinkt server --http-log file.log  # Log to file
thinkt server --no-open            # Don't auto-open browser

# Background management
thinkt server start                # Start in background
thinkt server status               # Check server status
thinkt server logs                 # View server logs
thinkt server logs -f              # Follow logs
thinkt server stop                 # Stop background server
```

## Default Ports

| Command | Port | Description |
|---------|------|-------------|
| `thinkt server` | 8784 | Full web interface and REST API |
| `thinkt web lite` | 8784 | Lightweight debug webapp (served at `/lite/`) |
| `thinkt server mcp --port` | 8786 | MCP server over HTTP |
| [VS Code extension](https://github.com/wethinkt/thinkt-vscode) | 8787 | Reserved for embedded server |

Use `-p` or `--port` to override the default port for any server.

## Authentication

Both the REST API server (`thinkt server`) and MCP server (`thinkt server mcp`) support Bearer token authentication to protect access to your conversation data.

### Generate a Token

```bash
thinkt server token
# Output: thinkt_20260205_cd3bf36d6e1fc71e9bf033a7131f77cb
```

### API Server Authentication

```bash
# Using environment variable
export THINKT_API_TOKEN=$(thinkt server token)
thinkt server

# Using command-line flag
thinkt server --token thinkt_20260205_...

# Client request
curl -H "Authorization: Bearer thinkt_20260205_..." http://localhost:8784/api/v1/sources
```

### MCP Server Authentication

For stdio transport (default), authentication uses environment variables:

```bash
# Claude Desktop configuration with authentication
export THINKT_MCP_TOKEN=$(thinkt server token)
```

For HTTP transport:

```bash
# Using environment variable
export THINKT_MCP_TOKEN=$(thinkt server token)
thinkt server mcp --port 8786

# Using command-line flag
thinkt server mcp --port 8786 --token thinkt_20260205_...
```

Clients must pass the token in the `Authorization` header:
```
Authorization: Bearer thinkt_20260205_...
```

## Cross-Platform Support

thinkt runs on macOS, Linux, and Windows. Platform-specific behavior is handled automatically:
- **Default apps**: Finder/Terminal/iTerm (macOS), xdg-open/x-terminal-emulator (Linux), Explorer/Windows Terminal/cmd (Windows), plus VS Code/Cursor/Zed on all platforms. Manage with `thinkt apps`
- **Machine fingerprint**: IOPlatformUUID (macOS), `/etc/machine-id` (Linux), registry MachineGuid (Windows)
- **Browser opening**: `open` (macOS), `xdg-open` (Linux), `rundll32` (Windows)

## Machine Fingerprint

Use `thinkt server fingerprint` to display a unique machine identifier. This fingerprint is derived from system identifiers (e.g., hardware UUID on macOS, `/etc/machine-id` on Linux, MachineGuid on Windows) and can be used to correlate sessions across different AI coding assistant sources on the same machine.

```bash
# Display fingerprint
thinkt server fingerprint

# JSON output
thinkt server fingerprint --json
```

The fingerprint is normalized to a consistent UUID format across all platforms.

## Lite Webapp Features

For full trace exploration, use `thinkt server` which provides the full web interface on port 8784.

The lightweight webapp (`thinkt web lite`) provides a quick debug interface:

- **Internationalization**: English, Spanish, and Chinese (auto-detected)
- **Connection Status**: Real-time indicator showing server connectivity
- **Source Visibility**: Toggle eye icons to show/hide projects by source
- **Open-In Buttons**: Quick buttons to open projects in VS Code, Cursor, etc.
- **Language Selector**: Switch between EN/ES/中文 in the top-right corner

## REST API

The `thinkt server` command provides a REST API for programmatic access:

```bash
# Start the server
thinkt server

# With authentication
export THINKT_API_TOKEN=$(thinkt server token)
thinkt server
```

### API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/sources` | List available trace sources |
| `GET /api/v1/projects` | List all projects |
| `GET /api/v1/projects/{id}/sessions` | List sessions for a project |
| `GET /api/v1/sessions/{path}` | Get session content |
| `GET /api/v1/search?q=query` | **Search indexed sessions** |
| `GET /api/v1/stats` | **Get usage statistics** |
| `GET /api/v1/indexer/health` | **Check indexer health** |
| `GET /api/v1/teams` | List agent teams |
| `POST /api/v1/open-in` | Open path in application |

Swagger documentation is available at `http://localhost:8784/swagger`.

## MCP Integration

Use `thinkt` as an MCP server for AI assistants like Claude Desktop:

```json
{
  "mcpServers": {
    "thinkt": {
      "command": "thinkt",
      "args": ["server", "mcp"],
      "env": {
        "THINKT_MCP_TOKEN": "your-secure-token-here"
      }
    }
  }
}
```

Generate a secure token with:
```bash
thinkt server token
```

Available MCP tools:
- `list_sources` - List available session sources
- `list_projects` - List projects from all sources
- `list_sessions` - List sessions for a project
- `get_session_metadata` - Get session metadata
- `get_session_entries` - Get session content with pagination
- `search_sessions` - Search across indexed sessions (supports regex)
- `get_usage_stats` - Get aggregate usage statistics

See [Authentication](#authentication) for more details on securing the MCP server.

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `THINKT_CLAUDE_HOME` | Claude Code data directory | `~/.claude` |
| `THINKT_KIMI_HOME` | Kimi Code data directory | `~/.kimi` |
| `THINKT_GEMINI_HOME` | Gemini CLI data directory | `~/.gemini` |
| `THINKT_COPILOT_HOME` | Copilot data directory | `~/.copilot` |
| `THINKT_CODEX_HOME` | Codex CLI data directory | `~/.codex` |
| `THINKT_API_TOKEN` | Bearer token for API server authentication | (none) |
| `THINKT_MCP_TOKEN` | Bearer token for MCP server authentication | (none) |
| `THINKT_PROFILE` | Write CPU profiling to this file path | (disabled) |

## Related Projects

- [Thinking Tracer](https://github.com/Brain-STM-org/thinking-tracer) - visualization tool for exploring LLM conversation traces

## License

Created with :heart: and :fire: by the team at [Neomantra](https://www.neomantra.net) and [BrainSTM](https://brain-stm.org).

Released under the MIT License - see [LICENSE.txt](./LICENSE.txt)
