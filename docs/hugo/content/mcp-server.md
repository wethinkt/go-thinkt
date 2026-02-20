---
title: "MCP Server"
weight: 10
---

# MCP Server

The `thinkt serve mcp` command starts a [Model Context Protocol](https://modelcontextprotocol.io) server that exposes your AI coding session data to MCP-compatible clients. This allows AI assistants to query your session history, browse projects, and analyze past conversations.

## Overview

```bash
thinkt serve mcp                # MCP server on stdio (default)
thinkt serve mcp --stdio        # Explicitly use stdio transport
thinkt serve mcp --port 8786    # MCP server over HTTP (SSE)
```

The MCP server supports two transport modes:
- **stdio** (default): For direct integration with Claude Desktop and other stdio-based clients
- **HTTP/SSE**: For web-based or networked clients using Server-Sent Events

## Available Tools

The MCP server exposes seven tools for exploring session data:

### list_sources

List available trace sources and their availability status.

**Parameters:** None

**Returns:**
```json
{
  "sources": [
    {
      "name": "claude",
      "available": true,
      "base_path": "/Users/you/.claude/projects"
    },
    {
      "name": "kimi",
      "available": true,
      "base_path": "/Users/you/.kimi/workspace"
    }
  ]
}
```

---

### list_projects

List all projects across all sources, optionally filtered by source.
By default, projects with `path_exists: false` are hidden unless `include_deleted` is set.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `source` | string | No | Filter by source (e.g., `claude`, `kimi`, `gemini`, `copilot`, `codex`) |
| `include_deleted` | bool | No | Include projects where `path_exists` is false (default: false) |

**Returns:**
```json
{
  "projects": [
    {
      "id": "abc123",
      "name": "my-project",
      "path": "/Users/you/code/my-project",
      "session_count": 15,
      "source": "claude"
    }
  ]
}
```

---

### list_sessions

List sessions for a specific project and source.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `project_id` | string | Yes | The project ID to list sessions for |
| `source` | string | Yes | Source name for project lookup (e.g., `claude`, `kimi`, `codex`, `qwen`) |

**Returns:**
```json
{
  "sessions": [
    {
      "id": "session-uuid",
      "path": "/full/path/to/session.jsonl",
      "created_at": "2024-01-15T10:30:00Z",
      "modified_at": "2024-01-15T11:45:00Z",
      "entry_count": 42,
      "file_size": 125000,
      "source": "claude"
    }
  ]
}
```

---

### get_session_metadata

Get session metadata and entry summaries without loading full content. Use this first to understand a session before fetching specific entries.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | Yes | Full path to the session file |
| `summary_only` | bool | No | Return lightweight user-message previews in `entry_summary` (default preview limit: 5) |
| `limit` | int | No | Max returned summaries/previews (default: 50, or 5 when `summary_only=true`) |
| `offset` | int | No | Number of summaries/previews to skip |

**Returns:**
```json
{
  "meta": {
    "id": "session-uuid",
    "path": "/full/path/to/session.jsonl",
    "created_at": "2024-01-15T10:30:00Z",
    "model": "claude-3-opus",
    "git_branch": "main",
    "source": "claude"
  },
  "description": "Help me refactor the authentication module...",
  "role_counts": {
    "user": 5,
    "assistant": 5
  },
  "entry_summary": [
    {
      "index": 0,
      "role": "user",
      "timestamp": "2024-01-15T10:30:00Z",
      "content_length": 250,
      "has_thinking": false,
      "has_tool_use": false,
      "has_tool_result": false,
      "preview": "Help me refactor the authentication module to use JWT tokens instead of..."
    }
  ],
  "total_entries": 10,
  "total_content_bytes": 45000
}
```

---

### get_session_entries

Get session entry content with pagination and filtering.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `path` | string | Yes | - | Full path to the session file |
| `limit` | int | No | 5 | Maximum entries to return |
| `offset` | int | No | 0 | Number of entries to skip |
| `entry_indices` | int[] | No | - | Specific entry indices to fetch (overrides limit/offset) |
| `roles` | string[] | No | - | Filter by role (e.g., `["user", "assistant"]`) |
| `max_content_length` | int | No | 500 | Truncate text content to this length (0 = no limit) |
| `include_thinking` | bool | No | false | Include thinking blocks in response |

**Returns:**
```json
{
  "entries": [
    {
      "index": 0,
      "uuid": "entry-uuid",
      "role": "assistant",
      "timestamp": "2024-01-15T10:31:00Z",
      "text": "I'll help you refactor the authentication...",
      "text_truncated": true,
      "thinking": "Let me analyze the current auth implementation...",
      "tool_uses": [
        {
          "id": "tool-use-id",
          "name": "Read",
          "input": "{\"file_path\": \"/src/auth.ts\"}"
        }
      ],
      "tool_results": [
        {
          "tool_use_id": "tool-use-id",
          "result": "export function authenticate...",
          "is_error": false
        }
      ],
      "model": "claude-3-opus"
    }
  ],
  "has_more": true,
  "total": 10,
  "returned": 5
}
```

---

### search_sessions

Search for text across all indexed sessions. Requires `thinkt-indexer` to be installed and sessions to be indexed with `thinkt-indexer sync`.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `query` | string | Yes | - | Search query text |
| `project` | string | No | - | Filter by project name |
| `source` | string | No | - | Filter by source (`claude`, `kimi`, `gemini`, `copilot`, `codex`) |
| `limit` | int | No | 50 | Maximum total matches |
| `limit_per_session` | int | No | 2 | Maximum matches per session (0 for no limit) |
| `case_sensitive` | bool | No | false | Enable case-sensitive matching |
| `regex` | bool | No | false | Treat query as a regular expression (Go RE2 syntax) |

**Returns:**
```json
{
  "sessions": [
    {
      "session_id": "abc-123",
      "project_name": "my-project",
      "source": "claude",
      "path": "/path/to/session.jsonl",
      "matches": [
        {
          "line_num": 42,
          "preview": "...matching text in context...",
          "role": "user"
        }
      ]
    }
  ],
  "total_matches": 5
}
```

---

### get_usage_stats

Get aggregate usage statistics including total tokens and most used tools.

**Parameters:** None

**Returns:**
```json
{
  "total_projects": 12,
  "total_sessions": 156,
  "total_entries": 4200,
  "total_tokens": 1250000,
  "tool_usage": {
    "Read": 450,
    "Edit": 280,
    "Bash": 190
  }
}
```

---

## Installation

### Claude Desktop

Add `thinkt` to your Claude Desktop configuration file:

{{< tabs "claude-desktop" >}}
{{< tab "macOS" >}}
Edit `~/Library/Application Support/Claude/claude_desktop_config.json`:
{{< /tab >}}
{{< tab "Windows" >}}
Edit `%APPDATA%\Claude\claude_desktop_config.json`:
{{< /tab >}}
{{< /tabs >}}

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

Restart Claude Desktop to load the new MCP server.

---

### Claude Code

Add `thinkt` to your Claude Code MCP settings via CLI:

```bash
claude mcp add --transport stdio thinkt -- thinkt serve mcp
claude mcp list
```

---

### Kimi Code

Add `thinkt` to your Claude Code MCP settings via CLI:

```bash
kimi mcp add --transport stdio thinkt -- thinkt serve mcp
kimi mcp list
```

---

### Gemini CLI

Add `thinkt` to your Gemini MCP settings via CLI:

```bash
gemini mcp add --transport stdio thinkt -- thinkt serve mcp
gemini mcp list
```


Add `thinkt` to your Gemini CLI MCP settings:

{{< tabs "gemini-cli" >}}
{{< tab "macOS/Linux" >}}
Edit `~/.gemini/settings.json`:
{{< /tab >}}
{{< tab "Windows" >}}
Edit `%USERPROFILE%\.gemini\settings.json`:
{{< /tab >}}
{{< /tabs >}}

```json
{
  "mcpServers": {
    "thinkt": {
      "transport": "stdio",
      "command": "thinkt",
      "args": ["serve", "mcp"]
    }
  }
}
```

---

### GitHub Copilot CLI

Add `thinkt` to your GitHub Copilot CLI configuration:

{{< tabs "copilot-cli" >}}
{{< tab "macOS/Linux" >}}
Edit `~/.config/github-copilot/mcp.json`:
{{< /tab >}}
{{< tab "Windows" >}}
Edit `%USERPROFILE%\.config\github-copilot\mcp.json`:
{{< /tab >}}
{{< /tabs >}}

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

### Charm Crush

Add `thinkt` to your [Charm Crush](https://github.com/charmbracelet/crush) CLI configuration:

{{< tabs "crush-cli" >}}
{{< tab "macOS/Linux" >}}
Edit `~/.crush.json`:
{{< /tab >}}
{{< tab "Windows" >}}
Edit `%USERPROFILE%\.config\crush\crush.json`:
{{< /tab >}}
{{< /tabs >}}

```json
{
  "mcp": {
    "thinkt": {
      "type": "stdio",
      "disabled": false,
      "timeout": 120,
      "command": "thinkt",
      "args": ["serve", "mcp"]
    }
  }
}
```

---

## HTTP Mode

For networked deployments or web-based clients, run the MCP server over HTTP:

```bash
thinkt serve mcp --port 8786
thinkt serve mcp --port 8786 --host 0.0.0.0  # Listen on all interfaces
```

The HTTP mode uses Server-Sent Events (SSE) for the MCP transport. Connect your client to `http://localhost:8786` (or your configured host/port).

## Usage Examples

Once configured, you can ask your AI assistant questions like:

- "What projects do I have in thinkt?"
- "Show me my recent Claude sessions"
- "Find sessions where I worked on authentication"
- "Get the details of my last coding session"
- "What tools did the assistant use in session X?"

The AI assistant will use the thinkt MCP tools to query your session data and provide relevant information.

## See Also

- [thinkt serve mcp](/command/thinkt_serve_mcp) - Command reference
- [Model Context Protocol](https://modelcontextprotocol.io) - MCP specification
