---
title: "thinkt relay"
---

## thinkt relay

Relay traces to a remote collector

### Synopsis

Relay local AI coding assistant traces to a remote collector endpoint.

By default, performs a one-shot relay of all traces found in source directories.
Use --forward for continuous watch mode that ships traces as they are written.

The collector endpoint is discovered automatically:
  1. --collector-url flag or THINKT_COLLECTOR_URL env var
  2. .thinkt/collector.json in the project directory
  3. Well-known endpoint discovery
  4. Local buffer only (no remote)

Examples:
  thinkt relay                          # One-shot relay of all traces
  thinkt relay --forward                # Watch mode: continuously forward traces
  thinkt relay --flush                  # Flush the disk buffer
  thinkt relay --source claude          # Relay only Claude traces
  thinkt relay --collector-url https://collect.example.com/v1/traces

```
thinkt relay [flags]
```

### Options

```
      --api-key string         API key for collector authentication
      --collector-url string   collector URL (default: auto-discover)
      --flush                  flush the disk buffer
      --forward                continuous watch mode
  -h, --help                   help for relay
  -q, --quiet                  suppress non-error output
      --source string          filter by source (claude, kimi, etc.)
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt/)	 - Tools for AI assistant session exploration and extraction

