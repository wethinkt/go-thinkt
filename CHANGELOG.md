# `go-thinkt` CHANGELOG

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
