---
title: "thinkt projects copy"
---

## thinkt projects copy

Copy project sessions to a target directory

### Synopsis

Copy all session files from a project to a target directory.

The project-path can be:
  - Full project path (e.g., /Users/evan/myproject)
  - Path relative to current directory

The target directory will be created if it doesn't exist.
Session files and index files are copied.

Examples:
  thinkt projects copy /Users/evan/myproject ./backup
  thinkt projects copy /Users/evan/myproject /tmp/export

```
thinkt projects copy <project-path> <target-dir> [flags]
```

### Options

```
  -h, --help   help for copy
```

### Options inherited from parent commands

```
  -s, --source stringArray   source to include (kimi|claude|gemini, can be specified multiple times, default: all)
  -v, --verbose              verbose output
```

### SEE ALSO

* [thinkt projects](thinkt_projects.md)	 - Manage and view projects

