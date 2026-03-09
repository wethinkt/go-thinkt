# `go-thinkt` CHANGELOG

## v0.8.0 (unreleased)

* **Local Summarization**
  - New `internal/indexer/summarize` package — local 3B generative model (Qwen 2.5 3B Instruct) for thinking block summarization via yzma/llama.cpp
  - Separate per-model summaries DuckDB at `~/.thinkt/summaries/<model-id>.duckdb` with `(session_id, entry_uuid)` common key matching entries and embeddings tables
  - Per-entry thinking block classification: summary, category (idea/discovery/concern/decision/pattern/rejected), entities, and relevance score
  - New tag suggestion flow for sharing/discovery: `SuggestTags()` API plus `thinkt-indexer summarize tags [text]` with normalized tag output and `--json`
  - Session-level summaries via `__session__` sentinel key
  - Greedy (temp=0) autoregressive generation with JSON-structured output and robust fallback parsing
  - Summarization/tagging prompts moved into embedded assets under `internal/indexer/summarize/prompts/`
  - `thinkt-indexer summarize` CLI with `list`, `run`, `enable`, `disable`, `model`, `status`, `sync`, `purge` subcommands (mirrors embeddings pattern)
  - Added command-level golden coverage for `thinkt-indexer summarize tags --json` and prompt/parser unit tests for the new tag extraction path
  - Added JSON struct tags to summarize/tag result types for stable machine-readable output
  - `SummarizationConfig` in `~/.thinkt/config.json` (opt-in, disabled by default)
  - `Ingester.SummarizeAllSessions()` for batch summarization of indexed sessions
  - Exported `embedding.EnsureRuntime()` for shared llama.cpp runtime setup

## v0.7.8 (2026-03-06)

* **Docker: Upgrade to Debian Trixie**
  - Upgraded all Dockerfiles from Debian Bookworm to Trixie (build and runtime stages)
  - Added `libffi8` runtime dependency for goreleaser and indexer images

* **Documentation**
  - Fixed broken SEE ALSO cross-reference links in Hugo command docs

## v0.7.7 (2026-03-06)

* **Server CLI Cleanup**
  - `server start` now accepts `--port`, `--host`, `--quiet`, `--token`, and `--cors-origin` flags, passing them through to `server run`
  - Removed `--no-open` flag and auto-open browser behavior from `server run`

* **Setup Suggestions Polish**
  - Refreshed post-setup suggested commands: `thinkt projects`, `thinkt web`, `thinkt search`, `thinkt theme`
  - Removed conditional suggestions for indexer watch and embedding enablement

## v0.7.6 (2026-03-06)

* **Setup Wizard: Apps, Terminal & Embedding Model**
  - Added app discovery step to `thinkt setup` — multi-select checklist to enable/disable discovered apps
  - Added default terminal detection from `TERM_PROGRAM` and `TERM` environment variables
  - Terminal confirmation step with auto-detected default and manual picker fallback
  - Embedding model picker when enabling embeddings — choose between nomic-embed-text-v1.5 (~140MB) and qwen3-embedding-0.6b (~800MB)
  - Setup flow is now: Welcome → Home → Sources → Apps → Terminal → Indexer → Embeddings → Model → Suggestions
  - `--ok` mode auto-detects terminal and accepts all defaults
  - Full i18n coverage (en, zh-Hans, es) for all new steps

* **Indexer: Copy-on-Read Snapshots**
  - Added filesystem snapshot-based copy-on-read fallback with platform-specific implementations (darwin, linux, windows)

* **Token Management**
  - Enhanced token management commands with improved help descriptions

* **Search: RPC-First with Direct DB Fallback**
  - TUI search and indexer CLI now use the indexer RPC when available, avoiding DuckDB lock contention on ext4 and other filesystems without reflink support
  - Falls back to direct read-only DB access when the indexer isn't running
  - Clear error messages when the index database doesn't exist ("run 'thinkt-indexer sync' to build the search index")

* **Bug Fixes**
  - Hardened setup, watcher, and Windows path handling

## v0.7.5 (2026-03-04)

  * **Consolidated Release & Homebrew Cask**
    - Single `brew install --cask thinkt` now installs all 4 binaries: `thinkt`, `thinkt-indexer`, `thinkt-exporter`, and `thinkt-collector`
    - Merged two goreleaser configs into one unified build via `goreleaser-cross` for darwin and linux
    - Added native Windows build job (`.goreleaser-windows.yml`) with CGO support for DuckDB
    - All platform archives now contain the full binary suite, shell completions, and man pages
    - Removed separate `thinkt-indexer` homebrew cask
    - Moved indexer Docker image config into the main goreleaser config
    - Dropped FreeBSD builds (can be re-added as non-CGO supplement later)
  
* **First-Run Discover Setup**
  - Added `thinkt discover` with interactive setup plus non-interactive `--ok` and machine-readable `--json` modes
  - Root command now auto-triggers discover when config does not exist, then reinitializes i18n with the chosen language
  - New inline discover wizard flow with progressive source scanning, per-source approval, sticky context, ESC-to-exit, and improved step UX
  - Discover defaults now save source selections, indexer/embeddings choices, and `discovered_at` metadata

* **Help System Overhaul**
  - `thinkt help` now shows help topics and all available commands instead of delegating to root help
  - Added `thinkt help cheat` — visual command tree with box-drawing glyphs and `--json` output
  - Added `thinkt help llms` — pipe-friendly usage guide for AI assistants (unchanged content)
  - All help text is i18n-aware with Spanish and Chinese translations
  - Added Hugo documentation pages for `thinkt help`, `thinkt help cheat`, and `thinkt help llms`
  - Updated README, llms.txt, and command reference docs

* **Session Metadata Cache**
  - Added `MetadataCache` type for persistent session metadata, avoiding repeated deep JSONL parsing on every list
  - Integrated cache-first metadata loading in Claude, Kimi, Gemini, Copilot, Codex, and Qwen stores
  - Added `ListSessionsOption` / `WithEnrich` to the `Store` interface so callers can opt into background enrichment
  - TUI and API server now trigger background enrichment for incremental session metadata updates

* **Error Handling & Domain Errors**
  - Added sentinel domain errors (`ErrSessionNotFound`, `ValidationError`, etc.)
  - API returns 404 instead of 500 for missing sessions
  - MCP tools now map domain errors to appropriate MCP error codes
  - Resume flow uses typed errors instead of string-prefix matching
  - Session resolver uses `ErrSessionNotFound` instead of `os.ErrNotExist`

* **CLI & TUI Polish**
  - Replaced `tabwriter` with lipgloss for styled CLI output
  - Styled `thinkt apps list` output
  - Improved error messages for config loading failures
  - Fixed discover wizard triggering during shell completion
  - Fixed MCP version info

* **Source Management**
  - Added `thinkt sources enable`, `thinkt sources disable`, and `thinkt sources status` commands (`--all` supported)
  - `sources list` and `sources status` now show richer source metadata including enabled state, project/session counts, size, workspace, and base path
  - Added `SourceConfig` support in config, and source registry filtering now respects configured enabled sources
  - Fixed source toggles not taking effect when `config.sources` was present by correctly applying source-map settings in registry/source status paths
  - Config loading now returns `ErrNoConfig` when missing instead of auto-creating files

* **Metrics & Observability**
  - Added API server Prometheus endpoint at `GET /metrics` (unauthenticated)
  - Added API request instrumentation (`http_requests_total`, request duration histogram, in-flight gauge)
  - Added indexer RPC `metrics` method plus `thinkt indexer metrics` command
  - Added `thinkt server metrics` command and merged indexer metrics into server `/metrics` output with `thinkt_indexer_` prefixing

* **Performance Improvements**
  - Codex and Copilot sources now cache full session scans and use lightweight typed metadata parsing for list operations
  - Kimi and Qwen listing paths now avoid deep/full JSONL parsing during project/session enumeration
  - Improved TUI loading messages for project/session pickers and shell startup hints

* **Exporter & Collector Fixes**
  - Fixed exporter watcher FD/memory growth by cleaning debounced timers and adding stale-watch pruning
  - Normalized collector URLs to consistently resolve to `/v1/traces`
  - Collector agent registry now sets `StartedAt` for implicitly created agents (heartbeat/trace activity path)

* **Internationalization**
  - Added extensive discover-wizard and loading-message translations for Spanish (`es`) and Simplified Chinese (`zh-Hans`)
  - `thinkt language set` now reinitializes the localizer immediately after changes

* **Packaging & Docs**
  - Updated generated command docs and manpage packaging for discover, sources enable/disable, and server/indexer metrics commands

## v0.7.0 (2026-03-01)

* **Trace Collector Server**: Push-based trace aggregation via `thinkt collect`
  - HTTP server with chi router on port 8785 (configurable `--port`, `--host`)
  - `POST /v1/traces` for batch trace ingestion from exporters
  - `GET /v1/traces/search` and `GET /v1/traces/stats` for querying
  - `POST /v1/agents/register` and `GET /v1/agents` for agent lifecycle
  - `POST /v1/sessions/activity` and `GET /v1/sessions/active` for session tracking
  - `GET /v1/collector/health` health check (no auth required)
  - Agent registration with heartbeat tracking and stale cleanup (5-min threshold)
  - Bearer token authentication with constant-time comparison
  - CORS support for browser-based clients
  - DuckDB storage with single-writer batch pattern (`~/.thinkt/dbs/collector.duckdb`)
  - Batch accumulation (100 entries / 2s flush interval) with transactional writes
  - Request normalization: role validation, whitespace cleanup, token clamping
  - Thinking/tool-use classification extracted from trace entries during ingest

* **WebSocket Streaming**: Real-time session tailing via `GET /v1/sessions/{id}/ws`
  - Backfills last 50 entries on connection, supports `?after=` timestamp filter
  - In-memory pub/sub fan-out to multiple subscribers
  - Ticket-based auth for browser clients (`POST /v1/ws/ticket`, 30-second single-use)

* **Trace Exporter**: Watch and ship local traces via `thinkt export`
  - One-shot export, continuous watch mode (`--forward`), and buffer flush (`--flush`)
  - Lazy directory watcher with on-demand recursive expansion and 2-second debounce
  - HTTP shipper with 3 retries and exponential backoff (1s/2s/4s)
  - Disk buffer for offline resilience (`~/.thinkt/export-buffer/`, 100 MB default)
  - Collector discovery cascade: `THINKT_COLLECTOR_URL`, project config, well-known, local fallback
  - File offset tracking for incremental-only shipping
  - Session activity tracking: start/active/end lifecycle events (5-min inactivity timeout)
  - Agent heartbeat registration every 2 minutes

* **Parquet Export**: `thinkt collect export-parquet`
  - Offline export from DuckDB to Parquet via native `COPY` statement
  - Time filtering with `--since` and `--until`
  - Configurable output directory (default `~/.thinkt/exports/parquet/`)

* **Agents System**: Unified local + remote agent detection
  - Agent hub merges filesystem-detected sessions with collector-reported agents
  - `thinkt agents` lists active agents with source, project, machine info (`--json` supported)
  - `thinkt agents follow [session-id]` for live conversation tailing (TUI, `--json`, `--raw`)
  - Local streaming via filesystem tail, remote streaming via WebSocket

* **TUI Views**: Collector, exporter, and agents pages in the interactive TUI
  - Collector page: live server status, agents, sessions, stats (auto-refreshes every 5s)
  - Exporter page: connection, watched directories, buffer, export statistics
  - Agents page: merged local/remote agent list with filter modes (all/local/remote)
  - Agent tail: live conversation streaming with role filters, auto-scroll, flash indicator
  - New navigation result types integrated into Shell

* **Prometheus Metrics**: Observability for collector and exporter
  - Collector: `thinkt_collector_ingest_entries_total`, `_requests_total`, `_duration_seconds`, `_tokens_total`, `_batch_flush_duration_seconds`, `_active_sessions`, `_active_agents`, `_db_size_bytes`, `_ws_connections_active`
  - Exporter: `thinkt_exporter_watched_directories`, `_file_events_total`, `_export_entries_shipped`, `_export_entries_failed`, `_buffer_size_bytes`, `_ship_requests_total`, `_ship_duration_seconds`
  - Available at `GET /metrics` (collector) and optional `--metrics-port` (exporter)

* **Standalone Binaries**: `thinkt-exporter` and `thinkt-collector`
  - Lightweight flag-based CLIs (no cobra dependency)
  - Environment variable fallbacks for `THINKT_COLLECTOR_URL` and `THINKT_API_KEY`
  - Exporter supports optional Prometheus metrics server via `--metrics-port`

* **Instance Registry**: Added `collector` instance type for port conflict prevention and service discovery

## v0.6.4 (2026-03-01)

* **RPC Refactor**
  - Replace `exec.Command` with RPC calls for MCP and REST handlers communicating with the indexer server
  - Formalize all RPC protocol types into named structs in `protocol.go` (SearchData, StatsData, SyncProgressData, etc.)
  - Add `OKResponse`/`ProgressFrom` helpers to reduce boilerplate
  - Clean up review issues: remove duplicate types, use typed responses, rename `SearchResponse.Sessions` to `Results`
  - Replace duplicated server response structs with type aliases to canonical `rpc.*` types

* **TUI**
  - Dual progress bars for sync progress display

* **Indexer**
  - Add tier and score fields to semantic search results

* **HTTP API & MCP**
  - Add `TotalEmbeddings` and `EmbedModel` to stats response
  - Replace `map[string]any` with typed `IndexerHealthResponse`
  - Normalize source filter server-side for consistent case-insensitive matching
  - Consistent query validation with `TrimSpace` across all search handlers


## v0.6.3 (2026-02-28)

* **Internationalization (i18n)**
  - Full i18n support across CLI, TUI, and server messages
  - Added **Spanish** (`es`) translation with 100% coverage
  - Added **Chinese (Simplified)** (`zh-Hans`) translation subset
  - Added `thinkt language` command tree: `get`, `list`, `set` (defaults to `get`)
  - Automatic locale detection from system environment (`LANG`, `LC_ALL`)
  - Support for overriding locale via `THINKT_LANG` environment variable
  - Standardized message ID patterns and fallback to English for missing translations
  - Created comprehensive i18n documentation (`docs/I18N.md`) and Hugo book page (`docs/hugo/content/languages.md`)

* **Configuration**
  - Added `THINKT_HOME` data directory section to README explaining directory layout and contents

* **REST API**
  - Added `GET /api/v1/languages` returning the active language and list of supported languages

* **TUI**
  - Interactive language picker in TUI (`thinkt language set`)

## v0.6.2 (2026-02-28)

* **Developer Tools & Quality**
  - Add comments to schema mostly for llms
  - Add simple database migration system
  - Added TOML syntax validation test to ensure translation files are correctly formatted
  - Added automated translation coverage test to identify missing keys in locale files
 
* **TUI**
  - Add scrollbar and mouse scroll support to the conversation viewer

* **Bug Fixes**
  - Fix sessions showing `0 msgs` in web UI: backfill `EntryCount` by counting JSONL lines when `sessions-index.json` is missing or has `messageCount: 0`

## v0.6.1 (2026-02-28)

* **Inline Image Rendering**
  - Display embedded images (screenshots pasted into Claude Code) directly in the TUI conversation viewer
  - Uses Kitty Unicode placeholder protocol for terminals that support it (Ghostty, Kitty, WezTerm)
  - Falls back to text placeholder with dimensions and size for unsupported terminals
  - `--raw` mode renders images via Kitty graphics or Sixel protocols
  - Added `5:Media` filter toggle to show/hide image and document blocks independently

* **Build & Distribution**
  - Added macOS entitlements (`com.apple.security.cs.disable-library-validation`) for notarized builds, allowing the unsigned libffi library used by yzma to load
  - Cleaner CLI output: suppress cobra usage on errors for `server` and `indexer` commands
  - Better stderr handling when launching the indexer subprocess

* **Bug Fixes**
  - Fix missing Claude sessions: `sessions-index.json` was trusted as sole source of truth; now always scans filesystem and uses the index only to enrich metadata
  - Fix image entries silently dropped from Claude sessions due to `imagePasteIds` type mismatch (integer vs string)
  - Fix user content blocks (images) not converted in lazy session path (`ToThinktEntry`)

## v0.6.0 (2026-02-27)

* **On-Device Semantic Search**
  - Added in-process embedding via yzma/llama.cpp — no external subprocess needed
  - Ships with Qwen3-Embedding-0.6B (1024-dim) and nomic-embed-text-v1.5 (768-dim)
  - Added `semantic search` MCP tool and REST endpoint (`GET /api/v1/semantic-search`)
  - Added tiered embedding extraction: conversation and reasoning tiers with role prefixes
  - Added `--tier` filter to `semantic search` (default: conversation)
  - Interactive TUI picker for semantic search results with session viewer

* **Embedding Management**
  - Added `embeddings` command tree: `list`, `model`, `status`, `sync`, `enable`, `disable`, `purge`
  - Per-model DuckDB files — switching models no longer requires re-embedding
  - `embeddings list` shows tabulated model info: dimensions, pooling, model file size, session count, DB size
  - `embeddings status` shows active model stats with conversation/reasoning tier breakdown
  - `embeddings model` interactive picker or direct `embeddings model <id>` switch
  - `embeddings purge` removes stale embedding databases from previous models
  - Server cancels and restarts embedding sync on model change

* **Indexer Server (RPC)**
  - Replaced `indexer watch` with `indexer serve` — persistent Unix socket RPC server
  - CLI commands (`search`, `semantic search`, `stats`, `embeddings sync`) try RPC first, fall back to inline
  - Added sync status reporting and progress callbacks over RPC
  - Added `indexer status` endpoint with embedding progress info
  - Config reload notification for live model/embedding changes

* **Non-TTY / Pipe-Safe Output**
  - All interactive TUI commands error with helpful messages when stdout is not a terminal
  - `search` and `semantic search` auto-fall back to `--list` output when piped
  - `embeddings list` strips ANSI colors when piped (lipgloss no longer leaks escape codes)
  - `embeddings model` without args errors in non-TTY instead of hanging
  - `thinkt` root, `theme browse`, `theme builder`, `apps enable/disable`, `apps set-terminal` all guarded

* **Top-Level CLI Aliases**
  - `thinkt search`, `thinkt semantic`, and `thinkt embeddings` now work as top-level commands (auto-start indexer)
  - `thinkt indexer embeddings` forwarding added alongside existing `search`, `stats`, `sessions`, `semantic`
  - Indexer commands remain available via `thinkt-indexer` for direct use

* **REST API**
  - Added `GET /api/v1/info` returning fingerprint, version, revision, uptime, PID, and auth status
  - Added `GET /api/v1/sessions/resolve` to retrieve session ownership metadata (project, source, workspace)
  - Added `default_terminal` field to `GET /api/v1/open-in/apps` response
  - Added `terminal` boolean to `AppInfo` for identifying terminal-capable apps

* **CLI Polish**
  - `stats` top tools sorted descending, limited to top 25 (was unsorted map iteration)
  - `stats` JSON output changed from `tool_usage` map to `top_tools` array (preserves order)
  - `embeddings list --json` includes `downloaded` flag and `size_bytes`
  - Friendly error messages when databases are locked by another process
  - Added `--json` output to `embeddings status`

* **Security Hardening**
  - Enhanced session path validation with symlink escape tests
  - Improved CORS handling and request sanitization for sensitive query parameters
  - Session resume endpoint restricted to POST-only with same-origin checks
  - DuckDB `enable_external_access=false` on all connections

* **TUI Polish**
  - Conversation viewer role filters renamed and reordered: `1:User 2:Assistant 3:Thinking 4:Tools 5:Other`
  - Role filters color-coded to match conversation view labels
  - Filters integrated into shell header bar (single-line header with project, session, filters, and branding)
  - "Other" role filter disabled by default (hides system/summary/progress entries)
  - Session separator shows full filename, no longer clips UUID-length names
  - Added `w` key to open current session in thinkt web
  - Styled loading screen with centered "Loading..." and "🧠 thinkt" branding
  - Theme browser header shows "🧠 thinkt" right-aligned
  - Fixed theme browser left/right pane height mismatch

* **Platform Fixes**
  - Fixed Ghostty and Alacritty open-in on macOS (`open -a` → `open -na` for new instance)
  - Fixed Terminal.app opening extra window on fresh launch (reordered AppleScript `do script`/`activate`)
  - Fixed `thinkt-indexer server stop` accepting extra arguments (added `cobra.NoArgs`)

* **Performance and Internals**
  - Concurrent project and session cache loading with deduplication
  - Defensive cache copying and invalidation methods
  - Centralized indexer binary lookup in config package
  - Removed Apple embedding backend (fully replaced by yzma)
  - Removed deprecated `ExtractText` in favor of `ExtractTiered`
  - Added model name validation for synthetic vs real assistant models

* **Web Lite**
  - Fixed authentication: reads token from URL hash fragment for API requests
  - Updated embedded web and web-lite assets

* **Docs**
  - Updated README, Hugo book, and LLM guide with top-level aliases, new API endpoints, and embeddings docs
  - Regenerated command reference (76 pages, includes new `search`, `semantic`, `embeddings` commands)

## v0.5.1 (2026-02-23)

* **Server and Indexer Lifecycle**
  - Auto-start indexer sidecar with web server; manage indexer lifetime alongside server
  - Added `--no-indexer` flag to disable automatic indexer sidecar
  - Server uses auth tokens by default
  - Improved server log management

* **Indexer Performance**
  - Added in-memory session index for O(1) file change lookups instead of O(N) project scanning
  - Added lazy database pool for file watcher to reduce overhead during bursts of file events
  - Improved watcher exclusion logic to prevent false positives on paths containing "thinkt" as a substring
  - Added warning when running `indexer watch` if a background indexer is already active

* **CLI**
  - Renamed `thinkt serve` commands to `thinkt server`
  - Added `thinkt server logs` and `thinkt indexer logs` commands
  - Added `--json` flag to `server status` and `indexer status` for machine-readable output
  - API updates for LLM ergonomics

* **Docs**
  - Updated docs for web-lite and server commands

## v0.5.0 (2026-02-19)

This release includes all changes from `v0.4.1..HEAD`.

* **Source Coverage and Cross-Source Identity**
  - Added `codex` and `qwen` as first-class sources across CLI, TUI, docs, and server flows
  - Added Codex home detection (`THINKT_CODEX_HOME`, default `~/.codex`)
  - Unified source discovery/factories and generalized source-specific code paths
  - Improved source+project keying to avoid cross-source project/session collisions
  - Fixed Codex parsing to dedupe duplicate assistant/thinking events from mixed log records
  - Fixed Qwen session loading and path detection

* **Sessions, Resume, and Rendering Performance**
  - Added session resume support, then hardened resume behavior and terminal config handling
  - Improved session loading concurrency and progressive rendering behavior
  - Switched TUI session -> conversation transitions to async rendering to reduce race artifacts
  - Added cross-cut logging improvements and session picker reliability fixes

* **Search, Indexer, and API Surface**
  - Exposed indexer capabilities through OpenAPI
  - Added REST endpoints:
    - `GET /api/v1/search`
    - `GET /api/v1/stats`
    - `GET /api/v1/indexer/health`
  - Added search flags `--regex/-E` and `--case-sensitive/-C` across CLI/API/MCP
  - Improved search UX, added indexer search in TUI, and added find-within-conversation
  - Improved DuckDB lock handling with copy-on-read fallback and watcher connection lifecycle fixes
  - Added instance registry-based port allocation to prevent collisions and clean stale entries

* **CLI and App Management**
  - Added `thinkt projects --json`
  - Added `thinkt apps` command tree
  - Added first-run app probing and expanded default app corpus
  - Added app opening hardening and platform-specific app behavior updates
  - Added `apps` tests
  - Added `theme`-focused command/default updates

* **Serve, Security, and Distribution**
  - Added `thinkt serve --dev` proxy flow
  - Hardened Open-in handling by disallowing metacharacters in paths
  - Added `thinkt-indexer` Homebrew tap support
  - Updated embedded `thinkt-web` and `web-lite` submodules

* **Docs, Build, and CI**
  - Added `thinkt help llms` and refreshed developer documentation
  - Updated Codex docs and burned latest docs output
  - Fixed swag doc generation source selection and local-swagger build failures
  - Fixed CGO build issues and Windows `isProcessAlive` behavior
  - Added lint testing and explicit promise/error handling cleanup
  - CI updates: main build fixes, shell tests, parameterized software versions, and lint config adjustments

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
