# Conversation Model Step 1–2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make modern Claude Code transcripts convert into correctly classified, text-bearing `thinkt.Entry` values with session titles, tool-result children, and diffs, and make every consumer stop treating bookkeeping records as empty system turns.

**Architecture:** A record classifier in `internal/sources/claude` decides, from the `type` field alone, whether a JSONL line is a conversation turn, a harness event, reader-relevant context, session state, or bookkeeping. Turns and events become entries through one shared conversion function that now fills the new source-neutral `Kind`, `Visibility`, `TurnID`, and metadata fields on `thinkt.Entry`; session state folds into `SessionMeta` through the existing enrichment scan and metadata cache; bookkeeping never leaves the parser. Consumers (export filter, TUI role filter, conversation document, indexer, file search) honor `Visibility` and a parser version so stale caches recompute lazily.

**Tech Stack:** Go 1.26, `encoding/json`, SQLite index (`internal/index`), goldmark HTML in `internal/conversation`, swag for API docs.

**Spec:** `docs/plans/2026-10-26-conversation-architecture-design.md` (Layers A, B, the `diff`/children slice of C, and D; rollout steps 1 and 2). Field reference: `etc/reports/CLAUDE_TRANSCRIPT_SCHEMA.md`.

## Global Constraints

- All new `thinkt.Entry`, `ContentBlock`, `SessionMeta`, and `Project` fields are additive with `json:"...,omitempty"` tags. `Role` values are unchanged (the collector normalizer validates them).
- `Kind` is the named string type `thinkt.EntryKind` with package constants, not a closed enum. `Visibility` is the named string type `thinkt.Visibility` with `VisibilityNormal = ""`, `VisibilitySecondary = "secondary"`, `VisibilityDebug = "debug"`.
- `EntryIndex`, `EntryCount`, REST `offset`/`total`, and MCP `entry_indices` all mean "index among emitted entries". The API and MCP return every emitted entry; clients filter by `Visibility`.
- Bookkeeping classes (`api-request`, `api-request-blob`, `api-request-shape`, `atis-latch`, `mode`, `permission-mode`, and non-allow-listed `attachment` types) are never emitted as entries and never appear in `EntryCount`.
- Session-state records (`ai-title`, `last-prompt`, `pr-link`, `relocated`, `worktree-state`, `cost-state`) are never entries; they fold into `SessionMeta`. The last `ai-title` wins; `SessionMeta.Summary` mirrors `Title` when `Summary` is empty.
- Claude's per-block assistant records are not merged. `Entry.TurnID` carries `message.id`.
- No real transcript lines are committed. Fixtures are synthetic.
- `thinkt.ParserVersion` is an integer constant, starting at `2`; bump it whenever classification or conversion changes which entries are emitted. The metadata cache and the index both compare against it.
- The classifier reads only the `type` (and for attachments `attachment.type`) field before deciding; it never unmarshals `message` or `params` of a line it is going to drop.
- Run `go test ./...` from the worktree with the sibling `go.work` present (see memory note); the two `internal/cmd` apps tests that need iTerm are pre-existing failures.

## Review Focus

1. A `tool_result.content` array holding an unfamiliar block type (`tool_reference`, a future type): the result must still render its text and keep the unknown child as a `ContentBlock` of that type with its JSON in `Text`, never dropping the whole result. Pinned in Task 8.
2. Repeated `ai-title` records where a later one has an empty `aiTitle`: the last non-empty title wins; an empty one never blanks an earlier title. Pinned in Task 7.
3. A `system` record with an unknown `subtype` and no `content`: it is emitted as `VisibilitySecondary` with `Text` equal to the subtype, never as a `VisibilityNormal` entry with empty text. Pinned in Task 4.
4. A `user` record whose `message.content` is a tool result but whose `toolUseResult` is a plain string: the string is ignored, the result text comes from `content`, and no children are invented. Pinned in Task 8.
5. A file whose only lines are bookkeeping (a session that was opened and closed): `EntryCount` is 0, `OpenSession` returns `io.EOF` on first read, and the session still lists with its `Title` if an `ai-title` exists. Pinned in Task 7.

---

### Task 1: Source-neutral core types

**Files:**
- Modify: `internal/thinkt/types.go` (after `type Role string` block at line ~143; `ContentBlock` at ~146; `Entry` at ~170; `TokenUsage` at ~196; `SessionMeta` at ~204; `Project` at ~228)
- Test: `internal/thinkt/types_kind_test.go`

**Interfaces:**
- Produces:
  ```go
  type EntryKind string
  const (
      KindNone             EntryKind = ""
      KindCompactSummary   EntryKind = "compact_summary"
      KindMeta             EntryKind = "meta"
      KindTaskNotification EntryKind = "task_notification"
      KindAgentMessage     EntryKind = "agent_message"
      KindCommandOutput    EntryKind = "command_output"
      KindAttachment       EntryKind = "attachment"
      KindTurnDuration     EntryKind = "turn_duration"
      KindAwaySummary      EntryKind = "away_summary"
      KindCompactBoundary  EntryKind = "compact_boundary"
      KindInformational    EntryKind = "informational"
      KindLocalCommand     EntryKind = "local_command"
      KindQueueOperation   EntryKind = "queue_operation"
      KindAPIError         EntryKind = "api_error"
      KindUnknown          EntryKind = "unknown"
  )
  type Visibility string
  const (
      VisibilityNormal    Visibility = ""
      VisibilitySecondary Visibility = "secondary"
      VisibilityDebug     Visibility = "debug"
  )
  const ParserVersion = 2
  type Attribution struct { Agent, Skill, Plugin, MCPServer, MCPTool string } // json: agent, skill, plugin, mcp_server, mcp_tool
  type PermissionDecision struct { Decision, Source, ReasonType, ReasonCode, DenialKind string }
  type CompactionInfo struct { Trigger string; PreTokens int; DurationMs int; HeadUUID, AnchorUUID, TailUUID string; LogicalParentUUID string }
  type PRLink struct { Number int; URL, Repository string; Timestamp time.Time }
  type WorktreeInfo struct { Path, Branch, OriginalCwd, OriginalBranch string }
  type SessionCost struct { TotalUSD float64; DurationMs, APIDurationMs, ToolDurationMs int64; LinesAdded, LinesRemoved int }
  ```
  New `Entry` fields (all `omitempty`): `Kind EntryKind`, `Subtype string`, `Visibility Visibility`, `TurnID string`, `PromptID string`, `ParentToolUseID string`, `ChildSessionID string`, `Effort string`, `RequestedModel string`, `StopReason string`, `Error string`, `Attribution *Attribution`, `Permission *PermissionDecision`, `Compaction *CompactionInfo`.
  New `ContentBlock` fields: `Children []ContentBlock`, `ChildSessionID string`. New block type string constant `BlockTypeDiff = "diff"`.
  New `TokenUsage` fields: `ThinkingTokens int`, `ServiceTier string`, `Speed string`.
  New `SessionMeta` fields: `Title string`, `LastPrompt string`, `SubagentCount int`, `ParentSessionID string`, `PRLinks []PRLink`, `Worktree *WorktreeInfo`, `Cost *SessionCost`.
  New `Project` field: `SubagentCount int`.
  Method: `func (e Entry) IsSecondary() bool` returns `e.Visibility == VisibilitySecondary`.

- [ ] **Step 1: Write the failing test**

```go
func TestEntryNewFieldsRoundTrip(t *testing.T) {
    in := Entry{UUID: "u", Role: RoleSystem, Kind: KindTurnDuration, Subtype: "turn_duration",
        Visibility: VisibilitySecondary, TurnID: "msg_1",
        Attribution: &Attribution{Agent: "Explore"},
        Compaction:  &CompactionInfo{Trigger: "auto", PreTokens: 12},
        ContentBlocks: []ContentBlock{{Type: "tool_result", Children: []ContentBlock{{Type: BlockTypeDiff, Text: "@@"}}}}}
    data, _ := json.Marshal(in)
    var out Entry
    if err := json.Unmarshal(data, &out); err != nil { t.Fatal(err) }
    if out.Kind != KindTurnDuration || !out.IsSecondary() || out.TurnID != "msg_1" { t.Fatalf("round trip lost fields: %+v", out) }
    if out.ContentBlocks[0].Children[0].Type != BlockTypeDiff { t.Fatal("children lost") }
    if strings.Contains(string(data), `"visibility":""`) || strings.Contains(string(data), `"kind":""`) { t.Fatal("zero values must be omitted") }
}
func TestEntryZeroValueJSONUnchanged(t *testing.T) {
    data, _ := json.Marshal(Entry{UUID: "u", Role: RoleUser})
    for _, key := range []string{"kind", "visibility", "turn_id", "attribution", "permission", "compaction", "children"} {
        if strings.Contains(string(data), `"`+key+`"`) { t.Fatalf("%s leaked into zero-value JSON: %s", key, data) }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/thinkt -run 'TestEntryNewFieldsRoundTrip|TestEntryZeroValueJSONUnchanged' -v`
Expected: FAIL to compile (undefined `KindTurnDuration`, `Attribution`, …).

- [ ] **Step 3: Add the types, constants, fields, and `IsSecondary` to `internal/thinkt/types.go`**

JSON tags are snake_case matching the Go names (`turn_id`, `prompt_id`, `parent_tool_use_id`, `child_session_id`, `requested_model`, `stop_reason`, `thinking_tokens`, `service_tier`, `last_prompt`, `subagent_count`, `parent_session_id`, `pr_links`). Document each in a doc comment that names the Claude source field it comes from.

- [ ] **Step 4: Run tests to verify they pass, and that nothing else broke**

Run: `go test ./internal/thinkt ./internal/collect ./internal/server/... 2>&1 | tail -5`
Expected: `ok` for each package.

- [ ] **Step 5: Commit**

```bash
git add internal/thinkt/types.go internal/thinkt/types_kind_test.go
git commit -m "feat(thinkt): add entry kind, visibility, turn id, and session state fields"
```

---

### Task 2: Claude record classifier

**Files:**
- Create: `internal/sources/claude/classify.go`
- Modify: `internal/sources/claude/entry.go:11-21` (add the new `EntryType` constants)
- Test: `internal/sources/claude/classify_test.go`

**Interfaces:**
- Produces:
  ```go
  // entry.go additions
  const (
      EntryTypeAttachment      EntryType = "attachment"
      EntryTypeAPIRequest      EntryType = "api-request"
      EntryTypeAPIRequestBlob  EntryType = "api-request-blob"
      EntryTypeAPIRequestShape EntryType = "api-request-shape"
      EntryTypeLastPrompt      EntryType = "last-prompt"
      EntryTypeAITitle         EntryType = "ai-title"
      EntryTypeMode            EntryType = "mode"
      EntryTypePermissionMode  EntryType = "permission-mode"
      EntryTypeAtisLatch       EntryType = "atis-latch"
      EntryTypePRLink          EntryType = "pr-link"
      EntryTypeRelocated       EntryType = "relocated"
      EntryTypeWorktreeState   EntryType = "worktree-state"
      EntryTypeCostState       EntryType = "cost-state"
      EntryTypeFileHistoryDelta EntryType = "file-history-delta"
  )
  // classify.go
  type RecordClass int
  const (
      ClassTurn RecordClass = iota // user, assistant
      ClassEvent                   // system, queue-operation
      ClassContext                 // allow-listed attachment
      ClassCheckpoint              // file-history-snapshot, file-history-delta, summary
      ClassSessionState            // ai-title, last-prompt, pr-link, relocated, worktree-state, cost-state
      ClassBookkeeping             // everything dropped
      ClassUnknown                 // type never seen; emitted as context so drift is visible
  )
  // ClassifyType is the pure type table. Attachments need the attachment type.
  func ClassifyType(t EntryType, attachmentType string) RecordClass
  // ClassifyLine decodes only "type" and "attachment.type" from a raw line.
  func ClassifyLine(line []byte) (EntryType, string, RecordClass)
  // IsEntryClass reports whether records of this class become thinkt entries.
  func (c RecordClass) IsEntry() bool // Turn, Event, Context, Checkpoint, Unknown
  // ContextAttachmentTypes is the allow-list. Exported so tests and docs can read it.
  var ContextAttachmentTypes = map[string]bool{...}
  ```
  Allow-list contents: `hook_success`, `hook_additional_context`, `queued_command`, `task_status`, `instructions`, `file`, `invoked_skills`, `skill_listing`, `diagnostics`, `edited_text_file`, `compact_file_reference`, `read_truncation_notice`. Every other attachment type is `ClassBookkeeping`. An empty `type` is `ClassBookkeeping`.

- [ ] **Step 1: Write the failing tests**

```go
func TestClassifyType(t *testing.T) {
    cases := []struct{ typ EntryType; att string; want RecordClass }{
        {EntryTypeUser, "", ClassTurn}, {EntryTypeAssistant, "", ClassTurn},
        {EntryTypeSystem, "", ClassEvent}, {EntryTypeQueueOperation, "", ClassEvent},
        {EntryTypeAttachment, "hook_success", ClassContext},
        {EntryTypeAttachment, "total_tokens_reminder", ClassBookkeeping},
        {EntryTypeAttachment, "", ClassBookkeeping},
        {EntryTypeAPIRequestBlob, "", ClassBookkeeping}, {EntryTypeMode, "", ClassBookkeeping},
        {EntryTypeAITitle, "", ClassSessionState}, {EntryTypeCostState, "", ClassSessionState},
        {EntryTypeFileHistorySnapshot, "", ClassCheckpoint}, {EntryTypeFileHistoryDelta, "", ClassCheckpoint},
        {EntryTypeSummary, "", ClassCheckpoint},
        {EntryType("brand-new-thing"), "", ClassUnknown}, {EntryType(""), "", ClassBookkeeping},
    }
    for _, c := range cases { if got := ClassifyType(c.typ, c.att); got != c.want { t.Errorf("%s/%s: got %v want %v", c.typ, c.att, got, c.want) } }
}
func TestClassifyLineReadsOnlyType(t *testing.T) {
    typ, att, class := ClassifyLine([]byte(`{"type":"attachment","attachment":{"type":"hook_success","stdout":"x"},"message":{"content":"not parsed"}}`))
    if typ != EntryTypeAttachment || att != "hook_success" || class != ClassContext { t.Fatalf("got %s %s %v", typ, att, class) }
    if _, _, class := ClassifyLine([]byte(`{"type":"api-request-blob","hash":"h","message":{"role":"user","content":[]}}`)); class != ClassBookkeeping { t.Fatal("blob must be bookkeeping") }
    if _, _, class := ClassifyLine([]byte(`not json`)); class != ClassBookkeeping { t.Fatal("malformed line must be dropped by classifier") }
}
func TestRecordClassIsEntry(t *testing.T) {
    for _, c := range []RecordClass{ClassTurn, ClassEvent, ClassContext, ClassCheckpoint, ClassUnknown} { if !c.IsEntry() { t.Errorf("%v should be entry", c) } }
    for _, c := range []RecordClass{ClassSessionState, ClassBookkeeping} { if c.IsEntry() { t.Errorf("%v should not be entry", c) } }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sources/claude -run 'TestClassify|TestRecordClass' -v`
Expected: FAIL to compile.

- [ ] **Step 3: Implement `classify.go` and the constants**

`ClassifyLine` unmarshals into `struct{ Type EntryType; Attachment struct{ Type string } }` only. Give `RecordClass` a `String()` for test output.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sources/claude -run 'TestClassify|TestRecordClass' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/claude/classify.go internal/sources/claude/classify_test.go internal/sources/claude/entry.go
git commit -m "feat(claude): classify transcript records by type before parsing"
```

---

### Task 3: One conversion path

**Files:**
- Modify: `internal/sources/claude/entry.go:341-400` (`ToThinktEntry`), `internal/sources/claude/store.go:519-592` (`convertEntry`)
- Test: `internal/sources/claude/convert_test.go`

**Interfaces:**
- Produces: `func (e *Entry) ToThinktEntry() thinkt.Entry` becomes a thin wrapper over `func convertEntry(e *Entry, source thinkt.Source, workspaceID string) *thinkt.Entry`, which is the single implementation. `convertEntry` keeps its current behavior (role, text, blocks, usage, metadata, `ParentUUID`, `SourceAgentID`) plus `IsCheckpoint` and `IsSidechain`, which today only `ToThinktEntry` sets.

- [ ] **Step 1: Write the failing test**

```go
func TestToThinktEntryMatchesConvertEntry(t *testing.T) {
    lines := []string{
        `{"type":"user","uuid":"u1","parentUuid":null,"isSidechain":true,"agentId":"a1","timestamp":"2026-10-20T10:00:00Z","gitBranch":"main","cwd":"/p","sessionId":"s","version":"2.1.310","message":{"role":"user","content":"hi"}}`,
        `{"type":"assistant","uuid":"a1","parentUuid":"u1","timestamp":"2026-10-20T10:00:01Z","sessionId":"s","message":{"id":"msg_1","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"yo"}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
        `{"type":"file-history-snapshot","messageId":"m","snapshot":{"trackedFileBackups":{"/a":{}}}}`,
    }
    for _, line := range lines {
        var e Entry
        if err := json.Unmarshal([]byte(line), &e); err != nil { t.Fatal(err) }
        a := e.ToThinktEntry()
        b := *convertEntry(&e, thinkt.SourceClaude, "")
        a.Metadata, b.Metadata = nil, nil
        if !reflect.DeepEqual(a, b) { t.Fatalf("paths differ for %s:\n%+v\n%+v", e.Type, a, b) }
    }
}
func TestConvertEntrySetsSidechainAndCheckpoint(t *testing.T) { /* IsSidechain true for first line, IsCheckpoint true and Text "File History Snapshot (1 file: /a)" for third */ }
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/sources/claude -run 'TestToThinktEntryMatchesConvertEntry|TestConvertEntrySetsSidechain' -v`
Expected: FAIL (fields such as `Source`, `Usage`, `IsSidechain` differ between the two paths).

- [ ] **Step 3: Make `ToThinktEntry` call `convertEntry(e, thinkt.SourceClaude, "")` and move `IsCheckpoint`/`IsSidechain` into `convertEntry`**

Delete the duplicated body in `entry.go`; keep `convertEntryTypeToRole` only if `convertRole` is not identical, otherwise delete it too.

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/sources/claude ./internal/tui ./internal/server/... 2>&1 | tail -5`
Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/claude/entry.go internal/sources/claude/store.go internal/sources/claude/convert_test.go
git commit -m "refactor(claude): single entry conversion path"
```

---

### Task 4: System events, kinds, and turn metadata

**Files:**
- Modify: `internal/sources/claude/entry.go` (`Entry` struct: add fields), `internal/sources/claude/store.go` (`convertEntry`, `convertRole`)
- Create: `internal/sources/claude/kinds.go` (wrapper detection and kind tables)
- Test: `internal/sources/claude/kinds_test.go`

**Interfaces:**
- Consumes: Task 1 constants and types.
- Produces: new raw `Entry` fields with JSON tags `promptId`, `permissionDecision` (`*PermissionDecisionJSON{Decision, Source, ReasonType, ReasonCode string}`), `toolDenialKind`, `turnOrigin`, `promptSource`, `effort`, `requestedModel`, `thinkingDurationMs`, `attributionAgent`, `attributionSkill`, `attributionPlugin`, `attributionMcpServer`, `attributionMcpTool`, `apiError`, `messageCount`; `AssistantMessage.StopReason` already exists.
  `func detectUserKind(text string, isMeta bool) thinkt.EntryKind` returns, from the trimmed text's first tag: `<system-reminder>` or `isMeta` → `KindMeta`; `<task-notification>` → `KindTaskNotification`; `<agent-message` → `KindAgentMessage`; `<local-command-stdout>` or `<local-command-caveat>` or `<bash-stdout>` → `KindCommandOutput`; otherwise `KindNone`.
  `func systemKind(subtype string) (thinkt.EntryKind, thinkt.Visibility)`: `turn_duration`→(`KindTurnDuration`, Secondary); `away_summary`→(`KindAwaySummary`, Normal); `compact_boundary`→(`KindCompactBoundary`, Normal); `informational`→(`KindInformational`, Secondary); `local_command`→(`KindLocalCommand`, Secondary); anything else→(`KindUnknown`, Secondary).

Conversion rules added to `convertEntry`:
- `system`: `Text = Content`; if `Content == ""`, `Text = Subtype`; `Subtype`, `Kind`, `Visibility` from `systemKind`; `compact_boundary` fills `Compaction` from `compactMetadata` and `LogicalParentUUID`; `turn_duration` puts `durationMs` and `messageCount` in `Metadata`.
- `queue-operation`: role system, `Kind = KindQueueOperation`, `Subtype = Operation`, `Visibility = Secondary`, `Text = Content`.
- `user`: `PromptID`; `Kind = KindCompactSummary` when `IsCompactSummary`; else `detectUserKind(text, IsMeta)`; `Visibility = Secondary` for `KindMeta` and `KindCommandOutput`; `Permission` from `permissionDecision` with `DenialKind = toolDenialKind`; `Metadata["turn_origin"]`, `Metadata["prompt_source"]` when present.
- `assistant`: `TurnID = message.id`; `Effort`; `RequestedModel`; `StopReason` from `message.stop_reason`; `Error` from `error` (and `Kind = KindAPIError`, `Visibility = Secondary` when `isApiErrorMessage`); `Attribution` when any attribution field is non-empty; `Usage.ThinkingTokens` from `usage.output_tokens_details.thinking_tokens`, `ServiceTier`, `Speed`; `Metadata["thinking_duration_ms"]`.
- `file-history-delta`: role checkpoint, `Text = "File History Delta: " + trackingPath`, Secondary.
- `ClassUnknown` types: role system, `Kind = KindUnknown`, `Subtype = type`, Secondary, `Text` = the raw line (pretty-printed JSON).

- [ ] **Step 1: Write the failing tests**

```go
func TestConvertSystemEvents(t *testing.T) {
    cases := []struct{ line, text string; kind thinkt.EntryKind; vis thinkt.Visibility }{
        {`{"type":"system","subtype":"turn_duration","uuid":"s1","durationMs":46913,"messageCount":60}`, "turn_duration", thinkt.KindTurnDuration, thinkt.VisibilitySecondary},
        {`{"type":"system","subtype":"away_summary","uuid":"s2","content":"Done. Next: merge."}`, "Done. Next: merge.", thinkt.KindAwaySummary, thinkt.VisibilityNormal},
        {`{"type":"system","subtype":"something_new","uuid":"s3"}`, "something_new", thinkt.KindUnknown, thinkt.VisibilitySecondary},
        {`{"type":"queue-operation","operation":"enqueue","content":"<cross-session-message>hi</cross-session-message>"}`, "<cross-session-message>hi</cross-session-message>", thinkt.KindQueueOperation, thinkt.VisibilitySecondary},
    }
    // assert Role == RoleSystem, Text, Kind, Visibility, Subtype == subtype/operation
}
func TestConvertCompactBoundary(t *testing.T) {
    line := `{"type":"system","subtype":"compact_boundary","uuid":"c1","parentUuid":null,"logicalParentUuid":"p9","content":"Conversation compacted","compactMetadata":{"trigger":"auto","preTokens":230993,"durationMs":90130,"preservedSegment":{"headUuid":"h","anchorUuid":"a","tailUuid":"t"}}}`
    // assert Kind KindCompactBoundary, Visibility Normal, Compaction{Trigger:"auto",PreTokens:230993,DurationMs:90130,HeadUUID:"h",AnchorUUID:"a",TailUUID:"t",LogicalParentUUID:"p9"}
}
func TestConvertUserKinds(t *testing.T) {
    cases := []struct{ content string; meta, compact bool; kind thinkt.EntryKind; vis thinkt.Visibility }{
        {`"plain prompt"`, false, false, thinkt.KindNone, thinkt.VisibilityNormal},
        {`"<system-reminder>\nx\n</system-reminder>"`, false, false, thinkt.KindMeta, thinkt.VisibilitySecondary},
        {`[{"type":"text","text":"<task-notification>\n<task-id>abc</task-id>\n</task-notification>"}]`, false, false, thinkt.KindTaskNotification, thinkt.VisibilityNormal},
        {`"<agent-message from=\"x\">report</agent-message>"`, true, false, thinkt.KindAgentMessage, thinkt.VisibilityNormal},
        {`"<local-command-stdout>ok</local-command-stdout>"`, false, false, thinkt.KindCommandOutput, thinkt.VisibilitySecondary},
        {`"This session is being continued..."`, false, true, thinkt.KindCompactSummary, thinkt.VisibilityNormal},
        {`"anything"`, true, false, thinkt.KindMeta, thinkt.VisibilitySecondary},
    }
}
func TestConvertUserPermissionAndPrompt(t *testing.T) {
    line := `{"type":"user","uuid":"u","promptId":"p1","permissionDecision":{"decision":"reject","source":"config","reasonType":"safetyCheck"},"toolDenialKind":"permission-rule","turnOrigin":"human","promptSource":"typed","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":"denied","is_error":true}]}}`
    // assert PromptID "p1", Permission{Decision:"reject",Source:"config",ReasonType:"safetyCheck",DenialKind:"permission-rule"}, Metadata["turn_origin"]=="human", Metadata["prompt_source"]=="typed"
}
func TestConvertAssistantMetadata(t *testing.T) {
    line := `{"type":"assistant","uuid":"a","effort":"high","requestedModel":"claude-fable-5-1","thinkingDurationMs":1200,"attributionAgent":"Explore","attributionSkill":"ntrecord","message":{"id":"msg_9","role":"assistant","model":"claude-fable-5-1","stop_reason":"refusal","content":[{"type":"text","text":"no"}],"usage":{"input_tokens":1,"output_tokens":2,"service_tier":"standard","speed":"standard","output_tokens_details":{"thinking_tokens":7}}}}`
    // assert TurnID "msg_9", Effort "high", RequestedModel, StopReason "refusal", Attribution{Agent:"Explore",Skill:"ntrecord"}, Usage.ThinkingTokens 7, ServiceTier, Speed, Metadata["thinking_duration_ms"]==1200
}
func TestConvertAssistantAPIError(t *testing.T) {
    line := `{"type":"assistant","uuid":"e","isApiErrorMessage":true,"error":"server_error","message":{"id":"x","role":"assistant","model":"<synthetic>","content":[{"type":"text","text":"API Error: Can't reach the API server"}]}}`
    // assert Kind KindAPIError, Visibility Secondary, Error "server_error", Text retained
}
func TestConvertUnknownType(t *testing.T) {
    // `{"type":"brand-new","uuid":"n","payload":1}` → Role system, Kind KindUnknown, Subtype "brand-new", Secondary, Text contains `"payload": 1`
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/sources/claude -run 'TestConvert' -v`
Expected: FAIL (fields unset, Text empty).

- [ ] **Step 3: Add raw fields to `Entry`, write `kinds.go`, extend `convertEntry` and `convertRole`**

`convertRole` maps `EntryTypeQueueOperation` and unknown types to `RoleSystem`, `EntryTypeFileHistoryDelta` to `RoleCheckpoint`. The unknown-type branch needs the raw line: add `Raw json.RawMessage` to `Entry` populated by the readers in Task 6 only for `ClassUnknown`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sources/claude -v -run 'TestConvert' && go test ./internal/sources/claude`
Expected: PASS, package `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/claude/entry.go internal/sources/claude/store.go internal/sources/claude/kinds.go internal/sources/claude/kinds_test.go
git commit -m "feat(claude): convert system events, user kinds, and assistant metadata"
```

---

### Task 5: Context attachments as entries

**Files:**
- Modify: `internal/sources/claude/entry.go` (`Entry.Attachment json.RawMessage`, `Rendered []struct{Content string}`, `RenderedRole string`), `internal/sources/claude/store.go` (`convertEntry` attachment branch)
- Create: `internal/sources/claude/attachment.go`
- Test: `internal/sources/claude/attachment_test.go`

**Interfaces:**
- Consumes: `ContextAttachmentTypes` from Task 2.
- Produces: `func attachmentText(attachmentType string, payload json.RawMessage, rendered []string) string` returning, per type: `hook_success` → `hookName + ": " + stdout` (stderr appended after a blank line when non-empty); `hook_additional_context` → joined `content[]`; `queued_command` → `prompt`; `task_status` → `description + " — " + status`; `instructions` → one line per `files[].path`; `file` → `filename`; `invoked_skills` → comma-joined `skills[].name`; `skill_listing` → `content`; `diagnostics` → `"<n> files with diagnostics"` followed by `uri: message` lines; `edited_text_file` → `filename`; `compact_file_reference` → `displayPath` or `filename`; `read_truncation_notice` → `banner`; fallback → joined `rendered`, then the raw payload JSON.
  Converted entry: `Role = RoleSystem` (or `RoleUser` when `renderedRole == "user"`), `Kind = KindAttachment`, `Subtype = attachment.type`, `Visibility = VisibilitySecondary`, `Text` from `attachmentText`, `Metadata["attachment_type"]`, `ParentToolUseID` from `toolUseID` when present.

- [ ] **Step 1: Write the failing tests**

```go
func TestAttachmentText(t *testing.T) {
    cases := []struct{ typ, payload, want string }{
        {"hook_success", `{"hookName":"SessionStart:startup","stdout":"loaded\n","stderr":""}`, "SessionStart:startup: loaded\n"},
        {"task_status", `{"description":"Implement Task 1","status":"completed"}`, "Implement Task 1 — completed"},
        {"instructions", `{"files":[{"path":"/p/AGENTS.md"},{"path":"/p/CLAUDE.md"}]}`, "/p/AGENTS.md\n/p/CLAUDE.md"},
        {"diagnostics", `{"files":[{"uri":"/a.go","diagnostics":[{"message":"bad import"}]}]}`, "1 files with diagnostics\n/a.go: bad import"},
        {"read_truncation_notice", `{"banner":"[Truncated]"}`, "[Truncated]"},
    }
}
func TestConvertAttachmentEntry(t *testing.T) {
    line := `{"type":"attachment","uuid":"at1","parentUuid":"p","timestamp":"2026-10-19T14:47:32Z","attachment":{"type":"hook_success","hookName":"PreToolUse:Bash","toolUseID":"toolu_1","stdout":"ok"},"renderedRole":"system"}`
    // assert Role system, Kind KindAttachment, Subtype "hook_success", Secondary, Text "PreToolUse:Bash: ok", ParentToolUseID "toolu_1", Metadata["attachment_type"]=="hook_success"
    line2 := `{"type":"attachment","uuid":"at2","attachment":{"type":"session_context","context":{}},"rendered":[{"content":"ctx"}],"renderedRole":"user"}`
    // session_context is not allow-listed: Task 6 drops it; here assert convertEntry still yields Role user and Text "ctx" via fallback so an allow-list change needs no converter change
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/sources/claude -run 'TestAttachment|TestConvertAttachment' -v`
Expected: FAIL to compile.

- [ ] **Step 3: Implement `attachment.go` and the converter branch**

- [ ] **Step 4: Run tests**

Run: `go test ./internal/sources/claude`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/claude/attachment.go internal/sources/claude/attachment_test.go internal/sources/claude/entry.go internal/sources/claude/store.go
git commit -m "feat(claude): emit reader-relevant attachments as secondary entries"
```

---

### Task 6: Drop bookkeeping at every read path

**Files:**
- Modify: `internal/sources/claude/parser.go:43-72` (`NextEntry`), `:252-357` (`LoadSessionWindow`), `internal/sources/claude/lazy_session.go:100-132` (`readNextEntry`), `internal/sources/claude/projects.go:406-421` (`countFileLines`)
- Test: `internal/sources/claude/readpaths_test.go`
- Create: `internal/sources/claude/testdata/modern-session.jsonl` (synthetic; 14 lines in this order: `attachment/environment` (bookkeeping), `ai-title` "First title", `user` prompt "hello", `attachment/instructions` (context), `assistant` text with `message.id` "msg_1", `assistant` tool_use Bash (same id, apiBlockIndex 1), `api-request-blob`, `user` tool_result string with toolUseResult stdout, `system/turn_duration`, `mode`, `ai-title` "Final title", `last-prompt`, `cost-state` totalCostUSD 0.12 totalLinesAdded 3, `file-history-snapshot`)

**Interfaces:**
- Consumes: `ClassifyLine` from Task 2.
- Produces: every reader skips lines whose class is not `IsEntry()`; `ClassUnknown` lines set `Entry.Raw`; `countFileLines` is renamed `countConversationRecords(path string) int` and counts only `IsEntry()` lines. `Parser.Errors()` is unaffected by skipped lines.

- [ ] **Step 1: Write the failing tests**

```go
func TestModernFixtureEntryCount(t *testing.T) {
    p, _ := NewParserFromFile("testdata/modern-session.jsonl")
    entries, err := p.ReadAllEntries()
    if err != nil { t.Fatal(err) }
    if len(entries) != 7 { t.Fatalf("want 7 entries (user, instructions, 2 assistant, tool result, turn_duration, snapshot), got %d", len(entries)) }
    if countConversationRecords("testdata/modern-session.jsonl") != 7 { t.Fatal("count must match parser") }
}
func TestLazySessionSkipsBookkeeping(t *testing.T) {
    ls, _ := OpenLazySession("testdata/modern-session.jsonl"); defer ls.Close()
    if err := ls.LoadAll(); err != nil { t.Fatal(err) }
    if ls.EntryCount() != 7 { t.Fatalf("got %d", ls.EntryCount()) }
    for _, e := range ls.Entries() { if e.Role == thinkt.RoleSystem && e.Text == "" { t.Fatalf("empty system entry leaked: %+v", e) } }
}
func TestLoadSessionWindowSkipsBookkeeping(t *testing.T) { /* LoadSessionWindow(path, 0, 1<<20) → EntryCount 7 */ }
func TestUnknownTypeKeepsRaw(t *testing.T) { /* parse `{"type":"brand-new","uuid":"n","x":1}` via NewParser(strings.NewReader) → Entry.Raw non-empty; convertEntry Text contains `"x": 1` */ }
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/sources/claude -run 'TestModernFixture|TestLazySessionSkips|TestLoadSessionWindowSkips|TestUnknownTypeKeepsRaw' -v`
Expected: FAIL (14 or 13 entries instead of 7; missing fixture).

- [ ] **Step 3: Write the fixture and add the `ClassifyLine` gate in the three readers and the counter**

In `NextEntry`, classify `p.scanner.Bytes()` before `json.Unmarshal` and `continue` on non-entry classes. In `readNextEntry` and `LoadSessionWindow`, `jsonl.Reader.ReadJSON` decodes directly; add `ReadRaw() ([]byte, error)` to `internal/jsonl` if it does not exist, classify, then unmarshal. Keep `extractMetadata` running only on emitted entries.

- [ ] **Step 4: Run the package and TUI tests**

Run: `go test ./internal/sources/claude ./internal/tui ./internal/target ./internal/export`
Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/claude internal/jsonl
git commit -m "feat(claude): drop bookkeeping records in every read path"
```

---

### Task 7: Session state into `SessionMeta` and the metadata cache

**Files:**
- Modify: `internal/sources/claude/projects.go:349-421` (`extractSessionHints` → `scanSessionFile`), `internal/sources/claude/store.go:153-275` (`ListSessions`, `startEnrichment`), `:305-357` (`getSessionMetaByPath`: delete the `sessions-index.json` branch), `internal/sources/claude/lazy_session.go:135-162` (`extractMetadata`), `:344-360` (`Metadata`), `internal/thinkt/metadata_cache.go:27-37` (`CachedSession`), `:88-100` (`Lookup`), `:182-215` (`MergeInto`), `internal/sources/claude/projects.go:122-210` (`ListProjects`: `SubagentCount`)
- Test: `internal/sources/claude/sessionstate_test.go`, `internal/thinkt/metadata_cache_test.go` (extend)

**Interfaces:**
- Produces:
  ```go
  type sessionScan struct {
      FirstPrompt, Model, Title, LastPrompt string
      EntryCount int
      PRLinks []thinkt.PRLink; Worktree *thinkt.WorktreeInfo; Cost *thinkt.SessionCost
  }
  func scanSessionFile(path string) sessionScan // one pass; classifies every line; last non-empty ai-title wins
  func countSubagents(sessionPath string) int    // len(glob <dir>/<id>/subagents/agent-*.jsonl)
  ```
  `CachedSession` gains `ParserVersion int`, `Title`, `LastPrompt`, `SubagentCount`. `Lookup` and `MergeInto` treat `ParserVersion != thinkt.ParserVersion` as stale. `startEnrichment` considers a session stale when the cache misses (which now includes version mismatch) and uses `scanSessionFile` once instead of `extractSessionHints` plus `countFileLines`. `MergeInto` fills `Title`, `LastPrompt`, `SubagentCount`, and sets `Summary = Title` when `Summary` is empty. `LazySession.extractMetadata` records `Title`/`LastPrompt` from session-state lines it sees (the readers in Task 6 still skip emitting them; expose a hook `onSessionState(typ EntryType, raw []byte)` called before the skip). `ListProjects` sums `countSubagents` over the project's session files into `Project.SubagentCount`.

- [ ] **Step 1: Write the failing tests**

```go
func TestScanSessionFileModernFixture(t *testing.T) {
    s := scanSessionFile("testdata/modern-session.jsonl")
    if s.Title != "Final title" || s.LastPrompt == "" || s.EntryCount != 7 || s.FirstPrompt != "hello" { t.Fatalf("%+v", s) }
    if s.Cost == nil || s.Cost.TotalUSD != 0.12 || s.Cost.LinesAdded != 3 { t.Fatalf("cost %+v", s.Cost) }
}
func TestScanSessionFileEmptyTitleDoesNotBlank(t *testing.T) {
    // temp file: ai-title "Keep", then ai-title "" → Title "Keep"
}
func TestScanSessionFileBookkeepingOnly(t *testing.T) {
    // temp file: ai-title "Only title", mode, permission-mode → EntryCount 0, Title "Only title"
    // and OpenSession on it returns io.EOF on first ReadNext; ListSessions lists it with Title
}
func TestMetadataCacheParserVersion(t *testing.T) {
    // Set entry with ParserVersion 1, Lookup with matching mtime/size → not ok; with ParserVersion == thinkt.ParserVersion → ok
}
func TestMergeIntoTitleFallsBackToSummary(t *testing.T) { /* Summary empty → Summary == Title; Summary set → untouched */ }
func TestCountSubagents(t *testing.T) { /* temp dir <id>/subagents/agent-a.jsonl, agent-b.jsonl, agent-a.meta.json → 2 */ }
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/sources/claude ./internal/thinkt -run 'TestScanSessionFile|TestMetadataCacheParserVersion|TestMergeIntoTitle|TestCountSubagents' -v`
Expected: FAIL to compile.

- [ ] **Step 3: Implement the scan, cache fields, merge, enrichment, and project count; delete the `sessions-index.json` lookup and `sessionsIndex` parsing in `projects.go:212-336` if nothing else uses it**

`scanSessionFile` uses `thinkt.NewScannerWithMaxCapacityCustom(f, 64*1024, thinkt.MaxLineCapacity)` and `ClassifyLine`; it unmarshals `message` only for the first user prompt and first real model, as `extractSessionHints` does today.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/sources/claude ./internal/thinkt ./internal/tui ./internal/server/...`
Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/claude internal/thinkt
git commit -m "feat(claude): fold session state into SessionMeta with parser-versioned cache"
```

---

### Task 8: Tool result children, `toolUseResult`, and diffs

**Files:**
- Modify: `internal/sources/claude/store.go:657-700` (`convertUserBlocks`, `convertAssistantBlocks`), `internal/sources/claude/entry.go` (`ToolUseResult` is already `json.RawMessage`)
- Create: `internal/sources/claude/toolresult.go`
- Test: `internal/sources/claude/toolresult_test.go`

**Interfaces:**
- Produces:
  ```go
  // nested content → children; returns joined text of text children
  func convertToolResultContent(raw json.RawMessage) (text string, children []thinkt.ContentBlock)
  type toolUseResult struct {
      Stdout, Stderr string; Interrupted bool; PersistedOutputPath string
      FilePath string; StructuredPatch []patchHunk
      AgentID string; Status string
  }
  type patchHunk struct { OldStart, OldLines, NewStart, NewLines int; Lines []string }
  func parseToolUseResult(raw json.RawMessage) (toolUseResult, bool) // false for string/array/empty
  func unifiedDiff(filePath string, hunks []patchHunk) string // "--- a/<path>\n+++ b/<path>\n@@ -o,l +n,m @@\n<lines…>" per hunk
  func applyToolUseResult(block *thinkt.ContentBlock, entry *thinkt.Entry, r toolUseResult)
  ```
  Rules: a string `content` sets `ToolResult` only. An array sets `ToolResult` to text children joined by `"\n"` and `Children` to one block per element: `text` → `{Type:"text",Text}`, `image` → `{Type:"image",MediaType,MediaData}`, anything else → `{Type: <type>, Text: <element JSON>}`. `applyToolUseResult`: `StructuredPatch` non-empty → append child `{Type: BlockTypeDiff, Text: unifiedDiff(...)}`; `Stdout`/`Stderr` non-empty and `ToolResult == ""` → `ToolResult = Stdout` and append `{Type:"text",Text: Stderr}` when stderr non-empty; `AgentID` → `block.ChildSessionID = entry.Metadata["session_id"] + "/agent-" + AgentID` and `entry.ChildSessionID` the same; `PersistedOutputPath` → `entry.Metadata["persisted_output_path"]`; `Interrupted` → `entry.Metadata["interrupted"] = true`. The `tool_use` block of the matching Agent call cannot be reached from the result entry; `ChildSessionID` on the entry is enough for the index work to pair them later.

- [ ] **Step 1: Write the failing tests**

```go
func TestConvertToolResultContentArray(t *testing.T) {
    text, kids := convertToolResultContent([]byte(`[{"type":"text","text":"a"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}},{"type":"tool_reference","tool_name":"X"}]`))
    // text "a"; len(kids) 3; kids[1].MediaType "image/png"; kids[2].Type "tool_reference" and Text contains `"tool_name"`
}
func TestConvertToolResultContentString(t *testing.T) { /* `"plain"` → text "plain", kids nil */ }
func TestParseToolUseResultShapes(t *testing.T) {
    // object with structuredPatch → ok; `"a string"` → !ok; `[1,2]` → !ok; empty → !ok
}
func TestUnifiedDiff(t *testing.T) {
    got := unifiedDiff("/p/a.md", []patchHunk{{OldStart: 34, OldLines: 2, NewStart: 34, NewLines: 3, Lines: []string{" keep", "-old", "+new", "+more"}}})
    want := "--- a//p/a.md\n+++ b//p/a.md\n@@ -34,2 +34,3 @@\n keep\n-old\n+new\n+more\n"
}
func TestConvertEntryToolResultWithPatch(t *testing.T) {
    line := `{"type":"user","uuid":"r","sessionId":"sess","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"The file has been updated."}]},"toolUseResult":{"filePath":"/p/a.md","oldString":"old","newString":"new","structuredPatch":[{"oldStart":1,"oldLines":1,"newStart":1,"newLines":1,"lines":["-old","+new"]}]}}`
    // block.ToolResult "The file has been updated."; block.Children[0].Type == BlockTypeDiff; Text starts with "--- a/"
}
func TestConvertEntryToolResultAgentID(t *testing.T) {
    // toolUseResult {"agentId":"a9","status":"async_launched"} → entry.ChildSessionID "sess/agent-a9" and block.ChildSessionID same
}
func TestConvertEntryToolResultStringToolUseResultIgnored(t *testing.T) {
    // content "x", toolUseResult "ignored" → ToolResult "x", Children nil, no metadata keys added
}
func TestConvertEntryToolResultStdoutFallback(t *testing.T) {
    // content "" , toolUseResult {"stdout":"out","stderr":"err","interrupted":true} → ToolResult "out", one child text "err", Metadata["interrupted"]==true
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/sources/claude -run 'ToolResult|UnifiedDiff|ToolUseResult' -v`
Expected: FAIL to compile.

- [ ] **Step 3: Implement `toolresult.go`; call it from `convertUserBlocks` (pass the entry's `ToolUseResult`) and from `convertEntry` for `ChildSessionID`**

`convertUserBlocks` gains the signature `convertUserBlocks(blocks []ContentBlock, toolUseResult json.RawMessage, entry *thinkt.Entry) []thinkt.ContentBlock`; the typed result applies to the first `tool_result` block only.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/sources/claude ./internal/export ./internal/server/...`
Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/claude
git commit -m "feat(claude): decode tool result children and toolUseResult diffs"
```

---

### Task 9: Consumers honor `Visibility`, `Children`, and `diff`

**Files:**
- Modify: `internal/target/filter.go:46-70` (`FilterEntries`), `:72-85` (`shouldIncludeEntry`), `internal/tui/filter.go:27-45` (`EntryVisible`), `internal/conversation/document.go:33-47` (`Block`), `:62-110` (`Build`), `internal/conversation/html.go:26-33` (`View`), `:40-110` (`WriteHTML`), `internal/conversation/conversation.css`, `internal/conversation/testdata/session.json`, `internal/conversation/testdata/conversation.html` (golden), `internal/export/markdown.go:38-80` (`renderEntryMarkdown`)
- Test: `internal/target/filter_test.go`, `internal/tui/filter_test.go`, `internal/conversation/document_test.go`, `internal/conversation/html_test.go`, `internal/export/export_test.go`

**Interfaces:**
- Consumes: Task 1 types.
- Produces: `target.FilterEntries` drops `VisibilitySecondary` entries unless `IncludeSystem`; `tui.RoleFilterSet.EntryVisible` returns `f.Other` for any `VisibilitySecondary` entry before the role switch. `conversation.Message` gains `Kind thinkt.EntryKind`, `Visibility thinkt.Visibility`, `TurnID string`, `Attribution *thinkt.Attribution`, `StopReason`, `Error string`, `Compaction *thinkt.CompactionInfo`. `conversation.Block` gains `Children []Block` (IDs `<parent>-child-<k>`, origin copied from the parent block) and `ChildSessionID string`; `Build` maps `BlockTypeDiff` → `Text` and builds children recursively. `View` gains `HiddenVisibility map[thinkt.Visibility]bool`; `WritePage` with a zero `View` hides `VisibilitySecondary` (`WriteHTML` with a zero `View` hides nothing, as today). `WriteHTML` renders a `diff` block as `<pre class="diff">` with each line wrapped in `<span class="diff-add|diff-del|diff-hunk|diff-ctx">`, renders `Children` inside the parent block body after its own content, adds `data-turn-id` and `data-kind` attributes on the message section, and appends attribution (`agent · skill`) and a `refusal`/error marker to the metadata line. CSS: four `.diff-*` color rules and `.block-child { margin-left: 1rem }`, nothing else. Markdown export: a `diff` child renders as a ```` ```diff ```` fence; other children as their text.

- [ ] **Step 1: Write the failing tests**

```go
// target
func TestFilterEntriesHidesSecondaryUnlessSystem(t *testing.T) { /* Secondary user KindMeta dropped by DefaultFilter(); kept with IncludeSystem */ }
// tui
func TestEntryVisibleSecondaryUsesOther(t *testing.T) { /* RoleUser + Secondary → false with Other=false, true with Other=true */ }
// conversation
func TestBuildChildrenAndDiff(t *testing.T) { /* tool_result with Children [diff, image] → Block.Children len 2, [0].Kind "diff", [0].Text preserved, IDs "<parent>-child-0" */ }
func TestBuildMessageKindAndTurn(t *testing.T) { /* Entry Kind/Visibility/TurnID/Attribution copied onto Message */ }
func TestWritePageHidesSecondaryByDefault(t *testing.T) { /* a Secondary message id absent from WritePage output, present in WriteHTML output */ }
func TestWriteHTMLDiffAndChildren(t *testing.T) { /* output contains `<pre class="diff">`, `<span class="diff-add">+new</span>`, `data-turn-id="msg_1"`, child section inside parent */ }
// export
func TestMarkdownDiffChild(t *testing.T) { /* contains "```diff\n--- a/" */ }
```

Also extend `testdata/session.json` with a tool result that has a `diff` child and an `image` child, a Secondary `turn_duration` system entry, and `turn_id` on the assistant entries; then regenerate the golden with `go test ./internal/conversation -run TestHTMLGolden -update` and inspect the diff of `conversation.html` by eye before committing.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/target ./internal/tui ./internal/conversation ./internal/export -run 'Secondary|Children|Diff|KindAndTurn' -v`
Expected: FAIL.

- [ ] **Step 3: Implement the filter changes, document fields, HTML rendering, CSS, and markdown fence**

- [ ] **Step 4: Run tests, regenerate the golden, and run the whole tree**

Run: `go test ./internal/conversation -run TestHTMLGolden -update && go test ./...  2>&1 | grep -v '^ok' | grep -v 'no test files'`
Expected: only the two pre-existing `internal/cmd` apps failures.

- [ ] **Step 5: Commit**

```bash
git add internal/target internal/tui/filter.go internal/tui/filter_test.go internal/conversation internal/export
git commit -m "feat: honor entry visibility and render tool result children and diffs"
```

---

### Task 10: File search skips bookkeeping lines

**Files:**
- Modify: `internal/index/search/search.go:294-343` (`scanFile`)
- Test: `internal/index/search/search_test.go` (extend)

**Interfaces:**
- Consumes: `claude.ClassifyLine` (import `github.com/wethinkt/go-thinkt/internal/sources/claude`; verify no import cycle with `go build ./...`).
- Produces: in `scanFile`, after `m.Match(text)` succeeds, call `claude.ClassifyLine([]byte(text))` and `continue` when the class is not `IsEntry()`; keep the existing role detection for other sources (a line without a recognized Claude `type` classifies as `ClassBookkeeping` only when `type` is empty, so Kimi lines with `role` are unaffected: guard with `entry.Type != ""`).

- [ ] **Step 1: Write the failing test**

```go
func TestScanFileSkipsBookkeepingLines(t *testing.T) {
    // temp file with: api-request-blob line containing "needle", user line containing "needle", attachment/total_tokens_reminder containing "needle", kimi-style {"role":"user","content":"needle"}
    // run the search service over it → 2 matches (the user line and the kimi line), roles "user"
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/index/search -run TestScanFileSkipsBookkeepingLines -v`
Expected: FAIL with 4 matches.

- [ ] **Step 3: Add the classification gate**

- [ ] **Step 4: Run tests**

Run: `go build ./... && go test ./internal/index/...`
Expected: build ok, tests `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/index/search
git commit -m "feat(search): skip Claude bookkeeping records when scanning session files"
```

---

### Task 11: Index re-ingests on parser version change

**Files:**
- Modify: `internal/index/db/db.go:86-105` (`migrate`: version 2), `internal/index/ingester.go:290-320` (`writeSession` sync_state insert, `ShouldSyncSession`)
- Test: `internal/index/ingester_test.go` (extend), `internal/index/db/db_test.go` (extend)

**Interfaces:**
- Produces: migration 2 runs `ALTER TABLE sync_state ADD COLUMN parser_version INTEGER NOT NULL DEFAULT 0` and inserts `(2)` into `migrations`. `writeSession` stores `thinkt.ParserVersion` in `sync_state.parser_version`. `ShouldSyncSession` returns `true` when the stored `parser_version != thinkt.ParserVersion`, in addition to the mtime/size checks.

- [ ] **Step 1: Write the failing tests**

```go
func TestMigrateToVersion2AddsParserVersion(t *testing.T) { /* open temp db → max(version)==2; PRAGMA table_info(sync_state) contains parser_version */ }
func TestShouldSyncSessionOnParserVersionChange(t *testing.T) {
    // insert sync_state row with same mtime/size and parser_version 1 → ShouldSyncSession true
    // update parser_version to thinkt.ParserVersion → false
}
func TestWriteSessionStoresParserVersion(t *testing.T) { /* after writeSession, SELECT parser_version == thinkt.ParserVersion */ }
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/index/... -run 'ParserVersion' -v`
Expected: FAIL (no such column).

- [ ] **Step 3: Implement migration 2 and the two ingester changes**

- [ ] **Step 4: Run tests**

Run: `go test ./internal/index/...`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/index
git commit -m "feat(index): re-ingest sessions when the parser version changes"
```

---

### Task 12: Attachment table in the index

**Files:**
- Modify: `internal/index/db/schema/init.sql` (new table), `internal/index/db/db.go` (migration 3 creates it for existing databases), `internal/index/ingester.go:207-300` (`writeSession`)
- Create: `internal/index/db/attachments.go`
- Test: `internal/index/ingester_test.go` (extend), `internal/index/db/attachments_test.go`

**Interfaces:**
- Produces:
  ```sql
  CREATE TABLE IF NOT EXISTS entry_attachments (
      session_id  TEXT NOT NULL,
      entry_index INTEGER NOT NULL,   -- index among emitted entries, 0-based
      kind        TEXT NOT NULL,      -- Entry.Subtype (attachment.type)
      label       TEXT,               -- first line of Entry.Text, max 200 chars
      timestamp   TEXT,
      PRIMARY KEY (session_id, entry_index)
  );
  CREATE INDEX IF NOT EXISTS idx_entry_attachments_kind ON entry_attachments(kind);
  ```
  ```go
  type AttachmentRow struct { SessionID string; EntryIndex int; Kind, Label string; Timestamp time.Time }
  func (d *DB) ListAttachments(sessionID string, kind string) ([]AttachmentRow, error) // kind "" = all
  ```
  `writeSession` deletes the session's rows then inserts one per entry with `Kind == thinkt.KindAttachment`. No text body is stored.

- [ ] **Step 1: Write the failing tests**

```go
func TestWriteSessionRecordsAttachments(t *testing.T) { /* entries: user, attachment(hook_success, Text "PreToolUse: ok\nmore"), attachment(task_status) → ListAttachments("s","") len 2, [0].EntryIndex 1, Label "PreToolUse: ok"; ListAttachments("s","task_status") len 1 */ }
func TestMigrateToVersion3CreatesAttachments(t *testing.T) { /* max(version)==3; table exists */ }
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/index/... -run 'Attachments' -v`
Expected: FAIL.

- [ ] **Step 3: Implement schema, migration, insert, and query**

- [ ] **Step 4: Run tests**

Run: `go test ./internal/index/...`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/index
git commit -m "feat(index): record attachment entries for quick access"
```

---

### Task 13: End-to-end check on the fixture and docs

**Files:**
- Modify: `internal/sources/claude/README.md` (record classes section), `internal/conversation/README.md` (new Message/Block fields, `HiddenVisibility`, `diff`), `etc/reports/CLAUDE_STRUCTURE.md` (rewrite "Directory Layout" from the schema report; remove `debug/`, `todos/`, `statsig/`, `sessions-index.json`; add `subagents/`, `tool-results/`, `session-env/`, `sessions/`, `memory/`, `file-history-delta`), `AGENTS.md` (Key Packages row for `internal/conversation`; one line under Architecture on `Visibility`), swagger via `task server:swag-v2`
- Test: `internal/sources/claude/store_test.go` (extend)

**Interfaces:**
- Consumes: everything above.

- [ ] **Step 1: Write the failing end-to-end test**

```go
func TestStoreOpenSessionModernFixture(t *testing.T) {
    // NewStore over a temp base dir containing projects/-p/<id>.jsonl copied from testdata/modern-session.jsonl
    // OpenSession → read all: 7 entries; roles in order user, system(attachment, Secondary), assistant, assistant, user(tool result), system(turn_duration, Secondary), checkpoint
    // assistant entries share TurnID "msg_1"; tool result block has ToolResult "ok" and nil Children (string content wins over toolUseResult stdout)
    // GetSessionMeta → Title "Final title", EntryCount 7, Cost.TotalUSD 0.12, Summary == Title
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/sources/claude -run TestStoreOpenSessionModernFixture -v`
Expected: FAIL until the store path wires `scanSessionFile` into `GetSessionMeta` (fix in `getSessionMetaByPath`: use the metadata cache then `scanSessionFile`).

- [ ] **Step 3: Fix what the test exposes, update the four docs, regenerate swagger**

Run: `task server:swag-v2`
Expected: `internal/server/docs/swagger.json` contains `"turn_id"` and `"visibility"` under `thinkt.Entry`.

- [ ] **Step 4: Full verification**

Run: `go build ./... && go vet ./internal/... && go test ./... 2>&1 | grep -v '^ok' | grep -v 'no test files'`
Expected: only the two pre-existing `internal/cmd` apps failures.

Run: `go build -o ./bin/thinkt ./cmd/thinkt && ./bin/thinkt export --system -f json -s <path/to/a/real/session.jsonl> -o /tmp/thinkt-export.json </dev/null && jq -r '.[] | select(.role=="system" and (.text//"")=="") | .role' /tmp/thinkt-export.json | wc -l`
Expected: `0` (down from 1555); and `jq -r '[.[] | .role] | group_by(.) | map({(.[0]): length}) | add' /tmp/thinkt-export.json` shows a few hundred entries, not 2,138.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/claude internal/conversation/README.md etc/reports/CLAUDE_STRUCTURE.md AGENTS.md internal/server/docs
git commit -m "docs: describe record classes, visibility, and current Claude Code layout"
```
