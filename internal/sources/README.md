# sources

Package `sources` aggregates all supported AI coding tool source factories.

## Supported Sources

- [`antigravity`](antigravity/README.md) — Antigravity CLI session source
- [`claude`](claude/README.md) — Claude Code session source
- [`codex`](codex/README.md) — Codex CLI session source
- [`copilot`](copilot/README.md) — Copilot CLI session source
- [`gemini`](gemini/README.md) — Gemini CLI session source
- [`kimi`](kimi/README.md) — Kimi Code session source
- [`qwen`](qwen/README.md) — Qwen Code session source

## Directory Structure

```
sources/
├── sources.go          # AllFactories() - registry of all source factories
├── antigravity/        # Antigravity CLI session source
├── claude/             # Claude Code session source
├── codex/              # Codex CLI session source
├── copilot/            # Copilot CLI session source
├── gemini/             # Gemini CLI session source
├── kimi/               # Kimi Code session source
└── qwen/               # Qwen Code session source
```

## Key Function

- [`AllFactories()`](sources.go) — returns a `[]thinkt.StoreFactory` for all supported sources. Add new sources here when adding support for a new AI coding tool.

## Adding a New Source

Each source subdirectory follows a common pattern:

1. **`discovery.go`** — implements [`thinkt.StoreFactory`](../thinkt/) (`Source()`, `Create()`, `IsAvailable()`) plus `Factory()` and `IsSessionPath()` helpers
2. **`store.go`** — implements [`thinkt.Store`](../thinkt/) (`ListProjects`, `ListSessions`, `LoadSession`, `OpenSession`, `WatchConfig`, etc.)
3. **`parser.go`** (optional) — reads source-specific JSONL/JSON formats and converts to [`thinkt.Entry`](../thinkt/)
4. Register the new factory in [`sources.go`](sources.go)
