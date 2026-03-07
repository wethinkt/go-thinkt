---
title: "thinkt server start"
---

## thinkt server start

Start server in background

```
thinkt server start [flags]
```

### Options

```
      --cors-origin string   CORS Access-Control-Allow-Origin (default "*" when unauthenticated, disabled when authenticated; env: THINKT_CORS_ORIGIN)
  -h, --help                 help for start
      --host string          server host (default "localhost")
      --no-auth              disable authentication (allow unauthenticated access)
      --no-indexer           don't auto-start the background indexer
  -p, --port int             server port (default 8784)
  -q, --quiet                suppress HTTP request logging (errors still go to stderr)
      --token string         bearer token for API authentication (default: use THINKT_API_TOKEN env var)
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt server](thinkt_server/)	 - Manage the local HTTP server for trace exploration

