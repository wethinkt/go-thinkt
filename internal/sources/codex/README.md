# codex

Package `codex` implements the session source for [Codex CLI](https://github.com/openai/codex).

## Directory Structure

```
codex/
├── discovery.go          # StoreFactory implementation, session path detection
├── parser.go             # JSONL entry parser converting to thinkt.Entry
├── store.go              # thinkt.Store implementation
├── *_test.go             # Tests
```

## Session File Format

Sessions are stored as JSONL files, read via the [`internal/jsonl`](../../jsonl/) streaming reader.

```
~/.codex/
└── sessions/
    └── {session-name}.jsonl
```

Each line is a JSON object with one of these entry types:

| Entry Type | Description |
|------------|-------------|
| `session_meta` | Session metadata including CWD (project path) and model. |
| `event_msg` | User message event. |
| `response_item` | Assistant response item. |

The [`Parser`](parser.go) converts these source-specific entries into [`thinkt.Entry`](../../thinkt/) values.

The base directory defaults to `~/.codex` and can be overridden with `THINKT_CODEX_HOME`.

## Key Types

- [`Discoverer`](discovery.go) — implements [`thinkt.StoreFactory`](../../thinkt/). Detects Codex CLI installations and creates stores.
- [`Parser`](parser.go) — reads Codex session JSONL entries and converts them to [`thinkt.Entry`](../../thinkt/).
- [`Store`](store.go) — implements [`thinkt.Store`](../../thinkt/). Manages project listing, session loading, caching, and file watching.

## Key Functions

- [`Factory()`](discovery.go) — returns a [`thinkt.StoreFactory`](../../thinkt/) for Codex CLI.
- [`IsSessionPath(path)`](discovery.go) — reports whether a path looks like a Codex session file.
- [`NewStore(baseDir)`](store.go) — creates a new Codex store.
- [`NewParser(r, sessionID)`](parser.go) — creates a parser that reads JSONL entries from an `io.Reader`.
