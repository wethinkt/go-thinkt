---
title: "thinkt teams"
---

## thinkt teams

List and inspect agent teams

### Synopsis

List and inspect multi-agent teams from supported sources.

Teams are groups of AI agents (team lead + teammates) that coordinate
via shared task boards and messaging to work on a project together.

Currently supported: Claude Code teams (~/.claude/teams/)

Examples:
  thinkt teams              # List all teams
  thinkt teams list         # Same as above
  thinkt teams --json       # Output as JSON

```
thinkt teams [flags]
```

### Options

```
      --active     show only active teams
  -h, --help       help for teams
      --inactive   show only inactive (historical) teams
      --json       output as JSON
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt](thinkt.md)	 - Tools for AI assistant session exploration and extraction
* [thinkt teams list](thinkt_teams_list.md)	 - List all discovered teams

