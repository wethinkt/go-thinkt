# Kimi Code Local Storage Structure

## Overview
Kimi Code stores all session data, configuration, and history locally in `~/.kimi/`. The architecture follows a **local-first, transparent** design philosophy where users have full access to their data in open formats.

## Directory Layout

```
~/.kimi/
├── bin/
│   └── rg                          # Bundled ripgrep binary (~4.2MB)
├── config.toml                     # Main configuration (models, providers, services)
├── device_id                       # Unique device identifier (32-char hex)
├── kimi.json                       # Working directory index
├── latest_version.txt              # Version check info
├── logs/
│   └── kimi.log                    # Application logs
├── sessions/                       # Session storage
│   └── {work_dir_hash}/            # MD5 hash of working directory path
│       └── {session_id}/           # UUID v4 for each session
│           ├── context.jsonl       # Structured conversation context
│           ├── context_sub_*.jsonl # Chunked context for large sessions
│           └── wire.jsonl          # Full wire protocol log
└── user-history/
    └── {work_dir_hash}.jsonl       # Command history per directory
```

## File Formats

### 1. config.toml
TOML configuration file containing:
- `default_model` - Default model selection
- `models` - Model configurations (capabilities, context size)
- `providers` - API provider settings with OAuth references
- `loop_control` - Execution limits (max_steps, max_retries)
- `services` - Search and fetch service endpoints
- `mcp.client` - MCP client configuration

### 2. device_id
Single line file containing a 32-character hexadecimal device identifier.
Example: `b3ac2e94370943b1a870d003adf36eb0`

### 3. kimi.json
JSON file tracking working directories and their last session:
```json
{
  "work_dirs": [
    {
      "path": "/Users/shannon/projects/kimi-tracer",
      "kaos": "local",
      "last_session_id": "056784bd-1def-4c32-a1cd-cd1d862ee239"
    }
  ]
}
```

### 4. Session Files (context.jsonl)
JSON Lines format with conversation messages:
```json
{"role": "_checkpoint", "id": 0}
{"role":"user","content":"user input here"}
{"role": "_usage", "token_count": 5997}
{"role":"assistant","content":[...],"tool_calls":[...]}
{"role":"tool","content":[...],"tool_call_id":"tool_xxx"}
```

Special roles:
- `_checkpoint` - Recovery markers
- `_usage` - Token count metadata
- `user` - User input
- `assistant` - Assistant response with content blocks
- `tool` - Tool execution results

### 5. Wire Protocol (wire.jsonl)
Complete protocol capture with timestamps:
```json
{"type": "metadata", "protocol_version": "1.1"}
{"timestamp": 1769528710.01941, "message": {"type": "TurnBegin", "payload": {...}}}
{"timestamp": 1769528846.708122, "message": {"type": "ContentPart", "payload": {"type": "think", ...}}}
{"timestamp": 1769528849.3468192, "message": {"type": "StatusUpdate", "payload": {"context_usage": 0.0228, ...}}}
```

Message types:
- `TurnBegin` - Start of user turn
- `StepBegin` - Start of processing step
- `ContentPart` - Content chunks (think, text, tool_use)
- `ToolCall` - Tool invocation
- `ToolResult` - Tool execution result
- `StatusUpdate` - Context usage, token counts

### 6. User History (user-history/{hash}.jsonl)
Simple command history:
```json
{"content":"modify my osx terminal login"}
{"content":"/yolo"}
{"content":"operate the instructions in AGENTS.md"}
```

## Session Identification

Two-level hashing system:
1. **Work Directory Hash**: MD5 of absolute path
   - Example: `cb138c0d413e29f1337ca17e6116842c`
   - Used for: Directory-based isolation
2. **Session ID**: UUID v4
   - Example: `056784bd-1def-4c32-a1cd-cd1d862ee239`
   - Used for: Individual session tracking

## Storage Characteristics

| Metric | Value |
|--------|-------|
| Configuration Format | TOML |
| Session Format | JSON Lines |
| Protocol Capture | Complete (wire.jsonl) |
| Chunking | Automatic for large sessions |
| Permissions | 0600 for sensitive files |
| **Breakdown** | |
| Sessions | ~3.2MB |
| Logs | ~56KB |
| User History | ~12KB |
| Bundled rg | ~4.2MB |
| Total Size | ~7.5MB |

## Security

- Sensitive files (device_id, session data) use 0600 permissions
- API keys stored in system keyring (referenced by key)
- OAuth tokens stored externally
- Device ID isolated from configuration

## Data Portability

- All formats are human-readable
- Can grep/search session history
- Can backup/restore individual sessions
- Can migrate by copying `~/.kimi/` directory

---
*Document based on Kimi CLI 1.1*
*Last reviewed: 2026-01-27*
