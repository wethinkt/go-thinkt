package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/cmd"
	"github.com/wethinkt/go-thinkt/internal/indexer"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch session directories for changes and index them in real-time",
	RunE: func(cmdObj *cobra.Command, args []string) error {
		database, err := getDB()
		if err != nil {
			return err
		}
		defer database.Close()

		registry := cmd.CreateSourceRegistry()
		ingester := indexer.NewIngester(database, registry)
		
		watcher, err := indexer.NewWatcher(ingester, registry)
		if err != nil {
			return fmt.Errorf("failed to create watcher: %w", err)
		}
		defer watcher.Stop()

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