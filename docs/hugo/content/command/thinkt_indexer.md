---
title: "thinkt indexer"
---

## thinkt indexer

Specialized indexing and search via DuckDB (requires thinkt-indexer)

### Synopsis

The indexer command provides access to DuckDB-powered indexing and
search capabilities. This requires the 'thinkt-indexer' binary to be installed
separately due to its CGO dependencies.

Examples:
  thinkt indexer sync                        # Sync all local sessions to the index
  thinkt indexer search "query"              # Search across all sessions
  thinkt indexer sync --db /custom/path.db --quiet
  thinkt indexer search "query" --limit 10
  thinkt indexer watch                       # Watch and index in real-time
  thinkt indexer stats --json                # Show usage statistics

Use 'thinkt indexer <command> --help' for detailed help on each command.

### Options

```
      --db string    path to DuckDB database file
  -h, --help         help for indexer
      --log string   path to log file
  -q, --quiet        suppress progress output
  -v, --verbose      verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction
* [thinkt indexer help](thinkt_indexer_help.md)	 - Help about any command
* [thinkt indexer search](thinkt_indexer_search.md)	 - Search for text across indexed sessions
* [thinkt indexer sessions](thinkt_indexer_sessions.md)	 - List sessions for a project from the index
* [thinkt indexer stats](thinkt_indexer_stats.md)	 - Show usage statistics from the index
* [thinkt indexer sync](thinkt_indexer_sync.md)	 - Synchronize all local sessions into the index
* [thinkt indexer version](thinkt_indexer_version.md)	 - Print version information
* [thinkt indexer watch](thinkt_indexer_watch.md)	 - Watch session directories for changes and index in real-time

