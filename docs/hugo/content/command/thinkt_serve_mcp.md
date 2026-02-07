---
title: "thinkt serve mcp"
---

## thinkt serve mcp

Start MCP server for AI tool integration

### Synopsis

Start an MCP (Model Context Protocol) server for AI tool integration.

By default, runs on stdio for use with Claude Desktop and other MCP clients.
Use --port to run over HTTP instead.

Authentication:
  For stdio transport: Set THINKT_MCP_TOKEN environment variable
  For HTTP transport: Use --token flag or THINKT_MCP_TOKEN environment variable
  Clients must pass the token in the Authorization header: "Bearer <token>"
  Generate a secure token with: thinkt serve token

Examples:
  thinkt serve mcp                          # MCP server on stdio (default)
  thinkt serve mcp --stdio                  # Explicitly use stdio transport
  thinkt serve mcp --port 8786              # MCP server over HTTP (default port)
  thinkt serve mcp --port 8786 --token xyz  # MCP server with authentication

```
thinkt serve mcp [flags]
```

### Options

```
      --allow-tools strings   explicitly allow only these tools (comma-separated, default: all)
      --deny-tools strings    explicitly deny these tools (comma-separated)
  -h, --help                  help for mcp
      --host string           host to bind MCP HTTP server (default "localhost")
      --log string            write debug log to file
      --no-indexer            don't auto-start the background indexer
  -p, --port int              run MCP over HTTP on this port
      --stdio                 use stdio transport (default if no --port)
      --token string          bearer token for HTTP authentication (default: use THINKT_MCP_TOKEN env var)
```

### Options inherited from parent commands

```
      --cors-origin string   CORS Access-Control-Allow-Origin (default "*", env: THINKT_CORS_ORIGIN)
  -v, --verbose              verbose output
```

### SEE ALSO

* [thinkt serve](thinkt_serve.md)	 - Start local HTTP server for trace exploration

