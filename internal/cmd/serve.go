package cmd

import (
	"context"
	"encoding/json"
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

	"github.com/wethinkt/go-thinkt/internal/fingerprint"
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
	mcpToken string // Bearer token for HTTP transport auth
)

// Serve API subcommand flags
var (
	apiToken string // Bearer token for API server authentication
)

// Serve fingerprint subcommand flags
var (
	fingerprintJSON bool // Output as JSON
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
  thinkt serve                    # Start HTTP server on default port 8784
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

Authentication:
  For stdio transport: Set THINKT_MCP_TOKEN environment variable
  For HTTP transport: Use --token flag or THINKT_MCP_TOKEN environment variable
  Clients must pass the token in the Authorization header: "Bearer <token>"
  Generate a secure token with: thinkt serve token

Examples:
  thinkt serve mcp                          # MCP server on stdio (default)
  thinkt serve mcp --stdio                  # Explicitly use stdio transport
  thinkt serve mcp --port 8786              # MCP server over HTTP (default port)
  thinkt serve mcp --port 8786 --token xyz  # MCP server with authentication`,
	RunE: runServeMCP,
}

var serveTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Generate a secure authentication token",
	Long: `Generate a cryptographically secure random token for API/MCP authentication.

The token can be used with:
  - thinkt serve --token <token>      # Secure the REST API
  - thinkt serve mcp --token <token>  # Secure the MCP server
  - THINKT_MCP_TOKEN env var          # Same as above

The token format is: thinkt_YYYYMMDD_<random>

Examples:
  thinkt serve token                  # Generate and print a token
  thinkt serve token | pbcopy         # Generate and copy to clipboard (macOS)
  export THINKT_MCP_TOKEN=$(thinkt serve token)
  thinkt serve mcp --port 8786        # Uses token from env`,
	RunE: runServeToken,
}

var serveFingerprintCmd = &cobra.Command{
	Use:   "fingerprint",
	Short: "Display the machine fingerprint",
	Long: `Display the unique machine fingerprint used to identify this workspace.

The fingerprint is derived from system identifiers when available:
  - macOS: IOPlatformUUID from ioreg
  - Linux: /etc/machine-id or /var/lib/dbus/machine-id
  - Windows: MachineGuid from registry

If no system identifier is available, a fingerprint is generated and cached
in ~/.thinkt/machine_id for consistency across restarts.

This fingerprint can be used to correlate sessions across different AI coding
assistant sources (Kimi, Claude, Gemini, Copilot) on the same machine.

Examples:
  thinkt serve fingerprint            # Display fingerprint
  thinkt serve fingerprint --json     # Output as JSON`,
	RunE: runServeFingerprint,
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
  thinkt serve lite                   # Start lite server on port 8785
  thinkt serve lite -p 8080           # Start on custom port
  thinkt serve lite --host 0.0.0.0    # Bind to all interfaces
  thinkt serve lite --no-open         # Don't auto-open browser`,
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

	// Configure authentication
	authConfig := server.DefaultAPIAuthConfig()
	if apiToken != "" {
		authConfig = server.APIAuthConfig{
			Mode:  server.APIAuthModeToken,
			Token: apiToken,
		}
	}

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
	srv := server.NewHTTPServerWithAuth(registry, config, authConfig)

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

func runServeToken(cmd *cobra.Command, args []string) error {
	token, err := server.GenerateSecureTokenWithPrefix()
	if err != nil {
		return fmt.Errorf("failed to generate token: %w", err)
	}
	fmt.Println(token)
	return nil
}

func runServeFingerprint(cmd *cobra.Command, args []string) error {
	info, err := fingerprint.Get()
	if err != nil {
		return fmt.Errorf("failed to get fingerprint: %w", err)
	}

	if fingerprintJSON {
		// JSON output
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
	} else {
		// Human-readable output
		fmt.Println(info.String())
	}

	return nil
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

	// Configure authentication
	authConfig := server.DefaultMCPAuthConfig()
	if mcpToken != "" {
		authConfig = server.MCPAuthConfig{
			Mode:  server.AuthModeToken,
			Token: mcpToken,
		}
	}

	// Create MCP server with authentication
	tuilog.Log.Info("Creating MCP server")
	mcpServer := server.NewMCPServerWithAuth(registry, authConfig)

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
