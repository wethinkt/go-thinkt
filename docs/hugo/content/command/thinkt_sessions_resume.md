---
title: "thinkt sessions resume"
---

## thinkt sessions resume

Resume a session in its original CLI tool

### Synopsis

Resume a session in its original CLI tool (e.g., claude --resume).

If no session is specified, shows an interactive picker.
Only sources that support resume (e.g., Claude Code, Kimi Code) are available.

The session can be specified as:
  - Full path to a known session file
  - Session ID (requires -p/--project)
  - Filename (requires -p/--project)

Examples:
  thinkt sessions resume                   # Interactive picker
  thinkt sessions resume -p ./myproject abc123
  thinkt sessions resume /full/path/to/session.jsonl

```
thinkt sessions resume [session] [flags]
```

### Options

```
  -h, --help   help for resume
```

### Options inherited from parent commands

```
      --log string           write debug log to file
      --pick                 force project picker even if in a known project directory
  -p, --project string       project path (auto-detects from cwd if not set)
  -s, --source stringArray   filter by source (claude|kimi|gemini|copilot|codex|qwen, can be specified multiple times)
  -v, --verbose              verbose output
```

### SEE ALSO

* [thinkt sessions](thinkt_sessions.md)	 - View and manage sessions across all sources

