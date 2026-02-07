---
title: "thinkt docs"
---

## thinkt docs

Generate documentation for thinkt

### Synopsis

Generate documentation for all thinkt commands.

Subcommands:
  markdown  Generate plain markdown (default)
  man       Generate man pages

The auto-generation tag (timestamp footer) is disabled by default for stable,
reproducible files. Use --enableAutoGenTag for publishing.

Examples:
  thinkt docs                       # Generate markdown docs in ./docs/
  thinkt docs markdown -o ./wiki    # Generate markdown in custom directory
  thinkt docs markdown --hugo -o docs/command  # Generate Hugo-compatible docs
  thinkt docs man -o /usr/share/man/man1

```
thinkt docs [flags]
```

### Options

```
      --enableAutoGenTag   include auto-generation tag (timestamp footer) for publishing
  -h, --help               help for docs
  -o, --output string      output directory for generated docs (default "./docs")
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction
* [thinkt docs man](thinkt_docs_man.md)	 - Generate man pages
* [thinkt docs markdown](thinkt_docs_markdown.md)	 - Generate markdown documentation

