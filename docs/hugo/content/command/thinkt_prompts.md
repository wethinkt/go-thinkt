---
title: "thinkt prompts"
---

## thinkt prompts

Extract and manage prompts from trace files

### Synopsis

Extract user prompts from LLM agent trace files
and generate output in various formats.

Supported trace types:
  claude    Claude Code JSONL traces (~/.claude/projects/)

Examples:
  thinkt prompts extract -i session.jsonl
  thinkt prompts extract            # uses latest session
  thinkt prompts list
  thinkt prompts info
  thinkt prompts templates

### Options

```
  -h, --help          help for prompts
  -t, --type string   trace type (claude) (default "claude")
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction
* [thinkt prompts extract](thinkt_prompts_extract.md)	 - Extract prompts from a trace file
* [thinkt prompts info](thinkt_prompts_info.md)	 - Show session information
* [thinkt prompts list](thinkt_prompts_list.md)	 - List available trace files
* [thinkt prompts templates](thinkt_prompts_templates.md)	 - List available templates and show template variables

