package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/docs/llms"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
)

var helpCmd = &cobra.Command{
	Use:   "help [command]",
	Short: "Help topics for thinkt",
	Long:  "Help about any command, or 'thinkt help llms' for AI assistant usage guide.",
	// When run without a recognized subcommand, look up the target in
	// rootCmd's command tree (so "thinkt help server" still works) or
	// fall back to the default root help.
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			printHelpOverview()
			return nil
		}
		target, _, err := rootCmd.Find(args)
		if err != nil || target == rootCmd {
			printHelpOverview()
			return nil
		}
		return target.Help()
	},
}

// helpEntry is a command name + i18n key for its description.
type helpEntry struct {
	name string
	key  string
	def  string
}

var helpTopics = []helpEntry{
	{"llms", "cmd.help.topic.llms", "Usage guide for LLMs and AI assistants"},
}

var helpCommands = []helpEntry{
	{"setup", "cmd.help.cmd.setup", "Scan for AI session sources and configure thinkt"},
	{"tui", "cmd.help.cmd.tui", "Launch interactive TUI explorer"},
	{"server", "cmd.help.cmd.server", "Manage the local HTTP server"},
	{"web", "cmd.help.cmd.web", "Open the web interface in your browser"},
	{"sources", "cmd.help.cmd.sources", "Manage and view available session sources"},
	{"projects", "cmd.help.cmd.projects", "List and manage projects"},
	{"sessions", "cmd.help.cmd.sessions", "View and manage sessions"},
	{"search", "cmd.help.cmd.search", "Search for text across indexed sessions"},
	{"semantic", "cmd.help.cmd.semantic", "Search sessions by meaning using on-device embeddings"},
	{"prompts", "cmd.help.cmd.prompts", "Extract and manage prompts from trace files"},
	{"agents", "cmd.help.cmd.agents", "List active agents (local and remote)"},
	{"teams", "cmd.help.cmd.teams", "List and inspect agent teams"},
	{"apps", "cmd.help.cmd.apps", "Manage open-in apps and default terminal"},
	{"embeddings", "cmd.help.cmd.embeddings", "Manage embedding model, storage, and sync"},
	{"indexer", "cmd.help.cmd.indexer", "Specialized indexing and search via DuckDB"},
	{"collect", "cmd.help.cmd.collect", "Start trace collector server"},
	{"export", "cmd.help.cmd.export", "Export traces to a remote collector"},
	{"language", "cmd.help.cmd.language", "Get or set the display language"},
	{"theme", "cmd.help.cmd.theme", "Browse and manage TUI themes"},
	{"version", "cmd.help.cmd.version", "Print the version information"},
	{"completion", "cmd.help.cmd.completion", "Generate the autocompletion script for the specified shell"},
}

func printHelpOverview() {
	fmt.Println(thinktI18n.T("cmd.help.heading", "Help for thinkt — AI coding session explorer."))
	fmt.Println()
	fmt.Println(thinktI18n.T("cmd.help.usage", "Usage:"))
	fmt.Println("  thinkt help <command>")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 2, 0, 2, ' ', 0)

	fmt.Println(thinktI18n.T("cmd.help.topics", "Help Topics:"))
	for _, t := range helpTopics {
		fmt.Fprintf(w, "  %s\t%s\n", t.name, thinktI18n.T(t.key, t.def))
	}
	w.Flush()
	fmt.Println()

	fmt.Println(thinktI18n.T("cmd.help.commands", "Available Commands:"))
	for _, c := range helpCommands {
		fmt.Fprintf(w, "  %s\t%s\n", c.name, thinktI18n.T(c.key, c.def))
	}
	w.Flush()
	fmt.Println()

	fmt.Println(thinktI18n.T("cmd.help.examples", "Examples:"))
	fmt.Println("  thinkt help llms           #", thinktI18n.T("cmd.help.example.llms", "AI assistant usage guide"))
	fmt.Println("  thinkt help server         #", thinktI18n.T("cmd.help.example.server", "Help for the server command"))
	fmt.Println("  thinkt help setup          #", thinktI18n.T("cmd.help.example.setup", "Help for source setup"))
}

var helpLlmsCmd = &cobra.Command{
	Use:   "llms",
	Short: "Usage guide for LLMs and AI assistants",
	Long:  "Print a guide for LLMs on using thinkt via CLI, MCP, and REST API.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(llms.Text)
	},
}
