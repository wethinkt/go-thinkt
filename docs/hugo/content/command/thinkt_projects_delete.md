---
title: "thinkt projects delete"
---

## thinkt projects delete

Delete a project and all its sessions

### Synopsis

Delete a project directory and all session data within it.

The project-path can be:
  - Full project path (e.g., /Users/evan/myproject)
  - Path relative to current directory

Before deletion, shows the number of sessions and last modified time,
then prompts for confirmation. Use --force to skip the confirmation.

Examples:
  thinkt projects delete /Users/evan/myproject
  thinkt projects delete ./myproject
  thinkt projects delete --force /Users/evan/myproject

```
thinkt projects delete <project-path> [flags]
```

### Options

```
  -f, --force   skip confirmation prompt
  -h, --help    help for delete
```

### Options inherited from parent commands

```
  -s, --source stringArray   source to include (kimi|claude|gemini, can be specified multiple times, default: all)
  -v, --verbose              verbose output
```

### SEE ALSO

* [thinkt projects](thinkt_projects.md)	 - Manage and view projects

