# thinking-tracer-tools

[![CI](https://github.com/Brain-STM-org/thinking-tracer-tools/actions/workflows/ci.yml/badge.svg)](https://github.com/Brain-STM-org/thinking-tracer-tools/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/Brain-STM-org/thinking-tracer-tools.svg)](https://pkg.go.dev/github.com/Brain-STM-org/thinking-tracer-tools)

Companion tools for [thinking-tracer](https://github.com/Brain-STM-org/thinking-tracer), providing utilities for extracting and processing LLM conversation traces.

## Overview

This project provides command-line tools to work with LLM conversation trace files. The initial focus is on Claude Code traces (`.jsonl` files from `~/.claude/projects/`).

### Features

- **Prompt Extraction**: Generate a timestamped log of user prompts from conversation traces
- **Multiple Formats**: Output as markdown, JSON, or plain text
- **Custom Templates**: Customize markdown output with Go templates
- **Session Inspection**: List trace files and view session metadata

## Installation

### Homebrew

```bash
brew install --cask brain-stm-org/tap/thinkt-prompts
```

### Go

```bash
go install github.com/Brain-STM-org/thinking-tracer-tools/cmd/thinkt-prompts@latest
```

### From Source

```bash
git clone https://github.com/Brain-STM-org/thinking-tracer-tools.git
cd thinking-tracer-tools
task build
```

## Tools

### thinkt-prompts

Extract user prompts from LLM agent trace files.

```bash
# Extract prompts from the latest Claude Code session
thinkt-prompts extract -t claude

# Extract from a specific trace file
thinkt-prompts extract -t claude -i session.jsonl

# Write to a file instead of stdout
thinkt-prompts extract -t claude -o PROMPTS.md

# Use a different base directory
thinkt-prompts extract -t claude -d /path/to/.claude

# List available trace files
thinkt-prompts list -t claude

# Show session info
thinkt-prompts info -t claude
```

Output defaults to stdout. Markdown, JSON, and plain text formats are supported via `-f`.

Templates control the markdown output format and can be customized with `--template`. See the [thinkt-prompts README](cmd/thinkt-prompts/README.md) for the full template reference and available variables.

## Related Projects

- [thinking-tracer](https://github.com/Brain-STM-org/thinking-tracer) - 3D visualization tool for exploring LLM conversation traces

## License

MIT License - see [LICENSE.txt](./LICENSE.txt)
