# PLAN: thinkt

Current implementation status and roadmap for `thinkt`.

## Current State

The core CLI is functional with multi-source support, TUI, analytics, HTTP/MCP servers, and lite webapp.

### Recently Completed

- [x] **GoReleaser Pro with goreleaser-cross** - CGO cross-compilation
  - Builds: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`
  - Uses `ghcr.io/goreleaser/goreleaser-cross` Docker image
  - Explicit CC/CXX per target for cross-compilation

- [x] **Multi-platform Docker Images**
  - Published to `ghcr.io/wethinkt/thinkt`
  - Platforms: `linux/amd64`, `linux/arm64`
  - User home at `/data` for easy bind mounts (`~/.claude` → `/data/.claude`)
  - Two Dockerfiles:
    - `Dockerfile` - Multi-stage build for CI/local
    - `etc/Dockerfile.goreleaser` - Simple runtime for releases

- [x] **CI Improvements**
  - Docker build verification in CI workflow
  - Tests run before Docker build (`needs: test`)

- [x] **Homebrew Formula** - `brews` section in goreleaser (standard, not Pro `homebrew_casks`)

- [x] **Documentation** - README and AGENTS.md updated with Docker usage

### Release Workflow

```
Tag push (v*) → GitHub Actions → goreleaser-cross container
  ├── Build binaries (5 platforms via CGO cross-compilation)
  ├── Build Docker images (linux/amd64, linux/arm64)
  ├── Push to GHCR
  ├── Create GitHub Release with archives
  └── Update Homebrew tap
```

## In Progress

- [ ] **Homebrew tap setup** - Need to create `wethinkt/homebrew-tap` repo
- [ ] **Test Docker images** - Verify bind mounts work correctly in practice

## Security TODOs

- [x] **MCP Server Authentication** - Implemented token-based auth
  - `THINKT_MCP_TOKEN` environment variable support
  - `--token` flag for HTTP transport
  - `Authorization: Bearer <token>` header validation
  - `thinkt serve token` command for secure token generation
  - Constant-time comparison to prevent timing attacks

- [x] **API Server Authentication** - Implemented token-based auth
  - `THINKT_API_TOKEN` environment variable support
  - `--token` flag for API server
  - Same `Authorization: Bearer <token>` header validation
  - Secures REST API endpoints from unauthorized access

- [ ] **Tighten `getAllowedBaseDirectories()` in `internal/server/security.go`**
  - Current implementation allows opening any directory under user's home
  - Consider restricting to only known project directories from the registry
  - Add explicit allowlist configuration option in `~/.thinkt/config.json`
  - Review symlink handling for edge cases (symlinks to other symlinks)

## Upcoming

### Short Term

- [ ] **`thinkt setup` command** - Interactive first-run configuration
  - Create `~/.thinkt/` directory
  - Step through each source, check existence and permissions
  - Prompt user to enable/disable each source
  - Generate `~/.thinkt/config.json` with preferences
  - `--reconfigure` flag to re-run setup
  - Sources respect enabled/disabled in config
  - Environment variables still override for Docker

- [ ] **Windows arm64 support** - Currently excluded (dependency limitations)
- [ ] **Shell completions** - Add to release archives
- [ ] **Manpage improvements** - Verify man pages work in Docker

### Medium Term

- [ ] **`thinkt serve` in Docker** - Document production deployment patterns
- [ ] **Health check endpoint** - For container orchestration
- [ ] **Prometheus metrics** - For monitoring

### Long Term

- [ ] **Helm chart** - Kubernetes deployment
- [ ] **Multi-user support** - For shared deployments
- [ ] **Authentication** - For exposed servers

## Architecture

```
cmd/thinkt/           CLI entry point (Cobra)
internal/
  thinkt/             Core types, Store interface
  sources/            Source implementations (claude, kimi, gemini, copilot)
  tui/                BubbleTea terminal UI
  server/             HTTP REST API, MCP server, lite webapp
  analytics/          Analytics
  prompt/             Prompt extraction
  config/             Configuration management
```

## Docker Usage

```bash
# Run HTTP server with session data
docker run -p 7433:7433 \
  -v ~/.claude:/data/.claude:ro \
  -v ~/.kimi:/data/.kimi:ro \
  ghcr.io/wethinkt/thinkt:latest serve --host 0.0.0.0

# Run any command
docker run -v ~/.claude:/data/.claude:ro \
  ghcr.io/wethinkt/thinkt:latest projects --long
```

## Configuration (`~/.thinkt/config.json`)

Planned config structure for `thinkt setup`:

```json
{
  "sources": {
    "claude": { "enabled": true, "path": "~/.claude" },
    "kimi": { "enabled": true, "path": "~/.kimi" },
    "gemini": { "enabled": false },
    "copilot": { "enabled": true, "path": "~/.copilot" }
  }
}
```

- **enabled**: Whether source is active (respects user consent)
- **path**: Custom path override (optional, defaults to standard location)
- Environment variables (`THINKT_*_HOME`) override config for Docker/CI use cases

## Build Targets

| Platform | Arch | CGO Compiler | Status |
|----------|------|--------------|--------|
| Linux | amd64 | x86_64-linux-gnu-gcc | ✅ |
| Linux | arm64 | aarch64-linux-gnu-gcc | ✅ |
| Darwin | amd64 | o64-clang | ✅ |
| Darwin | arm64 | oa64-clang | ✅ |
| Windows | amd64 | - | ❌ (pthread issues with mingw) |
| Windows | arm64 | - | ❌ (dependency limitations) |
