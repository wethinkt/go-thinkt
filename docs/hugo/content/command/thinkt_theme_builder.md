---
title: "thinkt theme builder"
---

## thinkt theme builder

Launch interactive theme builder

### Synopsis

Launch an interactive TUI for building and editing themes.

The theme builder shows a live preview of conversation styles and
allows editing colors for all theme elements interactively.

If no name is provided, edits a copy of the current theme.
If the theme doesn't exist, creates a new one based on the default.

Examples:
  thinkt theme builder             # Edit current theme
  thinkt theme builder my-theme    # Edit or create my-theme
  thinkt theme builder dark        # Edit the dark theme

```
thinkt theme builder [name] [flags]
```

### Options

```
  -h, --help   help for builder
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt theme](thinkt_theme.md)	 - Browse and manage TUI themes

