---
title: "thinkt"
type: docs
---

# thinkt

Tools for AI assistant session exploration and extraction.

**thinkt** provides tools for exploring and extracting data from AI coding assistant sessions.

## Supported Sources

We created a common `thinkt` interface to enable uniform access to various `Sources`.  We maintain a library of implementations and currently support:

  - `claude` [**Claude Code**](https://claude.com/product/claude-code) from Anthropic
  - `kimi` [**Kimi Code**](https://www.kimi.com/code) from Moonshot
  - `gemini` [**Gemini CLI**](https://geminicli.com) from Google
  - `copilot` [**Copilot CLI**](https://github.com/features/copilot/cli) from GitHub
  - `codex` [**Codex CLI**](https://github.com/openai/codex) from OpenAI
  - `qwen` [**Qwen Code**](https://www.qwen.ai) from Alibaba

## Quick Start

```bash
# Launch the interactive TUI, depending on context
# If in a project folder, it will open a session view
thinkt

# List all projects
thinkt projects

# View sessions for a project
thinkt sessions 

# Open the Web interface 
thinkt web

# Search across sessions
thinkt search "authentication"
```

## Features

### We have several ways to explore your conversations:

- **Interactive TUI** - Navigate projects, sessions, and conversations with a keyboard-driven terminal UI
- **Web Interface** - Full webapp for visual trace exploration via `thinkt web`
- [**MCP Server**](/mcp-server) - Expose session data via Model Context Protocol for AI assistants
- [**REST API**](/rest-api) - Programmatic access with OpenAPI/Swagger documentation


### All with the same shared features:
- **Multi-Source Support** - Auto-discovers Claude Code, Kimi Code, Gemini CLI, Copilot CLI, Codex CLI, and Qwen Code sessions
- **Full-Text Search** - Search across all indexed sessions via `thinkt search`
- **Semantic Search** - Find sessions by meaning with on-device embeddings via `thinkt semantic search`
- **Prompt Extraction** - Extract user prompts in markdown, JSON, or plain text

### The `thinkt` platform is meant to make your work easier:

- [**Themes**](/themes) - 14 built-in color schemes (Dracula, Nord, Catppuccin, etc.) with interactive browser and iTerm2 import
- [**Languages**](/languages) - Support for English, Chinese, and Spanish with easy switching
- **App Management** - Configure open-in apps and default terminal via `thinkt apps`
- **Embedding Management** - Configure and manage embedding models via `thinkt embeddings`

### With operational tools:
- [**Docker**](/docker) - Container images for sandboxed deployment
- **Agent Teams** - Inspect multi-agent teams (Claude Code), including members, tasks, and messages
- **Prometheus Metrics** - For server, indexer, and collect monitoring
- [**Lite Server**](/serve-lite) - Lightweight debug interface for quick inspection
- **Agent Teams** - Inspect multi-agent teams (Claude Code), including members, tasks, and messages

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
- Configure the [MCP Server](/mcp-server) for AI assistant integration
- Browse the [Command Reference](/command/) for detailed usage of all commands
- Run with [Docker](/docker) for sandboxed, read-only access
- Set up [Trace Collection](/collector) for multi-machine trace aggregation
- Explore the [REST API](/rest-api) with interactive Swagger documentation
- See the [LLM Guide](/for-llms) for AI assistant integration tips
- View the project on [GitHub](https://github.com/wethinkt/go-thinkt)
