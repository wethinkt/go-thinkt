# `go-thinkt` CHANGELOG

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
