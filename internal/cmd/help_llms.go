package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/docs/llms"
)

var helpCmd = &cobra.Command{
	Use:   "help [command]",
	Short: "Help topics for thinkt",
	Long:  "Help about any command, or 'thinkt help llms' for AI assistant usage guide.",
	// When run without a recognized subcommand, look up the target in
	// rootCmd's command tree (so "thinkt help serve" still works) or
	// fall back to the default root help.
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		target, _, err := rootCmd.Find(args)
		if err != nil || target == rootCmd {
			return rootCmd.Help()
		}
		return target.Help()
	},
}

var helpLlmsCmd = &cobra.Command{
	Use:   "llms",
	Short: "Usage guide for LLMs and AI assistants",
	Long:  "Print a guide for LLMs on using thinkt via CLI, MCP, and REST API.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(llms.Text)
	},
}
