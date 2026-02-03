---
title: "thinkt"
type: docs
---

# thinkt

Tools for AI assistant session exploration and extraction.

**thinkt** provides tools for exploring and extracting data from AI coding assistant sessions.

## Supported Sources

We have a common `thinkt` interface to enable uniform access to various `Sources`.  We maintain a library of implementations and currently support:
  - [**Claude Code**](https://claude.com/product/claude-code) from Anthropic
  - [**Kimi Code**](https://www.kimi.com/code) from Moonshot
  - [**Gemini CLI**](https://geminicli.com) from Google
  - [**Copilot CLI**](https://github.com/features/copilot/cli) from GitHub


## Quick Start

```bash
# Launch the interactive TUI
thinkt

# List all projects
thinkt projects

# View sessions for a project
thinkt sessions list <project-id>

# Search across all sessions
thinkt search "error handling"
```

## Features

- **Interactive TUI** - Browse and explore sessions with a terminal UI
- **Project Management** - Organize and manage projects across sources
- **Session Viewer** - View session entries with thinking blocks and tool usage
- **Search** - Full-text search across all sessions
- **Stats** - Token usage, model stats, tool usage analytics
- [**MCP Server**](/command/thinkt_serve_mcp) - Expose session data via Model Context Protocol

## Installation

```bash
go install github.com/wethinkt/thinking-tracer-tools/cmd/thinkt@latest
```

## Next Steps

- Browse the [Command Reference]({{< relref "/command" >}}) for detailed usage of all commands
- View the project on [GitHub](https://github.com/wethinkt/thinking-tracer-tools)
