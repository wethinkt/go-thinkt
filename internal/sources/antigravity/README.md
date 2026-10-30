# antigravity

Package `antigravity` implements the session source for the Antigravity
CLI (`agy`), reading its tree at `~/.gemini/antigravity-cli` (override with
`THINKT_ANTIGRAVITY_HOME`). The Gemini source shares `~/.gemini` and skips
this directory.

## Directory Structure

```
antigravity/
├── discovery.go          # StoreFactory, DefaultDir, IsSessionPath, baseDirForPath, sessionIDForPath, sessionFile
├── classify.go           # StepClass, stepHead, ClassifyLine, NewSearchFilter
├── convert.go            # step → entry: <USER_REQUEST> split, model turns, results, system messages
├── parser.go             # streaming Parser: order-based pairing, held model turn, subagent links
├── related.go            # relatedTracker: folders outside the project the conversation touched
├── summaries.go          # conversation_summaries.db, cache/last_conversations.json
├── sessionstate.go       # scanSessionFile, workspace resolution, applyTo, cached
├── store.go              # thinkt.Store implementation
├── resume.go             # `agy --conversation <id>`
├── unknown.go            # once-per-type debug log of unknown step types
└── testdata/             # main.jsonl, child.jsonl, noworkspace.jsonl, summaries.sql
```

## Layout on disk

```
~/.gemini/antigravity-cli/
├── brain/<conversation-id>/
│   ├── .system_generated/logs/transcript_full.jsonl   # the session (preferred)
│   ├── .system_generated/logs/transcript.jsonl        # read when there is no full file
│   ├── .system_generated/logs/chunks/…                # split copies: not session files
│   ├── .tempmediaStorage/                              # images attached to steps
│   └── task.md, implementation_plan.md, walkthrough.md # agent documents (not read yet)
├── conversation_summaries.db   # SQLite: title, preview, workspace_uris, parent_conversation_id, …
├── cache/last_conversations.json  # workspace path → its last conversation id
└── installation_id
```

Brain dirs without a transcript are empty conversations and are not
listed. Session ids are the conversation ids.

## Steps and classification

Each transcript line is one step: `{step_index, source, type, status,
created_at, content?, error?, media?}`. `ClassifyLine` decides the class
from `type`:

| type | class | entry |
|---|---|---|
| `USER_INPUT` | user input | user, visible; text inside `<USER_REQUEST>`, `<ADDITIONAL_METADATA>` and `<USER_SETTINGS_CHANGE>` in `Metadata`; a `Model Selection` change sets the model |
| `PLANNER_RESPONSE` | model turn | assistant: thinking, text, one `tool_use` per call (`<step_index>.<n>`), usage from `input_tokens`/`cache_read_tokens`/`output_tokens` |
| `GENERIC`, `VIEW_FILE`, `RUN_COMMAND`, `CODE_ACTION`, `GREP_SEARCH`, `LIST_DIRECTORY`, `SEARCH_WEB`, `READ_URL_CONTENT`, `ASK_QUESTION` | tool result | user with one `tool_result`; `Created At`/`Completed At` header lines stripped into `Metadata`; error when `status` is `ERROR` or `error` is set |
| `SYSTEM_MESSAGE` from `<id>/task-N` | system message | `KindTaskNotification`, secondary |
| `SYSTEM_MESSAGE` from a conversation id | system message | `KindAgentMessage`, secondary; in a subagent, the parent's first message is the task prompt (user, visible) |
| `SYSTEM_MESSAGE` from `system` | system message | `KindMeta`, secondary |
| `EPHEMERAL_MESSAGE`, `CONVERSATION_HISTORY` | harness | `KindMeta`, secondary; empty ones emit nothing |
| `CHECKPOINT` | checkpoint | `KindCompactSummary`, visible |
| `ERROR_MESSAGE` | api error | `KindAPIError`, secondary |
| anything else | unknown | `KindUnknown`, secondary, logged once per type |

Entry UUIDs are `<conversation-id>:<step_index>`. `PromptID` is the index of
the latest `USER_INPUT` (steps before the first one take the first step's
index). `TurnID` is the index of the planner step: its entry and the results
paired to its calls share it; user input and system steps have none. Attached `media` becomes a `[<mime_type>: <uri>]` text line; the
file is not read.

The search filter keeps user input, model turns, tool results, and a
subagent's task prompt (the first system message from another conversation
before any user input).

## Parser

Tool calls carry no ids. Each result pairs with the oldest unanswered call
of the latest model turn, and a new model turn drops the previous turn's
unanswered calls. The model-turn entry is held until its calls are answered
or the next model turn, user input or EOF arrives, so an `invoke_subagent`
result (`Created the following subagents:` followed by objects with
`conversationId`) can set `ChildSessionID` on its `tool_use` block; all the
ids go in the result's `Metadata["child_session_ids"]`. A result with no
waiting call becomes a secondary `KindUnknown` entry. A partial last line
(the file is being written) is skipped.

## Projects and session state

Projects are folders, keyed by path. A conversation's folder is the first
`workspace_uris` entry (where `agy` started); else the most common
`run_command` `Cwd` in its transcript (unquoted; Antigravity sometimes
stores it JSON-quoted); else the workspace whose
`last_conversations.json` entry names it; else the `no-workspace` project
(name and path `(no workspace)`, which names no real folder; its sessions
have an empty `ProjectPath`). Antigravity's `project_id` is not used:
one of its projects spans unrelated repos. A subagent takes its parent's
folder.

| `SessionMeta` field | source |
|---|---|
| `Title` | summary `title`, else `preview`, else the first prompt |
| `ParentSessionID` | summary `parent_conversation_id`; subagents are listed only with `thinkt.WithSubagents()` |
| `ModifiedAt` | the later of summary `last_modified_time` (the year-1 placeholder counts as missing) and the file mtime, in UTC: background results land in the transcript after the row is written |
| `CreatedAt`, `EntryCount`, `FirstPrompt`, `LastPrompt`, `Model`, `Cost` | one parser pass (`scanSessionFile`) |
| `RelatedPaths` | see below |

`conversation_summaries.db` is opened `mode=ro&immutable=1`: it is
`journal_mode=delete` and Antigravity writes it, so a shared lock would
stall its writer. Without the db every conversation is still listed, from
its transcript alone.

## Related folders

`RelatedPaths` lists the folders outside the project a conversation
touched, each folded to its repo root (nearest ancestor with `.git`):

| reason | from |
|---|---|
| `workspace` | `workspace_uris` entries after the first |
| `edited` | `replace_file_content`, `multi_replace_file_content`, `write_to_file` (`TargetFile`); `edit_file`, `write_file` (`Path`) |
| `command` | `run_command` `Cwd` |
| `read` | `view_file` (`AbsolutePath`), `read_file` (`Path`), `list_dir` (`DirectoryPath`), `grep_search` (`SearchPath`) |

`Count` counts tool calls; a `workspace` reason adds no count. Paths that
no longer exist outside any repo, paths inside the project, and paths
under the Antigravity home or the system temp dirs are not recorded.

## Relay

`WatchConfig` reaches `.system_generated/logs` (via `HiddenDirs`), so the
relay watcher sees activity there, but it cannot ship Antigravity traces
yet: steps carry no `uuid`, and both transcript files sit in `logs/`.

## Not read

The agent documents in each brain dir (`task.md`,
`implementation_plan.md`, `walkthrough.md`) and their history are on the
roadmap (`PLAN.md`). `thinkt.ParserVersion` 5 covers this source.
