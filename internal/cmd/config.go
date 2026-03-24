package cmd

import (
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage thinkt configuration",
	Long: `View and manage thinkt configuration settings.

Subcommands:
  apps       Manage open-in apps and default terminal
  language   Get or set the display language
  sources    Manage and view available session sources
  theme      Browse and manage TUI themes

Examples:
  thinkt config apps list
  thinkt config language set zh-Hans
  thinkt config sources status
  thinkt config theme set dracula`,
}
