# Agent documents over time

**Why.** Agents write plans, task lists and walkthroughs next to the
conversation, and revise them as they work. Today a reader sees those as
scattered tool calls. `PLAN.md` asks to track how they evolve: each version,
the final file, and the step that produced each version. The user skipped the
design questions, so the calls below are mine.

## Goals

- `thinkt.AgentDocument`: a document's name, path, kind, summary, final text,
  and versions in order, each linked to the entry that produced it.
- Sources that have documents implement an optional `thinkt.DocumentStore`.
- `thinkt sessions docs <session>` lists them; `--name` with `--version` prints
  one version. `GET /api/v1/sessions/<path>/documents` returns them as JSON.

## Non-goals

- Typeset rendering next to the conversation, and TUI or web views. This pass
  builds the data they would read.
- Ordinary repo files such as `docs/plans/*.md`. They are project files, and
  git has their history. A document here is one the agent keeps in its own
  storage, or states in the conversation.
- Todo lists that are not documents (Claude `TodoWrite`, Kimi `TodoList`).
- Gemini: no document records in the local corpus.
- Carrying documents through the relay. Tool calls in the archive hold what
  the rebuild reads.

## What each source has

Tallied from local sessions.

| Source | Where | Versions from |
|---|---|---|
| Antigravity | `brain/<id>/*.md` with `*.md.metadata.json` (`artifactType`, `summary`, `updatedAt`); 600+ writes | rebuilt from `write_to_file`, `write_file`, `replace_file_content`, `edit_file`, `multi_replace_file_content` |
| Kimi | `agents/main/plans/<key>.md`, snapshots `agents/main/plan/<key>/v<N>.md`, `plan.revision` records | the snapshots; `plan.revision` links each to its entry |
| Claude | `<home>/plans/*.md`, `ExitPlanMode` `plan` (none locally, both documented formats) | rebuilt from `Write`, `Edit`; each `ExitPlanMode` is a version of `plan` |
| Codex | `update_plan` wire calls (10), `Plan` items (1); no files | each is a version of `plan`; `update_plan` steps render as a checklist |

## Rebuild

`thinkt.DocumentTracker` walks a session's entries once. A source hands it a
table of tools (`DocTool`: the argument names for path, full content, old
text, new text, and a list of chunks) and a test for which paths are documents.

- A full write is a version with that content.
- An edit replaces the old text in the previous version, every match when the
  call says `AllowMultiple`, else the first.
- An edit whose old text is not found, or with no earlier version, still makes
  a version, marked `Partial`, keeping the text known so far. Reading a
  document must never fail because history is incomplete.
- A failed call still counts: results are not paired to calls here, and the
  final text on disk is the truth. (My call.)
- `Note` adds a version directly, for plans stated in the conversation.

`Final` is the file on disk when there is one. It can differ from the last
version when a tool outside these tables touched it. No version is invented.

The rebuild runs on request, never in listings, so listing costs nothing.

Measured on local Antigravity data: 83 documents, 548 versions; 70 documents
end exactly on the file on disk. 48 versions in 11 documents are `Partial`:
their old text is not in the rebuilt text, because a call failed or something
outside these tools changed the file. Some text arguments are JSON-quoted, as
paths are, and are unquoted first.

## Types

```go
type AgentDocument struct {
	Name, Path, Kind, Summary string
	UpdatedAt time.Time
	Final     string
	Versions  []DocumentVersion
}
type DocumentVersion struct {
	Number    int
	EntryUUID string
	Timestamp time.Time
	Tool      string
	Content   string
	Partial   bool
}
type DocumentStore interface {
	SessionDocuments(ctx context.Context, sessionID string) ([]AgentDocument, error)
}
```

Kinds: `plan`, `task`, `walkthrough`, `other`.

## Tasks

1. Types and `DocumentTracker`.
2. Antigravity. 3. Kimi. 4. Claude. 5. Codex.
6. CLI and REST, real-data check, docs, `PLAN.md`.

Each task: failing test first. A fresh reviewer reads the branch at the end,
with one fix pass.
