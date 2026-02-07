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
  thinkt indexer sync      # Sync all local sessions to the index
  thinkt indexer search    # Search across all sessions

```
thinkt indexer [flags]
```

### Options

```
  -h, --help   help for indexer
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction

