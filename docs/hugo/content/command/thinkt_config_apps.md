---
title: "thinkt config apps"
---

## thinkt config apps

Manage open-in apps and default terminal

### Synopsis

Manage the apps available for "open in" actions and the default terminal.

Apps are configured in ~/.thinkt/config.json.

Examples:
  thinkt config apps                        # List all apps
  thinkt config apps list                   # List all apps
  thinkt config apps enable vscode          # Enable an app
  thinkt config apps disable finder         # Disable an app
  thinkt config apps get-terminal           # Show default terminal
  thinkt config apps set-terminal ghostty   # Set default terminal
  thinkt config apps set-terminal           # Interactive terminal picker

```
thinkt config apps [flags]
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

* [thinkt config](thinkt_config/)	 - Manage thinkt configuration
* [thinkt config apps disable](thinkt_config_apps_disable/)	 - Disable an app
* [thinkt config apps enable](thinkt_config_apps_enable/)	 - Enable an app
* [thinkt config apps get-terminal](thinkt_config_apps_get-terminal/)	 - Show the configured default terminal app
* [thinkt config apps list](thinkt_config_apps_list/)	 - List all apps with enabled/disabled status
* [thinkt config apps set-terminal](thinkt_config_apps_set-terminal/)	 - Set the default terminal app

