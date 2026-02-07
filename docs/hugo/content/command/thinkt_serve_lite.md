---
title: "thinkt serve lite"
---

## thinkt serve lite

Start lightweight webapp for debugging and development

### Synopsis

Start a lightweight HTTP server with a simple debug interface.

The lite webapp provides:
  - Overview of available sources and their status
  - List of all projects with session counts
  - Quick links to API endpoints and documentation

The --ttl flag controls how long cached data (projects, sessions, teams)
is considered fresh before being re-read from disk. Default is 60s.

This is useful for developers and debugging. For the full experience,
use 'thinkt serve' (coming soon) or the TUI with 'thinkt'.

Examples:
  thinkt serve lite                   # Start lite server on port 8785
  thinkt serve lite -p 8080           # Start on custom port
  thinkt serve lite --host 0.0.0.0    # Bind to all interfaces
  thinkt serve lite --no-open         # Don't auto-open browser
  thinkt serve lite --ttl 30s         # Refresh cache every 30 seconds
  thinkt serve lite --ttl 0           # Cache forever (no refresh)

```
thinkt serve lite [flags]
```

### Options

```
  -h, --help           help for lite
      --host string    server host (default "localhost")
      --log string     write debug log to file
      --no-open        don't auto-open browser
  -p, --port int       server port (default 8785)
      --ttl duration   cache TTL for refreshing source data (0 to cache forever) (default 1m0s)
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt serve](thinkt_serve.md)	 - Start local HTTP server for trace exploration

