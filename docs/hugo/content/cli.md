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

# Search across all sessions
thinkt search "authentication"

# View token usage stats
thinkt stats tokens
```

## Interactive TUI

Running `thinkt` without arguments launches a three-column terminal interface:

```bash
thinkt
thinkt tui          # Explicit command
thinkt tui --log debug.log  # With debug logging
```

**Navigation:**
- **Column 1**: Project directories
- **Column 2**: Sessions with timestamps
- **Column 3**: Conversation content with colored blocks

**Keyboard shortcuts:**
- Arrow keys / `j`/`k`: Navigate
- `Enter`: Select / expand
- `T`: Open thinking-tracer for selected session
- `q`: Quit

{{< hint info >}}
**Tip:** The TUI auto-detects sessions from Claude Code, Kimi Code, Gemini CLI, and Copilot CLI.
{{< /hint >}}

---

## Sources

View which AI assistants have session data on your machine:

```bash
thinkt sources list     # List available sources
thinkt sources status   # Detailed status with paths
```

Supported sources: `claude`, `kimi`, `gemini`, `copilot`

---

## Projects

Projects correspond to directories where you've used AI coding assistants.

### List Projects

```bash
thinkt projects                        # Simple list (paths only)
thinkt projects --long                 # Detailed: source, sessions, modified time
thinkt projects --tree                 # Tree view grouped by parent directory
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

## Search

Full-text search across all sessions using DuckDB:

```bash
thinkt search "error handling"
thinkt search -p ./myproject "database"      # Limit to project
thinkt search --limit 100 "authentication"   # More results
thinkt search --json "api" | jq .            # JSON output
```

Search looks in both user messages and assistant responses.

**Reference:** [thinkt search](/command/thinkt_search)

---

## Statistics

Analyze your AI coding sessions with various analytics:

### Token Usage

```bash
thinkt stats tokens                   # Token usage by session
thinkt stats tokens -p ./myproject    # Limit to project
thinkt stats tokens --json            # JSON output
```

### Tool Usage

```bash
thinkt stats tools                    # Most used tools
thinkt stats tools --limit 50         # More results
thinkt stats errors                   # Tool errors and failures
```

### Activity

```bash
thinkt stats activity                 # Daily activity timeline
thinkt stats activity --days 7        # Last week
```

### Other Stats

```bash
thinkt stats models                   # Model usage breakdown
thinkt stats words                    # Word frequency in prompts
thinkt stats words --limit 100        # Top 100 words
```

**Reference:** [thinkt stats](/command/thinkt_stats)

---

## Raw SQL Queries

For advanced analysis, run raw SQL queries against session data:

```bash
thinkt query "SELECT COUNT(*) FROM read_json_auto('~/.claude/projects/*/*.jsonl')"

thinkt query "SELECT DISTINCT json_extract_string(entry, '$.model')
              FROM read_json_auto('~/.claude/projects/*/*.jsonl')"
```

DuckDB can read JSONL files directly with `read_json_auto()`.

**Reference:** [thinkt query](/command/thinkt_query)

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
thinkt serve                          # Default port 7433
thinkt serve -p 8080                  # Custom port
thinkt serve --no-open                # Don't open browser
thinkt serve --quiet                  # Suppress request logging
```

All data stays on your machine.

### MCP Server

Start an MCP server for AI tool integration:

```bash
thinkt serve mcp                      # stdio (for Claude Desktop)
thinkt serve mcp --port 8081          # HTTP/SSE transport
```

See the [MCP Server Guide](/mcp-server) for configuration details.

**Reference:** [thinkt serve](/command/thinkt_serve)

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
| `--profile <file>` | Write CPU profile to file |
| `-h, --help` | Help for any command |

---

## Common Workflows

### Daily Review

```bash
# See what you worked on today
thinkt stats activity --days 1

# Check token usage
thinkt stats tokens

# Browse sessions in TUI
thinkt
```

### Find Past Conversations

```bash
# Search for a topic
thinkt search "websocket implementation"

# View matching session
thinkt sessions view <session-id>
```

### Analyze Tool Usage

```bash
# What tools are used most?
thinkt stats tools

# Any errors?
thinkt stats errors
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
