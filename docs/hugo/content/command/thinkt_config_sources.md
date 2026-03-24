---
title: "thinkt config sources"
---

## thinkt config sources

Manage and view available session sources

### Synopsis

View and manage available AI assistant session sources.

Sources are the AI coding assistants that store session data
on this machine (e.g., Claude Code, Kimi Code, Gemini CLI, Copilot CLI, Codex CLI).

Examples:
  thinkt config sources list           # List all available sources
  thinkt config sources status         # Show detailed source status
  thinkt config sources enable claude  # Enable a source
  thinkt config sources disable kimi   # Disable a source
  thinkt config sources enable --all   # Enable all sources
  thinkt config sources disable --all  # Disable all sources

```
thinkt config sources [flags]
```

### Options

```
  -h, --help   help for sources
      --json   output as JSON
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt config](thinkt_config/)	 - Manage thinkt configuration
* [thinkt config sources disable](thinkt_config_sources_disable/)	 - Disable a source
* [thinkt config sources enable](thinkt_config_sources_enable/)	 - Enable a source
* [thinkt config sources list](thinkt_config_sources_list/)	 - List available session sources
* [thinkt config sources status](thinkt_config_sources_status/)	 - Show detailed source status

