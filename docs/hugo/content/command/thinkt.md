---
title: "thinkt"
---

## thinkt

Tools for AI assistant session exploration and extraction

### Synopsis

thinkt provides tools for exploring and extracting data from AI coding assistant sessions.

Supports: Claude Code, Kimi Code, Gemini CLI, GitHub Copilot CLI, Codex CLI

Running without a subcommand launches the interactive TUI.

Commands:
  sources   Manage and view available session sources
  tui       Launch interactive TUI explorer (default)
  prompts   Extract and manage prompts from trace files
  projects  List and manage projects
  sessions  List and manage sessions

Examples:
  thinkt                          # Launch TUI
  thinkt sources list             # List available sources (claude, kimi, gemini, copilot, codex)
  thinkt projects list            # List all projects from all sources

```
thinkt [flags]
```

### Options

```
  -h, --help         help for thinkt
      --log string   write debug log to file
  -v, --verbose      verbose output
```

### SEE ALSO

* [thinkt agents](thinkt_agents/)	 - List active agents (local and remote)
* [thinkt apps](thinkt_apps/)	 - Manage open-in apps and default terminal
* [thinkt collect](thinkt_collect/)	 - Start trace collector server
* [thinkt completion](thinkt_completion/)	 - Generate the autocompletion script for the specified shell
* [thinkt embeddings](thinkt_embeddings/)	 - Manage embedding model, storage, and sync
* [thinkt export](thinkt_export/)	 - Export traces to a remote collector
* [thinkt indexer](thinkt_indexer/)	 - Specialized indexing and search via DuckDB (requires thinkt-indexer)
* [thinkt language](thinkt_language/)	 - Get or set the display language
* [thinkt projects](thinkt_projects/)	 - Manage and view projects
* [thinkt prompts](thinkt_prompts/)	 - Extract and manage prompts from trace files
* [thinkt search](thinkt_search/)	 - Search for text across indexed sessions
* [thinkt semantic](thinkt_semantic/)	 - Search sessions by meaning using on-device embeddings
* [thinkt server](thinkt_server/)	 - Manage the local HTTP server for trace exploration
* [thinkt sessions](thinkt_sessions/)	 - View and manage sessions across all sources
* [thinkt setup](thinkt_setup/)	 - Scan for AI session sources and configure thinkt
* [thinkt sources](thinkt_sources/)	 - Manage and view available session sources
* [thinkt teams](thinkt_teams/)	 - List and inspect agent teams
* [thinkt theme](thinkt_theme/)	 - Browse and manage TUI themes
* [thinkt tui](thinkt_tui/)	 - Launch interactive TUI explorer
* [thinkt version](thinkt_version/)	 - Print the version information
* [thinkt web](thinkt_web/)	 - Open the web interface in your browser

