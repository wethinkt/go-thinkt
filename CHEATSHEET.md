# 🧠 `thinkt` Cheat Sheet 

You can see this [cheat sheet at the command line](https://wethinkt.github.io/go-thinkt/command/thinkt_help_cheat/) with `thinkt help cheat`.

```sh
$ thinkt help cheat
🧠 thinkt — command cheat sheet

  ├── agents      List active agents (local and remote)
  │   └── follow  Live-tail an agent's conversation
  │
  ├── apps              Manage open-in apps and default terminal
  │   ├── disable       Disable an app
  │   ├── enable        Enable an app
  │   ├── get-terminal  Show the configured default terminal app
  │   ├── list          List all apps with enabled/disabled status
  │   └── set-terminal  Set the default terminal app
  │
  ├── collect             Start trace collector server
  │   └── export-parquet  Export collected traces to Parquet files
  │
  ├── completion      Generate the autocompletion script for the specified shell
  │   ├── bash        Generate the autocompletion script for bash
  │   ├── fish        Generate the autocompletion script for fish
  │   ├── powershell  Generate the autocompletion script for powershell
  │   └── zsh         Generate the autocompletion script for zsh
  │
  ├── embeddings  Manage embedding model, storage, and sync
  │
  ├── export  Export traces to a remote collector
  │
  ├── indexer         Specialized indexing and search via DuckDB (requires thinkt-indexer)
  │   ├── embeddings  Manage embedding model, storage, and sync
  │   ├── help        Help about any command
  │   ├── logs        View indexer logs
  │   ├── metrics     Show Prometheus metrics from the running indexer
  │   ├── search      Search for text across indexed sessions
  │   ├── semantic    Semantic search and index management
  │   ├── sessions    List sessions for a project from the index
  │   ├── start       Start indexer in background
  │   ├── stats       Show usage statistics from the index
  │   ├── status      Show indexer status
  │   ├── stop        Stop background indexer
  │   ├── summarize   Manage summarization model, storage, sync, and tags
  │   ├── sync        Synchronize all local sessions into the index
  │   └── version     Print version information
  │
  ├── language  Get or set the display language
  │   ├── get   Show current display language
  │   ├── list  List available languages
  │   └── set   Set the display language
  │
  ├── login  Log in to wethinkt.com
  │
  ├── projects     Manage and view projects
  │   ├── copy     Copy project sessions to a target directory
  │   ├── list     List projects from all sources
  │   ├── summary  Show detailed project summary
  │   ├── tree     Show projects in a tree view
  │   └── view     Interactive project browser
  │
  ├── prompts        Extract and manage prompts from trace files
  │   ├── extract    Extract prompts from a trace file
  │   ├── info       Show session information
  │   ├── list       List available trace files
  │   └── templates  List available templates and show template variables
  │
  ├── push  Push a Thinkt to wethinkt.com
  │
  ├── search  Search for text across indexed sessions
  │
  ├── semantic  Search sessions by meaning using on-device embeddings
  │
  ├── server           Manage the local HTTP server for trace exploration
  │   ├── fingerprint  Display the machine fingerprint
  │   ├── http-logs    View HTTP access logs
  │   ├── logs         View server logs
  │   ├── mcp          Start MCP server for AI tool integration
  │   ├── metrics      Fetch Prometheus metrics from the running server
  │   ├── run          Start server in foreground
  │   ├── start        Start server in background
  │   ├── status       Show server status
  │   ├── stop         Stop background server
  │   └── token        Manage authentication tokens
  │
  ├── sessions     View and manage sessions across all sources
  │   ├── copy     Copy a session to a target location
  │   ├── delete   Delete a session
  │   ├── list     List sessions (auto-detects project from cwd)
  │   ├── resolve  Resolve a session query to its canonical path
  │   ├── resume   Resume a session in its original CLI tool
  │   ├── summary  Show detailed session summary
  │   └── view     View a session in the terminal (interactive picker)
  │
  ├── setup  Scan for AI session sources and configure thinkt
  │
  ├── sources      Manage and view available session sources
  │   ├── disable  Disable a source
  │   ├── enable   Enable a source
  │   ├── list     List available session sources
  │   └── status   Show detailed source status
  │
  ├── teams     List and inspect agent teams
  │   └── list  List all discovered teams
  │
  ├── theme        Browse and manage TUI themes
  │   ├── browse   Browse and preview themes interactively
  │   ├── builder  Launch interactive theme builder
  │   ├── import   Import an iTerm2 color scheme as a theme
  │   ├── list     List all available themes
  │   ├── set      Set the active theme
  │   └── show     Display a theme with styled samples
  │
  ├── tui  Launch interactive TUI explorer
  │
  ├── version  Print the version information
  │
  └── web       Open the web interface in your browser
      └── lite  Open the lightweight web interface

Run 'thinkt help <command>' for detailed usage.
```
