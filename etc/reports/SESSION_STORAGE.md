# Kimi Code vs Claude Code: Session Storage Analysis

## Fair Comparison Notice

This comparison is based on actual observed storage for **Kimi CLI 1.1** and **Claude Code 2.1.20** with active sessions on the same system. Both systems have been used for real work in the `/Users/shannon/projects/kimi-tracer` directory.

---

## Quick Reference

| Aspect | Kimi Code | Claude Code |
|--------|-----------|-------------|
| **Config Location** | `~/.kimi/config.toml` | `~/.claude.json` |
| **Data Directory** | `~/.kimi/` (~7.5MB) | `~/.claude/` (~6MB) |
| **Session Storage** | `~/.kimi/sessions/` | `~/.claude/projects/` |
| **Binary Location** | `~/.local/share/uv/tools/kimi-cli/` | `~/.local/share/claude/versions/` |
| **Config Format** | TOML | JSON |
| **Session Format** | JSON Lines | JSON Lines |

---

## Configuration Storage

### Kimi Code: TOML in `~/.kimi/config.toml`
```toml
default_model = "kimi-code/kimi-for-coding"
default_thinking = true

[models."kimi-code/kimi-for-coding"]
provider = "managed:kimi-code"
model = "kimi-for-coding"
max_context_size = 262144
capabilities = ["thinking", "video_in", "image_in"]

[loop_control]
max_steps_per_turn = 100
max_retries_per_step = 3
```

**Characteristics:**
- Human-readable TOML format
- Separates models, providers, services into sections
- OAuth references use keyring storage
- Loop control settings exposed

### Claude Code: JSON in `~/.claude.json`
```json
{
  "installMethod": "native",
  "autoUpdates": false,
  "cachedGrowthBookFeatures": { "tengu_mcp_tool_search": true, ... },
  "userID": "142c3dcf7992b559822714fb3ab2abb1...",
  "oauthAccount": { "emailAddress": "...", "displayName": "..." },
  "projects": {
    "/path/to/project": {
      "lastCost": 0.71,
      "lastModelUsage": { "claude-opus-4-5": { "inputTokens": 6313 } }
    }
  },
  "thinkingMigrationComplete": true
}
```

**Characteristics:**
- Single JSON file (~8KB with active projects)
- 30+ feature flags cached (GrowthBook)
- OAuth account info stored locally
- Per-project settings and usage statistics
- Migration flags for model transitions
- Atomic write with automatic backups

---

## Directory Structure

### Kimi Code
```
~/.kimi/
├── bin/
│   └── rg                          # Bundled ripgrep binary (4.2MB)
├── config.toml                     # Main configuration file
├── device_id                       # Unique device identifier (UUID)
├── kimi.json                       # Work directory tracking
├── latest_version.txt              # Version check info
├── logs/
│   └── kimi.log                    # Application logs (28KB)
├── sessions/                       # Session storage (3.3MB)
│   └── {work_dir_hash}/            # Hash of working directory path
│       └── {session_id}/           # UUID for each session
│           ├── context.jsonl       # Conversation context
│           ├── context_sub_*.jsonl # Chunked context files (for large sessions)
│           └── wire.jsonl          # Full wire protocol log
└── user-history/
    └── {work_dir_hash}.jsonl       # Command history per directory
```

### Claude Code
```
~/.claude.json                    # Global configuration file
~/.claude/
├── cache/
│   └── changelog.md              # Cached changelog (80KB)
├── debug/
│   ├── {session_uuid}.txt        # Debug logs per session
│   └── latest -> ...             # Symlink to latest log
├── downloads/                    # Downloaded files
├── history.jsonl                 # Global command history
├── plans/                        # Saved plans
├── plugins/
│   ├── known_marketplaces.json   # Plugin marketplace registry
│   └── marketplaces/             # Cloned plugin repos
├── projects/                     # Per-project session storage
│   └── {path-slug}/              # Escaped directory path
│       ├── sessions-index.json   # Session metadata index
│       └── {session_id}.jsonl    # Full conversation history
├── session-env/                  # Session environment state
│   └── {session_id}/             # Per-session env data
├── shell-snapshots/              # Shell environment captures
│   └── snapshot-{shell}-{ts}.sh  # Zsh/bash env snapshot
├── statsig/                      # Analytics & feature flags
│   ├── statsig.cached.evaluations.{id}
│   ├── statsig.failed_logs.{id}
│   ├── statsig.session_id.{id}
│   └── statsig.stable_id.{id}
└── todos/                        # Todo list persistence
    └── {session_id}-agent-{id}.json
```

---

## Session Storage Architecture

### Kimi Code: Hash-Based Directory Structure
```
~/.kimi/sessions/
└── {md5(work_dir)}/              # e.g., cb138c0d413e29f1337ca17e6116842c
    └── {uuid}/                   # e.g., 056784bd-1def-4c32-a1cd-cd1d862ee239
        ├── context.jsonl         # Clean conversation context
        ├── context_sub_*.jsonl   # Chunked for large sessions
        └── wire.jsonl            # Complete wire protocol
```

**Design Philosophy:**
- Work directory hashed for privacy
- Dual storage: clean context + raw wire protocol
- Automatic chunking for large conversations
- Session resumption by directory

**Session Identification Strategy:**
1. **Work Directory Hash**: MD5 hash of the absolute path to the working directory
   - Example: `cb138c0d413e29f1337ca17e6116842c` = `/Users/shannon/projects/kimi-tracer`
2. **Session ID**: UUID v4 generated per session
   - Example: `056784bd-1def-4c32-a1cd-cd1d862ee239`

This allows:
- Multiple projects with isolated sessions
- Multiple sessions per project
- Easy session resumption by directory

### Claude Code: Project-Path Directory Structure
```
~/.claude/projects/
└── {escaped-path}/               # e.g., -Users-shannon-projects-kimi-tracer
    ├── sessions-index.json       # Session metadata
    ├── {uuid}.jsonl              # e.g., 4bd88c0e-a515-4228-8031-3358ce9ee440.jsonl
    └── {uuid}/
        └── tool-results/         # Cached tool outputs
```

**Design Philosophy:**
- Human-readable project directory names
- Indexed session discovery (sessions-index.json)
- Tool result caching for replay
- Cloud-sync friendly structure

**Session Index Example** (`sessions-index.json`):
```json
{
  "version": 1,
  "entries": [
    {
      "sessionId": "20930eb4-a44f-4167-aff6-9950f1d0644d",
      "fullPath": "/Users/shannon/.claude/projects/-Users-shannon-projects-kimi-tracer/20930eb4-a44f-4167-aff6-9950f1d0644d.jsonl",
      "firstPrompt": "read the agents.md file and report...",
      "summary": "Claude Code Session Storage Analysis Setup",
      "messageCount": 4,
      "created": "2026-01-27T16:01:48.177Z",
      "modified": "2026-01-27T16:05:24.945Z",
      "gitBranch": "main"
    }
  ]
}
```

---

## Session File Format Comparison

### Kimi: context.jsonl
```json
{"role": "_checkpoint", "id": 0}
{"role":"user","content":"user input"}
{"role": "_usage", "token_count": 5997}
{"role":"assistant","content":[...],"tool_calls":[...]}
{"role":"tool","content":[...],"tool_call_id":"tool_xxx"}
```

### Claude: {session}.jsonl
```json
{"type":"file-history-snapshot","messageId":"...","snapshot":{...}}
{"parentUuid":null,"sessionId":"...","type":"user","message":{"role":"user","content":"..."}}
{"parentUuid":"...","type":"assistant","message":{"model":"claude-opus-4-5","content":[{"type":"thinking","thinking":"...","signature":"..."}]}}
```

### Key Differences

| Feature | Kimi | Claude |
|---------|------|--------|
| **Message Linking** | Linear sequence | Parent UUID tree (branching) |
| **Thinking Blocks** | Inline, `encrypted` field | Cryptographically signed |
| **Token Tracking** | Per-turn `_usage` role | In message `usage` object |
| **Checkpoints** | Explicit `_checkpoint` role | `file-history-snapshot` |
| **Tool Results** | In context.jsonl | Cached in tool-results/ dir |
| **Session Metadata** | Minimal | Rich (cwd, gitBranch, permissionMode) |
| **Conversation Branches** | Not supported | `isSidechain` flag |
| **API Tracking** | None | `requestId` per message |

---

## Protocol Capture

### Kimi: Complete Wire Protocol
`wire.jsonl` captures everything with timestamps:
```json
{"timestamp": 1769528710.01941, "message": {"type": "TurnBegin", ...}}
{"timestamp": 1769528849.3468192, "message": {"type": "StatusUpdate", "payload": {"context_usage": 0.0228, ...}}}
```

**Captured:** TurnBegin, StepBegin, ContentPart, ToolCall, ToolResult, StatusUpdate

**Notable Features:**
1. **Chunked Context**: Large sessions are split into `context_sub_1.jsonl`, `context_sub_2.jsonl`, etc.
2. **Dual Storage**: Both structured `context.jsonl` and raw `wire.jsonl` are maintained
3. **Checkpoints**: Internal `_checkpoint` markers for state recovery
4. **Usage Tracking**: Token counts stored after each turn
5. **Thinking Content**: Encrypted thinking blocks stored inline

### Claude: Debug Logs
`~/.claude/debug/{session}.txt` contains:
```
2026-01-27T16:06:44.912Z [DEBUG] Failed to check enabledPlatforms: ...
2026-01-27T16:06:45.007Z [DEBUG] Found 0 plugins (0 enabled, 0 disabled)
```

**Captured:** Debug messages, startup info, errors (not full protocol)

---

## Additional Features

### Kimi-Specific
| Feature | Location | Purpose |
|---------|----------|---------|
| User History | `~/.kimi/user-history/{hash}.jsonl` | Per-directory command history |
| Device ID | `~/.kimi/device_id` | Unique device identifier |
| Bundled Tools | `~/.kimi/bin/rg` | Self-contained ripgrep |

### Claude-Specific
| Feature | Location | Purpose |
|---------|----------|---------|
| Global History | `~/.claude/history.jsonl` | Cross-project command history |
| File History | `~/.claude/file-history/{id}/` | Versioned file backups (`{hash}@v1`) |
| Shell Snapshots | `~/.claude/shell-snapshots/` | Shell state capture for restoration |
| Todo Lists | `~/.claude/todos/` | Session-specific todos (`{session}-agent-{id}.json`) |
| Plugins | `~/.claude/plugins/` | Git-backed marketplace with skills |
| Statsig | `~/.claude/statsig/` | Feature flags, analytics, session tracking |
| Session Env | `~/.claude/session-env/{id}/` | Per-session environment state |
| Tool Results | `~/.claude/projects/{path}/{id}/tool-results/` | Cached tool outputs |

---

## Security Comparison

| Aspect | Kimi | Claude |
|--------|------|--------|
| **Sensitive File Permissions** | 0600 (device_id, sessions) | 0600 (sessions, projects/) |
| **API Key Storage** | System keyring (via oauth.storage=keyring) | OAuth tokens, not stored in config |
| **Thinking Protection** | `encrypted` field (nullable) | Cryptographic `signature` field |
| **Atomic Writes** | Standard | Temp file + rename pattern |
| **Automatic Backups** | No | Yes (.backup.{timestamp}) |
| **Device Identification** | `~/.kimi/device_id` (32-char hex) | `statsig.stable_id` (UUID) |
| **Path Privacy** | MD5 hash of work directory | Escaped path (human-readable) |
| **Account Info** | Not stored locally | `oauthAccount` in ~/.claude.json |

### Security Details

**Kimi Code:**
```
-rw-------  device_id              (restricted permissions)
-rw-------  context_sub_*.jsonl    (restricted permissions)
-rw-r--r--  config.toml            (readable config)
```
- Sensitive files have 0600 permissions
- API keys stored in system keyring (referenced, not stored)
- Device ID isolated
- Encrypted thinking blocks

**Claude Code:**
```
-rw-------  .claude.json           (restricted permissions)
-rw-------  projects/*/*.jsonl     (restricted session data)
drwx------  projects/              (restricted directories)
-rw-r--r--  debug/*.txt            (readable logs)
```
- Config and session files have 0600 permissions
- Project directories have 0700 permissions
- Atomic write pattern (temp file + rename)
- Signed thinking blocks with cryptographic verification
- OAuth tokens managed separately

---

## Data Portability

### Kimi
- ✅ Copy `~/.kimi/` to migrate
- ✅ All formats human-readable
- ✅ Can grep session history
- ✅ No cloud dependency

### Claude
- ✅ Copy `~/.claude/` to migrate
- ✅ JSONL is human-readable
- ✅ Cloud sync available
- ⚠️ Some state may be server-side

---

## Storage Size Comparison (Actual)

| Component | Kimi | Claude |
|-----------|------|--------|
| Configuration | 1KB | 8KB |
| Sessions | 3.2MB | 808KB (projects/) |
| Logs | 56KB | 628KB (debug/) |
| User History | 12KB | 4KB |
| Cache/Tools | 4.2MB (rg) | 80KB |
| Plugins | N/A | 4.9MB |
| Analytics | N/A | 40KB (statsig) |
| **Subtotal (Data)** | **~7.5MB** | **~6.3MB** |
| Binary | Minimal (Python via uv) | 188MB (native Bun) |
| **Total with Binary** | **~7.5MB** | **~194MB** |

Note: Kimi's Python runtime is managed by `uv` and shared across tools, while Claude Code bundles its own Bun runtime.

### Detailed File Size Comparison

| Component | Kimi Code | Claude Code |
|-----------|-----------|-------------|
| Configuration | 4KB (toml+json) | 4KB (json) |
| Sessions | 3.3MB | 82KB (2 sessions) |
| Session Index | in kimi.json | 1KB per project |
| Logs | 28KB | 226KB |
| Debug | - | per-session txt |
| Plugins | - | ~1MB (marketplace) |
| Cache | minimal | 80KB |
| Shell Snapshots | - | 1KB per session |
| Statsig/Analytics | - | 25KB |
| **~/.kimi/ or ~/.claude/** | **~3.4MB** | **~5.6MB** |
| Binaries | 4.2MB (rg) | 188MB (full app) |
| **Total with binaries** | **~7.6MB** | **~194MB** |

*Note: Claude's binary size includes the full application bundle in ~/.local/share/claude/, while Kimi uses a Python package installation. Session sizes grow with usage.*

---

## Architectural Philosophy

### Kimi: Transparency-First
- **Wire Protocol Logging**: Every API interaction captured with timestamps
- **Minimal State**: Config is concise (~1KB), focused on model/provider setup
- **Privacy by Hashing**: Work directory paths hashed (MD5) in storage
- **Single-Purpose Files**: context.jsonl for conversation, wire.jsonl for protocol
- **Data Ownership**: Users have full access to their conversation history

### Claude: Feature-Rich Integration
- **Deep IDE Features**: File versioning, shell snapshots, todo tracking
- **Per-Project State**: Usage stats, permissions, model costs tracked per project
- **Branching Support**: `parentUuid` and `isSidechain` enable conversation trees
- **Plugin Ecosystem**: Git-backed marketplace with skills and commands
- **Rich Metadata**: Session index with summaries and message counts

---

## Strengths Summary

### Kimi Code
1. **Transparent Protocol**: Complete wire capture for debugging
2. **Clean Architecture**: Clear separation of concerns
3. **Self-Contained**: Bundled tools (ripgrep)
4. **Predictable**: Simple directory hashing
5. **Lightweight Binary**: Python via uv (shared runtime)
6. **Offline Operation**: Designed for privacy, no cloud dependency

### Claude Code
1. **Rich Ecosystem**: Plugins, marketplace, skills
2. **File Versioning**: Automatic file backups with hash-based storage
3. **Session Index**: Easy session discovery with summaries
4. **Shell Integration**: State snapshots for environment restoration
5. **Robust Updates**: Atomic writes with backups
6. **Conversation Branching**: Sidechain support for parallel explorations

---

## Use Case Recommendations

### Choose Kimi If:
- You want complete protocol visibility
- You prefer minimal cloud integration
- You need to debug AI interactions deeply
- You want simple, predictable storage

### Choose Claude If:
- You want plugin ecosystem
- You need automatic file versioning
- You want rich IDE-like features
- You prefer managed cloud sync

---

## Technical Implementation Details

### Kimi Session File Format

**context.jsonl** stores the conversation as a sequence of JSON objects:
- `_checkpoint`: Internal recovery markers
- `user`: User input text
- `assistant`: Assistant response with content blocks
- `tool`: Tool execution results
- `_usage`: Token count metadata

**wire.jsonl** captures the actual protocol:
- Timestamps for every event
- Turn boundaries
- Content parts (think blocks, text, tool calls)
- Tool results
- Status updates (context usage, token counts)

### Claude Session File Format

**{session_id}.jsonl** stores the conversation as a sequence of JSON objects:
- `file-history-snapshot`: File state snapshots for undo capability
- `user`: User input with metadata (timestamp, cwd, version, git branch)
- `assistant`: Assistant response with content blocks (thinking, tool_use, text)
- Tool results appended after tool_use blocks

**sessions-index.json** provides session discovery:
- Session ID and file path mapping
- First prompt preview and AI-generated summary
- Message count, creation/modification timestamps
- Git branch context

### Claude Configuration Pattern

Claude uses an **atomic write pattern** for configuration updates:
1. Write to temp file: `.claude.json.tmp.{pid}.{timestamp}`
2. Set permissions: 0600
3. Atomic rename to `.claude.json`
4. Automatic backups created with timestamps

This ensures configuration integrity even during crashes.

---

*Merged report based on SESSION_STORAGE_REPORT.md and SESSION_STORAGE_COMPARISON.md*
*Last updated: 2026-01-27*
*Analyzed: Kimi CLI 1.1 and Claude Code 2.1.20*
