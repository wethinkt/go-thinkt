---
title: "thinkt sessions view"
---

## thinkt sessions view

View a session in the terminal (interactive picker)

### Synopsis

View a session in a full-terminal viewer.

If no session is specified, shows an interactive picker of all recent sessions.
The picker works across all discovered sources.

	The session can be specified as:
  - Full path to a known session file
  - Session ID (requires -p/--project)
  - Filename (requires -p/--project)

Navigation:
  ↑/↓/j/k     Scroll up/down
  PgUp/PgDn   Page up/down
  g/G         Go to top/bottom
  q/Esc       Quit

Use --raw to output undecorated text to stdout (no TUI).

Examples:
  thinkt sessions view              # Interactive picker across all sources
  thinkt sessions view /full/path/to/session
  thinkt sessions view -p ./myproject abc123
  thinkt sessions view -p ./myproject --all        # view all
  thinkt sessions view /path/to/session --raw      # raw output to stdout

```
thinkt sessions view [session] [flags]
```

### Options

```
  -a, --all    view all sessions in time order
  -h, --help   help for view
      --raw    output raw text without decoration/rendering
```

### Options inherited from parent commands

```
      --pick                 force project picker even if in a known project directory
  -p, --project string       project path (auto-detects from cwd if not set)
  -s, --source stringArray   filter by source (claude|kimi|gemini|copilot, can be specified multiple times)
  -v, --verbose              verbose output
```

### SEE ALSO

* [thinkt sessions](thinkt_sessions.md)	 - View and manage sessions across all sources

