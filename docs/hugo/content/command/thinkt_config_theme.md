---
title: "thinkt config theme"
---

## thinkt config theme

Browse and manage TUI themes

### Synopsis

Browse and manage TUI themes.

Running without a subcommand launches the interactive theme browser.

The theme controls colors for conversation blocks, labels, borders,
and other UI elements. Themes are stored in ~/.thinkt/themes/.

Examples:
  thinkt config theme               # Browse themes interactively
  thinkt config theme show          # Show current theme with samples
  thinkt config theme show --json   # Output theme as JSON
  thinkt config theme list          # List all available themes
  thinkt config theme set dracula   # Switch to a theme
  thinkt config theme builder       # Interactive theme builder
  thinkt config theme import f.itermcolors  # Import iTerm2 color scheme

```
thinkt config theme [flags]
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

* [thinkt config](thinkt_config/)	 - Manage thinkt configuration
* [thinkt config theme browse](thinkt_config_theme_browse/)	 - Browse and preview themes interactively
* [thinkt config theme builder](thinkt_config_theme_builder/)	 - Launch interactive theme builder
* [thinkt config theme import](thinkt_config_theme_import/)	 - Import an iTerm2 color scheme as a theme
* [thinkt config theme list](thinkt_config_theme_list/)	 - List all available themes
* [thinkt config theme set](thinkt_config_theme_set/)	 - Set the active theme
* [thinkt config theme show](thinkt_config_theme_show/)	 - Display a theme with styled samples

