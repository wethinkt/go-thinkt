# gemini

Package `gemini` implements the session source for [Gemini CLI](https://github.com/google-gemini/gemini-cli).

## Directory Structure

```
gemini/
├── discovery.go          # StoreFactory implementation, session path detection
├── session.go            # Session and message types with JSON parsing
├── store.go              # thinkt.Store implementation
├── *_test.go             # Tests
```

## Session File Format

Sessions are stored as single JSON files (not JSONL).

```
~/.gemini/
├── tmp/
│   └── {project-hash}/
│       ├── chats/
│       │   └── {session-id}.json     # Complete session
│       └── logs.json                 # Debug logs (used for project naming)
└── installation_id                   # Workspace ID
```

Each `.json` file deserializes into a [`Session`](session.go) containing an array of [`Message`](session.go) objects:

| Struct | Description |
|--------|-------------|
| [`Session`](session.go) | Top-level session with ID, project hash, timestamps, and messages. |
| [`Message`](session.go) | Single message with type, content, model, and token counts. Content is polymorphic (string or structured). |
| [`ToolCall`](session.go) | Tool execution with ID, name, arguments, and [`ToolResult`](session.go). |
| [`Thought`](session.go) | Agent thinking with subject, description, and timestamp. |
| [`Tokens`](session.go) | Token usage breakdown (input, output, cached, thoughts, tool, total). |

The base directory defaults to `~/.gemini` and can be overridden with `THINKT_GEMINI_HOME`.

## Key Types

### Discovery & Store

- [`Discoverer`](discovery.go) — implements [`thinkt.StoreFactory`](../../thinkt/). Detects Gemini CLI installations and creates stores.
- [`Store`](store.go) — implements [`thinkt.Store`](../../thinkt/). Manages project listing, session loading, caching, and file watching.

### Session Types

- [`Session`](session.go) — represents a Gemini CLI conversation session with ID, project hash, timestamps, and messages.
- [`Message`](session.go) — a single message with ID, timestamp, type, content, tool calls, thoughts, tokens, and model.
- [`ToolCall`](session.go) — tool execution with ID, name, arguments, and result.
- [`ToolResult`](session.go) / [`FunctionResponse`](session.go) — tool response wrapping with ID, name, and response data.
- [`Thought`](session.go) — agent's internal thinking process with subject, description, and timestamp.
- [`Tokens`](session.go) — token usage statistics (input, output, cached, thoughts, tool, total).

## Key Functions

- [`Factory()`](discovery.go) — returns a [`thinkt.StoreFactory`](../../thinkt/) for Gemini CLI.
- [`IsSessionPath(path)`](discovery.go) — reports whether a path looks like a Gemini session file.
- [`NewStore(baseDir)`](store.go) — creates a new Gemini store.
