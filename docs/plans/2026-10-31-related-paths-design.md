# RelatedPaths for every reworked source

**Why.** A session's project says where it started, not where it worked.
`SessionMeta.RelatedPaths` answers "which other repos did this session
touch, and how", but only Antigravity fills it. Claude, Codex, Kimi and
Gemini all record the same facts in their tool calls. The user skipped the
design questions, so the calls below are mine.

## Goals

- `RelatedPaths` filled for Claude, Codex, Kimi and Gemini, with the same
  meaning as Antigravity: repo roots outside the project, with reasons
  `workspace`, `edited`, `command`, `read` and a count of tool calls.
- One tracker, shared by the five sources.
- No slower listings than a cached scan already costs; the fast paths stay
  fast.

## Non-goals

- Parsing shell command text (`cd ../x && make`). Only structured working
  folders count.
- Copilot and Qwen, which have not had a source pass.
- Subagent sessions rolling up into their parent. Each session reports its
  own calls; a Claude subagent file is not scanned for its parent.
- A project view of related folders, or a new surface. `sessions summary`
  and the JSON outputs already print the field.
- Shipping the field through the relay snapshot: the archive holds every
  tool call, so it can be derived there.

## Shared tracker

`internal/thinkt/related.go` takes over `antigravity/related.go`:

```go
type ToolPath struct{ Arg, Reason string }
type ToolPaths map[string]ToolPath

func NewRelatedTracker(project string, exclude []string) *RelatedTracker
func (r *RelatedTracker) Touch(path, base, reason string)     // one tool call
func (r *RelatedTracker) Workspace(path string)               // a reason, no count
func (r *RelatedTracker) Call(t ToolPaths, name string, args map[string]any, base string)
func (r *RelatedTracker) Paths() []RelatedPath
func RelatedExclude(home string) []string // home, os.TempDir(), /tmp, /private/tmp
```

Rules, unchanged from Antigravity unless marked new:

- A path folds to its nearest ancestor with `.git`; a path in no repo is
  dropped. The path itself need not exist.
- A root inside the project, or containing it, is dropped. So are roots
  under an exclude prefix. Symlinks resolve before comparing.
- New: a relative path joins `base`; with no base it is dropped.
- New: a path already inside the project returns before touching the disk,
  and repo-root lookups are memoised per directory, so a session of
  thousands of calls stays cheap.
- New: a tracker with an empty project is nil and records nothing, for the
  new sources. "Outside the project" means nothing without a project.
  Antigravity keeps its behaviour, since it has workspaces of its own.
- All methods are safe on a nil tracker.

## Per source

| reason | Claude | Kimi | Gemini | Codex |
|---|---|---|---|---|
| `read` | `Read`, `NotebookRead`, `Grep`, `Glob`, `LS` | `Read`, `ReadFile`, `ReadMediaFile`, `Grep`, `Glob` | `read_file`, `read_many_files`*, `list_directory`, `grep_search`, `glob` | none structured |
| `edited` | `Edit`, `MultiEdit`, `Write`, `NotebookEdit` | `Edit`, `Write`, `WriteFile`, `StrReplaceFile` | `replace`, `write_file` | `apply_patch` file changes |
| `command` | `Bash`: the record's `cwd` | `Bash` `cwd` | `run_shell_command` `dir_path` | `exec_command` `workdir`, else the turn's `cwd` |

*`read_many_files` takes a list of patterns, so it is left out.

Argument names were tallied from local sessions: Claude `file_path`,
`path`, `notebook_path`; Kimi `path`, `cwd`; Gemini `file_path`, `dir_path`.
The base for relative paths is the record's `cwd` (Claude), the session
`CWD` or `WorkDir` (Kimi), the project (Gemini), the turn `cwd` (Codex).

- **Claude.** The scan stays a byte classifier. Only assistant lines
  containing `"tool_use"` are decoded, whatever the first-prompt early exit
  has decided. `project` comes from `meta.ProjectPath`.
- **Codex.** Takes the paths from raw records in its own scan: the keys of
  a file-change item's `changes`, and an exec item's or wire call's working
  folder. A call that has both a harness item and a wire record counts once.
- **Kimi.** The scan already runs the parser, so it reads the entries'
  tool blocks.
- **Gemini.** `jsonlFile` and `jsonFile` learn the project path. Calls are
  read from the replayed messages, so a rewound call does not count. The
  cached session keeps `RelatedPaths`, and the cache-hit path restores them.

`thinkt.ParserVersion` goes to 8, so cached metadata recomputes.

## Decisions (mine)

- Repo roots only, as Antigravity does. A bare folder like `~/Downloads`
  is not recorded unless it is a workspace.
- A git worktree of the project's own repo that sits outside the project
  counts as related: it is another folder on disk.
- Failed calls count. The session reached for that repo either way.

## Tasks

1. Shared tracker, Antigravity moved onto it, parser version.
2. Kimi. 3. Gemini. 4. Claude. 5. Codex.
6. Real-data check on all five, docs (`AGENTS.md`, source READMEs).

Each task: failing test first, then the code. A fresh reviewer reads the
whole branch at the end, with one fix pass.
