---
title: "thinkt server token generate"
---

## thinkt server token generate

Generate a new authentication token

### Synopsis

Generate a cryptographically secure random token for API/MCP authentication.

The token can be used with:
  - thinkt server run --token <token>  # Secure the REST API
  - thinkt server mcp --token <token>  # Secure the MCP server
  - THINKT_MCP_TOKEN env var           # Same as above

The token format is: thinkt_YYYYMMDD_<random>

Examples:
  thinkt server token generate                  # Generate and print a token
  thinkt server token generate | pbcopy         # Copy to clipboard (macOS)
  thinkt server token generate | xclip -sel c   # Copy to clipboard (Linux)
  thinkt server token generate | clip           # Copy to clipboard (Windows)
  export THINKT_MCP_TOKEN=$(thinkt server token generate)
  thinkt server mcp --port 8786                 # Uses token from env

```
thinkt server token generate [flags]
```

### Options

```
  -h, --help   help for generate
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt server token](thinkt_server_token/)	 - Manage authentication tokens

