---
title: "thinkt serve token"
---

## thinkt serve token

Generate a secure authentication token

### Synopsis

Generate a cryptographically secure random token for API/MCP authentication.

The token can be used with:
  - thinkt serve --token <token>      # Secure the REST API
  - thinkt serve mcp --token <token>  # Secure the MCP server
  - THINKT_MCP_TOKEN env var          # Same as above

The token format is: thinkt_YYYYMMDD_<random>

Examples:
  thinkt serve token                  # Generate and print a token
  thinkt serve token | pbcopy         # Generate and copy to clipboard (macOS)
  export THINKT_MCP_TOKEN=$(thinkt serve token)
  thinkt serve mcp --port 8786        # Uses token from env

```
thinkt serve token [flags]
```

### Options

```
  -h, --help   help for token
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt serve](thinkt_serve.md)	 - Start local HTTP server for trace exploration

