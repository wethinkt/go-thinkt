// thinkt-collector is a standalone binary that runs the trace collector server.
// It receives AI coding assistant traces from exporters via HTTP POST and stores
// them in a local DuckDB database.
//
// Usage:
//
//	thinkt-collector
//	thinkt-collector --port 8785 --token mytoken
//	thinkt-collector --storage ./traces.duckdb
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/wethinkt/go-thinkt/internal/collect"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// version is set via ldflags at build time.
var version = "dev"

func main() {
	var (
		port        int
		host        string
		storage     string
		token       string
		quiet       bool
		showVersion bool
		logFile     string
	)

	flag.IntVar(&port, "port", collect.DefaultPort, "server port")
	flag.StringVar(&host, "host", collect.DefaultHost, "server host")
	flag.StringVar(&storage, "storage", "", "DuckDB storage path (default ~/.thinkt/dbs/collector.duckdb)")
	flag.StringVar(&token, "token", "", "bearer token for authentication")
	flag.BoolVar(&quiet, "quiet", false, "suppress non-error output")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.StringVar(&logFile, "log", "", "write debug log to file")
	flag.Parse()

	if showVersion {
		fmt.Printf("thinkt-collector %s\n", version)
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

	cfg := collect.CollectorConfig{
		Port:   port,
		Host:   host,
		DBPath: storage,
		Token:  token,
		Quiet:  quiet,
	}

	srv, err := collect.NewServer(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: create collector: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "thinkt-collector %s\n", version)
		fmt.Fprintf(os.Stderr, "Listening on %s:%d\n", host, port)
		if token != "" {
			fmt.Fprintln(os.Stderr, "Authentication: enabled (bearer token)")
		} else {
			fmt.Fprintln(os.Stderr, "Authentication: disabled (use --token to secure)")
		}
	}

	if err := srv.ListenAndServe(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
