---
title: "Trace Collector"
weight: 12
---

# Trace Collector & Exporter

thinkt includes a push-based trace collection system for aggregating AI coding assistant traces from multiple machines into a central store. This enables team-wide observability, cross-machine analytics, and centralized trace governance.

## Architecture

```
┌────────────────────┐     ┌────────────────────┐
│  Machine A         │     │  Machine B         │
│  ┌──────────────┐  │     │  ┌──────────────┐  │
│  │ thinkt       │  │     │  │ thinkt       │  │
│  │ export       │──┼─┐   │  │ export       │──┼─┐
│  │ --forward    │  │ │   │  │ --forward    │  │ │
│  └──────────────┘  │ │   │  └──────────────┘  │ │
│  watches ~/.claude │ │   │  watches ~/.kimi   │ │
└────────────────────┘ │   └────────────────────┘ │
                       │                          │
                       │   POST /v1/traces        │
                       ▼                          ▼
                 ┌──────────────────────────────────┐
                 │         thinkt collect            │
                 │                                   │
                 │  ┌─────────┐  ┌───────────────┐  │
                 │  │ Agent   │  │   DuckDB      │  │
                 │  │ Registry│  │ collector.duckdb│ │
                 │  └─────────┘  └───────────────┘  │
                 │                                   │
                 │  REST API on port 4318            │
                 └──────────────────────────────────┘
```

The system has two components:

- **Exporter** (`thinkt export`) — Watches local JSONL session files and ships trace entries to a collector via HTTP POST
- **Collector** (`thinkt collect`) — HTTP server that receives, normalizes, and stores traces in DuckDB

## Quick Start

### Start a Collector

```bash
# Start the collector on the default port (4318)
thinkt collect

# With authentication
thinkt collect --token mytoken

# Custom storage location
thinkt collect --storage /path/to/traces.duckdb
```

### Export Traces

```bash
# One-shot export of all local traces
thinkt export

# Continuous watch mode (ships traces as they are written)
thinkt export --forward

# Export to a specific collector
thinkt export --collector-url http://collect.example.com:4318/v1/traces

# Export only Claude Code traces
thinkt export --source claude
```

---

## Collector

The collector is an HTTP server that receives trace payloads from exporters and stores them in DuckDB.

### CLI Usage

```bash
thinkt collect [flags]
```

| Flag | Description | Default |
|------|-------------|---------|
| `--port, -p` | Server port | `4318` |
| `--host` | Server host | `localhost` |
| `--storage` | DuckDB file path | `~/.thinkt/collector.duckdb` |
| `--token` | Bearer token for authentication | (none) |
| `--quiet, -q` | Suppress non-error output | `false` |
| `--log` | Write debug log to file | (none) |

### Standalone Binary

For deployment without the full thinkt CLI:

```bash
thinkt-collector --port 4318 --token mytoken --storage ./traces.duckdb
```

### API Endpoints

All endpoints are under the `/v1` prefix. When `--token` is set, all endpoints except health require a `Authorization: Bearer <token>` header.

#### Ingest Traces

```
POST /v1/traces
```

Accepts a batch of trace entries from an exporter:

```json
{
  "instance_id": "abc-123",
  "source": "claude",
  "project_path": "/home/user/my-project",
  "session_id": "session-uuid",
  "entries": [
    {
      "uuid": "entry-uuid",
      "role": "assistant",
      "timestamp": "2026-02-10T10:00:00Z",
      "model": "claude-opus-4-6",
      "text": "I'll help you with that...",
      "input_tokens": 1500,
      "output_tokens": 200
    }
  ]
}
```

**Response:**

```json
{
  "accepted": 5
}
```

#### Search Traces

```
GET /v1/traces/search?q=authentication&limit=50
```

Searches across collected traces by text content, tool names, and project paths.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `q` | Search query (required) | — |
| `limit` | Maximum results | `50` |

**Response:** Array of `SessionSummary` objects.

#### Usage Statistics

```
GET /v1/traces/stats
```

Returns aggregate collector statistics:

```json
{
  "total_traces": 12500,
  "total_sessions": 84,
  "total_agents": 3,
  "active_agents": 2,
  "db_size_bytes": 52428800,
  "uptime_seconds": 3600.5,
  "started_at": "2026-02-10T08:00:00Z"
}
```

#### Register Agent

```
POST /v1/agents/register
```

Registers an exporter agent with the collector for tracking:

```json
{
  "instance_id": "abc-123",
  "platform": "darwin",
  "hostname": "dev-machine",
  "version": "0.6.0",
  "started_at": "2026-02-10T08:00:00Z"
}
```

#### List Agents

```
GET /v1/agents
```

Returns all registered agents with status and trace counts.

#### Health Check

```
GET /v1/collector/health
```

Returns `{"status": "ok"}`. No authentication required.

### Storage

The collector uses its own DuckDB database file (`~/.thinkt/collector.duckdb`), separate from the indexer's `index.duckdb`. This ensures no contention between local indexing and remote trace collection.

**Schema:**

| Table | Purpose |
|-------|---------|
| `collected_sessions` | Session summaries with entry counts, source, model |
| `collected_entries` | Individual trace entries with tokens, thinking length |
| `collected_agents` | Registered exporter agents with heartbeat timestamps |

The collector uses a single-writer batch pattern: incoming HTTP requests are queued to a buffered channel, and a single goroutine drains and writes batches in transactions. This avoids DuckDB's single-writer limitation while handling concurrent HTTP ingestion.

### Authentication

When `--token` is provided, all endpoints except `/v1/collector/health` require a Bearer token:

```bash
# Start collector with auth
thinkt collect --token mytoken

# Client request
curl -H "Authorization: Bearer mytoken" http://localhost:4318/v1/traces/stats
```

Token comparison uses constant-time comparison to prevent timing attacks.

### Agent Registry

The collector tracks registered exporter agents:

- Agents register via `POST /v1/agents/register`
- Each ingest request updates the agent's heartbeat and trace count
- Agents idle for more than 5 minutes are marked as "stale"
- A background goroutine cleans stale agents every minute

---

## Exporter

The exporter watches local JSONL session files and ships new trace entries to a collector endpoint.

### CLI Usage

```bash
thinkt export [flags]
```

| Flag | Description | Default |
|------|-------------|---------|
| `--collector-url` | Collector endpoint URL | (auto-discover) |
| `--api-key` | Bearer token for collector auth | (none) |
| `--source` | Filter by source (`claude\|kimi\|gemini\|copilot\|codex`) | (all) |
| `--forward` | Continuous watch mode | `false` |
| `--flush` | Flush the disk buffer and exit | `false` |
| `--quiet, -q` | Suppress non-error output | `false` |
| `--log` | Write debug log to file | (none) |

### Modes

**One-shot export** (default): Scans all watch directories, ships all traces, and exits.

```bash
thinkt export
```

**Watch mode** (`--forward`): Continuously watches for new/modified session files and ships entries as they are written.

```bash
thinkt export --forward
```

**Buffer flush** (`--flush`): Drains the local disk buffer (useful after collector was temporarily unavailable).

```bash
thinkt export --flush
```

### Standalone Binary

```bash
thinkt-exporter --watch-dir ~/.claude/projects \
                --watch-dir ~/.kimi/sessions \
                --collector-url http://collect.example.com:4318/v1/traces \
                --api-key mytoken
```

| Flag | Description | Default |
|------|-------------|---------|
| `--watch-dir` | Directory to watch (repeatable) | (required) |
| `--collector-url` | Collector endpoint URL | (auto-discover) |
| `--api-key` | Bearer token | (none) |
| `--buffer-dir` | Disk buffer directory | `~/.thinkt/export-buffer/` |
| `--quiet` | Suppress output | `false` |
| `--version` | Print version | — |
| `--log` | Debug log file | (none) |

### Collector Discovery

The exporter discovers the collector endpoint via a 4-step cascade:

1. **Environment variable**: `THINKT_COLLECTOR_URL`
2. **Project config**: `.thinkt/collector.json` in the project directory
3. **Well-known endpoint**: HTTP-based discovery
4. **Local buffer**: Falls back to disk buffering only (no remote)

```bash
# Via environment variable
export THINKT_COLLECTOR_URL=http://collect.example.com:4318/v1/traces
thinkt export --forward

# Via flag (highest priority)
thinkt export --collector-url http://localhost:4318/v1/traces
```

### Disk Buffer

When the collector is unreachable, payloads are buffered to disk in `~/.thinkt/export-buffer/`. On the next successful connection (or via `--flush`), buffered payloads are drained in chronological order.

| Config | Default |
|--------|---------|
| Buffer directory | `~/.thinkt/export-buffer/` |
| Max buffer size | 100 MB |
| Batch size | 100 entries per POST |
| Flush interval | 5 seconds |

### File Offset Tracking

The exporter tracks read offsets for each session file, so re-processing only ships entries written since the last read. This makes `--forward` mode efficient even with large session files.

---

## TUI Integration

Both the collector and exporter have status pages in the interactive TUI:

### Collector Status Page

Accessible when a collector instance is running. Shows:
- Server URL, status, uptime
- Total traces, sessions, DB size
- Registered agents with heartbeat status (active/stale)
- Recent collected sessions

Auto-refreshes every 5 seconds. Keys: `r` refresh, `esc` back, `q` quit, `j/k` scroll.

### Exporter Status Page

Shows exporter configuration and real-time statistics:
- Collector URL and connection status
- Watched directories
- Buffer status (buffered traces, buffer size)
- Export stats (shipped, failed, last ship time)

Keys: `esc` back, `q` quit, `j/k` scroll.

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `THINKT_COLLECTOR_URL` | Collector endpoint URL for the exporter | (auto-discover) |
| `THINKT_API_KEY` | Bearer token for collector authentication | (none) |

---

## Default Ports

| Service | Port | Description |
|---------|------|-------------|
| `thinkt collect` | 4318 | Trace collector HTTP server |
| `thinkt serve` | 8784 | Full web interface and REST API |
| `thinkt serve lite` | 8785 | Lightweight debug webapp |
| `thinkt serve mcp --port` | 8786 | MCP server over HTTP |

---

## Deployment Examples

### Local Development

Run both the collector and exporter on the same machine:

```bash
# Terminal 1: Start collector
thinkt collect --token dev-token

# Terminal 2: Watch and forward traces
thinkt export --forward --api-key dev-token --collector-url http://localhost:4318/v1/traces
```

### Team Server

Run a central collector that receives traces from team members:

```bash
# On the server
thinkt-collector --host 0.0.0.0 --port 4318 --token team-secret --storage /data/traces.duckdb

# On each developer machine
export THINKT_COLLECTOR_URL=http://collect.team.internal:4318/v1/traces
export THINKT_API_KEY=team-secret
thinkt export --forward
```

### Docker

```bash
# Run collector in Docker
docker run -p 4318:4318 \
  -v /data/collector:/data/.thinkt \
  ghcr.io/wethinkt/thinkt:latest collect --host 0.0.0.0 --token mytoken
```
