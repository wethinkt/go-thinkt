---
title: "Lite Server"
weight: 12
---

# Lite Server

The `thinkt serve lite` command starts a lightweight web interface for exploring your AI coding sessions. It provides a quick overview of your sources, projects, and API access without the full TUI experience.

![Thinkt Lite Server](/images/serve-lite.jpg)

## Quick Start

```bash
thinkt serve lite                # Start on default port 8785
thinkt serve lite -p 8080        # Custom port
thinkt serve lite --no-open      # Don't auto-open browser
```

The server automatically opens your browser to `http://localhost:8785`.

## Purpose

The lite server is designed for:

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
thinkt serve lite [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-p, --port int` | Server port (default 8785) |
| `-h, --help` | Help for lite |

**Inherited flags:**
| Flag | Description |
|------|-------------|
| `--host string` | Server host (default "localhost") |
| `--http-log string` | Write HTTP access log to file |
| `--log string` | Write debug log to file |
| `--no-open` | Don't auto-open browser |
| `-q, --quiet` | Suppress HTTP request logging |
| `-v, --verbose` | Verbose output |

## Examples

### Basic Usage

```bash
# Start lite server
thinkt serve lite

# Opens browser to http://localhost:8785
```

### Custom Port

```bash
thinkt serve lite -p 3000
# Opens browser to http://localhost:3000
```

### Headless Mode

```bash
# Start without opening browser (useful for remote access)
thinkt serve lite --no-open --host 0.0.0.0

# Access from another machine at http://your-ip:8785
```

### With Logging

```bash
# Log HTTP requests to file
thinkt serve lite --http-log access.log

# Enable debug logging
thinkt serve lite --log debug.log --verbose
```

## Docker Usage

Run the lite server in Docker:

```bash
docker run --rm -p 8785:8785 \
  -v ~/.claude:/data/.claude:ro \
  -v ~/.kimi:/data/.kimi:ro \
  ghcr.io/wethinkt/thinkt serve lite --host 0.0.0.0
```

Access at `http://localhost:8785`.

## Comparison with Full Server

| Feature | `serve lite` | `serve` |
|---------|--------------|---------|
| Port | 8785 | 8784 |
| REST API | Yes | Yes |
| Web UI | Lightweight debug interface | Full webapp ([thinkt-web](https://github.com/wethinkt/thinkt-web)) |
| Swagger docs | Yes | Yes |
| Purpose | Debugging, quick inspection | Full trace exploration |

{{< hint info >}}
**Tip:** For full visual exploration of your AI coding sessions, use `thinkt serve` instead. The lite server is designed for quick inspection and API debugging.
{{< /hint >}}

## See Also

- [thinkt serve lite](/command/thinkt_serve_lite) - Command reference
- [REST API](/rest-api) - API documentation
- [Docker](/docker) - Running in containers
