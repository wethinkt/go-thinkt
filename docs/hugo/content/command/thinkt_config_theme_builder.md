---
title: "thinkt config theme builder"
---

## thinkt config theme builder

Launch interactive theme builder

### Synopsis

Launch an interactive TUI for building and editing themes.

The theme builder shows a live preview of conversation styles and
allows editing colors for all theme elements interactively.

If no name is provided, edits a copy of the current theme.
If the theme doesn't exist, creates a new one based on the default.

Examples:
  thinkt config theme builder             # Edit current theme
  thinkt config theme builder my-theme    # Edit or create my-theme
  thinkt config theme builder dark        # Edit the dark theme

```
thinkt config theme builder [name] [flags]
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

* [thinkt config theme](thinkt_config_theme/)	 - Browse and manage TUI themes

