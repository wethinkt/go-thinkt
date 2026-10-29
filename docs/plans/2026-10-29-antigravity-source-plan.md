# Antigravity Source Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A new `internal/sources/antigravity` package that reads `~/.gemini/antigravity-cli` step transcripts with the conversation model (classified steps, one assistant entry per model turn with usage, tool results paired by order, hidden harness steps, subagents as hidden linked sessions, projects by folder), plus `SessionMeta.RelatedPaths` in the object model, filled by this source.

**Architecture:** A classifier decides each transcript line's class from its `type`; a converter turns one step into zero or one entry; a streaming parser holds the queue of unanswered tool-use ids, the turn id, and the current model, and accumulates related folders as it goes. The store lists conversations from brain dirs joined to `conversation_summaries.db` (read-only SQLite), resolves each to a workspace folder, and computes `SessionMeta` by running the parser once (cached). Registration wires the source into the factory list, TUI, search, active detection, CLI help, and summary output; the gemini source stops claiming Antigravity files.

**Tech Stack:** Go 1.26; `github.com/mattn/go-sqlite3` (already a dependency) through `database/sql`. Tests with `go test`. No new dependencies.

**Spec:** `docs/plans/2026-10-29-antigravity-source-design.md` (sections A–H are binding; "Calls made after the builder check" overrides anything older in the document).

## Global Constraints

- Source id `antigravity` (`thinkt.SourceAntigravity`), display name "Antigravity CLI", env `THINKT_ANTIGRAVITY_HOME`, default `~/.gemini/antigravity-cli`.
- `thinkt.ParserVersion` stays 5.
- The only `thinkt` type added is `RelatedPath`; the only fields added are `SessionMeta.RelatedPaths` and `CachedSession.RelatedPaths`.
- Classification lives in one function (`ClassifyLine`) used by the parser and the search filter.
- Entry UUID is `<conversation-id>:<step_index>`; tool-use ids are `<step_index>.<n>`; tool pair halves never share a UUID because each is its own step.
- No entry is emitted with empty `Text` and no `ContentBlocks`.
- The summaries db is opened read-only (`file:<path>?mode=ro&_busy_timeout=200`); thinkt never writes to Antigravity's tree.
- Session file is `transcript_full.jsonl`, or `transcript.jsonl` when there is no full file; never both; never anything under `logs/chunks/`.
- Projects are folders keyed by path: first `workspace_uris` entry; else most common unquoted `run_command` `Cwd`; else `cache/last_conversations.json`; else the no-workspace project. Antigravity `project_id` is never used.

## Review Focus

1. A transcript being written while it is read: the last line is a partial JSON object. The parser skips it and returns `io.EOF`, no error. Pinned in Task 4 (`TestParserSkipsPartialLastLine`).
2. `conversation_summaries.db` missing or locked: listing still returns every conversation with a transcript, titles from the first prompt. Pinned in Task 6 (`TestListSessionsWithoutSummaryDB`).
3. A `PLANNER_RESPONSE` whose calls are superseded by the next one, then results for the new calls: each result pairs with the new response's calls, not the stale ones. Pinned in Task 4 (`TestParserNewResponseDropsStaleCalls`).
4. A touched path that does not exist on disk any more, or lives in `/tmp` or the Antigravity home: not recorded in `RelatedPaths`, no error. Pinned in Task 5 (`TestRelatedFolderSkipsMissingTempAndHome`).
5. A `Cwd` argument stored JSON-quoted (`"\"/repo\""`): unquoted for both workspace fallback and related paths. Pinned in Task 5 (`TestUnquotePath`) and Task 6 (`TestWorkspaceFallsBackToCwd`).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/thinkt/types.go` | `SourceAntigravity` (all switches, `AllSources`), `RelatedPath`, `SessionMeta.RelatedPaths`; fix `SourceKimi.Description()` to `~/.kimi-code` |
| `internal/thinkt/metadata_cache.go` | `CachedSession.RelatedPaths`, `MergeInto` copies it |
| `internal/sources/antigravity/classify.go` | `RecordClass`, `stepHead`, `ClassifyLine`, `NewSearchFilter` |
| `internal/sources/antigravity/unknown.go` | `unknownTally` (copy of Kimi's, `antigravity:` prefix) |
| `internal/sources/antigravity/convert.go` | step → entry, `<USER_REQUEST>` split, result header strip, system-message sender, subagent ids |
| `internal/sources/antigravity/related.go` | `relatedTracker`, `repoRoot`, `unquotePath` |
| `internal/sources/antigravity/parser.go` | streaming `Parser` |
| `internal/sources/antigravity/summaries.go` | read `conversation_summaries.db`, `last_conversations.json` |
| `internal/sources/antigravity/sessionstate.go` | `sessionScan`, `scanSessionFile`, workspace resolution, `applyTo`, `cached` |
| `internal/sources/antigravity/store.go` | `Store` (projects, sessions, subagents, cache, enrichment, readers, `WatchConfig`) |
| `internal/sources/antigravity/discovery.go` | `Discoverer`, `Factory`, `DefaultDir`, `IsSessionPath`, `BaseDirForPath`, `SessionIDForPath`, `sessionFile` |
| `internal/sources/antigravity/resume.go` | `agy --conversation <id>` |
| `internal/sources/antigravity/testdata/` | `main.jsonl`, `child.jsonl`, `noworkspace.jsonl`, `summaries.sql` |
| `internal/sources/antigravity/README.md` | package doc |
| `internal/sources/gemini/discovery.go` | `IsSessionPath` excludes the Antigravity home |
| `internal/sources/sources.go`, `internal/tui/styles.go`, `internal/tui/session_loader.go`, `internal/thinkt/active.go`, `internal/index/search/search.go`, `internal/cmd/help_env.go`, `internal/cmd/sources.go`, `internal/cli/sessions.go` | registration and the "Related" summary line |
| docs: `AGENTS.md`, `README.md`, `DEVELOP.md`, `etc/reports/SOURCE_FORMATS_2026-10.md`, swagger (`task swag` or the repo's generator) | source list, env var, report row |

---

### Task 1: Object model: `SourceAntigravity` and `RelatedPaths`

**Files:**
- Modify: `internal/thinkt/types.go`, `internal/thinkt/metadata_cache.go`
- Test: `internal/thinkt/related_test.go` (new)

**Interfaces:**
- Produces: `thinkt.SourceAntigravity Source = "antigravity"`; `type RelatedPath struct { Path string \`json:"path"\`; Reasons []string \`json:"reasons"\`; Count int \`json:"count"\` }`; constants `RelatedWorkspace = "workspace"`, `RelatedEdited = "edited"`, `RelatedCommand = "command"`, `RelatedRead = "read"`; `SessionMeta.RelatedPaths []RelatedPath \`json:"related_paths,omitempty"\``; `CachedSession.RelatedPaths []RelatedPath \`json:"related_paths,omitempty"\``.

- [ ] **Step 1: Write the failing tests**

```go
func TestSourceAntigravityNames(t *testing.T) {
	s := SourceAntigravity
	if s.String() != "antigravity" || s.DisplayName() != "Antigravity CLI" ||
		s.Description() != "Antigravity CLI conversations (~/.gemini/antigravity-cli)" {
		t.Fatalf("got %q %q %q", s.String(), s.DisplayName(), s.Description())
	}
	if !slices.Contains(AllSources, SourceAntigravity) {
		t.Fatal("AllSources lacks antigravity")
	}
	if SourceKimi.Description() != "Kimi Code sessions (~/.kimi-code)" {
		t.Fatalf("kimi description %q", SourceKimi.Description())
	}
}

func TestMergeIntoCopiesRelatedPaths(t *testing.T) {
	mc := &MetadataCache{Sessions: map[string]CachedSession{}}
	mod := time.Unix(100, 0)
	rel := []RelatedPath{{Path: "/r/ds4-go", Reasons: []string{RelatedEdited}, Count: 3}}
	mc.Set("/f", CachedSession{ModifiedAt: mod, FileSize: 7, RelatedPaths: rel})
	meta := SessionMeta{FullPath: "/f", ModifiedAt: mod, FileSize: 7}
	if !mc.MergeInto(&meta) || !reflect.DeepEqual(meta.RelatedPaths, rel) {
		t.Fatalf("got %+v", meta.RelatedPaths)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/thinkt -run 'TestSourceAntigravityNames|TestMergeIntoCopiesRelatedPaths'`
Expected: FAIL to compile (`undefined: SourceAntigravity`, `RelatedPath`).

- [ ] **Step 3: Implement** the constant in every `Source` switch and `AllSources` (after `SourceQwen`), `RelatedPath` and its reason constants next to `PRLink`, the two fields, and in `MergeInto`: `if meta.RelatedPaths == nil { meta.RelatedPaths = entry.RelatedPaths }`. Fix the Kimi description. If `MetadataCache` cannot be built as a literal in a test, use `LoadMetadataCache(SourceAntigravity, t.TempDir())`.

- [ ] **Step 4: Run** `go test ./internal/thinkt/...` — Expected: PASS (including `TestParserVersionIsFive`).

- [ ] **Step 5: Commit** `feat(thinkt): antigravity source id; RelatedPaths on SessionMeta and the metadata cache`

---

### Task 2: Classification, fixtures, unknown tally

**Files:**
- Create: `internal/sources/antigravity/classify.go`, `unknown.go`, `testdata/main.jsonl`, `testdata/child.jsonl`, `testdata/noworkspace.jsonl`
- Test: `internal/sources/antigravity/classify_test.go`

**Interfaces:**
- Produces: `type RecordClass int` with `ClassUserInput, ClassModelTurn, ClassToolResult, ClassSystemMessage, ClassHarness, ClassCheckpoint, ClassAPIError, ClassUnknown, ClassInvalid` (invalid = not JSON); `type stepHead struct { Index int; Source, Type, Status, Error string; CreatedAt time.Time; Content string; Raw []byte }` (Content decoded only for classes whose conversion needs it is fine); `func ClassifyLine(line []byte) (stepHead, RecordClass)`; `func NewSearchFilter() func(line []byte) bool`.

Fixtures (hand-written, one JSON object per line, field names exactly as in the evidence section). `main.jsonl` (conversation `11111111-aaaa`, workspace `/w/ds4dg`), in order:

| idx | type | what it carries |
|---|---|---|
| 0 | USER_INPUT | `<USER_REQUEST>\nfix the build\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\nThe current local time is: 2026-10-28T16:34:22-04:00.\n</ADDITIONAL_METADATA>\n<USER_SETTINGS_CHANGE>\nThe user changed setting \`Model Selection\` from None to Gemini 4 Argon (High). …\n</USER_SETTINGS_CHANGE>` |
| 1 | CONVERSATION_HISTORY | no content |
| 2 | PLANNER_RESPONSE | thinking `"look first"`, content `"Reading."`, tool_calls `view_file{AbsolutePath:<repoB>/a.go}`, `run_command{CommandLine:"go build", Cwd:"\"<repoB>\""}`; tokens 100/40/20 |
| 3 | VIEW_FILE | `Created At: 2026-10-28T20:34:23Z\nCompleted At: 2026-10-28T20:34:23Z\nfile body` |
| 4 | GENERIC | status ERROR, error `"exit 1"`, content with the two header lines then `build failed` |
| 5 | SYSTEM_MESSAGE | `[Message] … sender=11111111-aaaa/task-4 …` (task notification) |
| 6 | PLANNER_RESPONSE | tool_calls `replace_file_content{TargetFile:<repoB>/a.go}`, `grep_search{SearchPath:<repoB>}` |
| 7 | ERROR_MESSAGE | error `"The stream was interrupted. Please continue the task you were working on."`, no content |
| 8 | PLANNER_RESPONSE | tool_calls `invoke_subagent{Subagents:[…]}` (the previous two calls are superseded) |
| 9 | GENERIC | `Created the following subagents:\n{\n  "conversationId": "22222222-bbbb",\n  "logAbsoluteUri": "file:///x"\n}\n{\n  "conversationId": "33333333-cccc",\n  "logAbsoluteUri": "file:///y"\n}` |
| 10 | SYSTEM_MESSAGE | `[Message] … sender=22222222-bbbb …` (subagent reports back) |
| 11 | EPHEMERAL_MESSAGE | `<EPHEMERAL_MESSAGE>remember the plan</EPHEMERAL_MESSAGE>` |
| 12 | CHECKPOINT | `# Resuming from a compaction\n\n… summary …` |
| 13 | USER_INPUT | `<USER_REQUEST>\nthanks\n</USER_REQUEST>` |
| 14 | PLANNER_RESPONSE | content `"Done."` only |
| 15 | MYSTERY_STEP | unknown type |

`<repoB>` is a placeholder the tests replace with a temp-dir repo (write the fixture with the literal `__REPO_B__`; tests substitute before parsing). `child.jsonl` (`22222222-bbbb`): idx 0 SYSTEM_MESSAGE from sender `11111111-aaaa` (task prompt), idx 1 PLANNER_RESPONSE content `"Found it."`. `noworkspace.jsonl`: one USER_INPUT, one PLANNER_RESPONSE with `run_command` `Cwd` `"/w/old"` twice and `"/w/other"` once.

- [ ] **Step 1: Write the failing tests**

```go
func TestClassifyLineEveryFixtureStep(t *testing.T) {
	want := []RecordClass{ClassUserInput, ClassHarness, ClassModelTurn, ClassToolResult, ClassToolResult,
		ClassSystemMessage, ClassModelTurn, ClassAPIError, ClassModelTurn, ClassToolResult,
		ClassSystemMessage, ClassHarness, ClassCheckpoint, ClassUserInput, ClassModelTurn, ClassUnknown}
	for i, line := range fixtureLines(t, "main.jsonl") {
		h, c := ClassifyLine(line)
		if c != want[i] || h.Index != i {
			t.Errorf("line %d: class %v index %d", i, c, h.Index)
		}
	}
}

func TestClassifyLineEveryResultType(t *testing.T) {
	for _, ty := range []string{"GENERIC", "VIEW_FILE", "RUN_COMMAND", "CODE_ACTION", "GREP_SEARCH",
		"LIST_DIRECTORY", "SEARCH_WEB", "READ_URL_CONTENT", "ASK_QUESTION"} {
		if _, c := ClassifyLine([]byte(`{"step_index":1,"type":"` + ty + `","content":"x"}`)); c != ClassToolResult {
			t.Errorf("%s: %v", ty, c)
		}
	}
}

func TestClassifyLineInvalidJSON(t *testing.T) {
	if _, c := ClassifyLine([]byte(`{"step_index":1,"ty`)); c != ClassInvalid {
		t.Fatalf("got %v", c)
	}
}

func TestSearchFilterKeepsConversationLines(t *testing.T) {
	f := NewSearchFilter()
	var kept []int
	for i, line := range fixtureLines(t, "main.jsonl") {
		if f(line) {
			kept = append(kept, i)
		}
	}
	if !slices.Equal(kept, []int{0, 2, 3, 4, 6, 8, 9, 13, 14}) {
		t.Fatalf("kept %v", kept)
	}
}
```

`fixtureLines(t, name) [][]byte` lives in `classify_test.go` and splits the fixture file on newlines.

- [ ] **Step 2: Run** `go test ./internal/sources/antigravity -run 'Classify|SearchFilter'` — Expected: FAIL to compile.
- [ ] **Step 3: Implement** `ClassifyLine` (decode `stepHead`; class by `type` per design section A), `NewSearchFilter` (keeps user input, model turn, tool result), and `unknownTally`.
- [ ] **Step 4: Run** the same command — Expected: PASS.
- [ ] **Step 5: Commit** `feat(antigravity): step classification and fixtures`

---

### Task 3: Conversion

**Files:**
- Create: `internal/sources/antigravity/convert.go`
- Test: `internal/sources/antigravity/convert_test.go`

**Interfaces:**
- Consumes: `stepHead`, `RecordClass` (Task 2).
- Produces: `func splitUserInput(content string) (request string, meta map[string]string)` (keys `additional_metadata`, `settings_change`); `func modelFromSettingsChange(s string) string` (returns `"Gemini 4 Argon (High)"` for the fixture; `""` if no Model Selection change); `func stripResultHeader(content string) (body string, created, completed string)`; `func messageSender(content string) string`; `func subagentIDs(content string) []string`; `type converter struct { conversationID, parentID string; turnID string; model string; firstParentMessageSeen bool }`; `func (c *converter) convert(h stepHead, class RecordClass) (thinkt.Entry, bool)` for every class except tool results; `func (c *converter) resultEntry(h stepHead, toolUseID string) thinkt.Entry`; `func (c *converter) modelTurn(h stepHead) (thinkt.Entry, []toolCall, bool)` with `type toolCall struct { ID, Name string; Args map[string]any }`.

- [ ] **Step 1: Write the failing tests** (table-driven where it fits):

```go
func TestSplitUserInput(t *testing.T) {
	req, meta := splitUserInput(fixtureStep(t, "main.jsonl", 0).Content)
	if req != "fix the build" || !strings.Contains(meta["additional_metadata"], "16:34:22") ||
		modelFromSettingsChange(meta["settings_change"]) != "Gemini 4 Argon (High)" {
		t.Fatalf("%q %v", req, meta)
	}
	if req, _ := splitUserInput("plain text"); req != "plain text" {
		t.Fatalf("unwrapped: %q", req)
	}
}

func TestModelTurnBlocksUsageAndIDs(t *testing.T) {
	c := &converter{conversationID: "11111111-aaaa", model: "Gemini 4 Argon (High)"}
	e, calls, ok := c.modelTurn(fixtureStep(t, "main.jsonl", 2))
	// blocks: thinking, text, tool_use 2.0 view_file, tool_use 2.1 run_command
	if !ok || e.UUID != "11111111-aaaa:2" || len(e.ContentBlocks) != 4 || calls[1].ID != "2.1" ||
		e.ContentBlocks[3].ToolUseID != "2.1" || e.Usage.InputTokens != 100 ||
		e.Usage.CacheReadInputTokens != 40 || e.Usage.OutputTokens != 20 || e.Model != "Gemini 4 Argon (High)" {
		t.Fatalf("%+v", e)
	}
}

func TestModelTurnEmptyEmitsNothing(t *testing.T) {
	_, _, ok := (&converter{}).modelTurn(stepHead{Index: 3, Type: "PLANNER_RESPONSE"})
	if ok {
		t.Fatal("empty response emitted")
	}
}

func TestResultEntryStripsHeaderAndFlagsError(t *testing.T) {
	e := (&converter{conversationID: "c"}).resultEntry(fixtureStep(t, "main.jsonl", 4), "2.1")
	b := e.ContentBlocks[0]
	if e.Role != thinkt.RoleUser || b.ToolUseID != "2.1" || !b.IsError ||
		strings.Contains(b.ToolResult, "Created At") || e.Metadata["completed_at"] == nil {
		t.Fatalf("%+v", e)
	}
}

func TestSubagentIDs(t *testing.T) {
	got := subagentIDs(fixtureStep(t, "main.jsonl", 9).Content)
	if !slices.Equal(got, []string{"22222222-bbbb", "33333333-cccc"}) {
		t.Fatalf("%v", got)
	}
}

func TestConvertKindsAndVisibility(t *testing.T) {
	c := &converter{conversationID: "11111111-aaaa"}
	cases := []struct {
		idx       int
		kind      thinkt.EntryKind
		secondary bool
		emitted   bool
	}{
		{0, thinkt.KindNone, false, true},
		{1, "", false, false}, // empty history
		{5, thinkt.KindTaskNotification, kimiVisibility(thinkt.KindTaskNotification), true},
		{7, thinkt.KindAPIError, true, true},
		{10, thinkt.KindAgentMessage, kimiVisibility(thinkt.KindAgentMessage), true},
		{11, thinkt.KindMeta, true, true},
		{12, thinkt.KindCompactSummary, claudeCompactSecondary, true},
		{15, thinkt.KindUnknown, true, true},
	}
	// for each: h, class := ClassifyLine(line); e, ok := c.convert(h, class); check ok, Kind, IsSecondary()
}

func TestChildTaskPromptIsVisibleUserEntry(t *testing.T) {
	c := &converter{conversationID: "22222222-bbbb", parentID: "11111111-aaaa"}
	h, class := ClassifyLine(fixtureLines(t, "child.jsonl")[0])
	e, ok := c.convert(h, class)
	if !ok || e.Role != thinkt.RoleUser || e.IsSecondary() || !strings.Contains(e.Text, "Examine") {
		t.Fatalf("%+v", e)
	}
}
```

Before writing `TestConvertKindsAndVisibility`, read what the Kimi converter gives `KindTaskNotification` and `KindAgentMessage`, and what Claude gives `KindCompactSummary`; write those literal booleans into the table (the names `kimiVisibility`/`claudeCompactSecondary` above stand for the values you read, not helpers). Likewise read the Kimi subagent task-prompt kind and assert the same kind in `TestChildTaskPromptIsVisibleUserEntry`. The child fixture's message content must contain `Examine`.

- [ ] **Step 2: Run** `go test ./internal/sources/antigravity -run 'Split|ModelTurn|ResultEntry|SubagentIDs|ConvertKinds|ChildTask'` — Expected: FAIL to compile.
- [ ] **Step 3: Implement** per design section B. Entry fields common to all: `UUID`, `Timestamp` (`created_at`), `TurnID` (the converter's), `Source`, `Metadata["step_type"]`, `Metadata["status"]`. `ERROR_MESSAGE` text is `error`. A `USER_INPUT` sets `c.turnID` to its index and, when the settings change names a model, `c.model`.
- [ ] **Step 4: Run** the same command — Expected: PASS.
- [ ] **Step 5: Commit** `feat(antigravity): step conversion`

---

### Task 4: Streaming parser

**Files:**
- Create: `internal/sources/antigravity/parser.go`
- Test: `internal/sources/antigravity/parser_test.go`

**Interfaces:**
- Consumes: `ClassifyLine`, `converter`, `toolCall` (Tasks 2–3); `relatedTracker` (Task 5 — the parser calls `p.related.call(name, args)` and `p.related.workspace(...)` is not its job; until Task 5 exists, give `Parser` a nil-safe `related *relatedTracker` field and add the calls in Task 5).
- Produces: `func NewParser(r io.Reader, conversationID, parentID string) *Parser`; `func (p *Parser) NextEntry() (*thinkt.Entry, error)` (returns `io.EOF` at end); `func (p *Parser) UnknownTypes() map[string]int`.

- [ ] **Step 1: Write the failing tests**

```go
func TestParserFixtureEntries(t *testing.T) {
	entries := parseFixture(t, "main.jsonl", "11111111-aaaa", "")
	// 15 entries: steps 0,2,3,4,5,6,7,8,9,10,11,12,13,14,15 (step 1 is empty history)
	if len(entries) != 15 {
		t.Fatalf("got %d entries", len(entries))
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.UUID] || (e.Text == "" && len(e.ContentBlocks) == 0) {
			t.Fatalf("duplicate or empty: %+v", e)
		}
		seen[e.UUID] = true
	}
}

func TestParserPairsResultsByOrder(t *testing.T) {
	byStep := entriesByStep(parseFixture(t, "main.jsonl", "11111111-aaaa", ""))
	if byStep[3].ContentBlocks[0].ToolUseID != "2.0" || byStep[4].ContentBlocks[0].ToolUseID != "2.1" {
		t.Fatal("results 3 and 4 not paired with calls 2.0 and 2.1")
	}
}

func TestParserNewResponseDropsStaleCalls(t *testing.T) {
	byStep := entriesByStep(parseFixture(t, "main.jsonl", "11111111-aaaa", ""))
	if got := byStep[9].ContentBlocks[0].ToolUseID; got != "8.0" {
		t.Fatalf("step 9 paired with %q, want 8.0 (6.x are superseded)", got)
	}
}

func TestParserLinksSubagentChild(t *testing.T) {
	byStep := entriesByStep(parseFixture(t, "main.jsonl", "11111111-aaaa", ""))
	if byStep[8].ContentBlocks[0].ChildSessionID != "22222222-bbbb" {
		t.Fatalf("tool_use child = %q", byStep[8].ContentBlocks[0].ChildSessionID)
	}
	if ids, _ := byStep[9].Metadata["child_session_ids"].([]string); !slices.Equal(ids, []string{"22222222-bbbb", "33333333-cccc"}) {
		t.Fatalf("result child ids %v", byStep[9].Metadata["child_session_ids"])
	}
}

func TestParserTurnIDsAndModel(t *testing.T) {
	byStep := entriesByStep(parseFixture(t, "main.jsonl", "11111111-aaaa", ""))
	if byStep[2].TurnID != "0" || byStep[14].TurnID != "13" || byStep[14].Model != "Gemini 4 Argon (High)" {
		t.Fatalf("turn %q %q model %q", byStep[2].TurnID, byStep[14].TurnID, byStep[14].Model)
	}
}

func TestParserSkipsPartialLastLine(t *testing.T) {
	data := string(fixtureBytes(t, "main.jsonl")) + `{"step_index":16,"type":"PLANNER_RESP`
	p := NewParser(strings.NewReader(data), "11111111-aaaa", "")
	n := 0
	for {
		_, err := p.NextEntry()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 15 {
		t.Fatalf("got %d", n)
	}
}

func TestParserCountsUnknownTypes(t *testing.T) {
	p := NewParser(bytes.NewReader(fixtureBytes(t, "main.jsonl")), "11111111-aaaa", "")
	for _, err := p.NextEntry(); err == nil; _, err = p.NextEntry() {
	}
	if p.UnknownTypes()["MYSTERY_STEP"] != 1 {
		t.Fatalf("%v", p.UnknownTypes())
	}
}
```

The `ChildSessionID` goes on the `invoke_subagent` tool_use block of the step-8 entry, which the parser has already emitted when step 9 arrives. Hold each model-turn entry until its calls are answered or the next model turn / user input / EOF arrives, then emit it; results are emitted after it. This is the only buffering the parser does.

- [ ] **Step 2: Run** `go test ./internal/sources/antigravity -run Parser` — Expected: FAIL to compile.
- [ ] **Step 3: Implement** with `thinkt.NewScannerWithMaxCapacityCustom`; skip `ClassInvalid` lines.
- [ ] **Step 4: Run** the same command — Expected: PASS.
- [ ] **Step 5: Commit** `feat(antigravity): streaming parser`

---

### Task 5: Discovery, related folders, resume, gemini boundary

**Files:**
- Create: `internal/sources/antigravity/discovery.go`, `related.go`, `resume.go`
- Modify: `internal/sources/antigravity/parser.go` (feed the tracker), `internal/sources/gemini/discovery.go`
- Test: `internal/sources/antigravity/discovery_test.go`, `related_test.go`, `resume_test.go`, `internal/sources/gemini/discovery_test.go`

**Interfaces:**
- Produces: `func DefaultDir() (string, error)` (`THINKT_ANTIGRAVITY_HOME`, else `~/.gemini/antigravity-cli`); `func IsSessionPath(path string) bool`; `func BaseDirForPath(path string) string`; `func SessionIDForPath(path string) string`; `func sessionFile(baseDir, id string) string` (full, else plain, else `""`); `Discoverer`, `Factory()`; `func unquotePath(s string) string`; `type relatedTracker struct{…}`; `func newRelatedTracker(projectPath string, exclude []string) *relatedTracker`; `func (r *relatedTracker) call(name string, args map[string]any)`; `func (r *relatedTracker) workspace(path string)`; `func (r *relatedTracker) paths() []thinkt.RelatedPath`; `func repoRoot(path string) string` (nearest ancestor dir containing `.git`, `""` if none); `func (s *Store) ResumeCommand(thinkt.SessionMeta) (*thinkt.ResumeInfo, error)` (Store from Task 7; write the test now against a zero `Store{}`).
- Tool → reason, arg: `view_file.AbsolutePath`, `read_file.Path`, `list_dir.DirectoryPath`, `grep_search.SearchPath` → `read`; `replace_file_content.TargetFile`, `multi_replace_file_content.TargetFile`, `write_to_file.TargetFile`, `edit_file.Path`, `write_file.Path` → `edited`; `run_command.Cwd` → `command`. (Edit arguments cover every `CODE_ACTION` result's path, so result text is not parsed; ruling against design H's "and the path named in a `CODE_ACTION` result".)

- [ ] **Step 1: Write the failing tests**

```go
func TestIsSessionPath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "antigravity-cli")
	t.Setenv("THINKT_ANTIGRAVITY_HOME", home)
	logs := filepath.Join(home, "brain", "c1", ".system_generated", "logs")
	for path, want := range map[string]bool{
		filepath.Join(logs, "transcript_full.jsonl"):                          true,
		filepath.Join(logs, "transcript.jsonl"):                               true,
		filepath.Join(logs, "chunks", "transcript_full", "00000001.jsonl"):   false,
		filepath.Join(home, "brain", "c1", "task.md.metadata.json"):           false,
		filepath.Join(home+"x", "brain", "c1", ".system_generated", "logs", "transcript.jsonl"): false,
	} {
		if IsSessionPath(path) != want {
			t.Errorf("%s: want %v", path, want)
		}
	}
	if SessionIDForPath(filepath.Join(logs, "transcript.jsonl")) != "c1" ||
		BaseDirForPath(filepath.Join(logs, "transcript.jsonl")) != home {
		t.Fatal("id or base")
	}
}

func TestSessionFilePrefersFull(t *testing.T) // both present → full; only plain → plain; none → ""

func TestUnquotePath(t *testing.T) {
	for in, want := range map[string]string{`"/w/a"`: "/w/a", "/w/a": "/w/a", `""`: ""} {
		if unquotePath(in) != want {
			t.Errorf("%q", in)
		}
	}
}

func TestRelatedTrackerFoldsToRepoRoots(t *testing.T) {
	root := t.TempDir()
	proj, other := mkRepo(t, root, "ds4dg"), mkRepo(t, root, "ds4-go") // dirs with .git
	r := newRelatedTracker(proj, []string{filepath.Join(root, "home"), "/tmp"})
	r.workspace(other)
	r.call("replace_file_content", map[string]any{"TargetFile": filepath.Join(other, "x", "a.go")})
	r.call("run_command", map[string]any{"Cwd": `"` + other + `"`})
	r.call("view_file", map[string]any{"AbsolutePath": filepath.Join(proj, "b.go")}) // own project: ignored
	got := r.paths()
	want := []thinkt.RelatedPath{{Path: other, Reasons: []string{"command", "edited", "workspace"}, Count: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
}

func TestRelatedFolderSkipsMissingTempAndHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	r := newRelatedTracker(mkRepo(t, root, "p"), []string{home, "/tmp"})
	r.call("view_file", map[string]any{"AbsolutePath": "/no/such/repo/a.go"})
	r.call("view_file", map[string]any{"AbsolutePath": filepath.Join(os.TempDir(), "x.go")})
	r.call("write_to_file", map[string]any{"TargetFile": filepath.Join(home, "brain", "c1", "plan.md")})
	if got := r.paths(); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestResumeCommand(t *testing.T) {
	info, err := (&Store{}).ResumeCommand(thinkt.SessionMeta{ID: "c1", ProjectPath: "/w/ds4dg"})
	if err != nil || info.Command != "agy" || !slices.Equal(info.Args, []string{"--conversation", "c1"}) || info.Dir != "/w/ds4dg" {
		t.Fatalf("%+v %v", info, err)
	}
}
```

`Reasons` are sorted alphabetically; `Count` counts tool calls (the `workspace` relation adds a reason, not a count). Paths sort by `Count` descending, then path. The excluded prefixes are a parameter, because test repos live under the system temp dir: the signature is `newRelatedTracker(projectPath string, exclude []string)`, the store passes `[home, os.TempDir(), "/tmp", "/private/tmp"]`, and the tests above pass `[]string{home, "/tmp"}`. Match `ResumeInfo` field names to the Kimi `resume.go`.

Add to `internal/sources/gemini/discovery_test.go`:

```go
func TestIsSessionPathSkipsAntigravityHome(t *testing.T) {
	base := filepath.Join(t.TempDir(), ".gemini")
	t.Setenv("THINKT_GEMINI_HOME", base)
	t.Setenv("THINKT_ANTIGRAVITY_HOME", "")
	if IsSessionPath(filepath.Join(base, "antigravity-cli", "settings.json")) {
		t.Fatal("claimed default antigravity home")
	}
	if !IsSessionPath(filepath.Join(base, "tmp", "s.json")) {
		t.Fatal("lost a real gemini path")
	}
}
```

With `THINKT_ANTIGRAVITY_HOME` empty, `antigravity.DefaultDir()` resolves to `~/.gemini/antigravity-cli` from the real home, not from `THINKT_GEMINI_HOME`; so `gemini.IsSessionPath` excludes both the resolved Antigravity home and `<gemini base>/antigravity-cli`. `gemini` imports `antigravity`; `antigravity` must not import `gemini`.

- [ ] **Step 2: Run** `go test ./internal/sources/antigravity ./internal/sources/gemini -run 'IsSessionPath|SessionFile|Unquote|Related|Resume'` — Expected: FAIL.
- [ ] **Step 3: Implement**; in the parser, call `p.related.call(c.Name, c.Args)` for each tool call of each model turn; `Parser.RelatedPaths() []thinkt.RelatedPath`; `NewParser` gains no parameter — add `func (p *Parser) TrackRelated(projectPath string, exclude []string)` which the session scan calls before reading.
- [ ] **Step 4: Run** the same command plus `go test ./internal/sources/antigravity/...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(antigravity): discovery, related folders, resume; gemini skips the antigravity home`

---

### Task 6: Summaries, workspace resolution, session scan

**Files:**
- Create: `internal/sources/antigravity/summaries.go`, `sessionstate.go`, `testdata/summaries.sql`
- Test: `internal/sources/antigravity/sessionstate_test.go`

**Interfaces:**
- Produces: `type summaryRow struct { ID, Title, Preview, ParentID, AgentName string; Workspaces []string; Modified time.Time; StepCount int }`; `func loadSummaries(baseDir string) (map[string]summaryRow, error)` (paths from `file://` URIs; zero time → zero `time.Time`); `func loadLastConversations(baseDir string) map[string]string` (conversation id → workspace path); `type sessionScan struct { FirstPrompt, LastPrompt, Model string; EntryCount int; Cost *thinkt.SessionCost; Cwd string; CreatedAt time.Time; Related []thinkt.RelatedPath }`; `func scanSessionFile(path, conversationID, parentID, projectPath string, exclude []string) sessionScan` (`Cwd` = most common unquoted `run_command` `Cwd`, ties → first seen); `func resolveWorkspace(row summaryRow, scanCwd string, last map[string]string, id string) string` (`""` means no workspace); `applyTo`, `cached` as in Kimi plus `RelatedPaths`.
- `testdata/summaries.sql`: the `CREATE TABLE conversation_summaries` statement from the evidence (copy the schema verbatim from the real db with `sqlite3 'file:…?immutable=1' .schema`) and rows for `11111111-aaaa` (title `"Fix Build"`, workspaces `["file:///w/ds4dg","file:///w/ds4-go"]`), `22222222-bbbb` (parent `11111111-aaaa`, agent `research`, title `""`, preview `"Research Child"`), `44444444-dddd` (no workspaces, title `""`, preview `""`, `last_modified_time` `0001-01-01 00:00:00+00:00`). Tests build the db with `database/sql` from this file in a temp home.

- [ ] **Step 1: Write the failing tests**

```go
func TestLoadSummaries(t *testing.T) {
	home := testHome(t) // temp home with brain dirs for the three fixtures and the db
	rows, err := loadSummaries(home)
	r := rows["11111111-aaaa"]
	if err != nil || r.Title != "Fix Build" || !slices.Equal(r.Workspaces, []string{"/w/ds4dg", "/w/ds4-go"}) ||
		rows["22222222-bbbb"].ParentID != "11111111-aaaa" || !rows["44444444-dddd"].Modified.IsZero() {
		t.Fatalf("%+v %v", rows, err)
	}
}

func TestWorkspaceFirstEntryWins(t *testing.T) {
	if got := resolveWorkspace(summaryRow{Workspaces: []string{"/w/ds4dg", "/w/ds4-go"}}, "/w/x", nil, "c"); got != "/w/ds4dg" {
		t.Fatal(got)
	}
}

func TestWorkspaceFallsBackToCwd(t *testing.T) {
	s := scanSessionFile(fixturePath(t, "noworkspace.jsonl"), "44444444-dddd", "", "", nil)
	if s.Cwd != "/w/old" || resolveWorkspace(summaryRow{}, s.Cwd, nil, "44444444-dddd") != "/w/old" {
		t.Fatalf("%q", s.Cwd)
	}
}

func TestWorkspaceFallsBackToLastConversations(t *testing.T) {
	if got := resolveWorkspace(summaryRow{}, "", map[string]string{"c9": "/w/booba"}, "c9"); got != "/w/booba" {
		t.Fatal(got)
	}
}

func TestScanSessionFileSummarises(t *testing.T) {
	s := scanSessionFile(fixturePath(t, "main.jsonl"), "11111111-aaaa", "", "/w/ds4dg", nil)
	if s.EntryCount != 15 || s.FirstPrompt != "fix the build" || s.LastPrompt != "thanks" ||
		s.Model != "Gemini 4 Argon (High)" || s.Cost.InputTokens != 100 || s.CreatedAt.IsZero() {
		t.Fatalf("%+v", s)
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/sources/antigravity -run 'Summaries|Workspace|ScanSession'` — Expected: FAIL.
- [ ] **Step 3: Implement**; `summaries.go` blank-imports `github.com/mattn/go-sqlite3` and opens `file:<path>?mode=ro&_busy_timeout=200`; any open or query error returns an empty map and the error (callers log and continue).
- [ ] **Step 4: Run** the same command — Expected: PASS.
- [ ] **Step 5: Commit** `feat(antigravity): summaries db, workspace resolution, session scan`

---

### Task 7: Store

**Files:**
- Create: `internal/sources/antigravity/store.go`
- Test: `internal/sources/antigravity/store_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces: `type Store`; `NewStore(baseDir string) *Store`; `NewStoreWithCacheDir(baseDir, cacheDir string) *Store`; the full `thinkt.Store` interface following `internal/sources/kimi/store.go` (cache, `SetCacheTTL`, `ResetCache`, enrichment with `WithEnrichSync`/`WithEnrich`, `WithSubagents`, `GetSessionMeta` by id or path, `LoadSession`, `OpenSession` by id or path, `WatchConfig{IncludeDirs: []string{"brain"}, ExcludeDirs: []string{"chunks"}, MaxDepth: 4}`). Project id and path are the workspace path; the no-workspace project has id and path `""` and name `"(no workspace)"`. `Workspace()` id: `installation_id` file contents, trimmed. Listing `ModifiedAt`: summary `Modified`, else transcript mtime. Title: summary title, else preview, else first prompt (after enrichment).

- [ ] **Step 1: Write the failing tests**

```go
func TestListProjectsByFolder(t *testing.T)            // projects "/w/ds4dg" (1 session, SubagentCount 1 on it), "/w/old" (44444444-dddd via Cwd); no "/w/ds4-go" project
func TestListSessionsHidesSubagentsByDefault(t *testing.T) // default: 11111111-aaaa only under /w/ds4dg; WithSubagents: child listed with ParentSessionID 11111111-aaaa, ProjectPath /w/ds4dg (inherits parent's)
func TestListSessionsTitleFallbacks(t *testing.T)      // "Fix Build"; child "Research Child"; 44444444-dddd → its first prompt after WithEnrichSync
func TestListSessionsWithoutSummaryDB(t *testing.T)    // delete the db: all three conversations still listed; parent link lost is acceptable
func TestListSessionsRelatedPathsSurviveCache(t *testing.T) // EnrichSync twice on a home whose fixture uses a real temp repo for __REPO_B__: RelatedPaths equal and non-empty both times
func TestGetSessionMetaByIDAndPath(t *testing.T)
func TestOpenSessionReadsFullTranscript(t *testing.T)  // both files present with different content; entries come from transcript_full.jsonl
func TestEmptyBrainDirsAreSkipped(t *testing.T)        // brain dir with only scratch/: not listed
```

Write each with explicit `want` values as in the comments; reuse `testHome(t)`.

- [ ] **Step 2: Run** `go test ./internal/sources/antigravity -run 'List|GetSessionMeta|OpenSession|EmptyBrain'` — Expected: FAIL.
- [ ] **Step 3: Implement.** A subagent's project is its parent's resolved workspace when its own row has none.
- [ ] **Step 4: Run** `go test ./internal/sources/antigravity/...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(antigravity): store`

---

### Task 8: Registration and the "Related" summary line

**Files:**
- Modify: `internal/sources/sources.go`, `internal/tui/styles.go` (color `#3cc8b4`), `internal/tui/session_loader.go` (check `antigravity.IsSessionPath` before gemini; `openAntigravityLazySession` like `openKimiLazySession`), `internal/thinkt/active.go` (`"agy": SourceAntigravity`), `internal/index/search/search.go` (`searchFilters[antigravity] = antigravity.NewSearchFilter`), `internal/cmd/help_env.go` (`{"THINKT_ANTIGRAVITY_HOME", "~/.gemini/antigravity-cli", "Override Antigravity CLI data directory"}`), `internal/cmd/sources.go` (list line `Antigravity CLI (~/.gemini/antigravity-cli)` in both places Kimi appears), `internal/cli/sessions.go` (`SessionSummaryData.Related []string` formatted `"<path> (<reasons joined by ", ">)"`; template line `{{if .Related}}\n  Related:  {{join .Related "; "}}{{end}}` before Summary; help text entry `{{.Related}}`)
- Test: `internal/tui/session_loader_antigravity_test.go`, `internal/index/search/bookkeeping_test.go`, `internal/cli/sessions_test.go` (or the existing summary test file), `internal/thinkt/active_test.go` if one covers `knownProcesses`

- [ ] **Step 1: Write the failing tests**

```go
func TestOpenLazySessionAntigravity(t *testing.T)       // THINKT_ANTIGRAVITY_HOME temp home; OpenLazySession(<full transcript path>) reads entries; THINKT_GEMINI_HOME set to its parent so the gemini check would also match
func TestScanFileAntigravitySkipsHarnessLines(t *testing.T) // "needle" in steps 0 (user), 7 (error), 11 (ephemeral): one match, line 1
func TestFormatSummaryPrintsRelated(t *testing.T)       // meta with RelatedPaths [{/w/ds4-go, [command edited], 2}] → output contains "Related:  /w/ds4-go (command, edited)"
func TestKnownProcessesAgy(t *testing.T)               // knownProcesses["agy"] == SourceAntigravity
```

- [ ] **Step 2: Run** `go test ./internal/tui ./internal/index/search ./internal/cli ./internal/thinkt -run 'Antigravity|Related|Agy'` — Expected: FAIL.
- [ ] **Step 3: Implement**; regenerate swagger the way the repo does (`grep -n swag Taskfile.yml`), so `SourceAntigravity` appears in the enum.
- [ ] **Step 4: Run** `go build ./... && go test ./...` — Expected: PASS except the two pre-existing iTerm tests (`TestAppsList_JSON`, `TestSetTerminalRoundTrip`).
- [ ] **Step 5: Commit** `feat(antigravity): register the source; related folders in the session summary`

---

### Task 9: Real-tree measurement and docs

**Files:**
- Create: `internal/sources/antigravity/README.md`
- Modify: `AGENTS.md`, `README.md`, `DEVELOP.md`, `etc/reports/SOURCE_FORMATS_2026-10.md`, `PLAN.md` (nothing new; check the two roadmap items are present)
- Scratch: a measurement program in the session scratchpad (not committed) that opens `antigravity.NewStore("")` and walks every session with `WithSubagents()`

- [ ] **Step 1: Measure** on `~/.gemini/antigravity-cli`: sessions listed (expect 105 transcripts: main + 5 subagents); every transcript parses with no error; entries with empty text and no blocks = 0; duplicate UUIDs per session = 0; tool_result entries whose `ToolUseID` is not a tool_use in the same session = 0; unknown types = 0; the 5 subagents carry `ParentSessionID` and each parent's `invoke_subagent` tool_use links a child; projects listed (each a folder path, no Antigravity project ids); sessions in the no-workspace project (expect ≈5); a conversation known to span ds4dg and ds4-go shows `ds4-go` in `RelatedPaths`. Record the numbers in the ledger.
- [ ] **Step 2: Fix** anything the measurement finds, each with a failing test first, one commit per fix.
- [ ] **Step 3: Run** `thinkt sync` against a temp `THINKT_HOME` copy of the index (or the real one if the user's index is the norm in earlier passes) and search for a phrase from a recent Antigravity prompt; it is found.
- [ ] **Step 4: Write docs**: package README (layout, step-class table, pairing rule, workspace rule, subagents, related folders, what is not read); `AGENTS.md` source list, env table, package table, and the classification paragraph; `README.md` and `DEVELOP.md` source lists; report Summary row "after the Antigravity pass: …".
- [ ] **Step 5: Run** `go test ./...` — Expected: PASS except the two iTerm tests. Commit `docs(antigravity): README; source lists; report row`.
