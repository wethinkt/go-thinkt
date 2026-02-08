---
title: "REST API"
weight: 15
---

# REST API

thinkt provides a REST API for programmatic access to AI coding session data. The API is available through the HTTP server and includes interactive Swagger documentation.

## Quick Start

```bash
# Start the full server (port 8784)
thinkt serve

# Start the lightweight debug server (port 8785)
thinkt serve lite

# Custom port
thinkt serve -p 8080
```

Once running, access the API at `http://localhost:8784/api/v1/` (or your configured port).

## Server Modes

### Full Server (`thinkt serve`)

The main HTTP server with REST API and full web interface:

```bash
thinkt serve                    # Default port 8784
thinkt serve -p 8080            # Custom port
thinkt serve --no-open          # Don't auto-open browser
thinkt serve --quiet            # Suppress request logging
thinkt serve --http-log access.log  # Log requests to file
```

**Features:**
- Full REST API
- Full web interface ([thinkt-web](https://github.com/wethinkt/thinkt-web)) for visual trace exploration
- SPA routing — all non-API paths serve the webapp
- Auto-opens browser on startup

### Lite Server (`thinkt serve lite`)

A lightweight server for debugging and development:

```bash
thinkt serve lite               # Default port 8785
thinkt serve lite -p 8080       # Custom port
thinkt serve lite --no-open     # Don't auto-open browser
```

**Features:**
- REST API access
- Lightweight debug interface ([thinkt-web-lite](https://github.com/wethinkt/thinkt-web-lite)) showing:
  - Available sources and status
  - Project list with session counts
  - Quick links to API endpoints
  - Theme preview

**Reference:** [thinkt serve lite](/command/thinkt_serve_lite)

---

## OpenAPI Specification

The API is documented using OpenAPI (Swagger) 2.0. Access the interactive documentation at:

```
http://localhost:8784/swagger/
```

Download the specification:
- **JSON:** `http://localhost:8784/swagger/doc.json`
- **YAML:** Available in the source at `internal/server/docs/swagger.yaml`

---

## API Endpoints

**Base URL:** `/api/v1`

### Sources

#### List Sources

List available trace sources and their status.

```
GET /api/v1/sources
```

**Response:**
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
    },
    {
      "name": "gemini",
      "available": false,
      "base_path": ""
    },
    {
      "name": "copilot",
      "available": false,
      "base_path": ""
    }
  ]
}
```

**Example:**
```bash
curl http://localhost:8784/api/v1/sources
```

---

### Projects

#### List Projects

List all projects, optionally filtered by source.

```
GET /api/v1/projects
GET /api/v1/projects?source=claude
```

**Query Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `source` | string | Filter by source (`claude`, `kimi`, `gemini`, `copilot`) |

**Response:**
```json
{
  "projects": [
    {
      "id": "abc123",
      "name": "my-project",
      "path": "/Users/you/code/my-project",
      "display_path": "~/code/my-project",
      "session_count": 15,
      "source": "claude",
      "last_modified": "2024-01-15T10:30:00Z",
      "workspace_id": "my-machine"
    }
  ]
}
```

**Examples:**
```bash
# All projects
curl http://localhost:8784/api/v1/projects

# Only Claude projects
curl "http://localhost:8784/api/v1/projects?source=claude"

# Only Kimi projects
curl "http://localhost:8784/api/v1/projects?source=kimi"
```

---

### Sessions

#### List Sessions for Project

List all sessions belonging to a project.

```
GET /api/v1/projects/{projectID}/sessions
```

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `projectID` | string | URL-encoded project path |

**Response:**
```json
{
  "sessions": [
    {
      "id": "session-uuid",
      "full_path": "/Users/you/.claude/projects/abc123/session.jsonl",
      "project_path": "/Users/you/code/my-project",
      "created_at": "2024-01-15T10:30:00Z",
      "modified_at": "2024-01-15T11:45:00Z",
      "entry_count": 42,
      "file_size": 125000,
      "source": "claude",
      "model": "claude-3-opus",
      "first_prompt": "Help me refactor the auth module...",
      "git_branch": "main"
    }
  ]
}
```

**Example:**
```bash
# URL-encode the project path
curl "http://localhost:8784/api/v1/projects/%2FUsers%2Fyou%2Fcode%2Fmy-project/sessions"
```

#### Get Session Content

Get session metadata and conversation entries with optional pagination.

```
GET /api/v1/sessions/{path}
GET /api/v1/sessions/{path}?limit=10&offset=0
```

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `path` | string | URL-encoded session file path |

**Query Parameters:**
| Name | Type | Default | Description |
|------|------|---------|-------------|
| `limit` | int | 0 (all) | Maximum entries to return |
| `offset` | int | 0 | Number of entries to skip |

**Response:**
```json
{
  "meta": {
    "id": "session-uuid",
    "full_path": "/path/to/session.jsonl",
    "created_at": "2024-01-15T10:30:00Z",
    "modified_at": "2024-01-15T11:45:00Z",
    "entry_count": 42,
    "model": "claude-3-opus",
    "source": "claude"
  },
  "entries": [
    {
      "uuid": "entry-uuid",
      "role": "user",
      "timestamp": "2024-01-15T10:30:00Z",
      "text": "Help me refactor the authentication module",
      "content_blocks": [],
      "model": "",
      "source": "claude"
    },
    {
      "uuid": "entry-uuid-2",
      "role": "assistant",
      "timestamp": "2024-01-15T10:30:15Z",
      "text": "I'll help you refactor the authentication module...",
      "content_blocks": [
        {
          "type": "thinking",
          "thinking": "Let me analyze the current implementation..."
        },
        {
          "type": "tool_use",
          "tool_use_id": "tool-123",
          "tool_name": "Read",
          "tool_input": {"file_path": "/src/auth.ts"}
        }
      ],
      "model": "claude-3-opus",
      "usage": {
        "input_tokens": 1500,
        "output_tokens": 800
      }
    }
  ],
  "has_more": true,
  "total": 42
}
```

**Examples:**
```bash
# Get all entries
curl "http://localhost:8784/api/v1/sessions/%2Fpath%2Fto%2Fsession.jsonl"

# Paginate: first 10 entries
curl "http://localhost:8784/api/v1/sessions/%2Fpath%2Fto%2Fsession.jsonl?limit=10"

# Paginate: next 10 entries
curl "http://localhost:8784/api/v1/sessions/%2Fpath%2Fto%2Fsession.jsonl?limit=10&offset=10"
```

---

### Teams

#### List Teams

List all discovered agent teams (Claude Code).

```
GET /api/v1/teams
```

**Response:**
```json
{
  "teams": [
    {
      "name": "my-project",
      "members": [...],
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

**Example:**
```bash
curl http://localhost:8784/api/v1/teams
```

#### Get Team Details

Get a specific team with resolved member-to-session mappings.

```
GET /api/v1/teams/{teamName}
```

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `teamName` | string | Team name |

**Example:**
```bash
curl http://localhost:8784/api/v1/teams/my-project
```

#### List Team Tasks

Get the shared task board for a team.

```
GET /api/v1/teams/{teamName}/tasks
```

**Response:**
```json
{
  "tasks": [
    {
      "id": "1",
      "subject": "Implement feature X",
      "status": "completed",
      "owner": "researcher"
    }
  ]
}
```

**Example:**
```bash
curl http://localhost:8784/api/v1/teams/my-project/tasks
```

#### List Team Member Messages

Get inbox messages for a specific team member.

```
GET /api/v1/teams/{teamName}/members/{memberName}/messages
```

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `teamName` | string | Team name |
| `memberName` | string | Member name |

**Example:**
```bash
curl http://localhost:8784/api/v1/teams/my-project/members/researcher/messages
```

---

### Themes

#### List Themes

Get all available themes with their color definitions.

```
GET /api/v1/themes
```

**Response:**
```json
{
  "themes": [
    {
      "name": "dark",
      "description": "Default dark theme",
      "embedded": true,
      "active": true,
      "colors": {
        "accent": "#7c3aed",
        "border_active": "#7c3aed",
        "border_inactive": "#404040",
        "user_block": {"fg": "#ffffff", "bg": "#1e1e1e"},
        "user_label": {"fg": "#60a5fa", "bold": true},
        "assistant_block": {"fg": "#ffffff", "bg": "#1e1e1e"},
        "assistant_label": {"fg": "#34d399", "bold": true},
        "thinking_block": {"fg": "#9ca3af", "bg": "#1e1e1e", "italic": true},
        "thinking_label": {"fg": "#f472b6", "bold": true},
        "tool_call_block": {"fg": "#fbbf24", "bg": "#1e1e1e"},
        "tool_label": {"fg": "#fbbf24", "bold": true},
        "tool_result_block": {"fg": "#9ca3af", "bg": "#1e1e1e"},
        "text_primary": {"fg": "#ffffff"},
        "text_secondary": {"fg": "#9ca3af"},
        "text_muted": {"fg": "#6b7280"}
      }
    }
  ],
  "active": "dark"
}
```

---

### Open-In

Open paths in external applications (Finder, VS Code, etc.).

#### List Allowed Apps

Get the list of enabled applications for the open-in feature.

```
GET /api/v1/open-in/apps
```

**Response:**
```json
{
  "apps": [
    {"id": "finder", "name": "Finder", "enabled": true},
    {"id": "terminal", "name": "Terminal", "enabled": true},
    {"id": "vscode", "name": "VS Code", "enabled": true},
    {"id": "cursor", "name": "Cursor", "enabled": false},
    {"id": "zed", "name": "Zed", "enabled": false}
  ]
}
```

#### Open Path

Open a path in a specified application.

```
POST /api/v1/open-in
```

**Request Body:**
```json
{
  "app": "vscode",
  "path": "/Users/you/code/my-project"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Opened in VS Code"
}
```

**Example:**
```bash
curl -X POST http://localhost:8784/api/v1/open-in \
  -H "Content-Type: application/json" \
  -d '{"app": "vscode", "path": "/Users/you/code/my-project"}'
```

{{< hint warning >}}
**Security:** Only applications explicitly enabled in the configuration can be used. Requests for disabled apps return `403 Forbidden`.
{{< /hint >}}

---

## Data Types

### Entry Roles

| Role | Description |
|------|-------------|
| `user` | User messages |
| `assistant` | AI assistant responses |
| `tool` | Tool execution messages |
| `system` | System messages |
| `summary` | Conversation summaries |
| `progress` | Progress indicators |
| `checkpoint` | State recovery markers |

### Content Block Types

| Type | Description |
|------|-------------|
| `text` | Plain text content |
| `thinking` | AI thinking/reasoning blocks |
| `tool_use` | Tool invocation with name and input |
| `tool_result` | Tool execution result |

### Sources

| Source | Description |
|--------|-------------|
| `claude` | Claude Code |
| `kimi` | Kimi Code |
| `gemini` | Gemini CLI |
| `copilot` | GitHub Copilot CLI |

---

## Authentication

Both the REST API server and MCP server support Bearer token authentication.

### Generate a Token

```bash
thinkt serve token
# Output: thinkt_20260205_cd3bf36d6e1fc71e9bf033a7131f77cb
```

### Using Authentication

```bash
# Environment variable
export THINKT_API_TOKEN=$(thinkt serve token)
thinkt serve

# Command-line flag
thinkt serve --token thinkt_20260205_...

# Client request
curl -H "Authorization: Bearer thinkt_20260205_..." http://localhost:8784/api/v1/sources
```

When authentication is enabled, all API endpoints require a valid Bearer token. Unauthenticated requests receive a `401 Unauthorized` response with a `WWW-Authenticate` header.

---

## Error Handling

All endpoints return errors in a consistent format:

```json
{
  "error": "error_code",
  "message": "Human-readable error description"
}
```

**HTTP Status Codes:**
| Code | Description |
|------|-------------|
| `200` | Success |
| `400` | Bad Request - Invalid parameters |
| `401` | Unauthorized - Invalid or missing token |
| `403` | Forbidden - Action not allowed |
| `404` | Not Found - Resource doesn't exist |
| `500` | Internal Server Error |

---

## CORS

The API enables CORS for local development, allowing browser-based applications to access the API from different origins.

---

## Examples

### List All Sessions Across Projects

```bash
# Get all projects
projects=$(curl -s http://localhost:8784/api/v1/projects | jq -r '.projects[].id')

# For each project, list sessions
for proj in $projects; do
  encoded=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$proj', safe=''))")
  curl -s "http://localhost:8784/api/v1/projects/$encoded/sessions"
done
```

### Export Session to JSON

```bash
session_path="/Users/you/.claude/projects/abc123/session.jsonl"
encoded=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$session_path', safe=''))")
curl -s "http://localhost:8784/api/v1/sessions/$encoded" | jq . > session_export.json
```

### Filter Sessions by Model

```bash
curl -s "http://localhost:8784/api/v1/sessions/$encoded" | \
  jq '.entries[] | select(.model == "claude-3-opus")'
```

---

## See Also

- [thinkt serve](/command/thinkt_serve) - Full server command reference
- [thinkt serve lite](/command/thinkt_serve_lite) - Lite server command reference
- [MCP Server](/mcp-server) - Model Context Protocol integration
- [CLI Guide](/cli) - Command line interface guide
