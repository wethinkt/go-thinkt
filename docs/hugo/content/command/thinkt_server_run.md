---
title: "thinkt server run"
---

## thinkt server run

Start server in foreground

```
thinkt server run [flags]
```

### Options

```
      --dev string        dev mode: proxy non-API routes to this URL (e.g. http://localhost:5173)
  -h, --help              help for run
      --host string       server host (default "localhost")
      --http-log string   write HTTP access log to file (default: stdout, unless --quiet)
      --log string        write debug log to file
      --no-auth           disable authentication (allow unauthenticated access)
      --no-open           don't auto-open browser
  -p, --port int          server port (default 8784)
  -q, --quiet             suppress HTTP request logging (errors still go to stderr)
      --token string      bearer token for API authentication (default: use THINKT_API_TOKEN env var)
```

### Options inherited from parent commands

```
      --cors-origin string   CORS Access-Control-Allow-Origin (default "*" when unauthenticated, disabled when authenticated; env: THINKT_CORS_ORIGIN)
      --no-indexer           don't auto-start the background indexer
  -v, --verbose              verbose output
```

### SEE ALSO

* [thinkt server](thinkt_server.md)	 - Manage the local HTTP server for trace exploration

