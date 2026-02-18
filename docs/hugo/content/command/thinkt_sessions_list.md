---
title: "thinkt sessions list"
---

## thinkt sessions list

List sessions (auto-detects project from cwd)

### Synopsis

List all sessions in a project.

Project selection:
  - In a project directory: automatically uses that project
  - Otherwise: shows interactive project picker
  - -p/--project <path>: use specified project
  - --pick: force picker even if in a project directory

Examples:
  thinkt sessions list              # Auto-detect from cwd or picker
  thinkt sessions list --pick       # Force project picker
  thinkt sessions list -p ./myproject
  thinkt sessions list --source kimi
  thinkt sessions list --source qwen
  thinkt sessions list --json       # JSON output

```
thinkt sessions list [flags]
```

### Options

```
  -h, --help   help for list
      --json   output sessions as JSON
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

