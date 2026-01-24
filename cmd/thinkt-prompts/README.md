# thinkt-prompts

Extract user prompts from LLM agent trace files and generate formatted output.

## Installation

```bash
go install github.com/Brain-STM-org/thinking-tracer-tools/cmd/thinkt-prompts@latest
```

Or build from source:

```bash
task build
```

## Usage

```bash
# Extract prompts from the latest Claude Code session
thinkt-prompts extract -t claude

# Extract from a specific trace file
thinkt-prompts extract -t claude -i ~/.claude/projects/abc123/session.jsonl

# Output to stdout
thinkt-prompts extract -t claude -o -

# Output as JSON
thinkt-prompts extract -t claude -f json -o prompts.json

# Use a custom template
thinkt-prompts extract -t claude --template my-template.tmpl

# List available trace files
thinkt-prompts list -t claude

# Show session information
thinkt-prompts info -t claude

# Show template help
thinkt-prompts templates
```

## Commands

| Command | Description |
|---------|-------------|
| `extract` | Extract prompts from a trace file |
| `list` | List available trace files |
| `info` | Show session information |
| `templates` | List available templates and show template variables |

## Flags

### Global Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--type` | `-t` | `claude` | Trace type (claude) |
| `--verbose` | `-v` | `false` | Verbose output |

### Extract Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--input` | `-i` | (latest) | Input trace file (use `-` for stdin) |
| `--output` | `-o` | `PROMPTS.md` | Output file (use `-` for stdout) |
| `--format` | `-f` | `markdown` | Output format (markdown, json, plain) |
| `--append` | `-a` | `false` | Append to existing file |
| `--template` | | | Custom template file (for markdown format) |

## Supported Trace Types

| Type | Description | Location |
|------|-------------|----------|
| `claude` | Claude Code JSONL traces | `~/.claude/projects/` |

## Template Reference

Templates use Go's `text/template` syntax. The default template produces markdown output matching the format used by the prompt-history hook.

### Template Variables

#### Top-level

| Variable | Type | Description |
|----------|------|-------------|
| `.Prompts` | `[]Prompt` | List of all extracted prompts |
| `.Count` | `int` | Total number of prompts |
| `.SessionID` | `string` | Session identifier (if available) |
| `.Model` | `string` | Model used (if available) |
| `.Branch` | `string` | Git branch (if available) |
| `.Version` | `string` | Agent version (if available) |
| `.CWD` | `string` | Working directory (if available) |
| `.StartTime` | `string` | Session start time |
| `.EndTime` | `string` | Session end time |
| `.Duration` | `string` | Session duration |

#### Each Prompt in `.Prompts`

| Variable | Type | Description |
|----------|------|-------------|
| `.Text` | `string` | The prompt text content |
| `.Timestamp` | `string` | ISO 8601 timestamp (e.g., `2026-01-24T20:41:03Z`) |
| `.UUID` | `string` | Unique identifier for this prompt |

### Template Functions

| Function | Example | Description |
|----------|---------|-------------|
| `formatTime` | `{{formatTime .Timestamp "2006-01-02"}}` | Format timestamp using Go time layout |
| `truncate` | `{{truncate .Text 100}}` | Truncate string to max length |
| `lineCount` | `{{lineCount .Text}}` | Count lines in text |
| `wordCount` | `{{wordCount .Text}}` | Count words in text |

### Example Templates

#### Default (Markdown)

```gotemplate
{{- range .Prompts }}

---

## {{ .Timestamp }}

{{ .Text }}
{{- end }}
```

#### With Formatted Dates

```gotemplate
# Prompt History

Generated: {{formatTime .StartTime "January 2, 2006"}}
Total prompts: {{.Count}}

{{range .Prompts}}
## {{formatTime .Timestamp "Jan 02, 3:04 PM"}}

{{.Text}}

---
{{end}}
```

#### Summary Format

```gotemplate
# Session Summary

- **Prompts:** {{.Count}}
- **Model:** {{.Model}}
- **Branch:** {{.Branch}}

## Prompts

{{range $i, $p := .Prompts}}
{{add $i 1}}. {{truncate $p.Text 80}}
{{end}}
```

#### Plain List

```gotemplate
{{range .Prompts}}{{.Text}}

{{end}}
```

## Output Formats

### Markdown (default)

```markdown
---

## 2026-01-24T20:41:03Z

What is the capital of France?

---

## 2026-01-24T20:42:15Z

Explain quantum computing.
```

### JSON

```json
[
  {
    "Text": "What is the capital of France?",
    "Timestamp": "2026-01-24T20:41:03Z",
    "UUID": "abc123"
  }
]
```

### Plain

```
What is the capital of France?

Explain quantum computing.
```
