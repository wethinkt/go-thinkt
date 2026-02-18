---
title: "thinkt sessions resolve"
---

## thinkt sessions resolve

Resolve a session query to its canonical path

### Synopsis

Resolve a session query (ID, filename suffix, or absolute path)
to a known session from registered sources.

By default, outputs only the canonical full path.
Use --json for structured output.

```
thinkt sessions resolve <session> [flags]
```

### Options

```
  -h, --help   help for resolve
      --json   output resolved session metadata as JSON
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

