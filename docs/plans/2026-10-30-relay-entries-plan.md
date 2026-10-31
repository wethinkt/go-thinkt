# Relay Ships Parsed Entries Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The relay reads every enabled source's sessions through its store and ships full `thinkt.Entry` values with session metadata, diffed against what it last shipped. The collector archives them keyed by machine, source, session and uuid, keeping removed entries as superseded.

**Architecture:**
- A shared wire package (`internal/tracewire`) defines the version 2 payload; the relay builds it and the collector decodes it.
- The relay replaces byte-offset tailing with a per-session state (entry hashes, file mtime and size, generation), and reconciles at start.
- The collector replaces its two trace tables with `sessions` and `entries`. Existing endpoints read from them.

**Tech Stack:** Go, DuckDB (existing driver), `net/http/httptest` for tests.

**Spec:** `docs/plans/2026-10-30-relay-entries-design.md`

## Global Constraints

- **Wire:** `POST /v1/traces` body is `tracewire.Payload` with `"version": 2`. Any other version → HTTP 400 with error `unsupported_version` and a message saying the relay must be upgraded.
- **Keys:** an entry is `(machine_id, source, session_id, uuid)`; a session is `(machine_id, source, session_id)`. `session_id` is the store's `SessionMeta.ID`.
- **Removal:** `removed` lists only uuids the relay shipped before that the source, still returning the session, no longer produces. A vanished session sends nothing; the collector keeps everything.
- **Superseded:** collector `--superseded keep|delete` (env `THINKT_COLLECTOR_SUPERSEDED`), default `keep`. Switching to `delete` does not purge rows already marked.
- **Backfill:** relay `--backfill all|none` (env `THINKT_RELAY_BACKFILL`), default `all`.
- **Generation:** per session, +1 per payload, sent as `session.generation`. The collector ignores a payload whose generation is lower than the stored one, and still answers 200.
- **One relay per machine:** a new `config.InstanceRelay` instance type (`"relay"`). A second relay on the machine refuses to start with `relay already running (pid N)`.
- **Sources:** only sources in the relay's registry (enabled in config) ship. On start, buffered payloads of other sources are removed.
- **Request size:** the collector accepts up to 64 MB per request. The relay splits a batch so each encoded payload is at most 8 MB, as well as at most `BatchSize` entries.
- **Tests:** `internal/cmd` tests run through `sh /private/tmp/claude-501/-Users-evan-projects-go-thinkt/9b31cdbd-5d8b-4c00-8fe9-2e0b58718557/scratchpad/agy/cmdtest2.sh '<pattern>'` (edit its cd target to this worktree first). Known failures: `TestAppsList_JSON`, `TestSetTerminalRoundTrip`.

## Review Focus

1. **A collector down for a long time while sessions are rewritten:** buffered payloads drain in order. The final collector state equals the sessions' current entries, plus older entries marked superseded. (Task 7 end-to-end test, buffered path.)
2. **The relay restarted after files changed while it was off:** startup reconcile ships only those sessions; unchanged sessions ship nothing. (Task 5 test.)
3. **A very large tool result** (one entry over 8 MB): it ships alone in its own payload, and the collector accepts it. (Task 4 test with a 9 MB entry.)
4. **Two machines with the same session id:** they are two session rows and two sets of entries. (Task 2 test.)
5. **Old relay against the new collector:** clear 400 with `unsupported_version`, not a silent drop. (Task 3 test.)

---

### Task 1: Wire types

**Files:**
- Create: `internal/tracewire/tracewire.go`
- Test: `internal/tracewire/tracewire_test.go`

**Interfaces:** Produces:

```go
const Version = 2

type Session struct {
    ID              string              `json:"id"`
    ProjectPath     string              `json:"project_path,omitempty"`
    ParentSessionID string              `json:"parent_session_id,omitempty"`
    Title           string              `json:"title,omitempty"`
    Model           string              `json:"model,omitempty"`
    CreatedAt       time.Time           `json:"created_at,omitzero"`
    ModifiedAt      time.Time           `json:"modified_at,omitzero"`
    EntryCount      int                 `json:"entry_count,omitempty"`
    Cost            *thinkt.SessionCost `json:"cost,omitempty"`
    Generation      int64               `json:"generation"`
}

type Payload struct {
    Version    int            `json:"version"`
    InstanceID string         `json:"instance_id"`
    MachineID  string         `json:"machine_id"`
    Source     string         `json:"source"`
    Session    Session        `json:"session"`
    Entries    []thinkt.Entry `json:"entries,omitempty"`
    Removed    []string       `json:"removed,omitempty"`
}

func SessionFromMeta(m thinkt.SessionMeta) Session // Generation left 0
func EntryHash(e thinkt.Entry) string             // hex SHA-256 of json.Marshal(e)
```

- [ ] **Step 1: Failing tests:**
  - `TestPayloadRoundTrip`: a payload with one entry (content blocks, usage, `TurnID`) survives marshal and unmarshal (`reflect.DeepEqual`).
  - `TestEntryHashStable`: the same entry twice gives the same hash; a changed `Text` gives a different one; `Metadata` maps with keys inserted in different orders give the same hash.
  - `TestSessionFromMeta`: copies `ID`, `ProjectPath`, `ParentSessionID`, `Title`, `Model`, `CreatedAt`, `ModifiedAt`, `EntryCount` and `Cost`.
- [ ] **Step 2: Run** `go test ./internal/tracewire/`. Expected: FAIL (package missing).
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `feat(tracewire): version 2 relay payload`.

### Task 2: Collector store over sessions and entries

**Files:**
- Modify: `internal/collect/store_duckdb.go` (schema, write path, queries), `internal/collect/types.go` (`TraceStore`, `SessionFilter`, `SessionSummary`)
- Rename: `IngestEntry` → `EntrySummary` (same JSON fields, plus `ContentBlocks []thinkt.ContentBlock 'json:"content_blocks,omitempty"'` and `Superseded bool 'json:"superseded,omitempty"'`)
- Test: `internal/collect/store_duckdb_test.go` (old ingest tests rewritten for v2)

**Interfaces:**
- `TraceStore.IngestBatch(ctx, p tracewire.Payload) error`.
- `QueryEntries(ctx, sessionID string, limit, offset int) ([]EntrySummary, error)` returns live entries only, in timestamp order. It is keyed by session id alone; across machines the result is merged, as the spec accepts.
- `SessionSummary` gains `MachineID string 'json:"machine_id,omitempty"'` and `Title string 'json:"title,omitempty"'`.
- `NewDuckDBStore(dbPath string, batchSize int, flushInterval time.Duration, superseded SupersededMode)`, where `type SupersededMode string` with consts `SupersededKeep = "keep"` and `SupersededDelete = "delete"`.
- `func summarize(e thinkt.Entry) EntrySummary`, used by Task 3's stream:
  - `ToolName` is the first `tool_use` block's name.
  - `InputTokens`/`OutputTokens` come from `Usage`.
  - `HasThinking`/`ThinkingLen` come from the thinking blocks.
  - `HasToolUse` and `IsError` are set when any block is a tool use or an error.

**Schema** (replaces `collected_sessions` and `collected_entries`; both are dropped at startup if present):
- `sessions`: the spec §4 columns plus `generation BIGINT`, `status`, `last_activity`, `source_last_seen`. Primary key `(machine_id, source, session_id)`.
- `entries`: the spec §4 columns. Primary key `(machine_id, source, session_id, uuid)`.
- `session_activity` gains `machine_id VARCHAR`, added with `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` for existing files.

**Write path** (inside the existing single-writer batch transaction):
1. Read the session's stored `generation`. If `p.Session.Generation` is lower, skip the payload.
2. Upsert the session row: metadata, `generation`, `last_seen = now`, `source_last_seen = p.Session.ModifiedAt`.
3. Upsert each entry: `entry_json`, derived columns, `content_hash` (`tracewire.EntryHash`), `updated_at = now`, `superseded_at = NULL`.
4. For `Removed`: under keep, `UPDATE ... SET superseded_at = now WHERE key AND superseded_at IS NULL`; under delete, `DELETE`.

- [ ] **Step 1: Failing tests** (temp DuckDB file; `flushInterval` short; a helper that waits for the writer):
  - `TestIngestUpsertsByKey`: ingest one session with 2 entries, then again with entry 1 changed → 2 rows, entry 1 has the new text.
  - `TestIngestRemovedKeepMarksSuperseded`: removed `["u2"]` → `QueryEntries` returns only `u1`; the row for `u2` has `superseded_at` set.
  - `TestIngestRemovedDelete`: same with `SupersededDelete` → the `u2` row is gone.
  - `TestIngestReaddClearsSuperseded`.
  - `TestIngestStaleGenerationIgnored`: generation 5 then generation 4 with other text → the text of generation 5 remains.
  - `TestTwoMachinesSameSessionID` (Review Focus 4): two rows in `QuerySessions`, and 4 entries total.
  - `TestDropsOldTables`: a DB file with an old `collected_entries` table opens, and the table is gone.
  - Existing search, stats, active-sessions and parquet tests ported to v2 payloads with the same assertions.
- [ ] **Step 2: Run** `go test ./internal/collect/`. Expected: FAIL (compile).
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `feat(collect): archive full entries keyed by machine, source, session and uuid`.

### Task 3: Collector HTTP, settings, stream, activity

**Files:**
- Modify: `internal/collect/ingest.go` (v2 decode, version check, 64 MB limit, metrics from `Usage`), `internal/collect/normalizer.go` (validate a v2 payload)
- Modify: `internal/collect/pubsub.go` and `ws.go`, which publish and backfill `[]EntrySummary`
- Modify: `internal/collect/types.go` (`CollectorConfig.Superseded SupersededMode`; `SessionActivityEvent` gains `MachineID`)
- Modify: `internal/cmd/collect.go` and `cmd/thinkt-collector/main.go` (`--superseded` flag and env)
- Test: `internal/collect/normalizer_test.go`, plus a new `internal/collect/ingest_test.go`

**Interfaces:**
- Consumes Task 1 and Task 2.
- `NormalizePayload(p *tracewire.Payload) (dropped int, err error)`:
  - Errors on an empty `MachineID`, `Source` or `Session.ID`.
  - Drops entries with an empty `UUID`.
  - Trims whitespace in `Session.ProjectPath` and `Title`.
- `RecordSessionActivity` matches the session row on `(machine_id, source, session_id)`.

- [ ] **Step 1: Failing tests** (`httptest.NewServer(s.setupRouter())`):
  - `TestIngestRejectsOldVersion` (Review Focus 5): a v1 body → 400 and `"unsupported_version"`.
  - `TestIngestAcceptsLargePayload`: a 20 MB v2 body → 200.
  - `TestIngestPublishesSummaries`: subscribe to the session, ingest → one `EntrySummary` with `Role`, `Text` and `ContentBlocks`.
  - `TestSupersededSettingFromEnv`: `THINKT_COLLECTOR_SUPERSEDED=delete` → config mode delete; an invalid value → error at startup.
  - `TestActivityByFullKey`: activity for machine A does not change machine B's row with the same session id.
  - `TestNormalizePayload`: the rules above.
- [ ] **Step 2: Run** `go test ./internal/collect/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/collect/ && go build ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(collect): v2 ingest, superseded setting, stream summaries, activity by session key`.

### Task 4: Relay state and session sync

**Files:**
- Create: `internal/relay/state.go`, `internal/relay/sync.go`
- Modify: `internal/relay/types.go` (`TracePayload`/`TraceEntry` removed; the buffer and shipper use `tracewire.Payload`), `buffer.go`, `shipper.go`
- Test: `internal/relay/state_test.go`, `internal/relay/sync_test.go`

**Interfaces:**

```go
type sessionState struct {
    Hashes     map[string]string `json:"hashes"` // uuid → EntryHash
    ModTime    time.Time         `json:"mod_time"`
    Size       int64             `json:"size"`
    Generation int64             `json:"generation"`
    Path       string            `json:"path"`
}
type relayState struct { Sessions map[string]*sessionState `json:"sessions"` } // key: source + "\x00" + session id
func loadState(path string) (*relayState, error)   // missing file → empty state
func (s *relayState) save(path string) error       // atomic: temp file + rename
func stateKey(source thinkt.Source, sessionID string) string

// diffSession compares a session's current entries with its state and
// returns the payloads to ship, already split by BatchSize and 8 MB, each
// with its own generation (prev+1, prev+2, …). It does not mutate st;
// commit applies a shipped payload to it.
func diffSession(meta thinkt.SessionMeta, entries []thinkt.Entry, st *sessionState, base tracewire.Payload, batchSize int) []tracewire.Payload
func (st *sessionState) commit(p tracewire.Payload)
```

The state file is `<buffer dir parent>/state.json` (`~/.thinkt/relay/state.json` by default). It is one file. (Ruling at execution if it grows large: ledger the size seen on real data.)

**`diffSession` rules:**
- An unchanged session with known metadata → no payload.
- The first payload carries the session metadata.
- `Removed` goes on the last payload.
- A single entry larger than 8 MB goes alone in its own payload.

- [ ] **Step 1: Failing tests:**
  - `TestStateRoundTrip`.
  - `TestStateMissingFileIsEmpty`.
  - `TestDiffNewSession`: 3 entries, empty state → one payload with 3 entries, generation 1, no `Removed`.
  - `TestDiffNoChange`: after commit → no payload.
  - `TestDiffChangedAndRemoved`: change `u1`, drop `u3` → one payload, `Entries` = [`u1`], `Removed` = [`u3`], generation 2.
  - `TestDiffSplitsByBatchSize`: 250 entries with batch size 100 → 3 payloads with generations 1, 2, 3; `Removed` only on the last.
  - `TestDiffOversizedEntryAlone` (Review Focus 3): a 9 MB entry between two small ones → the big one is in a payload by itself.
- [ ] **Step 2: Run** `go test ./internal/relay/ -run 'State|Diff'`. Expected: FAIL.
- [ ] **Step 3: Implement.** Port the buffer and shipper to `tracewire.Payload` (same file layout and retry behaviour).
- [ ] **Step 4: Run** `go test ./internal/relay/`. Expected: PASS (old line-parsing tests deleted with `parseRawEntry`; ledger which).
- [ ] **Step 5: Commit** `feat(relay): per-session state and entry diffs`.

### Task 5: Relay reads through stores; reconcile and backfill

**Files:**
- Modify: `internal/relay/relay.go`:
  - `New` takes the registry.
  - `processFile` → `syncPath`.
  - `ExportOnce` → `reconcile`.
  - Remove `parseRawEntry`, `extractProjectPath` and `sessionIDFromPath`.
- Modify: `internal/relay/types.go` (`RelayConfig.Registry *thinkt.StoreRegistry`, `RelayConfig.Backfill string`, `RelayConfig.StatePath string`)
- Modify: `internal/cmd/relay.go` (`--backfill` flag and env; pass `CreateSourceRegistry()`), `cmd/thinkt-relay/main.go` (registry from `cmd.CreateSourceRegistry()`, filtered by its `--source` flag; `--backfill`)
- Test: `internal/relay/relay_test.go` (rewritten)

**Interfaces:**
- `func (e *Relay) syncPath(ctx context.Context, path string)`:
  1. Resolve with `sources.ResolveSessionPath`; skip a source not in the registry.
  2. `store.GetSessionMeta(ctx, path)`; nil → prune any state for that path and return.
  3. `OpenSession` and read every entry.
  4. Run `diffSession`, then ship or buffer each payload. `commit` runs after each accepted or buffered payload.
  5. Save the state.
- `func (e *Relay) reconcile(ctx context.Context) error`:
  1. For each store in the registry: `ListProjects`, then `ListSessions(…, thinkt.WithSubagents())`.
  2. For each session: stat `FullPath`.
     - Not in the state: backfill `all` → `syncPath`; backfill `none` → record a state with no hashes but the current mtime and size, ship nothing.
     - mtime or size differs from the state → `syncPath`.
  3. Afterwards, prune state entries whose file is gone.
- `Start` calls `reconcile` before the watch loop. One-shot runs `reconcile`, then `FlushBuffer`.

- [ ] **Step 1: Failing tests** (`httptest` collector stub that records payloads; temp homes from source `testdata` via `t.Setenv` for the Claude, Codex, Kimi and Gemini homes):
  - `TestReconcileShipsEverySource`: one session each of Claude, Codex, Kimi and Gemini → payloads for 4 sessions, each with entry counts equal to the store's `LoadSession` and session ids equal to the stores' IDs (Kimi not `wire`).
  - `TestReconcileSkipsUnchanged` (Review Focus 2): run twice → second run ships nothing; touch one file → only that session ships.
  - `TestBackfillNone`: first run ships nothing; appending to a session file then running ships only the change.
  - `TestVanishedSessionSendsNothing`: delete a file after the first sync → no `Removed`, and the state entry is gone.
  - `TestDisabledSourceNotShipped`: the registry without Codex → no Codex payload.
  - `TestGeminiRewindShipsRemoved`: a Gemini session, then append `$rewindTo` → payload with `Removed` for the rewound uuids.
- [ ] **Step 2: Run** `go test ./internal/relay/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/relay/ && go build ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(relay): ship every enabled source through its store; reconcile and backfill`.

### Task 6: One relay per machine, buffered sources, activity events

**Files:**
- Modify: `internal/config/instances.go` (`InstanceRelay InstanceType = "relay"`)
- Modify: `internal/relay/relay.go` (register on start and unregister on stop; refuse when a live relay instance exists; on start, drop buffered payloads of sources not in the registry; activity events use the resolved session id, `MachineID` and `Source`)
- Modify: `internal/relay/buffer.go` (`func (b *DiskBuffer) Discard(keep func(tracewire.Payload) bool) (removed int, err error)`)
- Test: `internal/relay/relay_test.go`, `internal/relay/buffer_test.go`

- [ ] **Step 1: Failing tests:**
  - `TestSecondRelayRefuses`: register a fake live relay instance (the current pid) in a temp `THINKT_HOME` → `Start` returns an error containing `relay already running`.
  - `TestBufferDiscardDisabledSources`: buffer holds codex and claude payloads; keep claude → 1 removed.
  - `TestActivityEventUsesSessionID`: a Kimi wire write → the activity event's `SessionID` is the Kimi store id and its `MachineID` is set.
- [ ] **Step 2: Run** `go test ./internal/relay/ ./internal/config/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `feat(relay): one relay per machine; drop buffered payloads of disabled sources; activity by session id`.

### Task 7: End to end, docs

**Files:**
- Create: `internal/relay/e2e_test.go` (an in-process collector: a real `collect.Server` over a temp DuckDB, served by `httptest`)
- Modify: `AGENTS.md`:
  - the relay and collector sections: the v2 payload, keys, superseded, backfill, one relay per machine
  - drop the "relay still cannot ship Antigravity or Gemini" sentence
  - the DuckDB schema list
- Modify: `internal/sources/gemini/README.md` and `internal/sources/antigravity/README.md` (relay lines)

- [ ] **Step 1: Failing tests:**
  - `TestEndToEndAllSources`: run the relay's reconcile against the collector. `QuerySessions` returns Claude, Codex, Kimi, Antigravity and Gemini sessions with the stores' ids. `QueryEntries` counts equal the stores' `LoadSession` counts, superseded excluded.
  - `TestEndToEndBufferedRewrites` (Review Focus 1):
    1. Stop the collector.
    2. Sync a Gemini session, append a rewind, sync again (both buffered).
    3. Start the collector and flush.
    4. Expect: live entries equal the session's current entries, and the rewound ones are superseded.
- [ ] **Step 2: Run** `go test ./internal/relay/ -run EndToEnd`. Expected: FAIL until wired (if it passes on first run, ledger it as a pin).
- [ ] **Step 3: Fix what the end-to-end run finds; write the docs.**
- [ ] **Step 4: Real-data check:**
  1. Build `thinkt` into the scratch folder, with `THINKT_HOME` a temp dir whose config enables the user's sources.
  2. Run `thinkt collect` on a temp port with a temp DB.
  3. Run `thinkt relay --collector-url http://localhost:<port>` (one-shot).
  4. Record per-source session and entry counts from `/v1/traces/stats` and DuckDB against `thinkt sessions list --json` counts, and the one-shot run's wall time.
  5. Run it again and expect nothing shipped.
- [ ] **Step 5: Run** the full suite `go test ./... 2>&1 | tail -40`. Expected: only the two known failures.
- [ ] **Step 6: Commit** `test(relay): end to end against the collector; docs for the v2 archive`.
