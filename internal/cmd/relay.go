package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/relay"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// Relay command flags
var (
	relayCollectorURL string
	relayAPIKey       string
	relaySource       string
	relayForward      bool
	relayFlush        bool
	relayQuiet        bool
)

var relayCmd = &cobra.Command{
	Use:   "relay",
	Short: "Relay traces to a remote collector",
	Long: `Relay local AI coding assistant traces to a remote collector endpoint.

By default, performs a one-shot relay of all traces found in source directories.
Use --forward for continuous watch mode that ships traces as they are written.

The collector endpoint is discovered automatically:
  1. --collector-url flag or THINKT_COLLECTOR_URL env var
  2. .thinkt/collector.json in the project directory
  3. Well-known endpoint discovery
  4. Local buffer only (no remote)

Examples:
  thinkt relay                          # One-shot relay of all traces
  thinkt relay --forward                # Watch mode: continuously forward traces
  thinkt relay --flush                  # Flush the disk buffer
  thinkt relay --source claude          # Relay only Claude traces
  thinkt relay --collector-url https://collect.example.com/v1/traces`,
	RunE: runRelay,
}

func runRelay(cmd *cobra.Command, args []string) error {
	// Initialize logger if requested
	if logPath != "" {
		if err := tuilog.Init(logPath); err != nil {
			return err
		}
		defer tuilog.Log.Close()
	}

	// Resolve collector URL from flag or env
	collectorURL := relayCollectorURL
	if collectorURL == "" {
		collectorURL = os.Getenv("THINKT_COLLECTOR_URL")
	}

	// Resolve API key from flag or env
	apiKey := relayAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("THINKT_API_KEY")
	}

	// Auto-discover watch directories from source registry
	var watchDirs []relay.WatchDir
	registry := CreateSourceRegistry()
	for _, store := range registry.All() {
		// Filter by source if specified
		if relaySource != "" && string(store.Source()) != relaySource {
			continue
		}
		ws := store.Workspace()
		if ws.BasePath != "" {
			watchDirs = append(watchDirs, relay.WatchDir{
				Path:   ws.BasePath,
				Source: string(store.Source()),
				Config: store.WatchConfig(),
			})
		}
	}

	if len(watchDirs) == 0 && !relayFlush {
		return fmt.Errorf("no source directories found (available sources: claude, kimi, gemini, copilot, codex)")
	}

	tuilog.Log.Info("Relay configuration",
		"collector_url", collectorURL,
		"watch_dirs", watchDirs,
		"forward", relayForward,
		"flush", relayFlush,
		"source", relaySource,
	)

	cfg := relay.ExporterConfig{
		CollectorURL: collectorURL,
		APIKey:       apiKey,
		WatchDirs:    watchDirs,
		Quiet:        relayQuiet,
	}

	exporter, err := relay.New(cfg)
	if err != nil {
		return fmt.Errorf("create exporter: %w", err)
	}

	// Create context that cancels on interrupt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		tuilog.Log.Info("Received interrupt signal, shutting down")
		if !relayQuiet {
			fmt.Fprintln(os.Stderr, "\nShutting down...")
		}
		cancel()
	}()

	if relayFlush {
		if !relayQuiet {
			fmt.Fprintln(os.Stderr, "Flushing relay buffer...")
		}
		return exporter.FlushBuffer(ctx)
	}

	if relayForward {
		if !relayQuiet {
			fmt.Fprintf(os.Stderr, "Relay watching %d directories (forward mode)\n", len(watchDirs))
			for _, wd := range watchDirs {
				fmt.Fprintf(os.Stderr, "  [%s] %s\n", wd.Source, wd.Path)
			}
			if collectorURL != "" {
				fmt.Fprintf(os.Stderr, "Collector: %s\n", collectorURL)
			} else {
				fmt.Fprintln(os.Stderr, "Collector: auto-discover")
			}
		}
		return exporter.Start(ctx)
	}

	// Default: one-shot relay
	if !relayQuiet {
		fmt.Fprintf(os.Stderr, "Relaying traces from %d directories...\n", len(watchDirs))
	}
	return exporter.ExportOnce(ctx)
}
