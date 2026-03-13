---
title: "thinkt export"
---

## thinkt export

Export a session as Markdown, HTML, or JSON

### Synopsis

Export a session as Markdown (default), self-contained HTML, or JSON.

Without arguments, exports the most recent session for the current project.
With a session argument, exports that specific session (ID, path, or suffix).

The --view flag previews the export: pipes Markdown through glow, or opens
HTML in the default browser.

Examples:
  thinkt export                          # Latest session as Markdown to stdout
  thinkt export --html -o session.html   # Export as HTML to file
  thinkt export --json                   # Export as JSON
  thinkt export --view                   # Preview Markdown in terminal via glow
  thinkt export --html --view            # Export HTML and open in browser
  thinkt export abc123                   # Export specific session

```
thinkt export [session] [flags]
```

### Options

```
      --format string   output format: md, html, json (default "md")
  -h, --help            help for export
      --html            shorthand for --format html
      --json            shorthand for --format json
      --md              shorthand for --format md (default)
      --no-media        exclude images and documents
      --no-thinking     exclude thinking blocks
      --no-tools        exclude tool use and results
  -o, --output string   output file (default: stdout)
      --system          include system entries
      --view            preview export (glow for md, browser for html)
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt/)	 - Tools for AI assistant session exploration and extraction
* [thinkt export template](thinkt_export_template/)	 - Export user prompts using a Go template

