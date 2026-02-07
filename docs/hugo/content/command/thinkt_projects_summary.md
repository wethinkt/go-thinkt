---
title: "thinkt projects summary"
---

## thinkt projects summary

Show detailed project summary

### Synopsis

Show detailed information about each project including
session count and last modified time.

By default, shows projects from ALL sources.
Use --source to limit to specific sources.

Sorting:
  --sort name|time    Sort by project name or modified time (default: time)
  --desc              Sort descending (default for time)
  --asc               Sort ascending (default for name)

Output can be customized with a Go text/template via --template.

Template Variables
==================

Each project in the list has:
  .Path          string  - Full project path (or "~" for home)
  .DisplayName   string  - Short name (last path component)
  .SessionCount  int     - Number of sessions
  .Modified      string  - Last modified time (may be empty)
  .DirPath       string  - Path to project directory
  .Source        string  - Source type (kimi, claude)
  .Sessions      []SessionSummary - Session details (with --with-sessions flag)

Each SessionSummary has:
  .ID            string  - Session ID
  .Name          string  - First prompt or session ID
  .EntryCount    int     - Number of entries/messages
  .Modified      string  - Last modified time
  .GitBranch     string  - Git branch (if any)

Example custom template:
  {{range .}}{{.DisplayName}}: {{.SessionCount}} sessions
  {{end}}

```
thinkt projects summary [flags]
```

### Options

```
      --asc               sort ascending (default for name)
      --desc              sort descending (default for time)
  -h, --help              help for summary
      --sort string       sort by: name, time (default "time")
      --template string   custom Go text/template for output
      --with-sessions     include session names in output
```

### Options inherited from parent commands

```
  -s, --source stringArray   source to include (kimi|claude, can be specified multiple times, default: all)
  -v, --verbose              verbose output
```

### SEE ALSO

* [thinkt projects](thinkt_projects.md)	 - List projects from all sources

