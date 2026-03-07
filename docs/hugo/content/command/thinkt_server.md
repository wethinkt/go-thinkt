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
  thinkt server                    # Show server status
  thinkt server run                # Start server in foreground
  thinkt server start              # Start in background
  thinkt server status             # Check server status
  thinkt server stop               # Stop background server
  thinkt server logs               # View server logs

```
thinkt server [flags]
```

### Options

```
  -h, --help   help for server
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt/)	 - Tools for AI assistant session exploration and extraction
* [thinkt server fingerprint](thinkt_server_fingerprint/)	 - Display the machine fingerprint
* [thinkt server http-logs](thinkt_server_http-logs/)	 - View HTTP access logs
* [thinkt server logs](thinkt_server_logs/)	 - View server logs
* [thinkt server mcp](thinkt_server_mcp/)	 - Start MCP server for AI tool integration
* [thinkt server metrics](thinkt_server_metrics/)	 - Fetch Prometheus metrics from the running server
* [thinkt server run](thinkt_server_run/)	 - Start server in foreground
* [thinkt server start](thinkt_server_start/)	 - Start server in background
* [thinkt server status](thinkt_server_status/)	 - Show server status
* [thinkt server stop](thinkt_server_stop/)	 - Stop background server
* [thinkt server token](thinkt_server_token/)	 - Manage authentication tokens

