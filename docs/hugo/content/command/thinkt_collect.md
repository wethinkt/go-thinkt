---
title: "thinkt collect"
---

## thinkt collect

Start trace collector server

### Synopsis

Start a collector server that receives AI coding assistant traces from exporters.

The collector provides:
  - POST /v1/traces endpoint for trace ingestion
  - Agent registration and heartbeat tracking
  - DuckDB-backed storage for collected traces
  - Bearer token authentication (optional)

All received traces are stored locally in DuckDB for analysis.

Examples:
  thinkt collect                           # Start collector on port 8785
  thinkt collect --port 8785               # Custom port
  thinkt collect --token mytoken           # Require bearer token auth
  thinkt collect --storage ./traces.duckdb # Custom storage path

```
thinkt collect [flags]
```

### Options

```
  -h, --help             help for collect
      --host string      collector host (default "localhost")
  -p, --port int         collector port (default 8785)
  -q, --quiet            suppress non-error output
      --storage string   DuckDB storage path
      --token string     bearer token for authentication
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction
* [thinkt collect export-parquet](thinkt_collect_export-parquet.md)	 - Export collected traces to Parquet files

