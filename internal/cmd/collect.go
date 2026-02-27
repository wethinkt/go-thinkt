package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/collect"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// Collect command flags
var (
	collectPort    int
	collectHost    string
	collectStorage string
	collectToken   string
	collectQuiet   bool
)

var collectCmd = &cobra.Command{
	Use:   "collect",
	Short: "Start trace collector server",
	Long: `Start a collector server that receives AI coding assistant traces from exporters.

The collector provides:
  - POST /v1/traces endpoint for trace ingestion
  - Agent registration and heartbeat tracking
  - DuckDB-backed storage for collected traces
  - Bearer token authentication (optional)

All received traces are stored locally in DuckDB for analysis.

Examples:
  thinkt collect                           # Start collector on port 8785
  thinkt collect --port 8785               # Custom port
  thinkt collect --token mytoken           # Require bearer token auth
  thinkt collect --storage ./traces.duckdb # Custom storage path`,
	RunE: runCollect,
}

func runCollect(cmd *cobra.Command, args []string) error {
	// Initialize logger if requested
	if logPath != "" {
		if err := tuilog.Init(logPath); err != nil {
			return err
		}
		defer tuilog.Log.Close()
	}

	tuilog.Log.Info("Starting collector server", "port", collectPort, "host", collectHost)

	cfg := collect.CollectorConfig{
		Port:   collectPort,
		Host:   collectHost,
		DBPath: collectStorage,
		Token:  collectToken,
		Quiet:  collectQuiet,
	}

	srv, err := collect.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("create collector server: %w", err)
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
		if !collectQuiet {
			fmt.Fprintln(os.Stderr, "\nShutting down...")
		}
		cancel()
	}()

	if !collectQuiet {
		fmt.Fprintf(os.Stderr, "Collector server starting on %s:%d\n", collectHost, collectPort)
		if collectToken != "" {
			fmt.Fprintln(os.Stderr, "Authentication: enabled (bearer token)")
		} else {
			fmt.Fprintln(os.Stderr, "Authentication: disabled (use --token to secure)")
		}
	}

	return srv.ListenAndServe(ctx)
}
