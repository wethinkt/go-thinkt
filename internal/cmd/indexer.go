package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var indexerCmd = &cobra.Command{
	Use:   "indexer",
	Short: "Specialized indexing and search via DuckDB (requires thinkt-indexer)",
	Long: `The indexer command provides access to DuckDB-powered indexing and 
search capabilities. This requires the 'thinkt-indexer' binary to be installed
separately due to its CGO dependencies.

Examples:
  thinkt indexer sync      # Sync all local sessions to the index
  thinkt indexer search    # Search across all sessions`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Detect binary
		path := findIndexerBinary()
		if path == "" {
			return fmt.Errorf("the 'thinkt-indexer' binary was not found. Please install it to use this command.\n" +
				"It is released as a separate binary due to CGO dependencies (DuckDB).")
		}

		// Forward execution
		c := exec.Command(path, args...)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

// findIndexerBinary attempts to locate the thinkt-indexer binary.
// It checks the current executable's directory first, then falls back to PATH.
func findIndexerBinary() string {
	// 1. Check same directory as current executable
	if execPath, err := os.Executable(); err == nil {
		binDir := filepath.Dir(execPath)
		indexerPath := filepath.Join(binDir, "thinkt-indexer")
		if _, err := os.Stat(indexerPath); err == nil {
			return indexerPath
		}
	}

	// 2. Check system PATH
	if path, err := exec.LookPath("thinkt-indexer"); err == nil {
		return path
	}

	return ""
}

func init() {
	// Root command is defined in root.go
}
