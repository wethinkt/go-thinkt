---
title: "thinkt export template"
---

## thinkt export template

Export user prompts using a Go template

### Synopsis

Extract user prompts from a session and render them using a Go template.

Outputs Markdown by default using the built-in template, or use --template
to provide a custom Go template file. Use --json for structured JSON output,
or --format plain for raw text.

Examples:
  thinkt export template                        # Prompts as Markdown
  thinkt export template -f json                # Prompts as JSON
  thinkt export template -f plain               # Raw prompt text
  thinkt export template --template my.tmpl     # Custom template

```
thinkt export template [session] [flags]
```

### Options

```
  -f, --format string     output format (markdown|json|plain) (default "markdown")
  -h, --help              help for template
  -o, --output string     output file (default: stdout)
  -p, --project string    project path or ID
  -s, --session string    session ID or path
      --source strings    filter by source
      --template string   custom Go template file
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt export](thinkt_export/)	 - Export a session as Markdown, HTML, or JSON

