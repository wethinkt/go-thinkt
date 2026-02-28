package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/export"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// Export command flags
var (
	exportCollectorURL string
	exportAPIKey       string
	exportSource       string
	exportForward      bool
	exportFlush        bool
	exportQuiet        bool
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export traces to a remote collector",
	Long: `Export local AI coding assistant traces to a remote collector endpoint.

By default, performs a one-shot export of all traces found in source directories.
Use --forward for continuous watch mode that ships traces as they are written.

The collector endpoint is discovered automatically:
  1. --collector-url flag or THINKT_COLLECTOR_URL env var
  2. .thinkt/collector.json in the project directory
  3. Well-known endpoint discovery
  4. Local buffer only (no remote)

Examples:
  thinkt export                          # One-shot export of all traces
  thinkt export --forward                # Watch mode: continuously forward traces
  thinkt export --flush                  # Flush the disk buffer
  thinkt export --source claude          # Export only Claude traces
  thinkt export --collector-url https://collect.example.com/v1/traces`,
	RunE: runExport,
}

func runExport(cmd *cobra.Command, args []string) error {
	// Initialize logger if requested
	if logPath != "" {
		if err := tuilog.Init(logPath); err != nil {
			return err
		}
		defer tuilog.Log.Close()
	}

	// Resolve collector URL from flag or env
	collectorURL := exportCollectorURL
	if collectorURL == "" {
		collectorURL = os.Getenv("THINKT_COLLECTOR_URL")
	}

	// Resolve API key from flag or env
	apiKey := exportAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("THINKT_API_KEY")
	}

	// Auto-discover watch directories from source registry
	var watchDirs []export.WatchDir
	registry := CreateSourceRegistry()
	for _, store := range registry.All() {
		// Filter by source if specified
		if exportSource != "" && string(store.Source()) != exportSource {
			continue
		}
		ws := store.Workspace()
		if ws.BasePath != "" {
			watchDirs = append(watchDirs, export.WatchDir{
				Path:   ws.BasePath,
				Source: string(store.Source()),
				Config: store.WatchConfig(),
			})
		}
	}

	if len(watchDirs) == 0 && !exportFlush {
		return fmt.Errorf("no source directories found (available sources: claude, kimi, gemini, copilot, codex)")
	}

	tuilog.Log.Info("Export configuration",
		"collector_url", collectorURL,
		"watch_dirs", watchDirs,
		"forward", exportForward,
		"flush", exportFlush,
		"source", exportSource,
	)

	cfg := export.ExporterConfig{
		CollectorURL: collectorURL,
		APIKey:       apiKey,
		WatchDirs:    watchDirs,
		Quiet:        exportQuiet,
	}

	exporter, err := export.New(cfg)
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
		if !exportQuiet {
			fmt.Fprintln(os.Stderr, "\nShutting down...")
		}
		cancel()
	}()

	if exportFlush {
		if !exportQuiet {
			fmt.Fprintln(os.Stderr, "Flushing export buffer...")
		}
		return exporter.FlushBuffer(ctx)
	}

	if exportForward {
		if !exportQuiet {
			fmt.Fprintf(os.Stderr, "Exporter watching %d directories (forward mode)\n", len(watchDirs))
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

	// Default: one-shot export
	if !exportQuiet {
		fmt.Fprintf(os.Stderr, "Exporting traces from %d directories...\n", len(watchDirs))
	}
	return exporter.ExportOnce(ctx)
}
