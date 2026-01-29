# AI Coding Assistant Storage Ontology Analysis

This report synthesizes the storage structures of Claude Code and Kimi Code to derive a common ontology, then evaluates how `internal/thinkt/types.go` maps to that ontology.

---

## Part 1: Ontology Synthesis

### 1.1 Core Domain Hierarchy

Both systems share a three-level hierarchy:

```
Workspace (implicit)
  └── Project (working directory)
        └── Session (conversation)
              └── Entry (turn/message)
                    └── ContentBlock (content unit)
```

| Level | Claude | Kimi | Common Abstraction |
|-------|--------|------|-------------------|
| Project | `~/.claude/projects/{escaped-path}/` | `~/.kimi/sessions/{md5-hash}/` | Working directory context |
| Session | `{uuid}.jsonl` | `{uuid}/context.jsonl` | Conversation instance |
| Entry | JSON object per line | JSON object per line | Single turn/event |
| Block | `content[]` array | `content[]` array | Content unit |

### 1.2 Entry Type Taxonomy

#### Conversational Entries (Both Systems)

| Role | Claude Type | Kimi Role | Description |
|------|-------------|-----------|-------------|
| User | `type: "user"` | `role: "user"` | Human input |
| Assistant | `type: "assistant"` | `role: "assistant"` | Model response |
| Tool | (embedded in user) | `role: "tool"` | Tool execution result |
| System | `type: "system"` | `role: "system"` | System messages |

#### Meta Entries (System-Specific)

| Category | Claude | Kimi | Purpose |
|----------|--------|------|---------|
| Checkpoint | `file-history-snapshot` | `_checkpoint` | State recovery point |
| Usage | In `message.usage` | `_usage` role | Token consumption |
| Progress | `type: "progress"` | (in wire.jsonl) | Streaming progress |
| Summary | `type: "summary"` | N/A | Conversation summary |
| Queue | `type: "queue-operation"` | N/A | Operation queuing |

### 1.3 Content Block Types

Both systems use content blocks within entries:

| Block Type | Claude | Kimi | Common Fields |
|------------|--------|------|---------------|
| Text | `type: "text"` | `type: "text"` | `text` |
| Thinking | `type: "thinking"` | `type: "thinking"` | `thinking`, `signature` |
| Tool Use | `type: "tool_use"` | `tool_calls[]` | `id`, `name`, `input` |
| Tool Result | `type: "tool_result"` | `type: "tool_result"` | `tool_use_id`, `content`, `is_error` |
| Image | `type: "image"` | (not documented) | `source.media_type`, `source.data` |
| Document | `type: "document"` | (not documented) | `source.media_type`, `source.data` |

### 1.4 Metadata Dimensions

#### Entry-Level Metadata

| Dimension | Claude | Kimi | Semantic |
|-----------|--------|------|----------|
| Identity | `uuid` | (generated) | Unique entry ID |
| Lineage | `parentUuid` | N/A | Conversation branching |
| Timestamp | `timestamp` (ISO8601) | `timestamp` (Unix float) | When created |
| Context | `cwd`, `gitBranch` | N/A | Working environment |
| Version | `version` | N/A | Client version |
| Branching | `isSidechain` | N/A | Parallel conversation |

#### Session-Level Metadata

| Dimension | Claude | Kimi | Source |
|-----------|--------|------|--------|
| Session ID | UUID in filename | UUID directory name | Path |
| Created | `sessions-index.json` | File mtime | Index/stat |
| Modified | `sessions-index.json` | File mtime | Index/stat |
| First Prompt | `sessions-index.json` | Must parse | Index/parse |
| Summary | `sessions-index.json` | N/A | Index |
| Message Count | `sessions-index.json` | Must count | Index/parse |
| Git Branch | `sessions-index.json` | N/A | Index |

#### Project-Level Metadata

| Dimension | Claude | Kimi |
|-----------|--------|------|
| Identity | Escaped path | MD5 hash |
| Display Name | Decoded from path | From `kimi.json` |
| Original Path | `sessions-index.json` | `kimi.json` |
| Session Count | Directory listing | Directory listing |

### 1.5 Auxiliary Data Streams

#### Protocol/Debug Data

| Stream | Claude | Kimi |
|--------|--------|------|
| Wire Protocol | N/A | `wire.jsonl` (complete) |
| Debug Logs | `~/.claude/debug/{id}.txt` | `~/.kimi/logs/kimi.log` |
| Event Types | N/A | TurnBegin, StepBegin, ContentPart, ToolCall, ToolResult, StatusUpdate |

#### Command History

| Aspect | Claude | Kimi |
|--------|--------|------|
| Location | `~/.claude/history.jsonl` (global) | `~/.kimi/user-history/{hash}.jsonl` (per-project) |
| Fields | `display`, `timestamp`, `project`, `sessionId` | `content` only |

#### Configuration

| Aspect | Claude | Kimi |
|--------|--------|------|
| Format | JSON (`~/.claude.json`) | TOML (`~/.kimi/config.toml`) |
| Per-Project | In main config | N/A |
| Models | Feature flags | Explicit model configs |

---

## Part 2: types.go Coverage Analysis

### 2.1 What's Well Modeled

| Concept | types.go Type | Coverage |
|---------|--------------|----------|
| Entry hierarchy | `Entry` | Good |
| Content blocks | `ContentBlock` | Good |
| Session metadata | `SessionMeta` | Good |
| Project | `Project` | Good |
| Token usage | `TokenUsage` | Good |
| Store abstraction | `Store` interface | Good |
| Streaming access | `SessionReader` | Good |
| Multi-source | `StoreRegistry` | Good |

### 2.2 What's Missing or Incomplete

#### A. Entry Types Not Modeled

```go
// Current roles:
RoleUser, RoleAssistant, RoleTool, RoleSystem, RoleSummary, RoleProgress

// Missing from taxonomy:
// - Checkpoint entries (Claude's file-history-snapshot, Kimi's _checkpoint)
// - Usage entries (Kimi's _usage role)
// - Queue operations (Claude's queue-operation)
```

**Recommendation:** Add `RoleCheckpoint` and consider whether `IsCheckpoint bool` is sufficient or if these should be first-class roles.

#### B. Wire Protocol Events (Kimi Only)

Kimi's `wire.jsonl` contains streaming events not representable in current model:

```go
// Not modeled:
type WireEvent struct {
    Timestamp float64     `json:"timestamp"`
    Type      string      `json:"type"`      // TurnBegin, StepBegin, ContentPart, etc.
    Payload   any         `json:"payload"`
}
```

**Recommendation:** Consider a `WireEvent` type or `EventLog` interface for protocol-level access.

#### C. Command History

Neither system's command history is modeled:

```go
// Not modeled:
type HistoryEntry struct {
    Content   string    `json:"content"`
    Timestamp time.Time `json:"timestamp"`
    Project   string    `json:"project,omitempty"`   // Claude only
    SessionID string    `json:"session_id,omitempty"` // Claude only
}
```

**Recommendation:** Add `HistoryEntry` type and extend `Store` interface with `ListHistory(ctx, projectID) ([]HistoryEntry, error)`.

#### D. Configuration

Configuration is not modeled at all:

```go
// Not modeled:
type Config interface {
    DefaultModel() string
    // Provider settings, feature flags, etc.
}
```

**Recommendation:** Out of scope for session storage, but note the gap.

#### E. File History/Snapshots (Claude Only)

Claude's file versioning system:

```go
// Not modeled:
type FileSnapshot struct {
    MessageID string
    Files     map[string]FileVersion // path -> version
}

type FileVersion struct {
    Hash    string
    Content []byte
}
```

**Recommendation:** Consider if file history should be accessible via the abstraction.

#### F. Session Chunking (Kimi Only)

Kimi splits large sessions into `context_sub_*.jsonl` files:

```go
// Current SessionMeta.FullPath points to context.jsonl
// But loading may need to read context_sub_1.jsonl, context_sub_2.jsonl, etc.
```

**Recommendation:** The `SessionReader` abstraction handles this transparently, but `SessionMeta` could indicate `IsChunked bool` or `ChunkCount int`.

#### G. Branching Metadata

Claude supports conversation branching (`parentUuid`, `isSidechain`), partially modeled:

```go
// Current:
Entry.ParentUUID  // Present
Entry.IsSidechain // Present

// Missing:
SessionMeta.IsSidechain  // Not present (it's in Claude's index)
// No way to query branches or navigate tree
```

**Recommendation:** Add branch-aware querying to `Store` interface or provide `GetBranches(sessionID) ([]Branch, error)`.

#### H. Tool Call Correlation

Both systems link tool uses to tool results via IDs, but there's no helper:

```go
// ContentBlock has ToolUseID for both tool_use and tool_result
// But no method to correlate them

// Useful addition:
func (s *Session) GetToolResult(toolUseID string) *ContentBlock
```

#### I. Request Tracking (Claude Only)

Claude tracks API requests:

```go
// Not modeled:
Entry.RequestID string  // Present in Claude entries
```

**Recommendation:** Add to `Entry.Metadata` or as explicit field if cross-source correlation is needed.

### 2.3 Structural Observations

#### Content vs Text Redundancy

```go
type Entry struct {
    ContentBlocks []ContentBlock  // Structured content
    Text          string          // "Simple text shortcut"
}
```

This creates ambiguity. Both may be set, or neither. Consider:
1. Make `Text` a computed property: `func (e *Entry) Text() string`
2. Or document the precedence clearly

#### Source Tracking Gap

```go
// SessionMeta has SourceType, but Entry does not
// When merging entries from multiple sources, provenance is lost

type Entry struct {
    // ...
    Source Source `json:"source,omitempty"` // Add this?
}
```

#### Timestamp Format Divergence

- Claude: ISO8601 string
- Kimi: Unix float

Both converters normalize to `time.Time`, which is correct. But wire protocol events use float timestamps that may need sub-second precision.

---

## Part 3: Taxonomy Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                         WORKSPACE                                │
│  (implicit - the user's machine)                                │
└─────────────────────────────────────────────────────────────────┘
                                │
                ┌───────────────┼───────────────┐
                ▼               ▼               ▼
┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐
│     PROJECT       │ │     PROJECT       │ │     PROJECT       │
│  /path/to/repo1   │ │  /path/to/repo2   │ │  ~/documents      │
│                   │ │                   │ │                   │
│  • SessionCount   │ │  • SessionCount   │ │  • SessionCount   │
│  • LastModified   │ │  • LastModified   │ │  • LastModified   │
│  • SourceType     │ │  • SourceType     │ │  • SourceType     │
└───────────────────┘ └───────────────────┘ └───────────────────┘
         │
         ├──────────────────┐
         ▼                  ▼
┌─────────────────┐ ┌─────────────────┐
│    SESSION      │ │    SESSION      │
│  uuid-1234...   │ │  uuid-5678...   │
│                 │ │                 │
│ • FirstPrompt   │ │ • FirstPrompt   │
│ • Summary       │ │ • Summary       │
│ • EntryCount    │ │ • EntryCount    │
│ • GitBranch     │ │ • GitBranch     │
│ • CreatedAt     │ │ • CreatedAt     │
│ • ModifiedAt    │ │ • ModifiedAt    │
└─────────────────┘ └─────────────────┘
         │
         ├──────┬──────┬──────┬──────┐
         ▼      ▼      ▼      ▼      ▼
┌──────┐┌──────┐┌──────┐┌──────┐┌──────┐
│ENTRY ││ENTRY ││ENTRY ││ENTRY ││ENTRY │
│user  ││asst  ││user  ││asst  ││tool  │
└──────┘└──────┘└──────┘└──────┘└──────┘
            │
            ├──────────┬──────────┐
            ▼          ▼          ▼
     ┌──────────┐┌──────────┐┌──────────┐
     │  BLOCK   ││  BLOCK   ││  BLOCK   │
     │ thinking ││  text    ││ tool_use │
     └──────────┘└──────────┘└──────────┘
```

---

## Part 4: Recommendations Summary

### High Priority

1. **Add `RoleCheckpoint`** - Both systems have checkpoint entries
2. **Add `Source` to `Entry`** - Enable provenance tracking after merge
3. **Document Text vs ContentBlocks precedence** - Or make Text computed

### Medium Priority

4. **Add `HistoryEntry` type** - Model command history
5. **Add wire event types** - Enable Kimi protocol analysis
6. **Add `IsSidechain` to `SessionMeta`** - Match Claude's index

### Low Priority (Future)

7. **File history abstraction** - Claude's undo/redo system
8. **Configuration abstraction** - Model/provider settings
9. **Branch navigation helpers** - Tree traversal for Claude branches

---

*Report generated: 2026-01-29*
*Based on: Claude Code 2.1.20, Kimi CLI 1.1*
