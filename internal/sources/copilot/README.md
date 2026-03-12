# copilot

Package `copilot` implements the session source for [GitHub Copilot CLI](https://docs.github.com/en/copilot).

## Directory Structure

```
copilot/
├── discovery.go          # StoreFactory implementation, session path detection
├── parser.go             # Event stream parser converting to thinkt.Entry
├── store.go              # thinkt.Store implementation
├── types.go              # Event types and data structures
├── *_test.go             # Tests
```

## Session File Format

Sessions are stored as JSONL event streams, read via the [`internal/jsonl`](../../jsonl/) streaming reader.

```
~/.copilot/
└── session-state/
    └── {session-uuid}/
        └── events.jsonl
```

Each line is an [`Event`](types.go) with a type, data payload, ID, timestamp, and optional parent ID. Event types:

| Event Type Constant | Description | Data Struct |
|---------------------|-------------|-------------|
| `EventTypeSessionStart` | Session initialization | — |
| `EventTypeSessionInfo` | Session metadata (CWD, model) | — |
| `EventTypeUserMessage` | User prompt | — |
| `EventTypeAssistantMsg` | Model response | [`AssistantMessageData`](types.go) |
| `EventTypeToolExecStart` | Tool execution begins | — |
| `EventTypeToolExecSuccess` | Tool returned result | [`ToolExecutionSuccessData`](types.go) |
| `EventTypeToolExecError` | Tool returned error | [`ToolExecutionErrorData`](types.go) |
| `EventTypeToolExecComplete` | Tool execution finished | — |

The [`Parser`](parser.go) converts events into [`thinkt.Entry`](../../thinkt/) values. Tool calls are represented by [`ToolRequest`](types.go).

The base directory defaults to `~/.copilot` and can be overridden with `THINKT_COPILOT_HOME`. The `rewind-snapshots/` and `backups/` directories are excluded from file watching.

## Key Types

### Discovery & Store

- [`Discoverer`](discovery.go) — implements [`thinkt.StoreFactory`](../../thinkt/). Detects Copilot CLI installations and creates stores.
- [`Store`](store.go) — implements [`thinkt.Store`](../../thinkt/). Manages project listing, session loading, caching, and file watching.

### Parsing

- [`Parser`](parser.go) — reads Copilot event streams and converts them to [`thinkt.Entry`](../../thinkt/).

### Event Types (`types.go`)

- [`Event`](types.go) — a single line in `events.jsonl` with type, data, ID, timestamp, and parent ID.
- [`AssistantMessageData`](types.go) — structured data for assistant messages including content, tool requests, and reasoning text.
- [`ToolRequest`](types.go) — a tool call request with ID, name, arguments, and type.
- [`ToolExecutionSuccessData`](types.go) — successful tool execution result.
- [`ToolExecutionErrorData`](types.go) — failed tool execution with error message.

### Event Type Constants

`EventTypeSessionStart`, `EventTypeSessionInfo`, `EventTypeUserMessage`, `EventTypeAssistantMsg`, `EventTypeToolExecStart`, `EventTypeToolExecSuccess`, `EventTypeToolExecComplete`, `EventTypeToolExecError`

## Key Functions

- [`Factory()`](discovery.go) — returns a [`thinkt.StoreFactory`](../../thinkt/) for Copilot.
- [`IsSessionPath(path)`](discovery.go) — reports whether a path looks like a Copilot session file.
- [`NewStore(baseDir)`](store.go) — creates a new Copilot store.
- [`NewParser(r)`](parser.go) — creates a parser that reads events from an `io.Reader`.
