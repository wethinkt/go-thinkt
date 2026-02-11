---
title: "thinkt serve"
---

## thinkt serve

Start local HTTP server for trace exploration

### Synopsis

Start a local HTTP server for exploring AI conversation traces.

The server provides:
  - REST API for accessing projects and sessions
  - Web interface for visual trace exploration

All data stays on your machine - nothing is uploaded to external servers.

Use 'thinkt serve mcp' for MCP (Model Context Protocol) server.

Examples:
  thinkt serve                    # Start HTTP server on default port 8784
  thinkt serve -p 8080            # Start on custom port
  thinkt serve --no-open          # Don't auto-open browser
  thinkt serve --dev http://localhost:5173  # Proxy to frontend dev server

```
thinkt serve [flags]
```

### Options

```
      --cors-origin string   CORS Access-Control-Allow-Origin (default "*", env: THINKT_CORS_ORIGIN)
      --dev string           dev mode: proxy non-API routes to this URL (e.g. http://localhost:5173)
  -h, --help                 help for serve
      --host string          server host (default "localhost")
      --http-log string      write HTTP access log to file (default: stdout, unless --quiet)
      --log string           write debug log to file
      --no-open              don't auto-open browser
  -p, --port int             server port (default 8784)
  -q, --quiet                suppress HTTP request logging (errors still go to stderr)
      --token string         bearer token for API authentication (default: use THINKT_API_TOKEN env var)
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction
* [thinkt serve fingerprint](thinkt_serve_fingerprint.md)	 - Display the machine fingerprint
* [thinkt serve lite](thinkt_serve_lite.md)	 - Start lightweight webapp for debugging and development
* [thinkt serve mcp](thinkt_serve_mcp.md)	 - Start MCP server for AI tool integration
* [thinkt serve token](thinkt_serve_token.md)	 - Generate a secure authentication token

