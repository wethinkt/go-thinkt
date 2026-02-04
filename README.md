# go-thinkt

[![CI](https://github.com/wethinkt/go-thinkt/actions/workflows/ci.yml/badge.svg)](https://github.com/wethinkt/go-thinkt/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/wethinkt/go-thinkt.svg)](https://pkg.go.dev/github.com/wethinkt/go-thinkt)
[![User Guide](https://img.shields.io/badge/User%20Guide-6B2DAD)](https://wethinkt.github.io/go-thinkt/)


**This tool is still in alpha stages and is for educational purposes only in its current state.**

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

- **Interactive TUI**: Three-column terminal interface for browsing projects, sessions, and conversation content
- **Multi-Source Support**: Works with Claude Code (`~/.claude`), Kimi Code (`~/.kimi`), Gemini CLI, and Copilot
- **Full-Text Search**: DuckDB-powered search across all sessions
- **Analytics**: Token usage, tool frequency, word analysis, activity timelines
- **Prompt Extraction**: Generate timestamped logs of user prompts in markdown, JSON, or plain text
- **MCP Server**: Model Context Protocol integration for use with AI assistants
- **REST API**: HTTP server for programmatic access
- **Lite Webapp**: Lightweight debug interface with i18n (EN/ES/中文), connection status, and "open-in" buttons
- **Themes**: Customizable color themes with interactive theme builder

## Installation

### Homebrew

*Homewbrew is NOT AVAILABLE YET*
```bash
brew install wethinkt/tap/thinkt
```

### Go

```bash
go install github.com/wethinkt/go-thinkt/cmd/thinkt@latest
```

### From Source

```bash
git clone https://github.com/wethinkt/go-thinkt.git
cd go-thinkt
task build
```

## Quick Start

```bash
# Launch interactive TUI (default)
thinkt

# List available sources
thinkt sources list

# Browse projects
thinkt projects
thinkt projects --long
thinkt projects --tree

# View sessions
thinkt sessions list
thinkt sessions view

# Search across all sessions
thinkt search "authentication"

# Analytics
thinkt stats tokens
thinkt stats tools
thinkt stats activity --days 7

# Start the lite webapp
thinkt serve lite

# Start HTTP server without opening browser
thinkt serve --no-open
```

## Commands

| Command | Description |
|---------|-------------|
| `thinkt` | Launch interactive TUI (default) |
| `thinkt tui` | Launch interactive TUI |
| `thinkt sources list` | List available sources (kimi, claude, gemini, copilot) |
| `thinkt sources status` | Show detailed source status |
| `thinkt projects` | List all projects |
| `thinkt projects summary` | Detailed project info |
| `thinkt sessions list` | List sessions in a project |
| `thinkt sessions view` | View session in terminal |
| `thinkt search <query>` | Full-text search with DuckDB |
| `thinkt stats tokens` | Token usage by session |
| `thinkt stats tools` | Tool usage frequency |
| `thinkt stats words` | Word frequency analysis |
| `thinkt stats activity` | Daily activity timeline |
| `thinkt stats models` | Model usage statistics |
| `thinkt stats errors` | Tool errors and failures |
| `thinkt query <sql>` | Run raw SQL with DuckDB |
| `thinkt prompts extract` | Extract prompts to markdown/JSON |
| `thinkt serve` | Start HTTP server (port 7433) |
| `thinkt serve lite` | Start lightweight webapp (port 7434) |
| `thinkt serve mcp` | Start MCP server |
| `thinkt theme` | Display current theme |
| `thinkt theme builder` | Interactive theme editor |

## Serve Options

```bash
# Control HTTP logging
thinkt serve --quiet              # Suppress HTTP logs
thinkt serve --http-log file.log  # Log to file
thinkt serve --no-open            # Don't auto-open browser

# These also work with serve lite
thinkt serve lite --quiet --no-open
```

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
      "args": ["serve", "mcp"]
    }
  }
}
```

Available MCP tools:
- `list_sources` - List available session sources
- `list_projects` - List projects from all sources
- `list_sessions` - List sessions for a project
- `get_session_metadata` - Get session metadata
- `get_session_entries` - Get session content with pagination

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `THINKT_CLAUDE_HOME` | Claude Code data directory | `~/.claude` |
| `THINKT_KIMI_HOME` | Kimi Code data directory | `~/.kimi` |
| `THINKT_GEMINI_HOME` | Gemini CLI data directory | `~/.gemini` |
| `THINKT_COPILOT_HOME` | Copilot data directory | `~/.copilot` |

## Related Projects

- [Thinking Tracer](https://github.com/Brain-STM-org/thinking-tracer) - visualization tool for exploring LLM conversation traces

## License

Created with :heart: and :fire: by the team at [Neomantra](https://www.neomantra.net) and [BrainSTM](https://brain-stm.org).

Released under the MIT License - see [LICENSE.txt](./LICENSE.txt)
