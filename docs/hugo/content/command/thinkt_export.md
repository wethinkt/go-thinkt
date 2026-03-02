---
title: "thinkt export"
---

## thinkt export

Export traces to a remote collector

### Synopsis

Export local AI coding assistant traces to a remote collector endpoint.

By default, performs a one-shot export of all traces found in source directories.
Use --forward for continuous watch mode that ships traces as they are written.

The collector endpoint is discovered automatically:
  1. --collector-url flag or THINKT_COLLECTOR_URL env var
  2. .thinkt/collector.json in the project directory
  3. Well-known endpoint discovery
  4. Local buffer only (no remote)

Examples:
  thinkt export                          # One-shot export of all traces
  thinkt export --forward                # Watch mode: continuously forward traces
  thinkt export --flush                  # Flush the disk buffer
  thinkt export --source claude          # Export only Claude traces
  thinkt export --collector-url https://collect.example.com/v1/traces

```
thinkt export [flags]
```

### Options

```
      --api-key string         API key for collector authentication
      --collector-url string   collector URL (default: auto-discover)
      --flush                  flush the disk buffer
      --forward                continuous watch mode
  -h, --help                   help for export
  -q, --quiet                  suppress non-error output
      --source string          filter by source (claude, kimi, etc.)
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction

