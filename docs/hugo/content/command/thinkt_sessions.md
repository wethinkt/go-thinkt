---
title: "thinkt sessions"
---

## thinkt sessions

View and manage sessions across all sources

### Synopsis

View and manage sessions from all discovered sources.

Running without a subcommand launches the interactive session viewer.

Project selection:
  - In a project directory: automatically uses that project
  - Otherwise: shows interactive project picker
  - -p/--project <path>: use specified project
  - --pick: force picker even if in a project directory

Use --source to filter by source (e.g. claude, kimi, gemini, copilot, codex).

Examples:
  thinkt sessions                   # Interactive viewer (same as view)
  thinkt sessions view              # Interactive picker
  thinkt sessions list              # Auto-detect or picker
  thinkt sessions list --pick       # Force project picker
  thinkt sessions list -p ./myproject
  thinkt sessions summary -p ./myproject --source kimi
  thinkt sessions delete -p ./myproject <session-id>
  thinkt sessions copy -p ./myproject <session-id> ./backup

```
thinkt sessions [flags]
```

### Options

```
  -a, --all                  view all sessions in time order
  -h, --help                 help for sessions
      --pick                 force project picker even if in a known project directory
  -p, --project string       project path (auto-detects from cwd if not set)
      --raw                  output raw text without decoration/rendering
  -s, --source stringArray   filter by source (claude|kimi|gemini|copilot|codex, can be specified multiple times)
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction
* [thinkt sessions copy](thinkt_sessions_copy.md)	 - Copy a session to a target location
* [thinkt sessions delete](thinkt_sessions_delete.md)	 - Delete a session
* [thinkt sessions list](thinkt_sessions_list.md)	 - List sessions (auto-detects project from cwd)
* [thinkt sessions resolve](thinkt_sessions_resolve.md)	 - Resolve a session query to its canonical path
* [thinkt sessions summary](thinkt_sessions_summary.md)	 - Show detailed session summary
* [thinkt sessions view](thinkt_sessions_view.md)	 - View a session in the terminal (interactive picker)

