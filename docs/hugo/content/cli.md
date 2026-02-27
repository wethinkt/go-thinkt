---
title: "CLI Guide"
weight: 5
---

# Command Line Interface

thinkt provides a comprehensive CLI for exploring and analyzing AI coding assistant sessions. This guide covers the main commands and common workflows.

## Quick Start

```bash
# Launch the interactive TUI (default)
thinkt

# List all your projects
thinkt projects

# Browse sessions
thinkt sessions list
thinkt sessions view
```

## Interactive TUI

Running `thinkt` without arguments launches an interactive terminal UI with a navigation stack:

```bash
thinkt
thinkt tui          # Explicit command
thinkt tui --log debug.log  # With debug logging
```

The TUI navigates through three screens: **Project Picker** → **Session Picker** → **Session Viewer**. Press `esc` to go back, `q` or `ctrl+c` to quit.

**Project Picker:**

| Key | Action |
|-----|--------|
| `enter` | Select project / toggle directory |
| `/` | Search/filter |
| `t` | Toggle tree view / flat list |
| `space` | Toggle directory expand/collapse |
| `left` / `right` | Collapse / expand directory |
| `d` | Sort by date |
| `n` | Sort by name |
| `s` | Filter by source |
| `esc` | Back |
| `q` / `ctrl+c` | Quit |

**Session Picker:**

| Key | Action |
|-----|--------|
| `enter` | Select session |
| `/` | Search/filter |
| `s` | Filter by source |
| `esc` | Back to project picker |
| `q` / `ctrl+c` | Quit |

**Session Viewer:**

| Key | Action |
|-----|--------|
| `up` / `down` / `j` / `k` | Scroll |
| `pgup` / `pgdn` | Page up/down |
| `g` / `G` | Go to top / bottom |
| `esc` | Back to session picker |
| `q` / `ctrl+c` | Quit |

{{< hint info >}}
**Tip:** The TUI auto-detects sessions from Claude Code, Kimi Code, Gemini CLI, Copilot CLI, and Codex CLI. If launched from a project directory, it skips straight to the session picker.
{{< /hint >}}

---

## Sources

View which AI assistants have session data on your machine:

```bash
thinkt sources list         # List available sources
thinkt sources status       # Detailed status with paths
```

Supported sources: `claude`, `kimi`, `gemini`, `copilot`, `codex`

---

## Projects

Projects correspond to directories where you've used AI coding assistants.

### List Projects

```bash
thinkt projects                        # Detailed: source, sessions, modified time
thinkt projects --short                # Simple list (paths only)
thinkt projects tree                   # Tree view grouped by parent directory
thinkt projects --source claude        # Only Claude Code projects
thinkt projects --source codex --source claude  # Multiple sources
```

### Project Details

```bash
thinkt projects summary               # Interactive picker
thinkt projects summary ./myproject   # Specific project
```

### Manage Projects

```bash
thinkt projects copy ./myproject ./backup    # Copy all sessions
```

**Reference:** [thinkt projects](/command/thinkt_projects)

---

## Sessions

Sessions are individual conversations within a project.

### List Sessions

```bash
thinkt sessions list                  # Auto-detect project from cwd
thinkt sessions list -p ./myproject   # Specific project
thinkt sessions list --pick           # Force interactive picker
thinkt sessions list --source kimi    # Filter by source
```

### View Sessions

```bash
thinkt sessions view                  # Interactive picker
thinkt sessions view -p ./myproject <session-id>  # View specific session
thinkt sessions summary -p ./myproject            # Detailed session info
```

### Manage Sessions

```bash
thinkt sessions resolve -p ./myproject <session-id>   # Canonical path
thinkt sessions copy -p ./myproject <session-id> ./backup
thinkt sessions delete -p ./myproject <session-id>
```

`sessions copy` and `sessions delete` operate only on sessions discovered from registered sources. Use `sessions resolve` to inspect the canonical path first.

**Reference:** [thinkt sessions](/command/thinkt_sessions)

---

## Prompt Extraction

Extract user prompts from sessions for analysis or reuse:

```bash
thinkt prompts extract                # Latest session
thinkt prompts extract -i session.jsonl
thinkt prompts list                   # List available trace files
thinkt prompts info                   # Session information
thinkt prompts templates              # Available output templates
```

**Reference:** [thinkt prompts](/command/thinkt_prompts)

---

## Servers

### Web Interface

Start a local web server for visual trace exploration:

```bash
thinkt server                          # Full webapp on port 8784
thinkt server -p 8080                  # Custom port
thinkt server --no-open                # Don't open browser
thinkt server --quiet                  # Suppress request logging
thinkt server --dev http://localhost:8784  # Proxy to frontend dev server
thinkt web                             # Open web interface in browser
thinkt web lite                        # Lightweight debug interface at /lite/
```

`thinkt server` starts the HTTP server. `thinkt web` opens it in your browser (auto-starting if needed). `thinkt web lite` provides a lightweight debug view. All data stays on your machine.

Use `--dev` to co-develop the [thinkt-web](https://github.com/wethinkt/thinkt-web) frontend: run the frontend dev server (e.g. `npm run dev` in the thinkt-web repo), then start the Go backend with `--dev` pointing to it. API routes are served by Go; everything else is proxied to the frontend with full hot reload support.

### MCP Server

Start an MCP server for AI tool integration:

```bash
thinkt server mcp                      # stdio (for Claude Desktop)
thinkt server mcp --port 8786          # HTTP/SSE transport
```

See the [MCP Server Guide](/mcp-server) for configuration details.

**Reference:** [thinkt server](/command/thinkt_server)

### Agent Teams

Inspect multi-agent teams from Claude Code:

```bash
thinkt teams                      # List all teams
thinkt teams list                 # Same as above
thinkt teams list --json          # JSON output
thinkt teams list --active        # Only active teams
thinkt teams list --inactive      # Only inactive teams
```

**Reference:** [thinkt teams](/command/thinkt_teams)

### Machine Fingerprint

Display the unique machine identifier for workspace correlation:

```bash
thinkt server fingerprint              # Human-readable output
thinkt server fingerprint --json       # JSON output
```

The fingerprint is derived from platform-specific system identifiers:

| Platform | Source |
|----------|--------|
| macOS | IOPlatformUUID from `ioreg` |
| Linux | `/etc/machine-id` or `/var/lib/dbus/machine-id` |
| Windows | MachineGuid from registry |
| Fallback | Generated and cached in `~/.thinkt/machine_id` |

The fingerprint is also available via the REST API at `GET /api/v1/info`.

---

## Trace Collector & Exporter

Aggregate traces from multiple machines with the push-based collector system. See the [Collector Guide](/collector) for the full architecture.

### Collector

Start a collector server that receives traces from exporters:

```bash
thinkt collect                              # Start on port 8785
thinkt collect --port 8785                  # Custom port
thinkt collect --token mytoken              # Require bearer token auth
thinkt collect --storage ./traces.duckdb    # Custom storage path
```

### Exporter

Watch local sessions and ship traces to a collector:

```bash
thinkt export                               # One-shot export of all traces
thinkt export --forward                     # Continuous watch mode
thinkt export --source claude               # Export only Claude traces
thinkt export --flush                       # Flush the disk buffer
thinkt export --collector-url http://host:8785/v1/traces  # Explicit endpoint
```

### Standalone Binaries

For deployment without the full CLI:

```bash
# Exporter (repeatable --watch-dir flag)
thinkt-exporter --watch-dir ~/.claude/projects --collector-url http://collect.example.com/v1/traces

# Collector
thinkt-collector --port 8785 --token mytoken
```

**Reference:** [Collector Guide](/collector), [thinkt export](/command/thinkt_export), [thinkt collect](/command/thinkt_collect)

---

## Indexer

The indexer provides DuckDB-powered indexing and search for your session data. Most indexer commands are accessible as top-level aliases through the main `thinkt` CLI, as well as directly via the `thinkt-indexer` binary:

```bash
# Start the indexer server (syncs, watches, and serves RPC)
thinkt indexer start

# Sync all sessions to the index
thinkt-indexer sync

# Search (case-insensitive by default)
thinkt search "authentication"

# Case-sensitive search
thinkt search "AuthManager" --case-sensitive

# Regex search (Go RE2 syntax)
thinkt search --regex "func\s+Test\w+"

# Filter by project or source
thinkt search "TODO" --project my-app --source codex

# Usage statistics
thinkt indexer stats
```

### Embeddings

Manage on-device embedding models for semantic search:

```bash
thinkt embeddings list              # List available models with stats
thinkt embeddings status            # Show embedding config and status
thinkt embeddings model             # Switch embedding model (interactive)
thinkt embeddings model <id>        # Switch to specific model
thinkt embeddings enable            # Enable semantic embeddings
thinkt embeddings disable           # Disable semantic embeddings
thinkt embeddings sync              # Run embedding sync
thinkt embeddings purge             # Remove old model embeddings
```

### Semantic Search

Search sessions by meaning using on-device embeddings (nomic-embed-text-v1.5 by default, configurable via `thinkt embeddings model`). Disabled by default.

```bash
# Enable semantic search (model downloads on first sync)
thinkt embeddings enable

# Search by meaning
thinkt semantic search "database migration strategy"

# Filter and options
thinkt semantic search "error handling" --project my-app --source claude
thinkt semantic search "testing patterns" --diversity --limit 10
thinkt semantic search "API design" --max-distance 0.5

# Output formats
thinkt semantic search "query" --list    # Plain text (for scripting)
thinkt semantic search "query" --json    # JSON output

# Check status
thinkt embeddings status
thinkt embeddings status --json

# Disable
thinkt embeddings disable
```

When the indexer server is running, `enable` and `disable` take effect immediately — the server loads or unloads the embedding model at runtime without restart.

**Reference:** [thinkt indexer search](/command/thinkt_indexer_search) · [thinkt indexer semantic](/command/thinkt_indexer_semantic)

---

## Apps

Manage the apps available for "open in" actions and the default terminal:

```bash
thinkt apps                           # List all apps with status
thinkt apps list                      # Same as above
thinkt apps list --json               # JSON output (includes terminal capability)
thinkt apps enable <id>               # Enable an app
thinkt apps enable                    # Interactive picker for disabled apps
thinkt apps disable <id>              # Disable an app
thinkt apps disable                   # Interactive picker for enabled apps
thinkt apps get-terminal              # Show the default terminal app
thinkt apps set-terminal <id>         # Set the default terminal
thinkt apps set-terminal              # Interactive picker for terminal apps
```

Apps with terminal capability (those that can run shell commands) are shown in the `TERMINAL` column. Only terminal-capable apps can be set as the default terminal. The default terminal is used by the REST API to spawn resume commands.

**Reference:** [thinkt apps](/command/thinkt_apps)

---

## Theming

Customize the TUI appearance with 14 built-in themes or import your own.

### Browse and Switch Themes

```bash
thinkt theme                          # Browse themes interactively (default)
thinkt theme list                     # List all available themes
thinkt theme set dracula              # Switch to a theme
thinkt theme show                     # Show current theme with samples
thinkt theme show --json              # Export theme as JSON
thinkt theme builder                  # Interactive theme builder
```

### Built-in Themes

| Theme | Description |
|-------|-------------|
| `dark` | Default dark theme |
| `light` | Light theme for bright terminals |
| `dracula` | Dracula color scheme |
| `nord` | Nord color scheme |
| `gruvbox-dark` | Gruvbox dark variant |
| `gruvbox-light` | Gruvbox light variant |
| `catppuccin-mocha` | Catppuccin Mocha (dark) |
| `catppuccin-latte` | Catppuccin Latte (light) |
| `solarized-dark` | Solarized dark variant |
| `solarized-light` | Solarized light variant |
| `tokyo-night` | Tokyo Night color scheme |
| `rose-pine` | Rose Pine color scheme |
| `one-dark` | Atom One Dark color scheme |
| `monokai` | Monokai color scheme |

### Import iTerm2 Color Schemes

Import any `.itermcolors` file from the [iTerm2-Color-Schemes](https://github.com/mbadolato/iTerm2-Color-Schemes) repository or other sources:

```bash
thinkt theme import ~/Downloads/Zenburn.itermcolors
thinkt theme import scheme.itermcolors --name my-theme
thinkt theme set my-theme
```

Imported themes are saved to `~/.thinkt/themes/` and can be further customized with `thinkt theme builder`.

**Reference:** [thinkt theme](/command/thinkt_theme)

---

## Shell Completion

Generate shell completion scripts:

```bash
# Bash
thinkt completion bash > ~/.bash_completion.d/thinkt

# Zsh
thinkt completion zsh > "${fpath[1]}/_thinkt"

# Fish
thinkt completion fish > ~/.config/fish/completions/thinkt.fish

# PowerShell
thinkt completion powershell > thinkt.ps1
```

**Reference:** [thinkt completion](/command/thinkt_completion)

---

## Global Options

These options work with all commands:

| Flag | Description |
|------|-------------|
| `-v, --verbose` | Verbose output |
| `--log <file>` | Write debug log to file |
| `-h, --help` | Help for any command |

---

## Common Workflows

### Daily Review

```bash
# Browse sessions in TUI
thinkt

# List recent sessions
thinkt sessions list
```

### View Sessions

```bash
# View a session
thinkt sessions view -p ./myproject <session-id>
```

### Export for Sharing

```bash
# Resolve then copy a known session to share
session_path=$(thinkt sessions resolve -p ./myproject <session-id>)
thinkt sessions copy -p ./myproject <session-id> ./export

# Extract just the prompts
thinkt prompts extract -i "$session_path" -o prompts.md
```

---

## Command Reference

For complete documentation of all commands and options, see the [Command Reference](/command/).
