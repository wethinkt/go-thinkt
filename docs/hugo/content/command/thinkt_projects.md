---
title: "thinkt projects"
---

## thinkt projects

Manage and view projects

### Synopsis

Manage and view projects from available sources (Kimi, Claude, Gemini, etc.).

By default, this command launches the interactive project browser (TUI).
Use subcommands to list, summarize, or manage projects via CLI.

Examples:
  thinkt projects                      # Launch interactive browser (default)
  thinkt projects list                 # List detailed columns
  thinkt projects list --short         # List paths only
  thinkt projects summary              # Detailed summary with session names
  thinkt projects tree                 # Tree view

```
thinkt projects [flags]
```

### Options

```
  -h, --help                 help for projects
  -s, --source stringArray   source to include (kimi|claude|gemini, can be specified multiple times, default: all)
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction
* [thinkt projects copy](thinkt_projects_copy.md)	 - Copy project sessions to a target directory
* [thinkt projects list](thinkt_projects_list.md)	 - List projects from all sources
* [thinkt projects summary](thinkt_projects_summary.md)	 - Show detailed project summary
* [thinkt projects tree](thinkt_projects_tree.md)	 - Show projects in a tree view
* [thinkt projects view](thinkt_projects_view.md)	 - Interactive project browser

