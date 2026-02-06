package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var indexerCmd = &cobra.Command{
	Use:   "indexer",
	Short: "Reasoning indexing and search (requires thinkt-indexer)",
	Long: `The indexer command provides access to DuckDB-powered indexing and 
search capabilities. This requires the 'thinkt-indexer' binary to be installed.

Examples:
  thinkt indexer sync      # Sync all local sessions to the index
  thinkt indexer search    # Search across all sessions`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if thinkt-indexer is in PATH
		path, err := exec.LookPath("thinkt-indexer")
		if err != nil {
			return fmt.Errorf("thinkt-indexer binary was not found. Please install it to use this command\n")
		}

		// Forward execution
		c := exec.Command(path, args...)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

func init() {
	// Root command is defined in root.go
}
