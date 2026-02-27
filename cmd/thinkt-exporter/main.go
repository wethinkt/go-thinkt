// thinkt-exporter is a standalone binary that watches local AI session files
// and exports trace entries to a remote collector endpoint.
//
// By default it auto-discovers all installed AI sources (Claude, Kimi, Gemini,
// Codex, Copilot, Qwen) and watches their session directories. Use --source to
// limit to specific sources. Source paths are controlled via environment variables
// (THINKT_CLAUDE_HOME, THINKT_KIMI_HOME, etc.).
//
// Usage:
//
//	thinkt-exporter --collector-url https://collect.example.com/v1/traces
//	thinkt-exporter --source claude --source kimi
//	THINKT_CLAUDE_HOME=/custom/path thinkt-exporter --source claude
//
// Environment variables:
//
//	THINKT_COLLECTOR_URL  Collector endpoint (fallback if --collector-url not set)
//	THINKT_API_KEY        Bearer token (fallback if --api-key not set)
//	THINKT_CLAUDE_HOME    Override Claude session directory
//	THINKT_KIMI_HOME      Override Kimi session directory
//	THINKT_GEMINI_HOME    Override Gemini session directory
//	THINKT_CODEX_HOME     Override Codex session directory
//	THINKT_COPILOT_HOME   Override Copilot session directory
//	THINKT_QWEN_HOME      Override Qwen session directory
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/wethinkt/go-thinkt/internal/export"
	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// version is set via ldflags at build time.
var version = "dev"

// sourceFlags is a repeatable flag for specifying sources to export.
type sourceFlags []string

func (s *sourceFlags) String() string { return strings.Join(*s, ",") }
func (s *sourceFlags) Set(val string) error {
	*s = append(*s, val)
	return nil
}

func main() {
	var (
		collectorURL string
		apiKey       string
		srcs         sourceFlags
		bufferDir    string
		quiet        bool
		showVersion  bool
		logFile      string
		metricsPort  int
	)

	allNames := sourceNames()
	flag.StringVar(&collectorURL, "collector-url", "", "collector endpoint URL (env: THINKT_COLLECTOR_URL)")
	flag.StringVar(&apiKey, "api-key", "", "bearer token for collector auth (env: THINKT_API_KEY)")
	flag.Var(&srcs, "source", "source to export (repeatable: "+strings.Join(allNames, ", ")+"); all if omitted")
	flag.StringVar(&bufferDir, "buffer-dir", "", "disk buffer directory (default ~/.thinkt/export-buffer/)")
	flag.BoolVar(&quiet, "quiet", false, "suppress non-error output")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.StringVar(&logFile, "log", "", "write debug log to file")
	flag.IntVar(&metricsPort, "metrics-port", 0, "port for Prometheus /metrics endpoint (0 = disabled)")
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

	// Build source filter set (empty = all sources)
	sourceFilter := make(map[thinkt.Source]bool)
	for _, s := range srcs {
		sourceFilter[thinkt.Source(s)] = true
	}

	// Discover watch dirs from source registry
	dirs := discoverWatchDirs(sourceFilter, quiet)
	if len(dirs) == 0 {
		if len(srcs) > 0 {
			fmt.Fprintf(os.Stderr, "error: no session directories found for sources: %s\n", strings.Join([]string(srcs), ", "))
		} else {
			fmt.Fprintln(os.Stderr, "error: no session directories found (is a supported AI tool installed?)")
		}
		os.Exit(1)
	}

	cfg := export.ExporterConfig{
		CollectorURL: collectorURL,
		APIKey:       apiKey,
		WatchDirs:    dirs,
		BufferDir:    bufferDir,
		Quiet:        quiet,
		Version:      version,
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

	// Start optional Prometheus metrics server
	if metricsPort > 0 {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		go func() {
			addr := fmt.Sprintf(":%d", metricsPort)
			if !quiet {
				fmt.Fprintf(os.Stderr, "Metrics: http://localhost%s/metrics\n", addr)
			}
			if err := http.ListenAndServe(addr, mux); err != nil {
				fmt.Fprintf(os.Stderr, "metrics server error: %v\n", err)
			}
		}()
	}

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

// discoverWatchDirs uses the source registry to find session directories.
// If filter is non-empty, only sources in the filter are included.
func discoverWatchDirs(filter map[thinkt.Source]bool, quiet bool) []string {
	discovery := thinkt.NewDiscovery(sources.AllFactories()...)
	registry, err := discovery.Discover(context.Background())
	if err != nil {
		return nil
	}

	var dirs []string
	for _, store := range registry.All() {
		src := store.Source()
		if len(filter) > 0 && !filter[src] {
			continue
		}
		ws := store.Workspace()
		if ws.BasePath == "" {
			continue
		}
		if _, err := os.Stat(ws.BasePath); err != nil {
			continue
		}
		dirs = append(dirs, ws.BasePath)
		if !quiet {
			fmt.Fprintf(os.Stderr, "Discovered %s: %s\n", src, ws.BasePath)
		}
	}
	return dirs
}

// sourceNames returns the string names of all known sources for help text.
func sourceNames() []string {
	names := make([]string, len(thinkt.AllSources))
	for i, s := range thinkt.AllSources {
		names[i] = string(s)
	}
	return names
}
