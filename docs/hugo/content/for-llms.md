---
title: "For LLMs"
weight: 25
---

# Guide for AI Assistants

This page is designed for AI assistants and LLMs to understand how to use thinkt tools effectively. For the plain-text version optimized for LLM context windows, see [`/llms.txt`](https://github.com/wethinkt/go-thinkt/blob/main/llms.txt).

## What is thinkt?

thinkt provides unified access to AI coding assistant session data from:
- **Claude Code** (`~/.claude/`)
- **Kimi Code** (`~/.kimi/`)
- **Gemini CLI** (`~/.gemini/`)
- **Copilot CLI** (`~/.copilot/`)
- **Codex CLI** (`~/.codex/`)

Access methods: CLI, REST API, MCP, Go library.

---

## Quick Reference

### Repository Structure

| Path | Description |
|------|-------------|
| `/cmd/thinkt/` | CLI application |
| `/internal/thinkt/` | Core library |
| `/internal/server/` | HTTP and MCP servers (embeds web + web-lite submodules) |
| `/docs/hugo/content/` | Documentation |
| `/internal/server/docs/swagger.yaml` | OpenAPI spec |

### Key Documentation

| Page | Purpose |
|------|---------|
| [CLI Guide](/cli) | Command-line usage |
| [REST API](/rest-api) | HTTP endpoints |
| [MCP Server](/mcp-server) | MCP tools and setup |
| [Docker](/docker) | Sandboxed execution |

---

## Integration Methods

### 1. MCP (Recommended for AI Assistants)

MCP provides direct tool access for AI assistants.

**Tools:**

| Tool | Purpose | Key Input |
|------|---------|-----------|
| `list_sources` | Available sources | none |
| `list_projects` | All projects | `source?`, `include_deleted?` |
| `list_sessions` | Sessions for project | `project_id`, `source` |
| `get_session_metadata` | Session overview | `path`, `summary_only?` |
| `get_session_entries` | Session content | `path`, `limit`, `offset`, `roles`, `include_thinking` |

**Best Practices:**
1. Call `get_session_metadata` with `summary_only: true` first for quick user-intent previews
2. Use `roles: ["user"]` to get just prompts
3. Use `entry_indices` to fetch specific entries
4. Set `include_thinking: true` only when needed
5. Default `limit` is 5; paginate with `offset`

### 2. CLI

```bash
thinkt projects                     # List projects
thinkt sessions list -p <path>      # List sessions
thinkt sessions view                # View session content
thinkt web                          # Open web interface (auto-starts server)
thinkt web lite                     # Open lightweight debug interface
thinkt server                        # Start HTTP server (foreground)
thinkt server start                  # Start HTTP server (background)
thinkt server status                 # Check server status
thinkt server stop                   # Stop background server
thinkt apps                         # List open-in apps and terminal capability
thinkt apps set-terminal            # Set default terminal for resume
```

### 3. REST API

Base: `http://localhost:8784/api/v1`

```
GET /sources
GET /projects?include_deleted=false
GET /projects/{source}/{id}/sessions
GET /sessions/{path}?limit=10&offset=0
GET /sessions/{path}/metadata?summary_only=true&limit=5
```

### 4. Go Library

```go
import "github.com/wethinkt/go-thinkt/internal/thinkt"

discovery := thinkt.NewDiscovery(claude.Factory(), kimi.Factory(), gemini.Factory(), copilot.Factory(), codex.Factory())
registry, _ := discovery.Discover(ctx)
projects, _ := registry.ListAllProjects(ctx)
```

---

## Data Model

```
Source (claude|kimi|gemini|copilot|codex)
  ├── Project (directory path)
  │     └── Session (JSONL file)
  │           └── Entry (message)
  │                 └── ContentBlock (text|thinking|tool_use|tool_result)
  └── Team (multi-agent coordination, Claude Code only)
        ├── Member (agent name + session reference)
        ├── Task (shared task board)
        └── Message (inter-agent inbox)
```

### Entry Roles

| Role | Description |
|------|-------------|
| `user` | User messages |
| `assistant` | AI responses |
| `tool` | Tool execution |
| `system` | System messages |

### Content Block Types

| Type | Fields |
|------|--------|
| `text` | `text` |
| `thinking` | `thinking` |
| `tool_use` | `tool_use_id`, `tool_name`, `tool_input` |
| `tool_result` | `tool_use_id`, `tool_result`, `is_error` |

---

## Common Tasks

| Task | Method |
|------|--------|

| Get user prompts only | MCP: `get_session_entries` with `roles: ["user"]` |
| Export session | API: `GET /sessions/{path}` |

---

## Tips

1. **Metadata first** - Check session size before loading full content
2. **Paginate** - Sessions can have hundreds of entries
3. **Filter by role** - Often only user or assistant messages are needed
