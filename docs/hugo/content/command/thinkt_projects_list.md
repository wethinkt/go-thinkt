---
title: "thinkt projects list"
---

## thinkt projects list

List projects from all sources

### Synopsis

List all projects from available sources (Kimi, Claude, Gemini, etc.).

By default, shows detailed columns (path, source, sessions, modified time).
Use --short for a compact list of project paths only.

Examples:
  thinkt projects list                 # Detailed columns
  thinkt projects list --short         # Paths only, one per line
  thinkt projects list --source kimi   # Only Kimi projects

```
thinkt projects list [flags]
```

### Options

```
  -h, --help    help for list
      --short   show project paths only
```

### Options inherited from parent commands

```
  -s, --source stringArray   source to include (kimi|claude|gemini, can be specified multiple times, default: all)
  -v, --verbose              verbose output
```

### SEE ALSO

* [thinkt projects](thinkt_projects.md)	 - Manage and view projects

