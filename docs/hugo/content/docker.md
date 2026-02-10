---
title: "Docker"
weight: 20
---

# Docker

Run thinkt in a Docker container for sandboxed, read-only access to your AI coding sessions. This provides isolation between the tool and your system while still allowing exploration of your conversation history.

## Quick Start

```bash
# Pull the image
docker pull ghcr.io/wethinkt/thinkt:latest

# List your Claude Code projects
docker run --rm \
  -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt projects

# Start the web server
docker run --rm -p 8784:8784 \
  -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt serve --host 0.0.0.0
```

## Why Use Docker?

Running thinkt in a container provides several benefits:

- **Sandboxed Access** - The container only sees the directories you explicitly mount
- **Read-Only Mode** - Mount session data as read-only to prevent any modifications
- **No Installation** - No need to install Go or compile from source
- **Consistent Environment** - Same behavior across different systems
- **Easy Cleanup** - Remove the container and nothing is left behind

## Image Details

**Registries:**
- `ghcr.io/wethinkt/thinkt` (GitHub Container Registry)
- `wethinkt/thinkt` (Docker Hub)

**Tags:**
| Tag | Description |
|-----|-------------|
| `latest` | Latest stable release |
| `v1.0.0` | Specific version |
| `nightly` | Latest development build |

**Platforms:**
- `linux/amd64`
- `linux/arm64`

**Image Details:**
- Base: `debian:bookworm-slim`
- User: `thinkt` (UID 5454)
- Working directory: `/data`

---

## Mounting Session Data

The container runs as user `thinkt` with home directory `/data`. Mount your local session directories to the corresponding paths inside `/data`.

### Source Directories

| Source | Host Path | Container Path |
|--------|-----------|----------------|
| Claude Code | `~/.claude` | `/data/.claude` |
| Kimi Code | `~/.kimi` | `/data/.kimi` |
| Gemini CLI | `~/.gemini` | `/data/.gemini` |
| Copilot CLI | `~/.copilot` | `/data/.copilot` |
| Codex CLI | `~/.codex` | `/data/.codex` |

### Mount Examples

**Single source (Claude Code):**
```bash
docker run --rm \
  -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt projects
```

**Multiple sources:**
```bash
docker run --rm \
  -v ~/.claude:/data/.claude:ro \
  -v ~/.kimi:/data/.kimi:ro \
  -v ~/.gemini:/data/.gemini:ro \
  -v ~/.codex:/data/.codex:ro \
  ghcr.io/wethinkt/thinkt projects
```

**All sources:**
```bash
docker run --rm \
  -v ~/.claude:/data/.claude:ro \
  -v ~/.kimi:/data/.kimi:ro \
  -v ~/.gemini:/data/.gemini:ro \
  -v ~/.copilot:/data/.copilot:ro \
  -v ~/.codex:/data/.codex:ro \
  ghcr.io/wethinkt/thinkt sources status
```

{{< hint info >}}
**Tip:** The `:ro` suffix mounts the volume as read-only, ensuring thinkt cannot modify your session files.
{{< /hint >}}

---

## Running Commands

The container's entrypoint is `thinkt`, so pass commands directly as arguments.

### List Projects

```bash
docker run --rm \
  -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt projects

# Paths only
docker run --rm \
  -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt projects --short
```

### List Sessions

```bash
docker run --rm \
  -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt sessions list -p /data/.claude/projects/your-project
```

### View Sessions

```bash
docker run --rm \
  -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt sessions view
```

---

## Running Servers

### Web Server (Full Interface)

Start the HTTP server with the full web interface:

```bash
docker run --rm -p 8784:8784 \
  -v ~/.claude:/data/.claude:ro \
  -v ~/.kimi:/data/.kimi:ro \
  -v ~/.codex:/data/.codex:ro \
  ghcr.io/wethinkt/thinkt serve --host 0.0.0.0
```

Access at `http://localhost:8784`

**Options:**
```bash
# Custom port
docker run --rm -p 8080:8080 \
  -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt serve --host 0.0.0.0 -p 8080

# Quiet mode (less logging)
docker run --rm -p 8784:8784 \
  -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt serve --host 0.0.0.0 --quiet
```

### Lite Server (Debug Interface)

Start the lightweight debug server:

```bash
docker run --rm -p 8785:8785 \
  -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt serve lite --host 0.0.0.0
```

Access at `http://localhost:8785`

### MCP Server (HTTP Mode)

Start the MCP server over HTTP for networked clients:

```bash
docker run --rm -p 8786:8786 \
  -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt serve mcp --port 8786 --host 0.0.0.0
```

{{< hint warning >}}
**Note:** The `--host 0.0.0.0` flag is required for the server to accept connections from outside the container.
{{< /hint >}}

---

## Shell Alias

Create a shell alias for convenience:

{{< tabs "alias" >}}
{{< tab "Bash/Zsh" >}}
Add to `~/.bashrc` or `~/.zshrc`:
```bash
alias thinkt-docker='docker run --rm \
  -v ~/.claude:/data/.claude:ro \
  -v ~/.kimi:/data/.kimi:ro \
  -v ~/.gemini:/data/.gemini:ro \
  -v ~/.codex:/data/.codex:ro \
  ghcr.io/wethinkt/thinkt'
```

Usage:
```bash
thinkt-docker projects
thinkt-docker sessions list
thinkt-docker sessions view
```
{{< /tab >}}
{{< tab "Fish" >}}
Add to `~/.config/fish/config.fish`:
```fish
alias thinkt-docker='docker run --rm \
  -v ~/.claude:/data/.claude:ro \
  -v ~/.kimi:/data/.kimi:ro \
  -v ~/.gemini:/data/.gemini:ro \
  -v ~/.codex:/data/.codex:ro \
  ghcr.io/wethinkt/thinkt'
```
{{< /tab >}}
{{< /tabs >}}

For the web server, create a separate alias:

```bash
alias thinkt-serve='docker run --rm -p 8784:8784 \
  -v ~/.claude:/data/.claude:ro \
  -v ~/.kimi:/data/.kimi:ro \
  -v ~/.codex:/data/.codex:ro \
  ghcr.io/wethinkt/thinkt serve --host 0.0.0.0'
```

---

## Docker Compose

For persistent server deployment, use Docker Compose:

```yaml
# docker-compose.yml
services:
  thinkt:
    image: ghcr.io/wethinkt/thinkt:latest
    command: serve --host 0.0.0.0
    ports:
      - "8784:8784"
    volumes:
      - ~/.claude:/data/.claude:ro
      - ~/.kimi:/data/.kimi:ro
      - ~/.gemini:/data/.gemini:ro
      - ~/.codex:/data/.codex:ro
    restart: unless-stopped
```

Start:
```bash
docker compose up -d
```

Stop:
```bash
docker compose down
```

---

## Building Locally

Build the image yourself from source:

```bash
git clone --recurse-submodules https://github.com/wethinkt/go-thinkt.git
cd go-thinkt

# Build
docker build -t thinkt:local .

# Run
docker run --rm \
  -v ~/.claude:/data/.claude:ro \
  thinkt:local projects
```

---

## Security Considerations

### Read-Only Mounts

Always use the `:ro` (read-only) suffix when mounting session directories:

```bash
-v ~/.claude:/data/.claude:ro
```

This ensures thinkt cannot modify, delete, or corrupt your session files.

### Network Isolation

For maximum isolation, run without network access when not needed:

```bash
docker run --rm --network none \
  -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt projects
```

### Non-Root User

The container runs as non-root user `thinkt` (UID 5454), limiting potential damage from any vulnerabilities.

### Minimal Image

The image is based on `debian:bookworm-slim` and contains only:
- The `thinkt` binary
- CA certificates (for HTTPS)
- Basic system libraries

No shell access, no package manager in the final image, no unnecessary tools.

---

## Troubleshooting

### Permission Denied

If you see permission errors, ensure your session directories are readable:

```bash
# Check permissions
ls -la ~/.claude

# The container runs as UID 5454
# Files need to be world-readable or match that UID
```

### No Sessions Found

Verify the mount paths are correct:

```bash
# List what the container sees
docker run --rm \
  -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt sources status
```

### Port Already in Use

If the port is taken, use a different host port:

```bash
# Map container port 8784 to host port 8080
docker run --rm -p 8080:8784 \
  -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt serve --host 0.0.0.0
```

---

## See Also

- [CLI Guide](/cli) - Command line interface reference
- [REST API](/rest-api) - API documentation
- [MCP Server](/mcp-server) - AI assistant integration
