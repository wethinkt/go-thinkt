# Thinkt Indexer Plan

This document outlines the architecture for the DuckDB-powered indexer for `thinkt`.

## 1. Goal
Provide a fast, searchable, and persistent storage layer for LLM conversation traces from multiple sources (Claude, Kimi, Gemini, Copilot).

## 2. Storage Strategy: The Hybrid Schema
We leverage DuckDB's native JSON support to maintain flexibility while ensuring high performance for metadata filtering.

### Key Tables
- `sync_state`: Tracks file modification times and line offsets for incremental indexing.
- `projects`: Normalizes workspace paths.
- `sessions`: High-level conversation metadata.
- `entries`: Individual turns, with the full structure stored as a JSON blob.

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
- [ ] Database Schema & Migrations
- [ ] Core Ingester (Incremental updates)
- [ ] Search API
- [ ] Filesystem Watcher (inotify/fsnotify)
