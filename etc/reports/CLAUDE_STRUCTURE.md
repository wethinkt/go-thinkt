# Claude Code Local Storage Structure

> Updated 2026-10-26 against Claude Code 2.1.3xx. Record-level detail lives in
> [CLAUDE_TRANSCRIPT_SCHEMA.md](./CLAUDE_TRANSCRIPT_SCHEMA.md). Sections below
> the directory layout describe older file formats and may lag.

## Overview
Claude Code stores configuration, session history, and supporting data locally across multiple locations. The architecture balances **local persistence with cloud integration**, storing complete conversation history locally while managing some state server-side.

## Directory Layout

Observed on macOS with Claude Code 2.1.310. Entries marked (docs) are described
at https://code.claude.com/docs/en/claude-directory; the rest are observed.

```
~/.claude.json                      # Main configuration file
~/.claude/                          # Supporting data directory
├── backups/                        # Settings backups
├── cache/                          # Changelog and misc caches
├── daemon.*, daemon/               # Background daemon state, lock, log
├── downloads/
├── feedback/
├── file-history/                   # Pre-edit snapshots for checkpoint restore (docs)
│   └── {session_id}/
│       └── {file_hash}@v{n}
├── history.jsonl                   # Prompt history: display, pastedContents, timestamp, project, sessionId (docs)
├── ide/                            # IDE integration lock files
├── jobs/
├── paste-cache/                    # Pasted content by hash (docs)
├── plugins/                        # Plugin marketplaces and cache
├── projects/                       # Session storage per project (docs)
│   └── {escaped-project-path}/     # "/" and non-alphanumerics replaced by "-"
│       ├── {session_id}.jsonl      # Main transcript
│       ├── {session_id}/
│       │   ├── subagents/          # Subagent transcripts (docs)
│       │   │   ├── agent-{agentId}.jsonl
│       │   │   └── agent-{agentId}.meta.json   # agentType, description, toolUseId, spawnDepth
│       │   └── tool-results/       # Spilled tool output and full-size MCP images (docs)
│       │       └── {id}.txt
│       └── memory/                 # Auto memory notes (docs)
│           ├── MEMORY.md
│           └── {slug}.md
├── session-env/                    # Per-session environment metadata, usually empty (docs)
│   └── {session_id}/
├── sessions/                       # One small file per running session (docs)
│   ├── {pid}.json                  # pid, sessionId, cwd, version, name, status, messagingSocketPath
│   └── {pid}.{hash}.key
├── settings.json
├── shell-snapshots/                # Shell state snapshots
├── skills/                         # User skills
├── state/
├── stats-cache.json
└── telemetry/
```

No longer present in this layout: `debug/`, `todos/`, `statsig/`, and the
per-project `sessions-index.json` (its role is taken by `ai-title` and
`last-prompt` records inside the transcript). thinkt still reads
`sessions-index.json` when an older install has one.

## File Formats

### 1. ~/.claude.json
Main configuration with atomic write pattern:
```json
{
  "numStartups": 5,
  "installMethod": "native",
  "autoUpdates": false,
  "tipsHistory": { "new-user-warmup": 1, "plan-mode-for-complex-tasks": 1 },
  "cachedGrowthBookFeatures": { "tengu_mcp_tool_search": true, ... },
  "userID": "142c3dcf7992b559822714fb3ab2abb1...",
  "firstStartTime": "2026-01-27T04:15:54.386Z",
  "oauthAccount": {
    "accountUuid": "...",
    "emailAddress": "user@example.com",
    "organizationUuid": "...",
    "displayName": "User"
  },
  "projects": {
    "/path/to/project": {
      "allowedTools": [],
      "hasTrustDialogAccepted": true,
      "lastCost": 0.71,
      "lastSessionId": "...",
      "lastModelUsage": { "claude-opus-4-5": { "inputTokens": 6313, ... } }
    }
  },
  "sonnet45MigrationComplete": true,
  "opus45MigrationComplete": true
}
```

Key sections:
- `tipsHistory` - Tip display tracking
- `cachedGrowthBookFeatures` - Feature flag cache (30+ flags)
- `userID` - Anonymous user identifier
- `oauthAccount` - Authenticated account info
- `projects` - Per-project settings, permissions, and usage stats
- Migration flags for model transitions
- `groveConfigCache`, `s1mAccessCache`, `passesEligibilityCache` - Service caches

### 2. Session Files (~/.claude/projects/{path}/{id}.jsonl)
Full conversation history in JSON Lines:
```json
{"type":"file-history-snapshot","messageId":"...","snapshot":{"trackedFileBackups":{}}}
{"parentUuid":null,"isSidechain":false,"userType":"external","cwd":"/path","sessionId":"...","version":"2.1.20","gitBranch":"main","type":"user","message":{"role":"user","content":"..."},"uuid":"...","timestamp":"...","thinkingMetadata":{"maxThinkingTokens":31999},"todos":[],"permissionMode":"default"}
{"parentUuid":"...","type":"assistant","message":{"model":"claude-opus-4-5-20251101","content":[{"type":"thinking","thinking":"...","signature":"EsAD..."}],"usage":{"input_tokens":10,"cache_creation_input_tokens":3489}},"requestId":"req_..."}
```

Message metadata includes:
- `parentUuid` - Message tree structure (branching support)
- `isSidechain` - Parallel conversation branch flag
- `cwd` - Working directory at message time
- `gitBranch` - Git branch context
- `thinkingMetadata` - Extended thinking config
- `permissionMode` - Permission level (default, bypass, etc.)
- `requestId` - API request tracking
- `usage` - Detailed token breakdown with cache stats

Message types:
- `user` - User input with full project context
- `assistant` - Assistant responses with cryptographically signed thinking blocks
- `tool_use` / `tool_result` - Tool interactions
- `file-history-snapshot` - File state tracking for undo/redo

### 3. Sessions Index (~/.claude/projects/{path}/sessions-index.json)
Metadata index for session discovery:
```json
{
  "version": 1,
  "entries": [
    {
      "sessionId": "20930eb4-a44f-4167-aff6-9950f1d0644d",
      "fullPath": "/Users/shannon/.claude/projects/.../20930eb4-...jsonl",
      "fileMtime": 1769530036992,
      "firstPrompt": "read the agents.md file...",
      "summary": "Claude Code Session Storage Analysis Setup",
      "messageCount": 4,
      "created": "2026-01-27T16:01:48.177Z",
      "modified": "2026-01-27T16:05:24.945Z",
      "gitBranch": "main",
      "projectPath": "/Users/shannon/projects/kimi-tracer",
      "isSidechain": false
    }
  ],
  "originalPath": "/Users/shannon/projects/kimi-tracer"
}
```

### 4. Global History (~/.claude/history.jsonl)
Cross-project command history:
```json
{"display":"read the agents.md file...","pastedContents":{},"timestamp":1769529708119,"project":"/Users/shannon/projects/kimi-tracer","sessionId":"20930eb4-a44f-4167-aff6-9950f1d0644d"}
```

### 5. Debug Logs (~/.claude/debug/{id}.txt)
Plain text debug output with timestamps:
```
2026-01-27T16:06:44.912Z [DEBUG] Failed to check enabledPlatforms: TypeError: ...
2026-01-27T16:06:44.991Z [DEBUG] [init] configureGlobalMTLS starting
2026-01-27T16:06:44.997Z [DEBUG] [STARTUP] Loading MCP configs...
2026-01-27T16:06:45.007Z [DEBUG] Found 0 plugins (0 enabled, 0 disabled)
```

### 6. Statsig Analytics (~/.claude/statsig/)
- `statsig.cached.evaluations.{id}` - Feature flag states (~23KB)
- `statsig.session_id.{id}` - Session tracking with start time
- `statsig.stable_id.{id}` - Persistent device ID
- `statsig.failed_logs.{id}` - Failed analytics events

### 7. Shell Snapshots (~/.claude/shell-snapshots/)
Captured shell state for restoration:
```bash
# Snapshot file
# Unset all aliases to avoid conflicts with functions
unalias -a 2>/dev/null || true
# Functions
# Shell Options
setopt nohashdirs
setopt login
# Aliases
alias -- run-help=man
alias -- which-command=whence
export PATH=/Users/shannon/.local/bin:...
```

### 8. Todo Lists (~/.claude/todos/)
Session-specific todo storage:
```json
[]  // Empty array when no todos
```

### 9. Plugin Marketplace (~/.claude/plugins/)
Git-backed plugin repository:
- `known_marketplaces.json` - Registry of marketplaces
- `marketplaces/claude-plugins-official/` - Official plugins git repo
  - Full git history
  - Plugin manifests
  - Skills and commands

## Session Identification

1. **Project Path**: Escaped directory path
   - Example: `-Users-shannon-projects-kimi-tracer`
   - Slashes converted to hyphens
2. **Session ID**: UUID v4
   - Example: `4bd88c0e-a515-4228-8031-3358ce9ee440`
3. **Stable ID**: Persistent device identifier
   - Example: `"2b8021fa-316f-45ed-a8a0-98f7bce16ca6"`

## Storage Characteristics

| Metric | Value |
|--------|-------|
| Configuration Format | JSON |
| Session Format | JSON Lines |
| Protocol Capture | Via debug logs |
| File History | Versioned backups |
| Plugin System | Git-backed marketplace |
| **Breakdown** | |
| Projects/Sessions | ~808KB |
| Debug Logs | ~628KB |
| Plugins | ~4.9MB |
| Statsig | ~40KB |
| Cache | ~80KB |
| Config (~/.claude.json) | ~8KB |
| Total Data Size | ~6.3MB |
| Binary Size | ~188MB |

## Security

- Sensitive files use 0600 permissions
- Atomic write pattern (temp + rename)
- Automatic backup creation (.backup.{timestamp})
- Signed thinking blocks with signatures
- Shell snapshots may contain sensitive env vars

## Atomic Write Pattern

Claude uses a robust configuration update mechanism:
1. Write to temp file: `.claude.json.tmp.{pid}.{timestamp}`
2. Set permissions: 0600
3. Atomic rename to target
4. Preserve original permissions
5. Create automatic backup

## Data Portability

- Session files are human-readable JSONL
- Can resume sessions across devices (via cloud sync)
- Tool results cached locally for replay
- File history enables undo/redo across sessions

---
*Document based on Claude Code 2.1.20*
*Last reviewed: 2026-01-27*
