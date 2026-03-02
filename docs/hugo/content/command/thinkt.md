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

* [thinkt agents](thinkt_agents.md)	 - List active agents (local and remote)
* [thinkt apps](thinkt_apps.md)	 - Manage open-in apps and default terminal
* [thinkt collect](thinkt_collect.md)	 - Start trace collector server
* [thinkt completion](thinkt_completion.md)	 - Generate the autocompletion script for the specified shell
* [thinkt docs](thinkt_docs.md)	 - Generate documentation for thinkt
* [thinkt embeddings](thinkt_embeddings.md)	 - Manage embedding model, storage, and sync
* [thinkt export](thinkt_export.md)	 - Export traces to a remote collector
* [thinkt indexer](thinkt_indexer.md)	 - Specialized indexing and search via DuckDB (requires thinkt-indexer)
* [thinkt language](thinkt_language.md)	 - Get or set the display language
* [thinkt projects](thinkt_projects.md)	 - Manage and view projects
* [thinkt prompts](thinkt_prompts.md)	 - Extract and manage prompts from trace files
* [thinkt search](thinkt_search.md)	 - Search for text across indexed sessions
* [thinkt semantic](thinkt_semantic.md)	 - Search sessions by meaning using on-device embeddings
* [thinkt server](thinkt_server.md)	 - Manage the local HTTP server for trace exploration
* [thinkt sessions](thinkt_sessions.md)	 - View and manage sessions across all sources
* [thinkt sources](thinkt_sources.md)	 - Manage and view available session sources
* [thinkt teams](thinkt_teams.md)	 - List and inspect agent teams
* [thinkt theme](thinkt_theme.md)	 - Browse and manage TUI themes
* [thinkt tui](thinkt_tui.md)	 - Launch interactive TUI explorer
* [thinkt version](thinkt_version.md)	 - Print the version information
* [thinkt web](thinkt_web.md)	 - Open the web interface in your browser

