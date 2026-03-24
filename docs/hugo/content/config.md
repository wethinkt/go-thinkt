---
title: "Configuration"
weight: 6
---

# Configuration

All thinkt configuration lives under `thinkt config`. This includes session sources, theming, language, and app management.

```bash
thinkt config                # Show available config subcommands
```

## Sources

Sources are the AI coding assistants whose session data thinkt reads from your machine (Claude Code, Kimi Code, Gemini CLI, Copilot CLI, Codex CLI, Qwen Code).

```bash
thinkt config sources                  # List sources with status
thinkt config sources status           # Detailed status with paths and sizes
thinkt config sources enable claude    # Enable a source
thinkt config sources disable kimi     # Disable a source
thinkt config sources enable --all     # Enable all sources
thinkt config sources disable --all    # Disable all sources
```

Disabled sources are excluded from all commands (sessions, projects, search, export, etc.).

**Reference:** [thinkt config sources](/command/thinkt_config_sources)

---

## Theme

Customize the TUI appearance with 14 built-in themes or create your own. See the [Themes](/themes) page for a full gallery and details.

```bash
thinkt config theme                    # Browse themes interactively
thinkt config theme list               # List all available themes
thinkt config theme set dracula        # Switch to a theme
thinkt config theme show               # Show current theme with samples
thinkt config theme show --json        # Export theme as JSON
thinkt config theme builder            # Interactive theme builder
thinkt config theme import f.itermcolors  # Import iTerm2 color scheme
```

**Reference:** [thinkt config theme](/command/thinkt_config_theme)

---

## Language

thinkt supports English, Chinese (Simplified), and Spanish. See the [Languages](/languages) page for details on contributing translations.

```bash
thinkt config language                 # Show current language
thinkt config language list            # List available languages
thinkt config language set zh-Hans     # Set directly
thinkt config language set             # Interactive picker
```

You can also set the language temporarily via the `THINKT_LANG` environment variable:

```bash
THINKT_LANG=es thinkt
```

**Reference:** [thinkt config language](/command/thinkt_config_language)

---

## Apps

Manage the apps available for "open in" actions and the default terminal emulator:

```bash
thinkt config apps                     # List all apps with status
thinkt config apps enable <id>         # Enable an app
thinkt config apps disable <id>        # Disable an app
thinkt config apps get-terminal        # Show the default terminal app
thinkt config apps set-terminal <id>   # Set the default terminal
thinkt config apps set-terminal        # Interactive picker
```

Apps with terminal capability (those that can run shell commands) are shown in the `TERMINAL` column. The default terminal is used by the REST API to spawn resume commands.

**Reference:** [thinkt config apps](/command/thinkt_config_apps)

---

## Config File

Configuration is stored in `~/.thinkt/config.json`. While you can edit it directly, using `thinkt config` subcommands is recommended as they validate inputs and handle defaults.

```bash
# View the config file location
thinkt help env
```
