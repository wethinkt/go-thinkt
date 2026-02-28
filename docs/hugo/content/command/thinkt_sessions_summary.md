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
  <no value>       Full path to session file
  <no value>  Session identifier
  <no value>     Source type (kimi, claude)
  <no value>    First prompt summary (if available)
  <no value>   Number of messages
  <no value>    Creation time (time.Time)
  <no value>   Last modified time (time.Time)
  <no value>     Git branch (if available)

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
      --log string           write debug log to file
      --pick                 force project picker even if in a known project directory
  -p, --project string       project path (auto-detects from cwd if not set)
  -s, --source stringArray   filter by source (claude|kimi|gemini|copilot|codex|qwen, can be specified multiple times)
  -v, --verbose              verbose output
```

### SEE ALSO

* [thinkt sessions](thinkt_sessions.md)	 - View and manage sessions across all sources

