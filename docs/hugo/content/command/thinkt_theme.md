---
title: "thinkt theme"
---

## thinkt theme

Display and manage TUI theme settings

### Synopsis

Display the current TUI theme with styled samples.

The theme controls colors for conversation blocks, labels, borders,
and other UI elements. Themes are stored in ~/.thinkt/themes/.

Built-in themes: dark, light
User themes can be added to ~/.thinkt/themes/

Examples:
  thinkt theme               # Show current theme with samples
  thinkt theme --json        # Output theme as JSON
  thinkt theme list          # List all available themes
  thinkt theme set light     # Switch to light theme
  thinkt theme builder       # Interactive theme builder

```
thinkt theme [flags]
```

### Options

```
  -h, --help   help for theme
      --json   output theme as JSON
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction
* [thinkt theme builder](thinkt_theme_builder.md)	 - Launch interactive theme builder
* [thinkt theme list](thinkt_theme_list.md)	 - List all available themes
* [thinkt theme set](thinkt_theme_set.md)	 - Set the active theme

