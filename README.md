# thinking-tracer-tools

[![CI](https://github.com/Brain-STM-org/thinking-tracer-tools/actions/workflows/ci.yml/badge.svg)](https://github.com/Brain-STM-org/thinking-tracer-tools/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/Brain-STM-org/thinking-tracer-tools.svg)](https://pkg.go.dev/github.com/Brain-STM-org/thinking-tracer-tools)

Companion tools for [thinking-tracer](https://github.com/Brain-STM-org/thinking-tracer), providing utilities for exploring and extracting data from AI coding assistant sessions.

## Overview

`thinkt` is a CLI tool for exploring conversation traces from AI coding assistants. It supports multiple sources including Claude Code and Kimi Code.

### Features

- **Interactive TUI**: Three-column terminal interface for browsing projects, sessions, and conversation content
- **Multi-Source Support**: Works with Claude Code (`~/.claude`) and Kimi Code (`~/.kimi`)
- **Full-Text Search**: DuckDB-powered search across all sessions
- **Analytics**: Token usage, tool frequency, word analysis, activity timelines
- **Prompt Extraction**: Generate timestamped logs of user prompts in markdown, JSON, or plain text
- **MCP Server**: Model Context Protocol integration for use with AI assistants
- **REST API**: HTTP server for programmatic access
- **Themes**: Customizable color themes with interactive theme builder

## Installation

### Homebrew

```bash
brew install brain-stm-org/tap/thinkt
```

### Go

```bash
go install github.com/Brain-STM-org/thinking-tracer-tools/cmd/thinkt@latest
```

### From Source

```bash
git clone https://github.com/Brain-STM-org/thinking-tracer-tools.git
cd thinking-tracer-tools
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
```

## Commands

| Command | Description |
|---------|-------------|
| `thinkt` | Launch interactive TUI (default) |
| `thinkt tui` | Launch interactive TUI |
| `thinkt sources list` | List available sources (kimi, claude) |
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
| `thinkt serve` | Start HTTP server |
| `thinkt serve mcp` | Start MCP server |
| `thinkt theme` | Display current theme |
| `thinkt theme builder` | Interactive theme editor |

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

## Related Projects

- [thinking-tracer](https://github.com/Brain-STM-org/thinking-tracer) - 3D visualization tool for exploring LLM conversation traces

## License

MIT License - see [LICENSE.txt](./LICENSE.txt)
