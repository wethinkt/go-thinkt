---
title: "thinkt theme import"
---

## thinkt theme import

Import an iTerm2 color scheme as a theme

### Synopsis

Import an iTerm2 .itermcolors file and convert it to a thinkt theme.

The imported theme is saved to ~/.thinkt/themes/ and can be activated
with 'thinkt theme set'.

thinkt can import any `.itermcolors` plist XML file from the
[iTerm2-Color-Schemes](https://github.com/mbadolato/iTerm2-Color-Schemes)
repository or any other source. The importer maps iTerm2 ANSI colors to
thinkt's semantic theme fields (labels, blocks, borders, etc.).

Examples:
  thinkt theme import ~/Downloads/Dracula.itermcolors
  thinkt theme import scheme.itermcolors --name my-theme

```
thinkt theme import <file.itermcolors> [flags]
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

* [thinkt theme](thinkt_theme.md)	 - Display and manage TUI theme settings
