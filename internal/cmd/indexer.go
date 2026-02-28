package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/indexer/db"
)

// indexerStatusJSON is the JSON schema for thinkt indexer status --json.
type indexerStatusJSON struct {
	Running       bool       `json:"running"`
	PID           int        `json:"pid,omitempty"`
	LogPath       string     `json:"log_path,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	UptimeSeconds int        `json:"uptime_seconds,omitempty"`
	Database      string     `json:"database,omitempty"`
}

var (
	// Mirror flags from thinkt-indexer for help and completion
	indexerDBPath    string
	indexerEmbDBPath string
	indexerLogPath   string
	indexerQuiet     bool
	indexerVerbose   bool
)

var indexerCmd = &cobra.Command{
	Use:   "indexer",
	Short: "Specialized indexing and search via DuckDB (requires thinkt-indexer)",
	Long: `The indexer command provides access to DuckDB-powered indexing and
search capabilities. This requires the 'thinkt-indexer' binary to be installed
separately due to its CGO dependencies.

Examples:
  thinkt indexer start                       # Start indexer in background
  thinkt indexer status                      # Check indexer status
  thinkt indexer stop                        # Stop background indexer
  thinkt indexer sync                        # Sync all local sessions to the index
  thinkt indexer search "query"              # Search across all sessions
  thinkt indexer server                      # Run indexer server (foreground)`,
}

var indexerLogsCmd = &cobra.Command{
	Use:          "logs",
	Short:        "View indexer logs",
	SilenceUsage: true,
	RunE:         runIndexerLogs,
}

func runIndexerLogs(cmd *cobra.Command, args []string) error {
	n, _ := cmd.Flags().GetInt("lines")
	follow, _ := cmd.Flags().GetBool("follow")

	// Try to get log path from running instance
	logFile := ""
	if inst := config.FindInstanceByType(config.InstanceIndexerServer); inst != nil {
		logFile = inst.LogPath
	}

	// Fall back to default
	if logFile == "" {
		confDir, err := config.Dir()
		if err != nil {
			return err
		}
		logFile = filepath.Join(confDir, "logs", "indexer.log")
	}

	return tailLogFile(logFile, n, follow)
}

var indexerStartCmd = &cobra.Command{
	Use:          "start",
	Short:        "Start indexer in background",
	SilenceUsage: true,
	RunE:         runIndexerStart,
}

var indexerStopCmd = &cobra.Command{
	Use:          "stop",
	Short:        "Stop background indexer",
	SilenceUsage: true,
	RunE:         runIndexerStop,
}

var indexerStatusCmd = &cobra.Command{
	Use:          "status",
	Short:        "Show indexer status",
	SilenceUsage: true,
	RunE:         runIndexerStatus,
}

func runIndexerStart(cmd *cobra.Command, args []string) error {
	path := config.FindIndexerBinary()
	if path == "" {
		return fmt.Errorf("the 'thinkt-indexer' binary was not found")
	}

	// Check if already running
	if inst := config.FindInstanceByType(config.InstanceIndexerServer); inst != nil {
		fmt.Printf("Indexer is already running (PID: %d)\n", inst.PID)
		return nil
	}

	fmt.Println("🚀 Starting indexer in background...")

	// Build arguments for indexer
	indexerArgs := []string{"server", "--quiet"}
	if indexerDBPath != "" {
		indexerArgs = append(indexerArgs, "--db", indexerDBPath)
	}
	if indexerEmbDBPath != "" {
		indexerArgs = append(indexerArgs, "--embeddings-db", indexerEmbDBPath)
	}

	// Determine log path
	logPath := indexerLogPath
	if logPath == "" {
		confDir, _ := config.Dir()
		logPath = filepath.Join(confDir, "logs", "indexer.log")
	}

	// Ensure log directory exists
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	indexerArgs = append(indexerArgs, "--log", logPath)

	// Truncate log at startup if it's grown too large
	truncateIfLarge(logPath)

	// Run server in background
	c := exec.Command(path, indexerArgs...)
	if err := config.StartBackground(c); err != nil {
		return fmt.Errorf("failed to start indexer: %w", err)
	}

	// Wait a moment to see if it crashes immediately
	time.Sleep(500 * time.Millisecond)
	if !config.IsProcessAlive(c.Process.Pid) {
		return fmt.Errorf("indexer started but exited immediately (check %s for errors)", logPath)
	}

	fmt.Printf("✅ Indexer started (PID: %d). Logging to %s\n", c.Process.Pid, logPath)
	return nil
}

func runIndexerStop(cmd *cobra.Command, args []string) error {
	inst := config.FindInstanceByType(config.InstanceIndexerServer)
	if inst == nil {
		fmt.Println("Indexer is not running.")
		return nil
	}

	fmt.Printf("🛑 Stopping indexer (PID: %d)...\n", inst.PID)
	if err := config.StopInstance(*inst); err != nil {
		return fmt.Errorf("failed to stop indexer: %w", err)
	}
	fmt.Println("✅ Indexer stopped.")
	return nil
}

func runIndexerStatus(cmd *cobra.Command, args []string) error {
	inst := config.FindInstanceByType(config.InstanceIndexerServer)

	if outputJSON {
		status := indexerStatusJSON{Running: inst != nil}
		if inst != nil {
			status.PID = inst.PID
			status.LogPath = inst.LogPath
			status.StartedAt = &inst.StartedAt
			status.UptimeSeconds = int(time.Since(inst.StartedAt).Seconds())
			dbFile := indexerDBPath
			if dbFile == "" {
				dbFile, _ = db.DefaultPath()
			}
			status.Database = dbFile
		}
		data, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if inst == nil {
		fmt.Println("● thinkt-indexer.service - DuckDB Session Indexer")
		fmt.Println("   Status: Not running")
		return nil
	}

	fmt.Println("● thinkt-indexer.service - DuckDB Session Indexer")
	fmt.Printf("   Status: Running (PID: %d)\n", inst.PID)
	fmt.Printf("   Uptime: %s\n", time.Since(inst.StartedAt).Round(time.Second))

	if inst.LogPath != "" {
		fmt.Printf("   Log: %s\n", inst.LogPath)
	}

	// Try to find DB path from flags or default
	dbP := indexerDBPath
	if dbP == "" {
		fmt.Println("   Database: (Standard path)")
	} else {
		fmt.Printf("   Database: %s\n", dbP)
	}

	return nil
}

// makeForwardingCommand creates a cobra command that forwards to thinkt-indexer
func makeForwardingCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:                use,
		Short:              short,
		DisableFlagParsing: true, // Forward all flags to thinkt-indexer
		SilenceUsage:       true, // Don't show help on error from thinkt-indexer
		SilenceErrors:      true, // thinkt-indexer already printed its error
		RunE: func(cmd *cobra.Command, args []string) error {
			path := config.FindIndexerBinary()
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

// makeAutoStartingCommand creates a forwarding command that ensures the indexer is running
func makeAutoStartingCommand(use, short string) *cobra.Command {
	fwd := makeForwardingCommand(use, short)
	oldRunE := fwd.RunE
	fwd.RunE = func(cmd *cobra.Command, args []string) error {
		// Check if running
		inst := config.FindInstanceByType(config.InstanceIndexerServer)
		if inst == nil {
			// Auto-start
			if err := runIndexerStart(cmd, nil); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to auto-start indexer: %v\n", err)
			}
		}
		return oldRunE(cmd, args)
	}
	return fwd
}

// makeIndexerAlias creates a top-level command that auto-starts the indexer
// and forwards to thinkt-indexer. Used for user-facing aliases like
// "thinkt search" → "thinkt-indexer search".
func makeIndexerAlias(use, short string) *cobra.Command {
	cmd := makeAutoStartingCommand(use, short)
	cmd.Hidden = false
	return cmd
}

func init() {
	// Register persistent flags on the parent command
	indexerCmd.PersistentFlags().StringVar(&indexerDBPath, "db", "", "path to DuckDB index database file")
	indexerCmd.PersistentFlags().StringVar(&indexerEmbDBPath, "embeddings-db", "", "path to DuckDB embeddings database file")
	indexerCmd.PersistentFlags().StringVar(&indexerLogPath, "log", "", "path to log file")
	indexerCmd.PersistentFlags().BoolVarP(&indexerQuiet, "quiet", "q", false, "suppress progress output")
	indexerCmd.PersistentFlags().BoolVarP(&indexerVerbose, "verbose", "v", false, "verbose output")

	// Service commands
	indexerCmd.AddCommand(indexerStartCmd)
	indexerCmd.AddCommand(indexerStopCmd)
	indexerStatusCmd.Flags().BoolVar(&outputJSON, "json", false, "output as JSON")
	indexerCmd.AddCommand(indexerStatusCmd)
	indexerLogsCmd.Flags().IntP("lines", "n", 50, "number of lines to show")
	indexerLogsCmd.Flags().BoolP("follow", "f", false, "follow log output")
	indexerCmd.AddCommand(indexerLogsCmd)

	// Create subcommands that forward to thinkt-indexer
	indexerCmd.AddCommand(makeAutoStartingCommand("sync", "Synchronize all local sessions into the index"))
	indexerCmd.AddCommand(makeAutoStartingCommand("search", "Search for text across indexed sessions"))
	indexerCmd.AddCommand(makeForwardingCommand("stats", "Show usage statistics from the index"))
	indexerCmd.AddCommand(makeForwardingCommand("sessions", "List sessions for a project from the index"))
	indexerCmd.AddCommand(makeAutoStartingCommand("embeddings", "Manage embedding model, storage, and sync"))
	indexerCmd.AddCommand(makeForwardingCommand("semantic", "Semantic search and index management"))
	indexerCmd.AddCommand(makeForwardingCommand("version", "Print version information"))
	indexerCmd.AddCommand(makeForwardingCommand("help", "Help about any command"))
}
