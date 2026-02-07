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
**Tip:** The TUI auto-detects sessions from Claude Code, Kimi Code, Gemini CLI, and Copilot CLI. If launched from a project directory, it skips straight to the session picker.
{{< /hint >}}

---

## Sources

View which AI assistants have session data on your machine:

```bash
thinkt sources list         # List available sources
thinkt sources status       # Detailed status with paths
```

Supported sources: `claude`, `kimi`, `gemini`, `copilot`

---

## Projects

Projects correspond to directories where you've used AI coding assistants.

### List Projects

```bash
thinkt projects                        # Detailed: source, sessions, modified time
thinkt projects --short                # Simple list (paths only)
thinkt projects tree                   # Tree view grouped by parent directory
thinkt projects --source claude        # Only Claude Code projects
thinkt projects --source kimi --source claude  # Multiple sources
```

### Project Details

```bash
thinkt projects summary               # Interactive picker
thinkt projects summary ./myproject   # Specific project
```

### Manage Projects

```bash
thinkt projects copy ./myproject ./backup    # Copy all sessions
thinkt projects delete ./myproject           # Delete project (with confirmation)
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
thinkt sessions view <session-id>     # View specific session
thinkt sessions summary               # Detailed session info
```

### Manage Sessions

```bash
thinkt sessions copy <session-id> ./backup   # Copy session
thinkt sessions delete <session-id>          # Delete session
```

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

Start a local web server for visual exploration:

```bash
thinkt serve                          # Default port 8784
thinkt serve -p 8080                  # Custom port
thinkt serve --no-open                # Don't open browser
thinkt serve --quiet                  # Suppress request logging
```

All data stays on your machine.

### MCP Server

Start an MCP server for AI tool integration:

```bash
thinkt serve mcp                      # stdio (for Claude Desktop)
thinkt serve mcp --port 8786          # HTTP/SSE transport
```

See the [MCP Server Guide](/mcp-server) for configuration details.

**Reference:** [thinkt serve](/command/thinkt_serve)

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
thinkt serve fingerprint              # Human-readable output
thinkt serve fingerprint --json       # JSON output
```

The fingerprint is derived from system identifiers:
- **macOS**: IOPlatformUUID from `ioreg`
- **Linux**: `/etc/machine-id` or `/var/lib/dbus/machine-id`
- **Windows**: MachineGuid from registry

If no system identifier is available, a fingerprint is generated and cached in `~/.thinkt/machine_id`.

---

## Theming

Customize the TUI appearance:

```bash
thinkt theme                          # Show current theme
thinkt theme list                     # List available themes
thinkt theme set light                # Switch theme
thinkt theme set dark
thinkt theme builder                  # Interactive theme builder
thinkt theme --json                   # Export theme as JSON
```

Built-in themes: `dark`, `light`

Custom themes can be added to `~/.thinkt/themes/`

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
thinkt sessions view <session-id>
```

### Export for Sharing

```bash
# Copy sessions to share
thinkt sessions copy <session-id> ./export

# Extract just the prompts
thinkt prompts extract -i ./export/session.jsonl -o prompts.md
```

---

## Command Reference

For complete documentation of all commands and options, see the [Command Reference](/command/).
