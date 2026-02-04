---
title: "thinkt"
type: docs
---

# thinkt

Tools for AI assistant session exploration and extraction.

**thinkt** provides tools for exploring and extracting data from AI coding assistant sessions.

## Supported Sources

We have a common `thinkt` interface to enable uniform access to various `Sources`.  We maintain a library of implementations and currently support:
  - `claude` [**Claude Code**](https://claude.com/product/claude-code) from Anthropic
  - `kimi` [**Kimi Code**](https://www.kimi.com/code) from Moonshot
  - `gemini` [**Gemini CLI**](https://geminicli.com) from Google
  - `copilot` [**Copilot CLI**](https://github.com/features/copilot/cli) from GitHub

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
- [**REST API**](/rest-api) - Programmatic access with OpenAPI/Swagger documentation
- [**Lite Server**](/serve-lite) - Lightweight web interface for quick exploration
- [**MCP Server**](/mcp-server) - Expose session data via Model Context Protocol for AI assistants
- [**Docker**](/docker) - Sandboxed, read-only access via container

## Installation

```bash
# Using Go
go install github.com/wethinkt/thinking-tracer-tools/cmd/thinkt@latest

# Using Docker (sandboxed, read-only access)
docker pull ghcr.io/wethinkt/thinkt:latest
```

See the [Docker Guide](/docker) for sandboxed usage with volume mounts.

## Next Steps

- Read the [CLI Guide](/cli) for common workflows and examples
- Run with [Docker](/docker) for sandboxed, read-only access
- Explore the [REST API](/rest-api) with interactive Swagger documentation
- Configure the [MCP Server](/mcp-server) for AI assistant integration
- Browse the [Command Reference](/command/) for detailed usage of all commands
- See the [LLM Guide](/for-llms) for AI assistant integration tips
- View the project on [GitHub](https://github.com/wethinkt/thinking-tracer-tools)
