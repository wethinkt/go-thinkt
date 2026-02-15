// thinkt-exporter is a standalone binary that watches local AI session files
// and exports trace entries to a remote collector endpoint.
//
// Usage:
//
//	thinkt-exporter --collector-url https://collect.example.com/v1/traces --watch-dir ~/.claude/projects
//	thinkt-exporter --watch-dir /path/a --watch-dir /path/b --api-key mytoken
//
// Environment variables:
//
//	THINKT_COLLECTOR_URL  Collector endpoint (fallback if --collector-url not set)
//	THINKT_API_KEY        Bearer token (fallback if --api-key not set)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/export"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// version is set via ldflags at build time.
var version = "dev"

// watchDirs is a repeatable flag for specifying directories to watch.
type watchDirs []string

func (w *watchDirs) String() string { return strings.Join(*w, ",") }
func (w *watchDirs) Set(val string) error {
	*w = append(*w, val)
	return nil
}

func main() {
	var (
		collectorURL string
		apiKey       string
		dirs         watchDirs
		bufferDir    string
		quiet        bool
		showVersion  bool
		logFile      string
	)

	flag.StringVar(&collectorURL, "collector-url", "", "collector endpoint URL (env: THINKT_COLLECTOR_URL)")
	flag.StringVar(&apiKey, "api-key", "", "bearer token for collector auth (env: THINKT_API_KEY)")
	flag.Var(&dirs, "watch-dir", "directory to watch for session files (repeatable)")
	flag.StringVar(&bufferDir, "buffer-dir", "", "disk buffer directory (default ~/.thinkt/export-buffer/)")
	flag.BoolVar(&quiet, "quiet", false, "suppress non-error output")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.StringVar(&logFile, "log", "", "write debug log to file")
	flag.Parse()

	if showVersion {
		fmt.Printf("thinkt-exporter %s\n", version)
		os.Exit(0)
	}

	// Initialize logger
	if logFile != "" {
		if err := tuilog.Init(logFile); err != nil {
			fmt.Fprintf(os.Stderr, "error: init log: %v\n", err)
			os.Exit(1)
		}
		defer tuilog.Log.Close()
	}

	// Resolve from env if flags not set
	if collectorURL == "" {
		collectorURL = os.Getenv("THINKT_COLLECTOR_URL")
	}
	if apiKey == "" {
		apiKey = os.Getenv("THINKT_API_KEY")
	}

	if len(dirs) == 0 {
		fmt.Fprintln(os.Stderr, "error: at least one --watch-dir is required")
		flag.Usage()
		os.Exit(1)
	}

	cfg := export.ExporterConfig{
		CollectorURL: collectorURL,
		APIKey:       apiKey,
		WatchDirs:    dirs,
		BufferDir:    bufferDir,
		Quiet:        quiet,
	}

	exporter, err := export.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: create exporter: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		if !quiet {
			fmt.Fprintln(os.Stderr, "\nShutting down...")
		}
		cancel()
	}()

	if !quiet {
		fmt.Fprintf(os.Stderr, "thinkt-exporter %s\n", version)
		fmt.Fprintf(os.Stderr, "Watching %d directories\n", len(dirs))
		for _, d := range dirs {
			fmt.Fprintf(os.Stderr, "  %s\n", d)
		}
		if collectorURL != "" {
			fmt.Fprintf(os.Stderr, "Collector: %s\n", collectorURL)
		} else {
			fmt.Fprintln(os.Stderr, "Collector: auto-discover")
		}
	}

	if err := exporter.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
