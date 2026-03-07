---
title: "thinkt server token show"
---

## thinkt server token show

Print the running server's authentication token

### Synopsis

Print the authentication token for a running server.

Exits with an error if no server is running or it has no token.

Examples:
  thinkt server token show             # Print the active token
  thinkt server token show | pbcopy    # Copy to clipboard (macOS)

```
thinkt server token show [flags]
```

### Options

```
  -h, --help   help for show
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt server token](thinkt_server_token/)	 - Manage authentication tokens

