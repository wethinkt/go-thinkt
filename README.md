# thinking-tracer-tools

Companion tools for [thinking-tracer](https://github.com/Brain-STM-org/thinking-tracer), providing utilities for extracting and processing LLM conversation traces.

## Overview

This project provides command-line tools to work with LLM conversation trace files. The initial focus is on Claude Code traces (`.jsonl` files from `~/.claude/projects/`).

### Features

- **PROMPTS.md Extraction**: Generate a timestamped log of user prompts from conversation traces
- **Claude Code Hooks**: Pre-built hooks for automatic prompt logging during sessions

## Installation

```bash
# Clone the repository
git clone https://github.com/Brain-STM-org/thinking-tracer-tools.git
cd thinking-tracer-tools

# Build using Task
task build
```

## Usage

### Prompt History Hook

Copy the `.claude/` directory to your project to enable automatic prompt logging:

```bash
cp -r .claude/ /path/to/your/project/
```

This creates a `PROMPTS.md` file that logs all user prompts with ISO 8601 timestamps.

### Manual Extraction

```bash
# Extract prompts from a trace file (coming soon)
thinking-tracer-tools extract --input ~/.claude/projects/my-project/session.jsonl
```

## Related Projects

- [thinking-tracer](https://github.com/Brain-STM-org/thinking-tracer) - 3D visualization tool for exploring LLM conversation traces

## License

MIT License - see [LICENSE.txt](./LICENSE.txt)
