---
title: "thinkt server fingerprint"
---

## thinkt server fingerprint

Display the machine fingerprint

### Synopsis

Display the unique machine fingerprint used to identify this workspace.

The fingerprint is derived from system identifiers when available:
  - macOS: IOPlatformUUID from ioreg
  - Linux: /etc/machine-id or /var/lib/dbus/machine-id
  - Windows: MachineGuid from registry

If no system identifier is available, a fingerprint is generated and cached
in ~/.thinkt/machine_id for consistency across restarts.

This fingerprint can be used to correlate sessions across different AI coding
assistant sources (Kimi, Claude, Gemini, Copilot, Codex, Qwen) on the same machine.

Examples:
  thinkt server fingerprint            # Display fingerprint
  thinkt server fingerprint --json     # Output as JSON

```
thinkt server fingerprint [flags]
```

### Options

```
  -h, --help   help for fingerprint
      --json   output as JSON
```

### Options inherited from parent commands

```
      --cors-origin string   CORS Access-Control-Allow-Origin (default "*", env: THINKT_CORS_ORIGIN)
      --no-indexer           don't auto-start the background indexer
  -v, --verbose              verbose output
```

### SEE ALSO

* [thinkt server](thinkt_server.md)	 - Manage the local HTTP server for trace exploration

