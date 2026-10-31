# Relay ships parsed entries

Date: 2026-10-30. Branch: `worktree-relay-entries`, stacked on
`worktree-gemini-source`.

## Why

The relay reads each new line of a watched session file and keeps only
lines shaped like Claude's records, with a `uuid` and a Claude-style
`message`. It ships a one-line summary of each: role, text, model, tool
name, tokens, and whether there was thinking. It names the session after
the file and derives the project from Claude's folder naming. So today
only Claude really works:

- Codex lines have no `uuid` and are dropped.
- Kimi sessions are all named `wire`, after `wire.jsonl`.
- Antigravity and Gemini do not come through at all. Their records carry
  no `uuid`, and Gemini can rewrite or rewind earlier lines.
- Codex holds an entry until its usage record, so a line is not an
  entry anyway.

The user sees the collector as a general archive across machines, for
analysis, metrics, storage and governance. Nobody depends on its current
format, so both the relay and the collector can change (decided with the
user).

## Goals

- **Every source ships.** Claude, Codex, Kimi, Antigravity, Gemini,
  Copilot and Qwen all reach the collector through their own reader,
  as full `thinkt.Entry` values with session metadata. That is the same
  shape the REST API and MCP return.
- **Rewritten sessions stay correct.** Entries are keyed, so a session
  that changes earlier entries (Gemini rewind and patch, Codex's held
  response) is updated in place, not appended to.
- **Removed entries are kept.** An entry the source no longer has is
  marked superseded, with when. A collector setting deletes them instead
  (decided with the user: configurable; keeping is my call for the
  default, because deleting cannot be undone).
- **A vanished session stays archived.** When a session file disappears
  (Claude's retention cleanup, `thinkt sessions delete`, a manual
  delete), the archive keeps everything (decided with the user). It is
  never reported as removed, and the session row only notes when its
  source was last seen.
- **History is backfilled.** On first run the relay ships every session
  already on disk, and at each later start it re-reads only sessions
  whose files changed while it was off (decided with the user, as a
  setting; see section 2).
- **Existing consumers keep working:** the collector's search and stats
  endpoints, the TUI collector page, and the live stream behind
  `thinkt agents follow` for remote agents.

## Non-goals

- Raw-line backup. Keeping the untouched source files is a separate
  backup operation, maybe later, and not this dataflow (decided with the
  user).
- A collector UI, or new query endpoints beyond what existing consumers
  need.
- Migrating existing collector data. The schema changes and old tables
  are dropped; nobody uses them.
- Changing what any reader emits.

## Anti-goals

- **Duplicates or a wrong session.** No entry shipped twice as two rows,
  and no entry attached to the wrong session or project.
- **Silent loss.** No entries lost silently when the collector is down,
  or when a session file is rewritten while the relay is offline.
- **Sources that are off.** Nothing from a source that is turned off is
  shipped (the visibility rule).

## 1. Identity

An entry's key in the archive is `(machine_id, source, session_id, uuid)`:

- **`machine_id`:** the relay's fingerprint; already in payloads.
- **`source`:** the thinkt source.
- **`session_id`:** the `SessionMeta.ID` the source's store gives, not
  the filename.
- **`uuid`:** `Entry.UUID`, which every source fills uniquely within a
  session.

A session's key is `(machine_id, source, session_id)`. Two machines can
hold the same session id (a Gemini filename stem, or copied files), so
`machine_id` is part of both keys.

## 2. Relay

On a file event (already debounced) the relay works through these steps:

1. **Resolve the source.** Use the shared resolver
   (`sources.ResolveSessionPath`), then the watch-root config as today.
   Only sources registered (enabled) in the relay's registry ship.
2. **Find the session.** Read it from the source's store
   (`GetSessionMeta(path)`), giving the session id, project path,
   parent, title and model. Not a session (a losing duplicate, a
   superseded `.json`, a non-session file) → nothing ships.
3. **Read the whole session** through `OpenSession`, collecting every
   entry, secondary entries included. The archive keeps everything;
   readers decide what to hide.
4. **Diff against what was last shipped** for that session. The relay
   keeps, per session key, a map of `uuid` → content hash (SHA-256 of
   the entry's canonical JSON). It ships three things:
   - entries whose hash is new or changed
   - the uuids that disappeared (as `removed`)
   - session metadata when it changed, or on first sight
5. **Ship in batches of `BatchSize` entries.** The state is updated only
   after the collector accepts a batch, or after the batch is written to
   the disk buffer, which already retries.

**State.** The per-session hashes persist in the relay's state directory
(`~/.thinkt/relay/state.json`, or one file per session if one file is too
large), so a restart does not re-ship everything. Each session's state
also keeps the file's mtime and size, and a generation number (below).
When the state is missing, the relay ships everything once; upsert makes
that harmless.

**Startup and one-shot mode.** Both reconcile through each enabled
source's store (`ListProjects`, then `ListSessions` with subagents):

- A session not in the state is shipped whole: the backfill.
- A session whose file mtime or size differs from the state is diffed
  and shipped.
- Others are skipped.

The setting `--backfill` (default `all`; `THINKT_RELAY_BACKFILL`) takes
`all` or `none`. With `none`, sessions found at first start are recorded
as seen without shipping, and only later changes ship. (Decided with the
user: backfill, as a setting. `all` as the default is my call.)

`thinkt relay` (one-shot) is this reconcile plus a buffer flush.
`--forward` runs the reconcile once, then reacts to file events. The old
top-level `*.jsonl` scan goes away.

**Vanished sessions.** A session the relay has state for, but whose file
no longer exists or which its store no longer returns (a losing
duplicate, a superseded `.json`), is dropped from the state. Nothing is
sent for it. The collector keeps what it has.

**One relay per machine.** Two relays on one machine would share a
`machine_id` and race. The relay already registers itself in the
instance registry (`InstanceRelay`); a second relay on the same machine
refuses to start, with a message naming the running one. (My call.)

**Ordering.** Each session's state carries a generation number. It goes
up by one for every payload the relay produces for that session, and is
sent as `session.generation`. The collector stores the last generation
per session and ignores a payload with a lower one. So a buffered older
batch that drains late cannot overwrite a newer one, and a late
`removed` cannot undo a re-add. (My call.)

**Sources switched off.** The relay builds its registry at start, from
the config's enabled sources. Turning a source off takes effect when the
relay restarts. On start, buffered payloads of sources that are now off
are discarded. (My call.)

**Cost.** This replaces the byte-offset tail. A changed file is re-read
whole. Session files are megabytes at most, and reads are debounced, so
this is acceptable. (My call. The readers already exist, and per-source
incremental reading would be a second reader for each source.)

**Removal.** `removed` is sent only for uuids that the relay itself
shipped before and that the source, still returning the session, no
longer produces. A session that vanished sends nothing.

**Subagent sessions** ship as their own sessions, with `parent_session_id`.

**The standalone binary.** `cmd/thinkt-relay` uses the same registry
construction as `thinkt relay`, so it also ships every enabled source.

## 3. Wire format

`POST /v1/traces` with a new body; the old body is gone:

```json
{
  "version": 2,
  "instance_id": "…",
  "machine_id": "…",
  "source": "gemini",
  "session": {
    "id": "session-2026-05-17T15-05-17503250",
    "project_path": "/Users/dev/proj",
    "parent_session_id": "",
    "title": "…",
    "model": "…",
    "created_at": "…",
    "modified_at": "…",
    "entry_count": 38,
    "cost": {},
    "generation": 7
  },
  "entries": [ { /* thinkt.Entry, as the REST API serialises it */ } ],
  "removed": ["uuid", "…"]
}
```

- `session` is always present, so a batch carrying only `removed` still
  says which session.
- A request with another `version` gets HTTP 400 with a clear message,
  so an old relay fails loudly.

## 4. Collector

The schema is replaced. Old tables are dropped at startup when the old
layout is detected; there is no data migration.

- **`sessions`**
  - Key: `(machine_id, source, session_id)`.
  - Columns: `project_path`, `parent_session_id`, `title`, `model`,
    `created_at`, `modified_at`, `entry_count`, `cost_json`,
    `instance_id`, `first_seen`, `last_seen`.
  - Activity status: `status` and `last_activity`, as today.
- **`entries`**
  - Key: `(machine_id, source, session_id, uuid)`.
  - `entry_json` holds the full entry.
  - Derived columns for cheap queries: `role`, `kind`, `visibility`,
    `timestamp`, `model`, `text`, `tool_names`, `is_error`,
    `input_tokens`, `output_tokens`, `cache_read_tokens`,
    `thinking_tokens`, `turn_id`, `prompt_id`.
  - `content_hash`, `ingested_at`, `updated_at`, `superseded_at`
    (null when live).
- **Upsert.** An entry with the same key replaces the row and clears
  `superseded_at`.
- **`removed`.** It sets `superseded_at = now`, or deletes the rows when
  the collector runs with `--superseded=delete`
  (`THINKT_COLLECTOR_SUPERSEDED`; default `keep`). The collector has no
  config file, so the setting is a flag and an environment variable
  only. Switching to `delete` does not purge rows already marked.
- **Generations.** `sessions.generation` holds the highest generation
  applied. A payload with a lower generation is acknowledged (HTTP 200,
  so the relay does not retry it) and not applied.
- **Readers of entries** skip superseded rows unless they ask for them.
- **`collected_agents`** stays as it is.
- **Session activity.** The relay's activity events (`session_start`,
  `session_active`, `session_end`) use the store's session id and carry
  `machine_id` and `source`. Today they use the filename stem, so every
  Kimi session is `wire`. `session_activity` gains `machine_id`, and
  status updates match on the full session key.

**Existing endpoints** keep their paths and response shapes, read from
the new tables:

- **Search** (`/v1/traces/search`): over live entries' `text`.
- **Stats** (`/v1/traces/stats`): `TotalTraces` counts live entries.
- **Sessions** (`/v1/sessions/active`, and `QuerySessions` for the
  stream backfill): one row per session key. The response gains `source`
  and `machine_id` where missing. The TUI collector page reads only stats
  and agents.
- **Session stream** (`/v1/sessions/{id}/ws`): publishes the upserted
  entries, mapped to the stream's existing `StreamEntry` shape (role,
  text, model, tool name, tokens, content blocks). Remote
  `agents follow` subscribers get correct data. The stream addresses
  sessions by id alone; when two machines share an id, entries of both
  reach the subscriber. (My call: rare, and changing the address is out
  of scope.)
  - **Already broken:** remote `agents follow` itself does not work
    today, because `internal/agents` subscribes with the relay's instance
    id instead of a session id. Fixing the hub is a separate small job,
    not this one. (My call.)
- **Parquet export** writes the new `entries` table, with `entry_json`
  as a column.

## Constraints

- Go, the existing `internal/relay` and `internal/collect` packages, and
  DuckDB as today.
- The relay reads only through `thinkt.Store` and the shared resolver;
  it has no source-specific parsing.
- Tests run without a network: an in-process collector (`httptest`) and
  temp session homes built from the sources' `testdata`.

## Left to the builder

- Whether the relay state is one file or one file per session, and the
  canonical JSON used for the hash.
- How the store registry inside the relay is built: the same factories
  as `thinkt`, honouring the config's enabled sources.
- Batching details beyond `BatchSize`.
