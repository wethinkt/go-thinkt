# Conversation architecture: catching up with current agent harnesses

Date: 2026-10-26. Status: proposal for review. Branch: `worktree-conversation-architecture-review`.

## Intent

`internal/conversation` turns `thinkt.Entry` into a presentation `Document` that
the HTML export, the opt-in nthtml viewer, and the in-flight navigation `Index`
all consume. That pipeline is only as good as the entries it is fed. The Claude
Code source parser was last substantively changed in March 2026
(`internal/sources/claude/entry.go`, `store.go`), and Claude Code has moved from
7 record types to more than 19 since then. The result today is that most of a
modern session is either dropped, mislabeled as empty "system" turns, or
flattened into JSON blobs.

This document records what the live data looks like now, what the current
pipeline does with it, and a layered plan to bolster the conversation model so
the typesetting/rendering rework has richer, correctly classified semantics to
lay out. It deliberately keeps rendering (nthtml, nttypeset, CSS) out of scope.

## Evidence

Survey of 368 Claude Code JSONL files modified in the last 14 days under
`~/.claude/projects` (CLI versions 2.1.296 to 2.1.310). Full field tallies are in
[etc/reports/CLAUDE_TRANSCRIPT_SCHEMA.md](../../etc/reports/CLAUDE_TRANSCRIPT_SCHEMA.md).

Record types by line count, and what the parser does with each today:

| `type` | lines | parser today |
|---|---|---|
| `attachment` | 37,176 | unknown → `RoleSystem`, empty text |
| `assistant` | 32,522 | handled; new metadata ignored |
| `user` | 19,046 | handled; new metadata ignored |
| `api-request-blob` | 17,653 | unknown → `RoleSystem`, empty |
| `api-request` | 12,438 | unknown → `RoleSystem`, empty |
| `last-prompt` | 7,478 | unknown → `RoleSystem`, empty |
| `atis-latch`, `mode`, `ai-title`, `permission-mode` | ~27,500 | unknown → `RoleSystem`, empty |
| `system` | 2,062 | role mapped, **`content` dropped** |
| `queue-operation` | 1,816 | typed but content unused |
| `pr-link`, `relocated`, `worktree-state`, `cost-state` | ~3,900 | unknown → `RoleSystem`, empty |
| `file-history-snapshot` | 1,125 | handled (checkpoint) |
| `file-history-delta`, `api-request-shape` | 756 | unknown → `RoleSystem`, empty |
| `summary` | **0** | handled, but no longer written |

Measured on one real 2,154-line session: `thinkt export
--system -f json` emits 2,138 entries, of which 1,555 (73%) are `system` entries
with no blocks and no text. The 63 genuine system events in that file (turn
durations, away summaries, compaction boundaries, local commands) are
indistinguishable from the noise and carry no text.

Other concrete gaps found:

- **Session titles.** `summary` records are gone; `ai-title` now carries the
  title and `last-prompt` carries the last prompt and leaf UUID. The store still
  looks for `sessions-index.json`, which no longer exists in project directories.
  `SessionMeta.Summary` is empty for every recent session.
- **Tool results.** `tool_result.content` is an array in 1,165 of 16,467 cases
  (text+image, `tool_reference`). The converter stores the raw JSON string, so
  the document shows a JSON blob and the 350 screenshots inside results are
  invisible. The top-level `toolUseResult` (typed: `stdout/stderr`,
  `structuredPatch`, `file`, `agentId`, `interrupted`) is never read. It is the
  only place a real diff exists for Edit/Write calls.
- **Subagents.** Subagent transcripts live in
  `<session>/subagents/agent-<id>.jsonl` with an `agent-<id>.meta.json`
  (`agentType`, `description`, `toolUseId`, `spawnDepth`). Main transcripts no
  longer contain sidechain entries. Only `teams.go` reads these files; the
  conversation viewer cannot follow an `Agent` call to its transcript even
  though `toolUseResult.agentId` names it.
- **Compaction.** `system/compact_boundary` carries `compactMetadata` (trigger,
  `preTokens`, `preservedSegment{headUuid,anchorUuid,tailUuid}`) and
  `logicalParentUuid`; the follow-up `user` entry is flagged
  `isCompactSummary`/`isVisibleInTranscriptOnly`. None of this reaches the
  document, so a compaction reads as a long user prompt.
- **Assistant metadata.** New per-record fields: `effort`, `requestedModel` vs
  `message.model`, `thinkingDurationMs`, `attributionAgent/Skill/Plugin/McpServer`,
  `isApiErrorMessage`/`error`/`apiError`, `truncatedAfterOutput`,
  `streamCutCause`; `message.stop_reason` includes `refusal`;
  `usage.output_tokens_details.thinking_tokens`, `usage.iterations`,
  `service_tier`, `speed`; `message.input_transformations` records dropped
  thinking. Only four usage counters are mapped.
- **User metadata.** `promptId`, `permissionDecision` (accept/reject with
  `reasonType` classifier/mode/safetyCheck and `circuitBreaker`),
  `toolDenialKind`, `turnOrigin`/`origin` (human, task_notification,
  coordinator), `promptSource` (typed, queued, system), `isMeta`,
  `interruptedMessageId`, `refusedMessageId`. User text is routinely a wrapper:
  `<system-reminder>`, `<task-notification>`, `<agent-message>`,
  `<pasted_content>`, `<local-command-stdout>`. The document labels all of it
  "User".
- **Attachments.** Of the 29 `attachment.type` values, a handful are meaningful
  to a reader (`hook_success`, `hook_additional_context`, `queued_command`,
  `task_status`, `instructions`, `file`, `invoked_skills`, `skill_listing`,
  `diagnostics`, `edited_text_file`, `compact_file_reference`,
  `read_truncation_notice`); most are harness bookkeeping
  (`total_tokens_reminder` alone is 14,888 lines; `deferred_tools_record`,
  `batching_reminder_sent`, `prompt_snapshot`, `credential_org`). Each carries
  `rendered[].content` and `renderedRole`, the exact text injected into context.
- **Request traces.** `api-request`, `api-request-blob`, `api-request-shape`
  are full prompt dumps (system prompt, tool schemas, message blobs keyed by
  hash). They are large, duplicative, and privacy-heavy. They should never be
  conversation entries.
- **Branching.** `parentUuid` is carried on `thinkt.Entry` but the document is
  linear; `compact_boundary` resets `parentUuid` to null and uses
  `logicalParentUuid`.
- **Docs.** `etc/reports/CLAUDE_STRUCTURE.md` describes `debug/`, `todos/`,
  `statsig/`, `sessions-index.json`, none of which exist now, and omits
  `subagents/`, `tool-results/`, `session-env/`, `sessions/`, `memory/`,
  `file-history-delta`, and the `attachment` record.

### What is published

Anthropic documents none of the on-disk record types; the sessions page says
the entry format "is internal to Claude Code and changes between versions"
and points scripts at `/export` or the SDK instead. What is documented, and
therefore safe to lean on, is narrower:

- Subagent transcripts at `<session>/subagents/agent-<agentId>.jsonl`, and
  `agentId` in the Agent tool's result (sub-agents and Agent SDK pages).
- The `compact_boundary` system message with `compact_metadata{trigger,
  pre_tokens}` (Agent SDK `SDKCompactBoundaryMessage`). `preservedSegment`,
  `logicalParentUuid`, and `isCompactSummary` are observed only.
- The SDK's `SDKMessageOrigin` kinds (`human`, `channel`, `peer`,
  `task-notification`, `coordinator`, `auto-continuation`, `unclassified`),
  which match the observed `origin.kind` on user records.
- `<session>/tool-results/` as the spill location for large tool outputs and
  full-size MCP images; `file-history/` for checkpoint snapshots;
  `projects/<project>/memory/` for auto memory (claude-directory page).
- Changelog: effort recorded on assistant records since 2.1.212; API request
  params, system prompt, and tools recorded since 2.1.300 (Oct 14, 2026);
  attachments are transcript entries on the `parentUuid` chain (2.1.83 and
  later fixes).

So the parser must be tolerant by design: unknown record and attachment types
are the normal case over time, not a bug.

The in-flight uncommitted work on `main` (`conversation/index.go`,
`tui/conversation_navigation.go`, `EntryAnchor`, `View.ExternalAnchors`) builds
an immutable session index on top of `conversation.Build`. Every semantic
improvement below lands in or below `Build`, so the index and the viewer inherit
it without changes to their navigation logic.

## Goals

1. A modern Claude Code session converts into entries that are all real
   conversation events, each correctly classified, with its text intact.
2. Session titles, last prompt, PR links, cost, and worktree state reach
   `SessionMeta` instead of being lost or emitted as empty turns.
3. Tool results carry their nested text and media, and Edit/Write calls carry a
   diff block.
4. An `Agent` call links to its subagent transcript, and subagent transcripts
   are reachable as child sessions of their parent.
5. Compaction, permission denials, API errors, refusals, and harness-injected
   context are visible as what they are, not disguised as user or system prose.
6. Everything above is expressed in the source-neutral `thinkt.Entry` and
   `conversation.Document` types, so Kimi, Codex, Gemini, Copilot, and Qwen can
   adopt the same kinds over time.

## Non-goals

- Changing nthtml, nttypeset, the CSS, or the typesetting flow. The document
  gains kinds and metadata; how they are laid out is the rendering rework's job.
- Ingesting `api-request*` records as conversation content.
- Rewriting the default TUI renderer (`multi_viewer.go`, `renderer_generic.go`).
  It should keep working with the new kinds via the existing "Other" filter.
- Teams. `teams.go` keeps its own subagent scan; this work only makes subagent
  transcripts addressable as sessions.

## Anti-goals

- Surfacing full system prompts, tool schemas, or `credential_org` UUIDs in any
  default view or export. Those stay out of entries entirely.
- Inflating `EntryCount`, search indexes, or MCP `get_session_entries` with
  bookkeeping records.
- Breaking anchor stability. Anchors hash `(sessionID, source, uuid,
  entryIndex)`. Dropping records at parse time changes `entryIndex` relative to
  the raw line number, so the index must be the index among emitted entries,
  and metadata caches must be invalidated when the parser's classification
  changes.

## Constraints

- `thinkt.Entry` is serialized over the REST API and MCP, and stored by the
  indexer. New fields must be additive, `omitempty`, and documented in
  `internal/server/docs`.
- Metadata caches (`internal/thinkt/metadata_cache.go`, `StoreCache`) key on
  file state. A parser schema version must be part of the cache key so old
  counts and empty summaries are recomputed.
- The relay/collector normalizer (`internal/collect`) validates roles. New
  kinds must not be new roles.
- Lines can be very large (`api-request-blob`); the classifier must decide from
  the `type` field before parsing the rest of the line.

## Design

### Layer A: Claude record classification (`internal/sources/claude`)

Add a `RecordClass` decided from `type` (and `subtype`/`attachment.type`) before
any `message` parsing:

| class | types | fate |
|---|---|---|
| Turn | `user`, `assistant` | entry |
| Event | `system` (all subtypes), `queue-operation` with `content` | entry with text from `content`, subtype kept; content-less queue records (dequeue, remove, popAll) are bookkeeping |
| Context | `attachment` with a reader-meaningful `attachment.type` | entry, role `system`, kind `attachment`, text from `rendered[].content` or the typed payload |
| Bookkeeping | `attachment` of a noisy type, `atis-latch`, `api-request*`, `permission-mode`, `mode` | not an entry; `mode`/`permission-mode` fold into `SessionMeta` |
| Session state | `ai-title`, `last-prompt`, `pr-link`, `relocated`, `worktree-state`, `cost-state` | not an entry; folded into `SessionMeta` |
| Checkpoint | `file-history-snapshot`, `file-history-delta` | entry (existing checkpoint role); delta becomes a short text |

The attachment allow-list is a package-level table so it can be extended
without touching the converter. Unknown `type` values become a Context-class
entry with kind `unknown` and the raw JSON as text, so a future format change
is visible rather than silent, but they are counted and logged once per file.

`ToThinktEntry` / store conversion gains:

- `system` → `Text = content`, `Subtype = subtype`, plus `durationMs`,
  `compactMetadata`, `logicalParentUuid` in typed metadata.
- `user` with `isCompactSummary` → `Kind = "compact_summary"`.
- `user` with `isMeta`, or whose only text is a `<system-reminder>`,
  `<task-notification>`, `<agent-message>`, `<local-command-stdout>` wrapper →
  `Kind` set accordingly (`meta`, `task_notification`, `agent_message`,
  `command_output`). Detection is prefix-based on trimmed text and tolerant of
  multiple wrappers; the raw text is kept untouched.
- `user` → `PromptID`, `PermissionDecision`, `ToolDenialKind`, `TurnOrigin`,
  `PromptSource`, `Sandboxed`, `InterruptedMessageID`.
- `assistant` → `Effort`, `RequestedModel`, `StopReason`, `Error`,
  `ThinkingDurationMs`, `Attribution{Agent,Skill,Plugin,MCPServer,MCPTool}`,
  `TruncatedAfterOutput`, and the extended usage counters.
- `tool_result` with array content → `ContentBlock.Children` (text and image
  blocks), `ToolResult` set to the joined text for compatibility.
- top-level `toolUseResult` → attached to the matching `tool_result` block:
  `structuredPatch` becomes a child block of type `diff` (unified text built
  from the hunks); `stdout`/`stderr` become children when the inline content is
  empty or truncated; `agentId` becomes `ChildSessionID` on the block;
  `interrupted`, `isImage`, `persistedOutputPath` go to block metadata. A
  `persistedOutputPath` under `<session>/tool-results/` is recorded, not read;
  loading spilled output is the job of the same explicit asset resolver that
  will eventually decode media.
- `Agent` `tool_use` blocks get `ChildSessionID` from the paired result so the
  link exists on both ends.

Session state folding: the store already scans each file for metadata; it
reads `ai-title` → `SessionMeta.Title`, `last-prompt` → `LastPrompt`,
`pr-link` → `PRLinks []PRLink`, `worktree-state`/`relocated` → `Worktree`,
`cost-state` → `Cost`. `Summary` is populated from `Title` for compatibility.

Subagent sessions: `ListSessions` gains an option to include subagent
transcripts. Each `subagents/agent-<id>.jsonl` becomes a `SessionMeta` with
`ParentSessionID`, `AgentID`, `AgentType`, `Description`, and `SpawnDepth` from
the `.meta.json`. `GetSessionMeta`/`OpenSession` resolve `agent-<id>` IDs. The
default listing excludes them so pickers are unchanged; the viewer asks for
them explicitly when following a link.

### Layer B: source-neutral core types (`internal/thinkt`)

Additive fields on `Entry`:

```go
Kind      EntryKind // sub-classification within a role: "", KindCompactSummary, KindMeta,
                    // KindTaskNotification, KindAgentMessage, KindCommandOutput, KindAttachment,
                    // KindTurnDuration, KindAwaySummary, KindCompactBoundary, KindLocalCommand, ...
Visibility Visibility // Normal (default), Secondary (hidden by default), Debug
TurnID    string      // groups records that belong to one API response (Claude message.id)
PromptID  string
ParentToolUseID string
ChildSessionID  string   // subagent transcript spawned by this entry
Effort, RequestedModel, StopReason, Error string
Attribution *Attribution
Permission  *PermissionDecision
Compaction  *CompactionInfo       // trigger, pre tokens, preserved head/anchor/tail UUIDs
```

`EntryKind` is a named string type with package constants, the same pattern
as `Role` and `Source`: sources can emit a kind the core does not name yet,
and the TUI still switches on constants. It is deliberately not a closed enum.
(My call; Evan was undecided between string and enum.)

`ContentBlock` gains `Children []ContentBlock` (for array tool results) and the
new block type `diff`. `TokenUsage` gains `ThinkingTokens`, `ServiceTier`,
`Speed`. `SessionMeta` gains `Title`, `LastPrompt`, `ParentSessionID`,
`AgentID`, `AgentType`, `Description`, `PRLinks`, `Cost`, `Worktree`, and
`SubagentCount`. `Project` gains `SubagentCount`; `SessionCount` continues to
count main transcripts only (Evan's decision).

`Visibility` replaces the ad-hoc "Other" bucket as the source-neutral hint:
sources set `Secondary` on bookkeeping-ish events (turn durations, attachments,
meta prompts) and the TUI/Web/export default filters hide `Secondary` while
`--system`/filter keys reveal it. `Role` is unchanged, so the collector and
indexer keep validating.

The other five sources need no changes to compile; they keep emitting
`Visibility: Normal` and empty kinds.

### Layer C: presentation document (`internal/conversation`)

- `Message` gains `Kind`, `Visibility`, `Effort`, `StopReason`, `Error`,
  `Attribution`, `ChildSessionID`, `Compaction`.
- `Block` gains `Children []Block` (built recursively, anchors
  `…-block-<j>-<k>`), kind `diff`, and `ChildSessionID`.
- `Related` gains cross-session targets: an `Agent` call relates to
  `EntryAnchor(childSessionID, firstEntry, 0)`; a compaction summary relates to
  the `logicalParentUuid` anchor; a `task_notification` user entry relates to
  the `Agent` call it reports on (matched by `tool-use-id` in its text or
  `ParentToolUseID`). Hosts already handle `ExternalAnchors` for links outside
  the loaded window; cross-session anchors are the same mechanism with a
  different `Origin.SessionID`, which `Location` already carries.
- `View` gains `HiddenVisibility map[Visibility]bool` with `Secondary` hidden
  by default in `WritePage` exports and in the viewer's initial filter set;
  `HiddenKinds` keeps working for block kinds.
- `WriteHTML`: a `diff` block renders as `<pre class="diff">` with per-line
  `<span class="add|del|hunk">` wrappers (CSS only); nested children render
  inside the tool result body; the metadata line adds attribution
  (`agent · skill`), effort, and `stop_reason` when it is `refusal` or an
  error; messages with `Error` get `is-error`. Compaction boundaries render as
  a horizontal rule-styled section with trigger and token counts.
- The `Index` (in-flight on `main`) needs only to index `Children` text and
  accept cross-session anchors in `Related`; `NextMessage` and `Search` are
  unchanged.

### Layer D: documentation

- Replace `etc/reports/CLAUDE_STRUCTURE.md` with a current layout (this survey)
  and keep `CLAUDE_TRANSCRIPT_SCHEMA.md` as the field reference.
- Document the new `Entry` fields and `Visibility` in `internal/server/docs`
  and `internal/conversation/README.md`.

## Rollout order

1. Layer A classification, system text, session-state folding, cache key bump.
   Removes the 73% noise and restores titles. Verify with the real-session
   export count and a new golden fixture built from redacted real records.
2. Tool result children and `toolUseResult` diffs.
3. Subagent sessions and cross-session links.
4. Layer B/C metadata (attribution, effort, errors, compaction, kinds) and the
   HTML/CSS for `diff` and compaction.
5. Docs refresh.

Each step is independently shippable and keeps `go test ./...` green. Step 1 is
the one to land before the rendering rework depends on entry indices.

## Calls made after the builder check

A fresh reviewer read this document against the worktree and asked where a
wrong guess would hurt. These are the resulting decisions; change them if you
disagree.

- **Sequencing against the in-flight index work.** `conversation/index.go`,
  `EntryAnchor`, and `View.ExternalAnchors` are uncommitted on `main`, not on
  this branch. Steps 1 and 2 are built against the current `Build`,
  `linkTools`, and `View` and touch `document.go` only additively (new fields,
  recursive block build). Steps 3 and 4 wait until the index work is committed
  and this branch is rebased on it; cross-session `Related` targets are then
  added to the index, not re-implemented here.
- **API and MCP return every emitted entry.** `GET /api/v1/sessions/{id}` and
  `get_session_entries` keep `offset`, `limit`, and `entry_indices` over the
  full emitted list, so `EntryCount` and anchors mean the same thing
  everywhere. `Visibility` is a field on each entry; clients (TUI, export,
  thinkt-web) filter. A `visibility` query parameter can come later without
  changing semantics.
- **Subagent session identity is qualified.** A subagent session's ID is
  `<parentSessionID>/agent-<agentID>`. The store resolves it in one path join;
  bare `agent-<id>` is not accepted because agent IDs are short hashes that are
  not unique across sessions. `ChildSessionID` and `Origin.SessionID` carry
  the qualified form.
- **Fixtures are synthetic.** No real transcript lines go into git. The golden
  fixture is authored from `CLAUDE_TRANSCRIPT_SCHEMA.md` and reproduces the
  shapes that matter: array tool results with an image, a `structuredPatch`,
  a `compact_boundary` plus summary, an `ai-title` rewrite, an `Agent` call
  with `toolUseResult.agentId`, an API error record, and a bookkeeping
  attachment that must be dropped. Real-session verification stays a manual
  step with the export count.
- **Index re-ingestion is lazy.** The SQLite `sessions` table gains a
  `parser_version` column (migration bump). The freshness check treats a
  version mismatch like a changed file, so sessions are re-ingested as they
  are next touched or listed rather than in one forced full scan. `Secondary`
  entries are excluded from FTS rows in the same change (Evan's decision).
  Attachments are still recorded for easy access: a small
  `entry_attachments` table (session id, entry index, attachment kind,
  timestamp, short label such as hook name or file path) with no text body,
  so hook output, loaded instructions, invoked skills, and task status can be
  listed and jumped to by anchor without searching them.
- **No merging of per-block assistant records in this pass.** Claude writes
  one record per content block (shared `message.id`, increasing
  `apiBlockIndex`). Merging would change every anchor and count. The document
  keeps one message per record and exposes `message.id` as `Entry.TurnID` so
  a renderer can group consecutive records into a turn without the parser
  deciding for it.

## Decisions left to the builder

- Exact attachment allow-list membership beyond the twelve named above.
- Whether `queue-operation` content (cross-session messages) is `Secondary` or
  `Normal`; default `Secondary`.
- Unified-diff text format for `structuredPatch` hunks (use the `oldStart`,
  `newStart`, `lines` fields as emitted; no context recomputation).
- How the TUI exposes subagent sessions in the session picker (proposal: not
  at all in this pass; only via links).

## Decisions from Evan (2026-10-26)

1. `Secondary` entries stay out of the full-text index, but attachments are
   noted in a lightweight table for easy access (see the re-ingestion call
   above).
2. Subagent transcripts are not counted in `SessionCount`; a separate
   `SubagentCount` is kept on `Project` and `SessionMeta`.
3. `Kind` type: undecided. Builder default is the named string type with
   constants described in Layer B.
4. One message per record, grouped by `TurnID`. No merging.
