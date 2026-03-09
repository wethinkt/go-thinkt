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
  thinkt indexer start                       # Start indexer in background
  thinkt indexer status                      # Check indexer status
  thinkt indexer stop                        # Stop background indexer
  thinkt indexer sync                        # Sync all local sessions to the index
  thinkt indexer search "query"              # Search across all sessions
  thinkt indexer summarize tags "trace tag"  # Suggest shareable tags

### Options

```
      --db string              path to DuckDB index database file
      --embeddings-db string   path to DuckDB embeddings database file
  -h, --help                   help for indexer
      --log string             path to log file
  -q, --quiet                  suppress progress output
  -v, --verbose                verbose output
```

### SEE ALSO

* [thinkt](thinkt/)	 - Tools for AI assistant session exploration and extraction
* [thinkt indexer embeddings](thinkt_indexer_embeddings/)	 - Manage embedding model, storage, and sync
* [thinkt indexer help](thinkt_indexer_help/)	 - Help about any command
* [thinkt indexer logs](thinkt_indexer_logs/)	 - View indexer logs
* [thinkt indexer metrics](thinkt_indexer_metrics/)	 - Show Prometheus metrics from the running indexer
* [thinkt indexer search](thinkt_indexer_search/)	 - Search for text across indexed sessions
* [thinkt indexer semantic](thinkt_indexer_semantic/)	 - Semantic search and index management
* [thinkt indexer sessions](thinkt_indexer_sessions/)	 - List sessions for a project from the index
* [thinkt indexer start](thinkt_indexer_start/)	 - Start indexer in background
* [thinkt indexer stats](thinkt_indexer_stats/)	 - Show usage statistics from the index
* [thinkt indexer status](thinkt_indexer_status/)	 - Show indexer status
* [thinkt indexer stop](thinkt_indexer_stop/)	 - Stop background indexer
* [thinkt indexer summarize](thinkt_indexer_summarize/)	 - Manage summarization model, storage, sync, and tags
* [thinkt indexer sync](thinkt_indexer_sync/)	 - Synchronize all local sessions into the index
* [thinkt indexer version](thinkt_indexer_version/)	 - Print version information

