---
title: "thinkt server token"
---

## thinkt server token

Generate a secure authentication token

### Synopsis

Generate a cryptographically secure random token for API/MCP authentication.

The token can be used with:
  - thinkt server --token <token>      # Secure the REST API
  - thinkt server mcp --token <token>  # Secure the MCP server
  - THINKT_MCP_TOKEN env var           # Same as above

The token format is: thinkt_YYYYMMDD_<random>

Examples:
  thinkt server token                  # Generate and print a token
  thinkt server token | pbcopy         # Copy to clipboard (macOS)
  thinkt server token | xclip -sel c   # Copy to clipboard (Linux)
  thinkt server token | clip           # Copy to clipboard (Windows)
  export THINKT_MCP_TOKEN=$(thinkt server token)
  thinkt server mcp --port 8786        # Uses token from env

```
thinkt server token [flags]
```

### Options

```
  -h, --help   help for token
```

### Options inherited from parent commands

```
      --cors-origin string   CORS Access-Control-Allow-Origin (default "*", env: THINKT_CORS_ORIGIN)
      --no-indexer           don't auto-start the background indexer
  -v, --verbose              verbose output
```

### SEE ALSO

* [thinkt server](thinkt_server.md)	 - Manage the local HTTP server for trace exploration

