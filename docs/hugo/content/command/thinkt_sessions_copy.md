---
title: "thinkt sessions copy"
---

## thinkt sessions copy

Copy a session to a target location

### Synopsis

Copy a known session file to a target location.

The session can be specified as:
  - Full path to a known session file
  - Session ID (requires -p/--project)
  - Filename (requires -p/--project)

The target can be a file path or directory.

Examples:
  thinkt sessions copy /full/path/to/session ./backup/
  thinkt sessions copy -p ./myproject abc123 ./backup/
  thinkt sessions copy -p ./myproject abc123 ./backup/renamed-session

```
thinkt sessions copy <session> <target> [flags]
```

### Options

```
  -h, --help   help for copy
```

### Options inherited from parent commands

```
      --pick                 force project picker even if in a known project directory
  -p, --project string       project path (auto-detects from cwd if not set)
  -s, --source stringArray   filter by source (claude|kimi|gemini|copilot|codex, can be specified multiple times)
  -v, --verbose              verbose output
```

### SEE ALSO

* [thinkt sessions](thinkt_sessions.md)	 - View and manage sessions across all sources

