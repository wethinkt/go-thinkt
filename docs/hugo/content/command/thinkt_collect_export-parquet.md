---
title: "thinkt collect export-parquet"
---

## thinkt collect export-parquet

Export collected traces to Parquet files

### Synopsis

Export collected traces from the DuckDB store to Parquet files.

This is a standalone operation that should be run when the collector is not running.
DuckDB handles Parquet encoding natively — no external library needed.

Examples:
  thinkt collect export-parquet
  thinkt collect export-parquet --out /tmp/parquet-export
  thinkt collect export-parquet --since 2025-01-01 --until 2025-02-01

```
thinkt collect export-parquet [flags]
```

### Options

```
  -h, --help             help for export-parquet
      --out string       output directory (default: ~/.thinkt/exports/parquet/)
      --since string     only entries after this time (RFC3339 or YYYY-MM-DD)
      --storage string   path to collector.duckdb (default: ~/.thinkt/dbs/collector.duckdb)
      --until string     only entries before this time (RFC3339 or YYYY-MM-DD)
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt collect](thinkt_collect.md)	 - Start trace collector server

