package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/server"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// Serve command flags
var (
	servePort     int
	serveLitePort int
	serveHost     string
	serveNoOpen   bool
	serveQuiet    bool
	serveHTTPLog  string
)

// Serve MCP subcommand flags
var (
	mcpStdio bool
	mcpPort  int
	mcpHost  string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start local HTTP server for trace exploration",
	Long: `Start a local HTTP server for exploring AI conversation traces.

The server provides:
  - REST API for accessing projects and sessions
  - Web interface for visual trace exploration

All data stays on your machine - nothing is uploaded to external servers.

Use 'thinkt serve mcp' for MCP (Model Context Protocol) server.

Examples:
  thinkt serve                    # Start HTTP server on default port 7433
  thinkt serve -p 8080            # Start on custom port
  thinkt serve --no-open          # Don't auto-open browser`,
	RunE: runServeHTTP,
}

var serveMcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server for AI tool integration",
	Long: `Start an MCP (Model Context Protocol) server for AI tool integration.

By default, runs on stdio for use with Claude Desktop and other MCP clients.
Use --port to run over HTTP instead.

Examples:
  thinkt serve mcp                # MCP server on stdio (default)
  thinkt serve mcp --stdio        # Explicitly use stdio transport
  thinkt serve mcp --port 8081    # MCP server over HTTP`,
	RunE: runServeMCP,
}

var serveLiteCmd = &cobra.Command{
	Use:   "lite",
	Short: "Start lightweight webapp for debugging and development",
	Long: `Start a lightweight HTTP server with a simple debug interface.

The lite webapp provides:
  - Overview of available sources and their status
  - List of all projects with session counts
  - Quick links to API endpoints and documentation

This is useful for developers and debugging. For the full experience,
use 'thinkt serve' (coming soon) or the TUI with 'thinkt'.

Examples:
  thinkt serve lite               # Start lite server on port 7434
  thinkt serve lite -p 8080       # Start on custom port
  thinkt serve lite --no-open     # Don't auto-open browser`,
	RunE: runServeLite,
}

func runServeHTTP(cmd *cobra.Command, args []string) error {
	// Initialize logger if requested
	if logPath != "" {
		if err := tuilog.Init(logPath); err != nil {
			return err
		}
		defer tuilog.Log.Close()
	}

	tuilog.Log.Info("Starting HTTP server", "port", servePort, "host", serveHost)

	// Create source registry
	registry := CreateSourceRegistry()
	tuilog.Log.Info("Source registry created", "stores", len(registry.All()))

	// Create context that cancels on interrupt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		tuilog.Log.Info("Received interrupt signal, shutting down")
		fmt.Fprintln(os.Stderr, "\nShutting down...")
		cancel()
	}()

	// HTTP mode: start HTTP server
	config := server.Config{
		Mode:    server.ModeHTTPOnly,
		Port:    servePort,
		Host:    serveHost,
		Quiet:   serveQuiet,
		HTTPLog: serveHTTPLog,
	}
	srv := server.NewHTTPServer(registry, config)

	// Print startup message
	fmt.Println("🚀 Thinkt server starting...")
	fmt.Println("📁 Serving traces from local sources")

	// Auto-open browser if requested (after small delay for server to start)
	if !serveNoOpen {
		go func() {
			url := fmt.Sprintf("http://%s", srv.Addr())
			fmt.Printf("🌐 Opening %s in browser...\n", url)
			openBrowser(url)
		}()
	}

	// Start server
	return srv.ListenAndServe(ctx)
}

func runServeLite(cmd *cobra.Command, args []string) error {
	// Initialize logger if requested
	if logPath != "" {
		if err := tuilog.Init(logPath); err != nil {
			return err
		}
		defer tuilog.Log.Close()
	}

	tuilog.Log.Info("Starting Lite HTTP server", "port", serveLitePort, "host", serveHost)

	// Create source registry
	registry := CreateSourceRegistry()
	tuilog.Log.Info("Source registry created", "stores", len(registry.All()))

	// Create context that cancels on interrupt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		tuilog.Log.Info("Received interrupt signal, shutting down")
		fmt.Fprintln(os.Stderr, "\nShutting down...")
		cancel()
	}()

	// HTTP mode: start HTTP server
	config := server.Config{
		Mode:    server.ModeHTTPOnly,
		Port:    serveLitePort,
		Host:    serveHost,
		Quiet:   serveQuiet,
		HTTPLog: serveHTTPLog,
	}
	srv := server.NewHTTPServer(registry, config)

	// Auto-open browser if requested
	if !serveNoOpen {
		go func() {
			url := fmt.Sprintf("http://%s", srv.Addr())
			openBrowser(url)
		}()
	}

	// Start server
	return srv.ListenAndServe(ctx)
}

func runServeMCP(cmd *cobra.Command, args []string) error {
	// Initialize logger if requested
	if logPath != "" {
		if err := tuilog.Init(logPath); err != nil {
			return err
		}
		defer tuilog.Log.Close()
	}

	// Determine transport mode: stdio (default) or HTTP
	useStdio := mcpStdio || mcpPort == 0

	tuilog.Log.Info("Starting MCP server", "stdio", useStdio, "port", mcpPort)

	// Create source registry
	registry := CreateSourceRegistry()
	tuilog.Log.Info("Source registry created", "stores", len(registry.All()))

	// Create context that cancels on interrupt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		tuilog.Log.Info("Received interrupt signal, shutting down")
		fmt.Fprintln(os.Stderr, "\nShutting down...")
		cancel()
	}()

	// Create MCP server
	tuilog.Log.Info("Creating MCP server")
	mcpServer := server.NewMCPServer(registry)

	if useStdio {
		// Stdio transport
		fmt.Fprintln(os.Stderr, "Starting MCP server on stdio...")
		tuilog.Log.Info("Running MCP server on stdio")
		err := mcpServer.RunStdio(ctx)
		tuilog.Log.Info("MCP server exited", "error", err)
		// EOF on stdin is normal termination (client disconnected), not an error
		if err != nil {
			if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "EOF") {
				tuilog.Log.Info("EOF received, treating as normal termination")
				return nil
			}
			return err
		}
		return nil
	}

	// HTTP transport (SSE)
	tuilog.Log.Info("Running MCP server on HTTP", "host", mcpHost, "port", mcpPort)
	return mcpServer.RunHTTP(ctx, mcpHost, mcpPort)
}

// openBrowser opens a URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		fmt.Printf("Please open %s in your browser\n", url)
		return
	}
	cmd.Start()
}
