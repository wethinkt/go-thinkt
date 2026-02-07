---
title: "thinkt sessions summary"
---

## thinkt sessions summary

Show detailed session summary

### Synopsis

Show detailed information about each session in a project.

Sorting:
  --sort name|time    Sort by session name or modified time (default: time)
  --desc              Sort descending (default for time)
  --asc               Sort ascending (default for name)

Output can be customized with a Go text/template via --template.

Template variables:
  {{.Path}}       Full path to session file
  {{.SessionID}}  Session identifier
  {{.Source}}     Source type (kimi, claude)
  {{.Summary}}    First prompt summary (if available)
  {{.Messages}}   Number of messages
  {{.Created}}    Creation time (time.Time)
  {{.Modified}}   Last modified time (time.Time)
  {{.Branch}}     Git branch (if available)

```
thinkt sessions summary [flags]
```

### Options

```
      --asc               sort ascending (default for name)
      --desc              sort descending (default for time)
  -h, --help              help for summary
      --sort string       sort by: name, time (default "time")
      --template string   custom Go text/template for output
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

