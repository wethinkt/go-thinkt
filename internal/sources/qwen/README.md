# qwen

Package `qwen` implements the session source for [Qwen Code](https://qwen.ai).

## Directory Structure

```
qwen/
├── discovery.go          # StoreFactory implementation, session path detection
├── store.go              # thinkt.Store implementation
├── *_test.go             # Tests
```

## Session File Format

Sessions are stored as JSONL files, read via the [`internal/jsonl`](../../jsonl/) streaming reader.

```
~/.qwen/
├── projects/
│   └── {project-hash}/
│       └── chats/
│           └── {session-id}.jsonl
├── tmp/
│   └── {temp-dir}/
│       └── logs.json                   # Debug logs (used for project naming)
└── installation_id                     # Workspace ID
```

Project directories use dash-encoded paths as hashes (e.g., `-Users-evan-project` decodes to `/Users/evan/project`). Each session is a single `.jsonl` file.

The base directory defaults to `~/.qwen` and can be overridden with `THINKT_QWEN_HOME`.

## Key Types

- `qwenFactory` (unexported) — implements [`thinkt.StoreFactory`](../../thinkt/). Detects Qwen Code installations and creates stores.
- [`Store`](store.go) — implements [`thinkt.Store`](../../thinkt/). Manages project listing, session loading, caching, and file watching.

## Key Functions

- [`Factory()`](discovery.go) — returns a [`thinkt.StoreFactory`](../../thinkt/) for Qwen Code.
- [`IsSessionPath(path)`](discovery.go) — reports whether a path looks like a Qwen session file.
- [`IsAvailable()`](discovery.go) — checks if the Qwen data directory exists.
- [`NewStore(baseDir)`](store.go) — creates a new Qwen store.

## Notes

- Qwen uses dash-encoded directory names for projects.
- Sessions are stored as single `.jsonl` files per session.
