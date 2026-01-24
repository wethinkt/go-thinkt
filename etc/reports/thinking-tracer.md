# Research Report: thinking-tracer

**Date**: 2026-01-24
**Source**: https://github.com/Brain-STM-org/thinking-tracer

## Overview

thinking-tracer is a 3D visualization application for exploring LLM conversation traces. It enables users to navigate complex conversations with thinking blocks, tool calls, and multi-turn interactions in an interactive WebGL environment.

## Core Features

### Spiral Cluster Layout

- Conversations display as a spiral helix where each turn pair occupies a spatial cluster
- Creates a "slinky effect" that compresses at conversation ends while expanding near the focal point
- Click clusters to focus, double-click to expand and view individual content blocks

### 3D Navigation

- Orbit controls: drag to rotate, scroll to zoom
- Keyboard navigation via arrow keys
- Jump to first/last nodes with Home/End keys
- Click-based selection

### Metrics Dashboard

A resizable panel displays per-turn stacked bar charts showing:
- Token counts (total, input, output)
- Thinking block counts
- Tool call counts
- Content length

Click bars to jump to specific turns.

### Detail Panel

Comprehensive node information including:
- Turn summaries
- Complete thinking content
- Tool calls with JSON inputs
- Tool results with status indicators
- Toggleable raw JSON views

### Session Metadata & Exports

- Displays model information, git branches, duration, working directories
- Export to HTML (styled, collapsible sections)
- Export to Markdown

## Technical Architecture

| Component | Technology |
|-----------|------------|
| Language | TypeScript |
| 3D Rendering | Three.js (WebGL) |
| Build Tool | Vite |
| Testing | Vitest |

### Code Structure

- `core/` - 3D rendering logic
- `data/` - Types and parsers
- `utils/` - File handling, storage

## Supported Formats

| Format | Source | Status |
|--------|--------|--------|
| Claude Code JSONL | `~/.claude/projects/*/*.jsonl` | Supported |
| Additional agent formats | Various | Planned |

## Deployment

- Automated GitHub Actions workflows
- GitHub Pages deployment

## Implications for thinking-tracer-tools

### Integration Points

1. **Trace Format Compatibility**: Tools must produce output compatible with the JSONL format expected by thinking-tracer
2. **Metadata Extraction**: Can extract session metadata (model, git branch, duration) for reporting
3. **Export Formats**: Consider generating Markdown that aligns with thinking-tracer's export format

### Opportunities

1. **Pre-processing Tools**: Filter, transform, or aggregate traces before visualization
2. **Batch Analysis**: Extract metrics from multiple sessions
3. **Format Conversion**: Support additional agent formats upstream
