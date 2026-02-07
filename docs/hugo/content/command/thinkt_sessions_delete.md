---
title: "thinkt sessions delete"
---

## thinkt sessions delete

Delete a session

### Synopsis

Delete a Claude Code session file.

The session can be specified as:
  - Full path to the .jsonl file
  - Session ID (requires -p/--project)
  - Filename (requires -p/--project)

Before deletion, shows session info and prompts for confirmation.
Use --force to skip the confirmation.

Examples:
  thinkt sessions delete /full/path/to/session.jsonl
  thinkt sessions delete -p ./myproject abc123
  thinkt sessions delete -p ./myproject --force abc123

```
thinkt sessions delete <session> [flags]
```

### Options

```
  -f, --force   skip confirmation prompt
  -h, --help    help for delete
```

### Options inherited from parent commands

```
      --pick                 force project picker even if in a known project directory
  -p, --project string       project path (auto-detects from cwd if not set)
  -s, --source stringArray   filter by source (kimi|claude, can be specified multiple times)
  -v, --verbose              verbose output
```

### SEE ALSO

* [thinkt sessions](thinkt_sessions.md)	 - View and manage sessions across all sources

