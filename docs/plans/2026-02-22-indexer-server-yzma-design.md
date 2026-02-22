# Indexer Server with Yzma Embeddings

## Overview

Redesign `thinkt-indexer` from a CLI tool into a long-running server that owns the DuckDB connection and embedding model. Replace Apple's NLContextualEmbedding (macOS-only, 512-dim, black-box) with Qwen3-Embedding via yzma (cross-platform, 1024-dim, controllable).

## Architecture

```
thinkt-indexer server
  │
  ├── RPC listener (Unix socket ~/.thinkt/indexer.sock)
  │     JSON-over-newline protocol
  │
  ├── Sync manager (goroutine, high priority)
  │     Triggered by: startup, RPC sync, watch events
  │     Indexes session metadata into DuckDB
  │     Mutex prevents concurrent syncs
  │
  ├── Embed worker (background goroutine, low priority)
  │     Polls for unembedded sessions when sync is idle
  │     Pauses when sync is active, resumes after
  │     Holds yzma model reference (loaded once)
  │
  ├── File watcher (fsnotify, --no-watch disables)
  │     Detects .jsonl changes, debounces, triggers sync
  │
  └── DuckDB (single write connection, server-owned)
```

### CLI Commands as RPC Clients

```
thinkt-indexer sync          →  socket if available, else inline
thinkt-indexer search        →  socket if available, else inline (copy-on-read)
thinkt-indexer semantic search → socket if available, else inline
thinkt-indexer stats         →  socket if available, else inline
```

### thinkt Integration

`thinkt serve` launches `thinkt-indexer server` as a managed child process (SIGTERM on shutdown). MCP/HTTP tools shell out to `thinkt-indexer` CLI commands, which connect to the server via socket. Binary separation preserved (no DuckDB/yzma dependency in thinkt).

## Server Command

### `thinkt-indexer server`

Replaces the old `watch` command. Lifecycle:

1. Open DuckDB (sole write connection)
2. Load yzma model from `~/.thinkt/models/` (auto-download on first run)
3. Start Unix socket listener at `~/.thinkt/indexer.sock`
4. Register in `~/.thinkt/instances.json`
5. Start file watcher (unless `--no-watch`)
6. Run initial sync
7. Start background embed worker

Shutdown (SIGTERM/SIGINT): unregister instance, remove socket file, unload model, close DB.

### Flags

- `--no-watch` — disable fsnotify file watching
- `--db <path>` — DuckDB path (default `~/.thinkt/index.duckdb`)
- `--log <path>` — log file

## Unix Socket RPC Protocol

Listener: `~/.thinkt/indexer.sock` (removed on shutdown).

### Request Format

One JSON object per line:

```json
{"method": "sync", "params": {}}
{"method": "sync", "params": {"force": true}}
{"method": "search", "params": {"query": "auth bug", "limit": 20}}
{"method": "semantic_search", "params": {"query": "authentication timeout"}}
{"method": "stats", "params": {}}
{"method": "status", "params": {}}
```

### Response Format

Single response or streamed progress lines followed by a final response:

```json
{"ok": true, "data": {...}}
{"ok": false, "error": "sync already in progress"}
```

Progress (for sync):
```json
{"progress": true, "data": {"phase": "index", "done": 5, "total": 60, "message": "..."}}
{"progress": true, "data": {"phase": "embed", "done": 12, "total": 152, "message": "..."}}
{"ok": true, "data": {"indexed": 60, "embedded": 152}}
```

### Status Response

```json
{
  "ok": true,
  "data": {
    "state": "embedding",
    "sync_progress": {"done": 60, "total": 60},
    "embed_progress": {"done": 45, "total": 152},
    "model": "qwen3-embedding-0.6b",
    "model_dim": 1024,
    "uptime_seconds": 3600,
    "watching": true
  }
}
```

## Priority & Coordination

- **Sync manager** holds a mutex. Only one sync runs at a time.
- **Embed worker** checks the mutex before each session. If sync is active, it waits.
- **Watch events** feed into the sync manager (same path as RPC sync, but scoped to changed files).
- All DB access through the single server-owned connection — no concurrency issues.

### Sync Command Behavior

- **Server running:** send sync request, stream progress, report completion
- **No server:** run inline (current behavior — open DB, load model on demand, index, embed, close)
- **Sync already in progress:** report current progress, don't start a second one
- **`--force`:** drop sync state, re-index everything from scratch

## Yzma Embedding Integration

### Model

- **Model:** Qwen3-Embedding-0.6B-Q8_0.gguf
- **Location:** `~/.thinkt/models/Qwen3-Embedding-0.6B-Q8_0.gguf`
- **Auto-download:** on first use via `yzma/pkg/download`
- **Runtime:** yzma auto-installs llama.cpp libs to `.yzma/lib`

### Configuration

- Dimension: 1024
- Pooling: `PoolingTypeLast`
- Normalization: L2
- Context size: 2048
- Batch size: 1024

### Interface

The `embedding` package keeps a similar interface:

```go
type Embedder struct {
    model   llama.Model
    ctx     llama.Context
    vocab   llama.Vocab
    dim     int
}

func NewEmbedder(modelPath string) (*Embedder, error)
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error)
func (e *Embedder) Dim() int
func (e *Embedder) Close()
```

No subprocess, no JSON IPC — direct Go function calls.

## Schema Changes

### Embeddings Table

```sql
-- Change FLOAT[512] to FLOAT[1024]
CREATE TABLE IF NOT EXISTS embeddings (
    id          VARCHAR PRIMARY KEY,
    session_id  VARCHAR NOT NULL,
    entry_uuid  VARCHAR NOT NULL,
    chunk_index INTEGER NOT NULL DEFAULT 0,
    model       VARCHAR NOT NULL,
    dim         INTEGER NOT NULL,
    embedding   FLOAT[1024] NOT NULL,
    text_hash   VARCHAR NOT NULL,
    created_at  TIMESTAMP DEFAULT current_timestamp,
    UNIQUE(entry_uuid, chunk_index, model)
);
```

### Migration

On startup, if existing embeddings have a different model than the configured one:
1. Drop all rows from `embeddings` table
2. Log a message explaining the re-embed
3. Embed worker picks them up naturally (sessions with entries but no embeddings)

## Removals

- `tools/thinkt-embed-apple/` — Swift CLI binary
- `build:embed-apple` — Taskfile task
- `embedding.Client` — subprocess-based embedding client
- `watch` command — replaced by `server`
- `embedding.Available()` — no longer needed (yzma is always available if model is downloaded)

## CLI Discovery Logic

All CLI commands (`sync`, `search`, `semantic search`, `stats`) follow:

```
1. Does ~/.thinkt/indexer.sock exist?
2. Can we connect?
3. Yes → send RPC request, return response
4. No  → run inline (current behavior)
```

For write operations (sync), inline mode loads the model on demand.
For read operations (search, stats), inline mode uses copy-on-read DB fallback.
