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
  - `codex` [**Codex CLI**](https://github.com/openai/codex) from OpenAI

## Quick Start

```bash
# Launch the interactive TUI
thinkt

# List all projects
thinkt projects

# View sessions for a project
thinkt sessions list <project-id>

# View a session
thinkt sessions view

# Search across sessions
thinkt search "authentication"
```

## Features

- **Interactive TUI** - Navigate projects, sessions, and conversations with a keyboard-driven terminal UI
- **Tree View** - Browse projects in a collapsible tree grouped by directory
- **Multi-Source Support** - Auto-discovers Claude Code, Kimi Code, Gemini CLI, Copilot CLI, and Codex CLI sessions
- **Agent Teams** - Inspect multi-agent teams (Claude Code), including members, tasks, and messages
- **Prompt Extraction** - Extract user prompts in markdown, JSON, or plain text
- [**Themes**](/themes) - 14 built-in color schemes (Dracula, Nord, Catppuccin, etc.) with interactive browser and iTerm2 import
- **App Management** - Configure open-in apps and default terminal via `thinkt apps`
- **Full-Text Search** - Search across all indexed sessions via `thinkt search`
- **Semantic Search** - Find sessions by meaning with on-device embeddings via `thinkt semantic search`
- **Embedding Management** - Configure and manage embedding models via `thinkt embeddings`

- [**REST API**](/rest-api) - Programmatic access with OpenAPI/Swagger documentation
- [**Web Interface**](/rest-api#server-modes) - Full webapp for visual trace exploration via `thinkt web`
- [**Lite Server**](/serve-lite) - Lightweight debug interface for quick inspection
- [**MCP Server**](/mcp-server) - Expose session data via Model Context Protocol for AI assistants
- [**Docker**](/docker) - Sandboxed, read-only access via container

## Installation

```bash
# Using Go
go install github.com/wethinkt/go-thinkt/cmd/thinkt@latest

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
- View the project on [GitHub](https://github.com/wethinkt/go-thinkt)
