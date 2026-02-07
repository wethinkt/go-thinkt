package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	// Mirror flags from thinkt-indexer for help and completion
	indexerDBPath  string
	indexerLogPath string
	indexerQuiet   bool
	indexerVerbose bool
)

var indexerCmd = &cobra.Command{
	Use:   "indexer",
	Short: "Specialized indexing and search via DuckDB (requires thinkt-indexer)",
	Long: `The indexer command provides access to DuckDB-powered indexing and
search capabilities. This requires the 'thinkt-indexer' binary to be installed
separately due to its CGO dependencies.

Examples:
  thinkt indexer sync                        # Sync all local sessions to the index
  thinkt indexer search "query"              # Search across all sessions
  thinkt indexer sync --db /custom/path.db --quiet
  thinkt indexer search "query" --limit 10
  thinkt indexer watch                       # Watch and index in real-time
  thinkt indexer stats --json                # Show usage statistics

Use 'thinkt indexer <command> --help' for detailed help on each command.`,
}

// makeForwardingCommand creates a cobra command that forwards to thinkt-indexer
func makeForwardingCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:                use,
		Short:              short,
		DisableFlagParsing: true, // Forward all flags to thinkt-indexer
		RunE: func(cmd *cobra.Command, args []string) error {
			path := findIndexerBinary()
			if path == "" {
				return fmt.Errorf("the 'thinkt-indexer' binary was not found")
			}

			// Forward the subcommand name and all args
			cmdArgs := []string{cmd.Use}
			cmdArgs = append(cmdArgs, args...)

			c := exec.Command(path, cmdArgs...)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}

// findIndexerBinary attempts to locate the thinkt-indexer binary.
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
	// Register persistent flags on the parent command
	indexerCmd.PersistentFlags().StringVar(&indexerDBPath, "db", "", "path to DuckDB database file")
	indexerCmd.PersistentFlags().StringVar(&indexerLogPath, "log", "", "path to log file")
	indexerCmd.PersistentFlags().BoolVarP(&indexerQuiet, "quiet", "q", false, "suppress progress output")
	indexerCmd.PersistentFlags().BoolVarP(&indexerVerbose, "verbose", "v", false, "verbose output")

	// Create subcommands that forward to thinkt-indexer
	indexerCmd.AddCommand(makeForwardingCommand("sync", "Synchronize all local sessions into the index"))
	indexerCmd.AddCommand(makeForwardingCommand("search", "Search for text across indexed sessions"))
	indexerCmd.AddCommand(makeForwardingCommand("watch", "Watch session directories for changes and index in real-time"))
	indexerCmd.AddCommand(makeForwardingCommand("stats", "Show usage statistics from the index"))
	indexerCmd.AddCommand(makeForwardingCommand("sessions", "List sessions for a project from the index"))
	indexerCmd.AddCommand(makeForwardingCommand("version", "Print version information"))
	indexerCmd.AddCommand(makeForwardingCommand("help", "Help about any command"))
}
