package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/cmd"
	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/indexer"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch session directories for changes and index them in real-time",
	RunE: func(cmdObj *cobra.Command, args []string) error {
		registry := cmd.CreateSourceRegistry()

		watcher, err := indexer.NewWatcher(dbPath, registry)
		if err != nil {
			return fmt.Errorf("failed to create watcher: %w", err)
		}
		defer func() {
			_ = watcher.Stop() // Ignore error, cleanup is best-effort
		}()

		// Register instance for discovery
		inst := config.Instance{
			Type:      config.InstanceIndexerWatch,
			PID:       os.Getpid(),
			StartedAt: time.Now(),
		}
		if err := config.RegisterInstance(inst); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to register instance: %v\n", err)
		}
		defer func() {
			_ = config.UnregisterInstance(os.Getpid()) // Ignore error, cleanup is best-effort
		}()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start watching
		if err := watcher.Start(ctx); err != nil {
			return fmt.Errorf("failed to start watcher: %w", err)
		}

		// Initialize progress reporter for TTY-aware output
		progress := NewProgressReporter()

		if progress.ShouldShowProgress(quiet, verbose) {
			fmt.Println("Watching for changes... Press Ctrl+C to stop.")
		}

		// Wait for interrupt
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt)
		<-sigChan

		if progress.ShouldShowProgress(quiet, verbose) {
			fmt.Println("\nStopping watcher...")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(watchCmd)
}