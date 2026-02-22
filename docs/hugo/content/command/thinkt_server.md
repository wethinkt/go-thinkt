---
title: "thinkt server"
---

## thinkt server

Manage the local HTTP server for trace exploration

### Synopsis

Manage the local HTTP server for exploring AI conversation traces.

The server provides:
  - REST API for accessing projects and sessions
  - Web interface for visual trace exploration
  - MCP (Model Context Protocol) server

All data stays on your machine - nothing is uploaded to external servers.

Examples:
  thinkt server                    # Start HTTP server on default port 8784
  thinkt server start              # Start in background
  thinkt server status             # Check server status
  thinkt server stop               # Stop background server
  thinkt server logs               # View server logs
  thinkt server -p 8080            # Start on custom port

```
thinkt server [flags]
```

### Options

```
      --cors-origin string   CORS Access-Control-Allow-Origin (default "*", env: THINKT_CORS_ORIGIN)
      --dev string           dev mode: proxy non-API routes to this URL (e.g. http://localhost:5173)
  -h, --help                 help for server
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
* [thinkt server fingerprint](thinkt_server_fingerprint.md)	 - Display the machine fingerprint
* [thinkt server logs](thinkt_server_logs.md)	 - View server logs
* [thinkt server mcp](thinkt_server_mcp.md)	 - Start MCP server for AI tool integration
* [thinkt server start](thinkt_server_start.md)	 - Start server in background
* [thinkt server status](thinkt_server_status.md)	 - Show server status
* [thinkt server stop](thinkt_server_stop.md)	 - Stop background server
* [thinkt server token](thinkt_server_token.md)	 - Generate a secure authentication token

