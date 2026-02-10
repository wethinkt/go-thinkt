# `go-thinkt` CHANGELOG

## v0.5.0 (2026-02-09)

* **Codex CLI Source Support**: Added `codex` as a first-class source across thinkt
  - New source implementation under `internal/sources/codex`
  - Source discovery and registry wiring in CLI and TUI
  - Session loading and JSONL detection now recognize Codex traces
  - Added `THINKT_CODEX_HOME` environment variable (`~/.codex` by default)
* **CLI/TUI Source Coverage**: Updated source filters and labels to include Codex
  - `--source` flags now support `codex`
  - Source picker, project/session filters, and source color mapping updated

* **Indexer REST API**: New OpenAPI endpoints for search and statistics
  - `GET /api/v1/search` - Search across indexed sessions
  - `GET /api/v1/stats` - Get usage statistics (tokens, tools)
  - `GET /api/v1/indexer/health` - Check indexer health
* **Search Enhancements**: Case-insensitive by default, with regex support
  - `--case-sensitive` / `-C` flag for exact case matching
  - `--regex` / `-E` flag for regular expression queries (Go RE2 syntax)
  - Available via CLI, REST API (`case_sensitive`, `regex` params), and MCP
* **DuckDB Concurrency Fix**: Copy-on-read fallback for locked databases
  - `OpenReadOnly()` retries and falls back to copying DB when locked
  - Watcher opens/closes DB per ingestion instead of holding long-lived connection
* **Port Allocation Fix**: Instance registry prevents port conflicts
  - PID-based instance tracking in `~/.thinkt/instances.json`
  - Clear error messages when port already in use
  - Automatic cleanup of stale entries

## v0.4.0 (2026-02-08)

* Moar polish 💎

 ## v0.3.4 (2026-02-07)

  * Add `thinkt-indexer`
  * Lots of polish 💎

 ## v0.2.4 (2026-02-03)

So much more stuff...

 * Added Kimi, Gemini, Copilot
 * Added `thinkt serve` OpenAPI server
 * Added `thinkt serve mcp` MCP server

 ## v0.1.0 (2026-01-24)

 * Initial release

