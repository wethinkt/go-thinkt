---
title: "Lite Interface"
weight: 12
---

# Lite Interface

The `thinkt web lite` command opens a lightweight web interface for exploring your AI coding sessions. It provides a quick overview of your sources, projects, and API access without the full TUI experience.

The lite interface is served at `/lite/` on the main server (port 8784 by default).

![Thinkt Lite Server](/images/serve-lite.jpg)

## Quick Start

```bash
thinkt web lite                # Open lite interface in browser
thinkt web lite --no-open      # Don't auto-open browser
```

If the server is not already running, it will be started in the background.

## Purpose

The lite interface is designed for:

- **Quick inspection** - See all your projects and sources at a glance
- **API exploration** - Test API endpoints with built-in viewers
- **Debugging** - Verify thinkt can find your session files
- **Development** - Lightweight alternative when building integrations

## Features

### Sources Panel

The left sidebar shows all detected AI coding assistant sources:

| Source | Color | Description |
|--------|-------|-------------|
| Claude | Orange | Claude Code sessions |
| Kimi | Purple | Kimi Code sessions |
| Gemini | Blue | Gemini CLI sessions |
| Copilot | Green | GitHub Copilot CLI sessions |
| Codex | Azure | Codex CLI sessions |

Each source displays:
- **Name** with color-coded indicator
- **Base path** where sessions are stored
- **Status** (OK or N/A)
- **Visibility toggle** - Click the eye icon to show/hide projects from that source

### Projects Panel

Projects are aggregated by path across all sources. If you've used multiple AI assistants in the same project directory, they appear as a single entry with multiple source badges.

**Features:**
- **Aggregated view** - Same project path from different sources combined
- **Source badges** - Color-coded badges showing which sources have sessions, with counts
- **Dimmed home directory** - The `/Users/you` or `/home/you` portion is dimmed for readability
- **Session counts** - Total sessions across all sources
- **Last modified** - Relative date (today, yesterday, 3 days ago)
- **Click to copy** - Click a project path to copy it to clipboard
- **Open-in dropdown** - Quick open in Finder, VS Code, Cursor, etc.

### Apps Panel

Shows configured applications for the "Open In" feature. Available apps depend on the platform:
- **macOS**: Finder, Terminal, iTerm
- **Linux**: File Manager (xdg-open), Terminal (x-terminal-emulator)
- **Windows**: Explorer, Windows Terminal, Command Prompt
- **All platforms**: VS Code, Cursor, Zed (when installed)

### Themes Panel

Displays available TUI themes with the active theme highlighted.

### API Viewers

Quick-access buttons to view raw API responses:
- **API: Sources** - `/api/v1/sources`
- **API: Projects** - `/api/v1/projects`
- **API: Apps** - `/api/v1/open-in/apps`
- **API: Themes** - `/api/v1/themes`

Click any button to open a modal with syntax-highlighted JSON.

### Connection Status

The top-right shows real-time connection status:
- **Green** - Connected to API
- **Orange** - Checking connection
- **Red** - Disconnected

Click to manually refresh the connection status.

### Language Support

Switch between languages using the top-right selector:
- English (EN)
- Spanish (ES)
- Chinese (中文)

## Command Reference

```bash
thinkt web lite [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-h, --help` | Help for lite |
| `--no-open` | Don't auto-open browser |

## Examples

### Basic Usage

```bash
# Open lite interface
thinkt web lite

# Opens browser to http://localhost:8784/lite/
```

### Direct Access

```bash
# If the server is already running, access directly:
# http://localhost:8784/lite/
```

### With Logging

```bash
# Start server with logging, then open lite
thinkt server start
thinkt web lite
thinkt server http-logs -f    # Follow HTTP access logs
```

## Docker Usage

Run the server in Docker and access the lite interface:

```bash
docker run --rm -p 8784:8784 \
  -v ~/.claude:/data/.claude:ro \
  -v ~/.kimi:/data/.kimi:ro \
  -v ~/.codex:/data/.codex:ro \
  ghcr.io/wethinkt/thinkt server run --host 0.0.0.0 --no-auth
```

Access the lite interface at `http://localhost:8784/lite/`.

## Comparison with Full Web App

| Feature | Lite (`/lite/`) | Full (`/`) |
|---------|-----------------|------------|
| REST API | Yes | Yes |
| Web UI | Lightweight debug interface | Full webapp ([thinkt-web](https://github.com/wethinkt/thinkt-web)) |
| Swagger docs | Yes | Yes |
| Purpose | Debugging, quick inspection | Full trace exploration |

Both are served on the same port (8784 by default).

{{< hint info >}}
**Tip:** For full visual exploration of your AI coding sessions, use `thinkt web` instead. The lite interface is designed for quick inspection and API debugging.
{{< /hint >}}

## See Also

- [REST API](/rest-api) - API documentation
- [Docker](/docker) - Running in containers
