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
  config    Manage configuration (apps, language, sources, theme)
  tui       Launch interactive TUI explorer (default)
  projects  List and manage projects
  sessions  List and manage sessions

Examples:
  thinkt                                  # Launch TUI
  thinkt config sources list              # List available sources
  thinkt config theme set dracula         # Set theme
  thinkt projects list                    # List all projects from all sources

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

* [thinkt completion](thinkt_completion/)	 - Generate the autocompletion script for the specified shell
* [thinkt config](thinkt_config/)	 - Manage thinkt configuration
* [thinkt docs](thinkt_docs/)	 - Generate documentation for thinkt
* [thinkt export](thinkt_export/)	 - Export a session as Markdown, HTML, or JSON
* [thinkt help](thinkt_help/)	 - Help topics for thinkt
* [thinkt projects](thinkt_projects/)	 - Manage and view projects
* [thinkt search](thinkt_search/)	 - Search for text across indexed sessions
* [thinkt server](thinkt_server/)	 - Manage the local HTTP server for trace exploration
* [thinkt sessions](thinkt_sessions/)	 - View and manage sessions across all sources
* [thinkt setup](thinkt_setup/)	 - Scan for AI session sources and configure thinkt
* [thinkt stats](thinkt_stats/)	 - Show usage statistics from the index
* [thinkt sync](thinkt_sync/)	 - Synchronize all local sessions into the SQLite index
* [thinkt tui](thinkt_tui/)	 - Launch interactive TUI explorer
* [thinkt version](thinkt_version/)	 - Print the version information
* [thinkt web](thinkt_web/)	 - Open the web interface in your browser

