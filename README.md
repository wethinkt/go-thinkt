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

Right now much of the implementation is in package `internal`, but we will eventually build out a public package as it stabilizes.

## [User Guide](https://wethinkt.github.io/go-thinkt/)

## Features

- **Interactive TUI**: Navigate projects, sessions, and conversation content with a keyboard-driven terminal interface
- **Multi-Source Support**: Works with Claude Code (`~/.claude`), Kimi Code (`~/.kimi`), Gemini CLI, and Copilot — sessions from all sources are shown together
- **Tree View**: Browse projects in a collapsible tree grouped by directory, or switch to a flat list
- **Agent Teams**: Inspect multi-agent teams (Claude Code), including members, tasks, and messages
- **Analytics**: Token usage, tool frequency, word analysis, activity timelines
- **Prompt Extraction**: Generate timestamped logs of user prompts in markdown, JSON, or plain text
- **MCP Server**: Model Context Protocol integration for use with AI assistants
- **REST API**: HTTP server for programmatic access
- **Lite Webapp**: Lightweight debug interface with i18n (EN/ES/中文), connection status, and "open-in" buttons
- **Themes**: Customizable color themes with interactive theme builder

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
  ghcr.io/wethinkt/thinkt:latest serve --host 0.0.0.0

# Run any command
docker run -v ~/.claude:/data/.claude:ro \
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

# List agent teams
thinkt teams

# Start the lite webapp
thinkt serve lite

# Start HTTP server without opening browser
thinkt serve --no-open

# Debug logging
thinkt tui --log /tmp/thinkt-debug.log
```

## Commands

| Command | Description |
|---------|-------------|
| `thinkt` | Launch interactive TUI (default) |
| `thinkt tui` | Launch interactive TUI |
| `thinkt sources` | List available sources (kimi, claude, gemini, copilot) |
| `thinkt sources status` | Show detailed source status |
| `thinkt projects` | List all projects (detailed columns) |
| `thinkt projects --short` | List project paths only |
| `thinkt projects tree` | Tree view grouped by parent directory |
| `thinkt projects summary` | Detailed project info |
| `thinkt sessions list` | List sessions in a project |
| `thinkt sessions view` | View session in terminal |
| `thinkt teams` | List agent teams (Claude Code) |
| `thinkt teams list` | Same as above |
| `thinkt prompts extract` | Extract prompts to markdown/JSON |
| `thinkt serve` | Start HTTP server (port 8784) |
| `thinkt serve lite` | Start lightweight webapp (port 8785) |
| `thinkt serve mcp` | Start MCP server |
| `thinkt serve token` | Generate secure authentication token |
| `thinkt serve fingerprint` | Display machine fingerprint for workspace correlation |
| `thinkt theme` | Display current theme |
| `thinkt theme builder` | Interactive theme editor |

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

## Serve Options

```bash
# Control HTTP logging
thinkt serve --quiet              # Suppress HTTP logs
thinkt serve --http-log file.log  # Log to file
thinkt serve --no-open            # Don't auto-open browser

# These also work with serve lite
thinkt serve lite --quiet --no-open
```

## Default Ports

| Command | Port | Description |
|---------|------|-------------|
| `thinkt serve` | 8784 | REST API and web interface |
| `thinkt serve lite` | 8785 | Lightweight debug webapp |
| `thinkt serve mcp --port` | 8786 | MCP server over HTTP |
| [VS Code extension](https://github.com/wethinkt/thinkt-vscode) | 8787 | Reserved for embedded server |

Use `-p` or `--port` to override the default port for any server.

## Authentication

Both the REST API server (`thinkt serve`) and MCP server (`thinkt serve mcp`) support Bearer token authentication to protect access to your conversation data.

### Generate a Token

```bash
thinkt serve token
# Output: thinkt_20260205_cd3bf36d6e1fc71e9bf033a7131f77cb
```

### API Server Authentication

```bash
# Using environment variable
export THINKT_API_TOKEN=$(thinkt serve token)
thinkt serve

# Using command-line flag
thinkt serve --token thinkt_20260205_...

# Client request
curl -H "Authorization: Bearer thinkt_20260205_..." http://localhost:8784/api/v1/sources
```

### MCP Server Authentication

For stdio transport (default), authentication uses environment variables:

```bash
# Claude Desktop configuration with authentication
export THINKT_MCP_TOKEN=$(thinkt serve token)
```

For HTTP transport:

```bash
# Using environment variable
export THINKT_MCP_TOKEN=$(thinkt serve token)
thinkt serve mcp --port 8786

# Using command-line flag
thinkt serve mcp --port 8786 --token thinkt_20260205_...
```

Clients must pass the token in the `Authorization` header:
```
Authorization: Bearer thinkt_20260205_...
```

## Machine Fingerprint

Use `thinkt serve fingerprint` to display a unique machine identifier. This fingerprint is derived from system identifiers (e.g., hardware UUID on macOS, `/etc/machine-id` on Linux) and can be used to correlate sessions across different AI coding assistant sources on the same machine.

```bash
# Display fingerprint
thinkt serve fingerprint

# JSON output
thinkt serve fingerprint --json
```

The fingerprint is normalized to a consistent UUID format across all platforms.

## Lite Webapp Features

The lightweight webapp (`thinkt serve lite`) provides:

- **Internationalization**: English, Spanish, and Chinese (auto-detected)
- **Connection Status**: Real-time indicator showing server connectivity
- **Source Visibility**: Toggle eye icons to show/hide projects by source
- **Open-In Buttons**: Quick buttons to open projects in VS Code, Cursor, etc.
- **Language Selector**: Switch between EN/ES/中文 in the top-right corner

## MCP Integration

Use `thinkt` as an MCP server for AI assistants like Claude Desktop:

```json
{
  "mcpServers": {
    "thinkt": {
      "command": "thinkt",
      "args": ["serve", "mcp"],
      "env": {
        "THINKT_MCP_TOKEN": "your-secure-token-here"
      }
    }
  }
}
```

Generate a secure token with:
```bash
thinkt serve token
```

Available MCP tools:
- `list_sources` - List available session sources
- `list_projects` - List projects from all sources
- `list_sessions` - List sessions for a project
- `get_session_metadata` - Get session metadata
- `get_session_entries` - Get session content with pagination

See [Authentication](#authentication) for more details on securing the MCP server.

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `THINKT_CLAUDE_HOME` | Claude Code data directory | `~/.claude` |
| `THINKT_KIMI_HOME` | Kimi Code data directory | `~/.kimi` |
| `THINKT_GEMINI_HOME` | Gemini CLI data directory | `~/.gemini` |
| `THINKT_COPILOT_HOME` | Copilot data directory | `~/.copilot` |
| `THINKT_API_TOKEN` | Bearer token for API server authentication | (none) |
| `THINKT_MCP_TOKEN` | Bearer token for MCP server authentication | (none) |
| `THINKT_PROFILE` | Write CPU profiling to this file path | (disabled) |

## Related Projects

- [Thinking Tracer](https://github.com/Brain-STM-org/thinking-tracer) - visualization tool for exploring LLM conversation traces

## License

Created with :heart: and :fire: by the team at [Neomantra](https://www.neomantra.net) and [BrainSTM](https://brain-stm.org).

Released under the MIT License - see [LICENSE.txt](./LICENSE.txt)
