---
title: "thinkt server http-logs"
---

## thinkt server http-logs

View HTTP access logs

```
thinkt server http-logs [flags]
```

### Options

```
  -f, --follow      follow log output
  -h, --help        help for http-logs
  -n, --lines int   number of lines to show (default 50)
```

### Options inherited from parent commands

```
      --cors-origin string   CORS Access-Control-Allow-Origin (default "*" when unauthenticated, disabled when authenticated; env: THINKT_CORS_ORIGIN)
      --no-indexer           don't auto-start the background indexer
  -v, --verbose              verbose output
```

### SEE ALSO

* [thinkt server](thinkt_server.md)	 - Manage the local HTTP server for trace exploration

