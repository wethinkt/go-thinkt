---
title: "thinkt config theme import"
---

## thinkt config theme import

Import an iTerm2 color scheme as a theme

### Synopsis

Import an iTerm2 .itermcolors file and convert it to a thinkt config theme.

The imported theme is saved to ~/.thinkt/themes/ and can be activated
with 'thinkt config theme set'.

Examples:
  thinkt config theme import ~/Downloads/Dracula.itermcolors
  thinkt config theme import scheme.itermcolors --name my-theme

```
thinkt config theme import <file.itermcolors> [flags]
```

### Options

```
  -h, --help          help for import
      --name string   theme name (default: derived from filename)
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt config theme](thinkt_config_theme/)	 - Browse and manage TUI themes

