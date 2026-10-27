# Claude Code transcript schema (observed, October 2026)

Survey date: 2026-10-26. Sample: 368 `*.jsonl` files modified in the previous
14 days under `~/.claude/projects` (main transcripts and `subagents/agent-*.jsonl`),
written by Claude Code CLI 2.1.296 through 2.1.310, `entrypoint: "cli"` only.
Counts are line counts across the sample. Field names are as written on disk.

Anthropic's published position: the transcript entry format "is internal to
Claude Code and changes between versions" (https://code.claude.com/docs/en/sessions.md).
Only the items marked **documented** below have a published description; the
rest is observed behavior and must be parsed defensively. Companion report:
[CLAUDE_STRUCTURE.md](./CLAUDE_STRUCTURE.md) for the directory layout.

## Record types

| `type` | lines | role in a conversation | documented |
|---|---|---|---|
| `attachment` | 37,176 | context injected into the model turn (see below) | changelog only |
| `assistant` | 32,522 | model turn | SDK `SDKAssistantMessage` |
| `user` | 19,046 | human prompt or tool results | SDK `SDKUserMessage` |
| `api-request-blob` | 17,653 | request message dump keyed by hash | changelog 2.1.300 |
| `api-request` | 12,438 | request params (model, betas, thinking, …) | changelog 2.1.300 |
| `last-prompt` | 7,478 | `{lastPrompt, leafUuid}` session state | no |
| `atis-latch` | 7,413 | `{atis: ""}` session state | no |
| `mode` | 7,411 | `{mode: "normal"}` session state | no |
| `ai-title` | 7,276 | `{aiTitle}` generated session title | no |
| `permission-mode` | 5,422 | `{permissionMode: "auto"\|…}` | no |
| `system` | 2,062 | harness event; see subtypes | SDK, partially |
| `queue-operation` | 1,816 | prompt queue enqueue/dequeue/remove/popAll; only enqueue carries `content`, the rest are timestamps and are classified bookkeeping | changelog 2.1.295 |
| `pr-link` | 1,803 | `{prNumber, prUrl, prRepository, timestamp}` | no |
| `file-history-snapshot` | 1,125 | checkpoint marker `{messageId, snapshot{trackedFileBackups}, isSnapshotUpdate}` | checkpointing docs |
| `relocated` | 958 | `{relocatedCwd}` session moved (worktree) | changelog |
| `worktree-state` | 957 | `{worktreeSession{originalCwd, worktreePath, worktreeBranch, originalBranch, originalHeadCommit}}` | no |
| `file-history-delta` | 378 | `{messageId, snapshotMessageId, trackingPath, backup{version, backupTime}}` | no |
| `api-request-shape` | 378 | system prompt + tool schema shape keyed by `shapeHash` | changelog 2.1.300 |
| `cost-state` | 152 | `{totalCostUSD, totalDuration, totalAPIDuration, totalLinesAdded/Removed, modelUsage}` | no |
| `summary` | **0** | superseded by `ai-title`; `sessions-index.json` no longer written | n/a |

Only `user`, `assistant`, `system`, and `attachment` carry the conversation
envelope (`uuid`, `parentUuid`, `timestamp`, `sessionId`, `version`, `cwd`,
`gitBranch`, `isSidechain`, `userType`, `entrypoint`, optional `agentId`,
`slug`). The session-state records carry only `sessionId` and their payload.
`api-request*` records can be hundreds of kilobytes; decide from `type` before
parsing the line.

## `system` subtypes

| `subtype` | lines | fields | notes |
|---|---|---|---|
| `turn_duration` | 1,486 | `durationMs`, `messageCount`, `pendingBackgroundAgentCount` | one per completed turn |
| `away_summary` | 472 | `content` | recap text written while the user was away |
| `compact_boundary` | 42 | `content: "Conversation compacted"`, `level: info`, `logicalParentUuid`, `compactMetadata` | **documented** at SDK level as `{trigger, pre_tokens}` |
| `informational` | 40 | `content`, `level: notice` | e.g. "Compacted while idle, before the prompt cache expired" |
| `local_command` | 22 | `content` with `<command-name>`, `<command-message>`, `<command-args>`, `commandRun` | slash commands run locally |

`compactMetadata` observed shape: `{trigger: "auto"|"manual", preTokens,
durationMs, preCompactDiscoveredTools[], preservedSegment{headUuid, anchorUuid,
tailUuid}, preservedMessages{anchorUuid, uuids[], allUuids[]}}`. A
`compact_boundary` has `parentUuid: null`; `logicalParentUuid` points at the
last pre-compaction entry. The next `user` entry is the summary, flagged
`isCompactSummary: true` and `isVisibleInTranscriptOnly: true`.

## `attachment` records

Envelope plus `attachment{type, …}`. When the attachment was injected into the
model context, `rendered: [{content}]` holds the exact text and `renderedRole`
is `system` or `user`; `renderedInHumanTurn` marks user-turn injection.

| `attachment.type` | lines | rendered | payload |
|---|---|---|---|
| `total_tokens_reminder` | 14,888 | system | `text` |
| `deferred_tools_record` | 7,282 | no | `entries[]`, `toolInputCopies[]` |
| `batching_reminder_sent` | 4,066 | no | `text`, `model`, `clearAt` |
| `bash_output_audience_note` | 3,457 | system | `toolUseID` |
| `prompt_snapshot` | 810 | no | `systemPrompt[]`, `tools`, `systemTurns`, `contextRendering`, … |
| `environment` | 808 | system | `snapshot{workingDirectory, isWorktree, isGitRepo, platform, shell, osVersion, scratchpadDirectory}`, `changes` |
| `queued_command` | 450 | system | `prompt`, `origin`, `commandMode`, `delivery_id`, `runId`, `source_uuid`, `timestamp` |
| `date` | 424 | system | `date` |
| `model` | 412 | system | `identity{modelId, marketingName, knowledgeCutoff}`, `text` |
| `deferred_tools_delta` | 410 | system | `addedNames[]`, `removedNames[]`, `surfacedNames[]`, `addedLines[]` |
| `mcp_instructions_delta` | 409 | system | `addedNames[]`, `addedBlocks[]`, `removedNames[]` |
| `agent_listing_delta` | 408 | system | `addedTypes[]`, `addedLines[]`, `builtInTypes[]`, `isInitial` |
| `session_context` | 407 | user | `context{userEmail, gitStatus, …}` |
| `credential_org` | 407 | no | `organizationUuid` |
| `skill_listing` | 401 | system | `content`, `names[]`, `skillCount`, `isInitial`, `digests` |
| `auto_mode` | 401 | system | `bashFirst`, `bashFirstSteer`, `steerOnly`, `bypass`, `autoModeConsentFlow` |
| `silent_turn_reminder` | 383 | system | `text` |
| `instructions` | 341 | user | `files[]{path, type: Project\|User\|…, content}` (CLAUDE.md/AGENTS.md) |
| `edited_text_file` | 283 | system | `filename`, `snippet` |
| `diagnostics` | 200 | system | `files[]{uri, diagnostics[]{message, severity, range, source, code}}`, `isNew` |
| `file` | 139 | system | `filename`, `content{type, file{filePath, content}}` (@-mentioned files, memory) |
| `command_permissions` | 128 | no | `allowedTools[]` |
| `hook_additional_context` | 127 | system | `content[]`, `hookEvent`, `hookName`, `toolUseID` |
| `hook_success` | 121 | no | `command`, `stdout`, `stderr`, `exitCode`, `durationMs`, `hookEvent`, `hookName`, `toolUseID` |
| `compact_file_reference` | 49 | system | `filename`, `displayPath` |
| `invoked_skills` | 40 | system | `skills[]{name, path, content}` |
| `thinking_drop` | 18 | no | `requestId`, `model`, `newlyDropped{blockCount, reason}`, `blockHashes[]` |
| `task_status` | 4 | system | `taskId`, `taskType`, `description`, `status`, `outputFilePath`, `canContinueAgent` |
| `read_truncation_notice` | 3 | system | `banner` |

Changelog evidence that attachments are first-class transcript entries and
part of the `parentUuid` chain: 2.1.83, 2.1.97, 2.1.217, 2.1.259, 2.1.275,
2.1.285 (https://code.claude.com/docs/en/changelog).

## `assistant` records

Envelope plus `message` (a Messages API `Message`) and per-record metadata:

| field | lines | values |
|---|---|---|
| `requestId` | 32,510 | `req_…` |
| `effort` / `perTurnEffort` | 32,508 | `medium`, `high` (recorded since 2.1.212) |
| `requestedModel` / `advisorModel` | 32,319 | model id; `message.model` can be `<synthetic>` on errors |
| `apiBlockIndex` | 32,508 | index of this block within the API response |
| `serverClassifierRequest` | 32,508 | opaque |
| `requestRef` | 24,190 | UUID linking to `api-request.id` |
| `wireToolInputs` | 16,453 | raw tool input copies |
| `thinkingDurationMs` / `thinkingDisplay` | 11,356 | `thinkingDisplay: "updates"` |
| `agentId` | 8,903 | present on subagent records |
| `attributionAgent` / `attributionSkill` / `attributionPlugin` / `attributionMcpServer` / `attributionMcpTool` | 8,375 / 4,753 / 3,842 / 208 / 208 | which agent type, skill, plugin, MCP server produced the turn |
| `isApiErrorMessage`, `error`, `apiError`, `apiErrorStatus`, `errorDetails` | 17 | `error: "server_error"`; text block holds "API Error: …" |
| `truncatedAfterOutput`, `streamCutCause` | 6 / 1 | stream truncation |

`message` keys always present: `id`, `type`, `role`, `model`, `content`,
`stop_reason`, `stop_sequence`, `stop_details`, `usage`, `container`,
`context_management`, `diagnostics`, `input_transformations`.

- `stop_reason`: `tool_use` 26,279; null 4,033 (streamed block records);
  `end_turn` 2,214; `stop_sequence` 17; `refusal` 2.
- `content[].type`: `tool_use` 16,459 (with `caller: {type: "direct"}`);
  `thinking` 11,358 (`thinking`, `signature`); `text` 4,716. One block per
  record; a multi-block API response is split across records sharing
  `message.id` with increasing `apiBlockIndex`.
- `usage`: `input_tokens`, `output_tokens`, `cache_read_input_tokens`,
  `cache_creation_input_tokens`, `cache_creation{ephemeral_5m_input_tokens,
  ephemeral_1h_input_tokens}`, `service_tier`, `speed` (`standard`),
  `inference_geo`, `server_tool_use{web_search_requests, web_fetch_requests}`,
  `output_tokens_details{thinking_tokens}`, `iterations[]`, `fallback_credit`.
- `diagnostics.cache_miss_reason.type`: `previous_message_not_found`, `unavailable`.
- `input_transformations[]`: `{type: "thinking_dropped", path, reason: "prefix_binding_mismatch"}`.

Messages API block types that can appear but were not seen in this sample
(https://platform.claude.com/docs/en/api/messages.md): `redacted_thinking`
(`data`), `server_tool_use`, `web_search_tool_result`, `web_fetch_tool_result`,
`code_execution_tool_result`, `tool_search_tool_result`, `tool_reference`
(`tool_name`), `container_upload` (`file_id`), `search_result`, and the beta
`compaction` block (`content`, `signature`, with `stop_reason: "compaction"`).

## `user` records

Envelope plus `message{role, content}` and per-record metadata:

| field | lines | values |
|---|---|---|
| `promptId` | 19,021 | UUID shared by a prompt and its tool-result turns |
| `sourceToolAssistantUUID` | 16,454 | assistant record that issued the tool call |
| `permissionDecision` | 16,344 | `{decision: accept\|reject, source: config\|user_temporary, reasonType: classifier\|mode\|other\|safetyCheck, reasonCode?, grantScope?, classifierCategory?, circuitBreaker?}` |
| `toolUseResult` | 16,222 | typed result; see below |
| `serverClassifierContext` | 14,844 | opaque |
| `agentId` | 6,046 | subagent records |
| `sandboxed` | 4,739 | bool |
| `turnPosition`, `turnOrigin`, `origin`, `promptSource` | ~1,500 | `turnOrigin: human\|task_notification`; `origin.kind: human\|task-notification\|coordinator` (+ `producer`, `runId`); `promptSource: typed\|queued\|system` |
| `permissionMode` | 1,448 | mode at prompt time |
| `isMeta` | 910 | harness-authored user turn (system reminders, `<agent-message>` relays) |
| `queueSkipAttachments`, `queueSkipSlashCommands`, `queueTranscriptOnly` | 440 / 302 / 11 | queued prompt flags |
| `toolEndsTurn` | 312 | bool |
| `turnCompanion` | 144 | opaque |
| `sourceToolUseID` | 138 | tool_use id for synthetic turns |
| `isCompactSummary`, `isVisibleInTranscriptOnly` | 42 | the post-compaction summary |
| `toolDenialKind` | 26 | `permission-rule`, `automode-blocked`, `automode-unavailable`, `user-rejected` |
| `interruptedMessageId`, `refusedMessageId` | 6 / 1 | message ids |
| `imagePasteIds`, `mcpMeta`, `summarizedTurnPromptUuids` | ≤2 | rare |

`origin.kind` matches the SDK's documented `SDKMessageOrigin` (`human`,
`channel`, `peer`, `task-notification`, `coordinator`, `auto-continuation`,
`unclassified`).

`message.content` shapes: a single `tool_result` block 16,466; a plain string
2,468; `[text]` 126; `[text, image]` 2. `tool_result` blocks are
`{type, tool_use_id, content, is_error?}` where `content` is a string 15,304
times, `[text]` 743, `[image]` 350, `[tool_reference]` 57, `[image, text]` 15.
Image sources are `{type: "base64", media_type, data}` up to ~160 KB.

Human prompt text frequently begins with a harness wrapper. Opening tags seen
in `text` content: `<system-reminder>` 355, `<agent-message>` 308,
`<task-notification>` 134 (with `<task-id>`, `<tool-use-id>`, `<output-file>`,
`<status>`, `<summary>`, `<usage>`, `<result>`), `<bash-input>`/`<bash-stdout>`
82, `<pasted_content>` 71, `<command-name>` 33, `<local-command-caveat>` 31,
`<local-command-stdout>` 17.

### `toolUseResult` shapes

Top-level, typed, not sent to the model. Grouped by key set; count is records:

| keys | count | tool |
|---|---|---|
| `stdout, stderr, interrupted, isImage, noOutputExpected` (+ `bashEditDiff`, `returnCodeInterpretation`, `backgroundTaskId`, `backgroundCwdHint`, `persistedOutputPath`, `persistedOutputSize`, `timedOutAfterMs`, `gitOperation`, `dangerouslyDisableSandbox`, `staleReadFileStateHint`) | 13,100+ | Bash |
| `type, file{filePath, content, numLines, startLine, totalLines}` | 810 | Read |
| `type, filePath, content, originalFile, structuredPatch[], userModified` (+ `memdirStamped`) | 403 | Write |
| `filePath, oldString, newString, originalFile, replaceAll, structuredPatch[], userModified` (+ `contentNotInModelContext`, `staleRecovered`) | 282 | Edit |
| string | 381 | various |
| `agentId, status, isAsync, description, outputFile, prompt, resolvedModel, canContinueAgent, canReadOutputFile` | 278 | Agent (`status: async_launched`) |
| `agentId, agentType, status, content, handback, handbackReport, prompt, resolvedModel, usage, totalDurationMs, totalTokens, totalToolUseCount, toolStats, harness*` | 2 | Agent (completed inline) |
| `message, success` (+ `pin`, `resumedAgentId`, `revivedAgentId`, `worktreeBranch`, `worktreePath`, `display`, `msg_id`) | 360 | SendMessage, EnterWorktree, Skill, … |
| `commandName, success` | 125 | Skill |
| array | 122 | MCP tools |
| `bytes, code, codeText, durationMs, result, url` | 84 | WebFetch |
| `matches, query, total_deferred_tools` | 57 | ToolSearch |
| `durationSeconds, query, results, searchCount` | 36 | WebSearch |
| `questions, answers, annotations` | 13 | AskUserQuestion |
| `command, message, task_id, task_type` | 13 | TaskStop |
| `persistent, taskId, timeoutMs` | 11 | Monitor |

`structuredPatch[]` hunks are `{oldStart, oldLines, newStart, newLines,
lines[]}` with `lines` carrying ` `, `+`, `-` prefixes (jsdiff format).
`persistedOutputPath` points into `<session>/tool-results/` (documented:
"large tool outputs spilled to separate files, and full-size copies of images
that MCP tools return").

## Subagents

**Documented** (https://code.claude.com/docs/en/sub-agents.md): transcripts
are `~/.claude/projects/<project>/<sessionId>/subagents/agent-<agentId>.jsonl`;
the Agent tool result carries `agentId`; main-conversation compaction does not
touch them. Observed and undocumented:

- `agent-<agentId>.meta.json`: `{agentType, description, toolUseId,
  spawnDepth, requestShape: "background"|…, requestNonInteractive}`.
- Every record in an agent file has `agentId` and (for the first prompt and
  most turns) `isSidechain: true`; the first record has `parentUuid: null`.
  Main transcripts no longer contain sidechain entries.
- `toolUseResult.agentId` on the `Agent` tool result names the file.
  `task_status` attachments and `<task-notification>` user prompts report
  completion, quoting the Agent call's `tool-use-id`.
- `SubagentHandback` is the tool a subagent calls to return its report; the
  parent receives it as an `<agent-message>` user turn with `isMeta: true`.

## Session sidecars (outside the transcript)

| path | documented | content |
|---|---|---|
| `~/.claude/sessions/<pid>.json` (+ `.key`) | yes, as "one small file per running session" | `{pid, sessionId, cwd, startedAt, version, kind, entrypoint, name, nameSource, status, messagingSocketPath, peerFeatures[]}` |
| `~/.claude/session-env/<sessionId>/` | yes, "per-session environment metadata" | usually empty |
| `~/.claude/file-history/<sessionId>/<hash>@v<n>` | yes | pre-edit file snapshots for checkpoint restore |
| `<project>/<sessionId>/tool-results/<id>.txt` | yes | spilled tool output |
| `<project>/<sessionId>/subagents/` | yes | subagent transcripts |
| `<project>/memory/*.md` | yes | auto memory notes |
| `~/.claude/history.jsonl` | yes | `{display, pastedContents, timestamp, project, sessionId}` |
| `<project>/sessions-index.json` | no longer present | was the title index; superseded by `ai-title` records |

## Parser implications

- Decide the record class from `type` before unmarshalling the whole line.
- Treat unknown `type` and unknown `attachment.type` values as expected, not
  as errors; count them and keep them inspectable.
- Session title comes from the last `ai-title`; last prompt from `last-prompt`.
- A conversation turn is `user`, `assistant`, `system`, a reader-relevant
  `attachment`, or a checkpoint. Session-state and `api-request*` records are
  not turns.
- Use `toolUseResult` for diffs, spilled output paths, and subagent ids; use
  `tool_result.content[]` for nested text and images.
