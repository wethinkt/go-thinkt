---
title: "thinkt projects"
---

## thinkt projects

List projects from all sources

### Synopsis

List all projects from available sources (Kimi, Claude, etc.).

By default, shows detailed columns (path, source, sessions, modified time).
Use --short for a compact list of project paths only.
Use --source to limit to specific sources (can be specified multiple times).

Examples:
  thinkt projects                      # Detailed columns (default)
  thinkt projects --short              # Paths only, one per line
  thinkt projects --source kimi        # Only Kimi projects
  thinkt projects --source claude      # Only Claude projects
  thinkt projects --source kimi --source claude  # Both sources
  thinkt projects tree                 # Tree view grouped by parent directory
  thinkt projects summary              # Detailed summary with session names

```
thinkt projects [flags]
```

### Options

```
  -h, --help                 help for projects
      --short                show project paths only
  -s, --source stringArray   source to include (kimi|claude, can be specified multiple times, default: all)
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction
* [thinkt projects copy](thinkt_projects_copy.md)	 - Copy project sessions to a target directory
* [thinkt projects delete](thinkt_projects_delete.md)	 - Delete a project and all its sessions
* [thinkt projects summary](thinkt_projects_summary.md)	 - Show detailed project summary
* [thinkt projects tree](thinkt_projects_tree.md)	 - Show projects in a tree view

