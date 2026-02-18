---
title: "thinkt apps"
---

## thinkt apps

Manage open-in apps and default terminal

### Synopsis

Manage the apps available for "open in" actions and the default terminal.

Apps are configured in ~/.thinkt/config.json.

Examples:
  thinkt apps                        # List all apps
  thinkt apps list                   # List all apps
  thinkt apps enable vscode          # Enable an app
  thinkt apps disable finder         # Disable an app
  thinkt apps get-terminal           # Show default terminal
  thinkt apps set-terminal ghostty   # Set default terminal
  thinkt apps set-terminal           # Interactive terminal picker

```
thinkt apps [flags]
```

### Options

```
  -h, --help   help for apps
      --json   output as JSON
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction
* [thinkt apps disable](thinkt_apps_disable.md)	 - Disable an app
* [thinkt apps enable](thinkt_apps_enable.md)	 - Enable an app
* [thinkt apps get-terminal](thinkt_apps_get-terminal.md)	 - Show the configured default terminal app
* [thinkt apps list](thinkt_apps_list.md)	 - List all apps with enabled/disabled status
* [thinkt apps set-terminal](thinkt_apps_set-terminal.md)	 - Set the default terminal app

