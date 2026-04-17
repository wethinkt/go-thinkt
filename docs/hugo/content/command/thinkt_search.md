---
title: "thinkt search"
---

## thinkt search

Search for text across indexed sessions

### Synopsis

Search for text within session files using the SQLite index.

The index is used to find relevant files, then scans them directly.
Your private content stays in local files, not the index.

By default opens an interactive TUI. Use --list for scripting output.

```
thinkt search <query> [flags]
```

### Options

```
  -C, --case-sensitive          Case-sensitive matching
  -h, --help                    help for search
      --json                    Output as JSON
  -n, --limit int               Limit total matches (default 50)
      --limit-per-session int   Limit hits per session (0 for no limit) (default 2)
      --list                    Output as list instead of TUI
  -p, --project string          Filter by project name
  -E, --regex                   Treat query as regex
  -s, --source string           Filter by source (claude, kimi)
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt/)	 - Tools for AI assistant session exploration and extraction

