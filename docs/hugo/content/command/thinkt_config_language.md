---
title: "thinkt config language"
---

## thinkt config language

Get or set the display language

### Synopsis

Get or set the display language.

Running without a subcommand shows the current language.

Examples:
  thinkt config language              # show current language
  thinkt config language get --json   # JSON output
  thinkt config language list         # list available languages
  thinkt config language set zh-Hans  # set directly
  thinkt config language set          # interactive picker

```
thinkt config language [flags]
```

### Options

```
  -h, --help   help for language
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt config](thinkt_config/)	 - Manage thinkt configuration
* [thinkt config language get](thinkt_config_language_get/)	 - Show current display language
* [thinkt config language list](thinkt_config_language_list/)	 - List available languages
* [thinkt config language set](thinkt_config_language_set/)	 - Set the display language

