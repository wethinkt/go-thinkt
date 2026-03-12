# kimi

Package `kimi` implements the session source for [Kimi Code](https://kimi.ai).

## Directory Structure

```
kimi/
├── discovery.go          # StoreFactory implementation, session path detection
├── resume.go             # Session resume command generation
├── store.go              # thinkt.Store implementation
├── *_test.go             # Tests
```

## Session File Format

Sessions are stored as JSONL files, read via the [`internal/jsonl`](../../jsonl/) streaming reader. Kimi is unique in supporting **chunked sessions** that span multiple files.

```
~/.kimi/
├── sessions/
│   └── {work-dir-hash}/
│       └── {session-uuid}/
│           ├── context.jsonl           # Primary session file
│           ├── context_sub_1.jsonl      # Chunk 1 (optional)
│           ├── context_sub_2.jsonl      # Chunk 2 (optional)
│           └── ...
├── kimi.json                           # Work directory → hash mapping
└── device_id                           # Workspace ID
```

Project directories are identified by MD5 hash of the working directory path. The `kimi.json` file maps readable paths to their hashes. Chunked files (`context_sub_*.jsonl`) are read in numeric order after the primary `context.jsonl`.

The base directory defaults to `~/.kimi` and can be overridden with `THINKT_KIMI_HOME`.

## Key Types

- [`Discoverer`](discovery.go) — implements [`thinkt.StoreFactory`](../../thinkt/). Detects Kimi Code installations and creates stores.
- [`Store`](store.go) — implements [`thinkt.Store`](../../thinkt/). Manages project listing, session loading, caching, and file watching. Supports chunked session files (`context.jsonl` + `context_sub_*.jsonl`).

## Key Functions

- [`Factory()`](discovery.go) — returns a [`thinkt.StoreFactory`](../../thinkt/) for Kimi Code.
- [`IsSessionPath(path)`](discovery.go) — reports whether a path looks like a Kimi session file.
- [`DefaultDir()`](discovery.go) — returns the Kimi base directory.
- [`NewStore(baseDir)`](store.go) — creates a new Kimi store.
- [`(*Store).ResumeCommand(session)`](resume.go) — returns the command to resume a Kimi Code session.

## Notes

- Kimi uses MD5 hashing for project directory paths.
- Sessions may span multiple chunked JSONL files.
