# Thinkt Indexer Plan

This document outlines the architecture for the DuckDB-powered indexer for `thinkt`.

## 1. Goal
Provide a fast, searchable, and persistent storage layer for LLM conversation traces from multiple sources (Claude, Kimi, Gemini, Copilot).

## 2. Storage Strategy: The Hybrid Schema
We leverage DuckDB's native JSON support to maintain flexibility while ensuring high performance for metadata filtering.

### Key Tables

**Index database** (`~/.thinkt/index.duckdb`):
- `sync_state`: Tracks file modification times and line offsets for incremental indexing.
- `projects`: Normalizes workspace paths.
- `sessions`: High-level conversation metadata.
- `entries`: Individual turns, with the full structure stored as a JSON blob.

**Embeddings database** (`~/.thinkt/embeddings.duckdb`):
- `embeddings`: Float vectors for semantic search. Stored separately because embeddings are large, have a different lifecycle (model changes invalidate them), and are essentially a derived cache.

## 3. Indexing Pipeline
1. **Discovery**: Startup scan or `fsnotify` (inotify) events.
2. **Filtering**: Check `sync_state` to skip unchanged files or resume from the last line.
3. **Parsing**: Use existing `internal/thinkt` and source-specific parsers.
4. **Ingestion**: Use DuckDB Appender API for high-throughput bulk inserts.

## 4. Query Patterns
- **Full-Text Search**: Using DuckDB's FTS extension on the `body->>'text'` field.
- **Aggregations**: Token usage and tool frequency calculations directly via SQL.
- **Filtering**: By model, git branch, or time range using structured columns.

## 5. Implementation Roadmap
- [x] Database Schema & Migrations
- [x] Core Ingester (Incremental updates)
- [x] Search API (CLI and REST)
- [x] Filesystem Watcher (fsnotify)
- [x] Copy-on-Read concurrency (DuckDB lock handling)

## 6. REST API Endpoints

The indexer is exposed via the REST API (`thinkt serve`):

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/search?q=query` | Search across indexed sessions (supports `case_sensitive`, `regex` params) |
| `GET /api/v1/stats` | Get usage statistics (tokens, tools) |
| `GET /api/v1/indexer/health` | Check indexer availability |

See Swagger docs at `http://localhost:8784/swagger` when server is running.
