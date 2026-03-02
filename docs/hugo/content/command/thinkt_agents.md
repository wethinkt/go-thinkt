---
title: "thinkt agents"
---

## thinkt agents

List active agents (local and remote)

### Synopsis

List all currently active AI coding agents across local and remote infrastructure.

Local agents are detected via process inspection, IDE lock files, and file modification times.
Remote agents are discovered from running collector instances.

Examples:
  thinkt agents                      # List all active agents
  thinkt agents --local              # Local agents only
  thinkt agents --remote             # Remote agents only (from collectors)
  thinkt agents --source claude      # Filter by source
  thinkt agents --json               # JSON output

```
thinkt agents [flags]
```

### Options

```
  -h, --help             help for agents
      --json             output as JSON
      --local            show only local agents
      --machine string   filter by machine fingerprint
      --remote           show only remote agents
      --source string    filter by source (claude, kimi, etc.)
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction
* [thinkt agents follow](thinkt_agents_follow.md)	 - Live-tail an agent's conversation

