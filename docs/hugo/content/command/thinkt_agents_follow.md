---
title: "thinkt agents follow"
---

## thinkt agents follow

Live-tail an agent's conversation

### Synopsis

Stream new conversation entries from an active agent in real-time.

Local agents are tailed directly from their session files.
Remote agents are streamed via WebSocket from the collector.

Examples:
  thinkt agents follow a3f8b2c1          # Tail agent conversation
  thinkt agents follow a3f8b2c1 --json   # Structured JSON output
  thinkt agents follow a3f8b2c1 --raw    # Raw JSONL

```
thinkt agents follow [session-id] [flags]
```

### Options

```
  -h, --help   help for follow
      --json   structured JSON output
      --raw    raw JSONL output
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt agents](thinkt_agents.md)	 - List active agents (local and remote)

