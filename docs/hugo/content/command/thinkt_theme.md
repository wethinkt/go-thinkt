---
title: "thinkt theme"
---

## thinkt theme

Browse and manage TUI themes

### Synopsis

Browse and manage TUI themes.

Running without a subcommand launches the interactive theme browser.

The theme controls colors for conversation blocks, labels, borders,
and other UI elements. Themes are stored in ~/.thinkt/themes/.

Examples:
  thinkt theme               # Browse themes interactively
  thinkt theme show          # Show current theme with samples
  thinkt theme show --json   # Output theme as JSON
  thinkt theme list          # List all available themes
  thinkt theme set dracula   # Switch to a theme
  thinkt theme builder       # Interactive theme builder
  thinkt theme import f.itermcolors  # Import iTerm2 color scheme

```
thinkt theme [flags]
```

### Options

```
  -h, --help   help for theme
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction
* [thinkt theme browse](thinkt_theme_browse.md)	 - Browse and preview themes interactively
* [thinkt theme builder](thinkt_theme_builder.md)	 - Launch interactive theme builder
* [thinkt theme import](thinkt_theme_import.md)	 - Import an iTerm2 color scheme as a theme
* [thinkt theme list](thinkt_theme_list.md)	 - List all available themes
* [thinkt theme set](thinkt_theme_set.md)	 - Set the active theme
* [thinkt theme show](thinkt_theme_show.md)	 - Display a theme with styled samples

