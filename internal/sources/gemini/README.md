# gemini

Package `gemini` implements the session source for [Gemini CLI](https://github.com/google-gemini/gemini-cli).

## Directory Structure

```
gemini/
├── discovery.go          # StoreFactory, IsSessionPath (the Antigravity home excluded)
├── pathresolve.go        # thinkt.SessionPathResolver: projectHash from the file's first line
├── projects.go           # tmp/ folders → projectHash and the real project path
├── replay.go             # JSONL replay: repeats, $patch, $rewindTo, $set.messages
├── classify.go           # classifyLine (thinkt.RecordClass)
├── convert.go            # replayed messages → thinkt entries
├── sessionstate.go       # scanJSONL: one replay per file for SessionMeta
├── searchfilter.go       # thinkt.SearchFileFilterer: hits only in live lines
├── usage.go              # tokens → TokenUsage (shared by both formats)
├── session.go            # the older single-document .json format
├── store.go              # thinkt.Store over both formats
├── resume.go             # gemini --resume <sessionId>
└── testdata/             # scrubbed real sessions and a synthetic replay file
```

## Layout

```
~/.gemini/
├── projects.json                        # {"projects": {"<abs path>": "<slug>"}}
└── tmp/
    ├── <slug>/                          # renamed from <sha256(path)> when the project is opened
    │   ├── .project_root                # the absolute project path
    │   └── chats/
    │       ├── session-<minute>-<8 hex>.jsonl       # main session (Gemini CLI 0.39+)
    │       ├── session-<minute>-<8 hex>.json        # older single-document session
    │       └── <parent sessionId>/<agentId>.jsonl   # subagent session
    └── <sha256(path)>/                  # a folder the CLI has not migrated yet
```

The base directory defaults to `~/.gemini` and can be overridden with
`THINKT_GEMINI_HOME`. `antigravity-cli/` inside it belongs to the
antigravity source.

## Projects

`Project.ID` is the `projectHash` every session file records (sha256 of
the project path), so a leftover hash folder and its slug folder are one
project.

- **`Path`:** from `.project_root`, else the `projects.json` key whose
  sha256 is the hash. It is empty when neither knows the path; the
  project then shows as `gemini://<8 hex>`.
- **`Name`:** the path's base name, else the slug.
- **Duplicates:** a file present in both folders is listed once. The slug
  copy wins, or the newer copy if the two differ.

## JSONL sessions

The file is append-only. The first line is a header (`sessionId`,
`projectHash`, `startTime`, `lastUpdated`, `kind`: `main` or `subagent`).

`replay` rebuilds the message list the way the CLI's loader does:
- A message written again under the same id replaces the first copy, in
  its position.
- `$patch` edits content or a tool result, or removes or reorders ids.
- `$rewindTo` deletes a message and everything after it.
- `$set.messages` replaces the list.
- Other `$set` fields are metadata (`summary`, `lastUpdated`).
- A file with no header is not a session.

Conversion:
- **user:** a prompt, which opens a `PromptID`.
  - `Please continue.` and the loop-recovery and `[System:` nudges are
    secondary `KindMeta` entries; they keep the current `PromptID`.
  - A user message of `functionResponse` parts already emitted inline is
    dropped.
- **gemini:** one assistant entry, with `TurnID` set to the message id.
  - Blocks: thoughts (`subject: description`), text, and a `tool_use` per
    call.
  - Each call becomes a `tool_result` entry (`<call id>:result`) carrying
    the same `TurnID`. Its body is `response.output`, or `response.error`
    for a failed call.
  - An `invoke_agent` call links its child through `ChildSessionID` set
    to `<session>/<agentId>`.
  - `update_topic` calls (the CLI's status line) are secondary.
- **info, warning, error:** secondary system entries.

Session ids:
- A main session's `SessionMeta.ID` is its file stem; a subagent's is
  `<parent stem>/<agentId>`. Gemini's `sessionId` is not unique per file.
- The store keeps each file's `sessionId` for `ResumeCommand`. A
  subagent resumes its parent.

Metadata:
- `Title` is `$set.summary` when it is a single line and not JSON.
- `FirstPrompt` skips the text the CLI refuses as resumable: empty,
  `/…`, `?…`, `<session_context>`, `<hook_context>`.

A `.json` session is hidden when a `.jsonl` with the same `sessionId`
contains its last message: the CLI converted it on resume and left the
old file.

## Usage

Gemini's `output` excludes thinking (`total = input + output + thoughts`).
thinkt counts thinking as part of output:
- `OutputTokens` = `output + thoughts`
- `ThinkingTokens` = `thoughts`
- `CacheReadInputTokens` = `cached`

Both formats use this mapping.

## Search and watching

- **Search:** the file filter replays a `.jsonl` file before scanning it,
  so a hit counts only in a line whose content survives. Rewound turns,
  earlier copies and metadata lines do not count.
- **Watching:** `WatchConfig` reaches `tmp/<p>/chats/<parent>/` (depth 4). The relay re-reads a changed session through this store, so a rewind or patch ships as changed and removed entries.

`thinkt.ParserVersion` 7 covers this format.
