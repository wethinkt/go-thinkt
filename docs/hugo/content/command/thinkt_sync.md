---
title: "thinkt sync"
---

## thinkt sync

Synchronize all local sessions into the SQLite index

### Synopsis

Index all local AI assistant sessions into the SQLite search database.

This scans all registered sources and indexes session metadata (no private
content is stored). The index enables fast search and stats queries.

Run this once after install, then the background watcher keeps it up to date.

```
thinkt sync [flags]
```

### Options

```
  -h, --help    help for sync
  -q, --quiet   suppress progress output
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt/)	 - Tools for AI assistant session exploration and extraction

