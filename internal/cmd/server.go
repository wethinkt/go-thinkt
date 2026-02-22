package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/fingerprint"
	"github.com/wethinkt/go-thinkt/internal/server"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// serverStatusJSON is the JSON schema for thinkt server status --json.
type serverStatusJSON struct {
	Running       bool       `json:"running"`
	PID           int        `json:"pid,omitempty"`
	Host          string     `json:"host,omitempty"`
	Port          int        `json:"port,omitempty"`
	Address       string     `json:"address,omitempty"`
	LogPath       string     `json:"log_path,omitempty"`
	HTTPLogPath   string     `json:"http_log_path,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	UptimeSeconds int        `json:"uptime_seconds,omitempty"`
}

// Serve command flags
var (
	servePort       int
	serveHost       string
	serveNoOpen     bool
	serveQuiet      bool
	serveHTTPLog    string
	serveCORSOrigin string
	serveNoIndexer  bool
)

// Serve mcp subcommand flags
var (
	mcpStdio      bool
	mcpPort       int
	mcpHost       string
	mcpToken      string
	mcpAllowTools []string
	mcpDenyTools  []string
)

// Serve subcommand flags
var (
	apiToken    string // Bearer token for API server authentication
	serveDev    string // Dev proxy URL (e.g. http://localhost:5173)
	serveNoAuth bool   // Disable authentication entirely
)

// Serve fingerprint subcommand flags
var (
	fingerprintJSON bool // Output as JSON
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage the local HTTP server for trace exploration",
	Long: `Manage the local HTTP server for exploring AI conversation traces.

The server provides:
  - REST API for accessing projects and sessions
  - Web interface for visual trace exploration
  - MCP (Model Context Protocol) server

All data stays on your machine - nothing is uploaded to external servers.

Examples:
  thinkt server                    # Show server status
  thinkt server run                # Start server in foreground
  thinkt server start              # Start in background
  thinkt server status             # Check server status
  thinkt server stop               # Stop background server
  thinkt server logs               # View server logs`,
	SilenceUsage: true,
	Args:         cobra.NoArgs,
	RunE:         runServerStatus,
}

var serverRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Start server in foreground",
	RunE:  runServerHTTP,
}

var serverStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start server in background",
	RunE:  runServerStart,
}

var serverStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop background server",
	RunE:  runServerStop,
}

var serverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show server status",
	RunE:  runServerStatus,
}

var serverLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View server logs",
	RunE:  runServerLogs,
}

var serverHTTPLogsCmd = &cobra.Command{
	Use:   "http-logs",
	Short: "View HTTP access logs",
	RunE:  runServerHTTPLogs,
}

func runServerLogs(cmd *cobra.Command, args []string) error {
	n, _ := cmd.Flags().GetInt("lines")
	follow, _ := cmd.Flags().GetBool("follow")

	// Try to get log path from running instance
	logFile := ""
	if inst := config.FindInstanceByType(config.InstanceServer); inst != nil {
		logFile = inst.LogPath
	}

	// Fall back to default
	if logFile == "" {
		confDir, err := config.Dir()
		if err != nil {
			return err
		}
		logFile = filepath.Join(confDir, "logs", "server.log")
	}

	return tailLogFile(logFile, n, follow)
}

func runServerHTTPLogs(cmd *cobra.Command, args []string) error {
	n, _ := cmd.Flags().GetInt("lines")
	follow, _ := cmd.Flags().GetBool("follow")

	// Try to get log path from running instance
	logFile := ""
	if inst := config.FindInstanceByType(config.InstanceServer); inst != nil {
		logFile = inst.HTTPLogPath
	}

	// Fall back to default
	if logFile == "" {
		confDir, err := config.Dir()
		if err != nil {
			return err
		}
		logFile = filepath.Join(confDir, "logs", "server.http.log")
	}

	return tailLogFile(logFile, n, follow)
}

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Open the web interface in your browser",
	Long: `Open the thinkt web interface in your default browser.
If the server is not already running, it will be started in the background.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWebOpen(false)
	},
}

var webLiteCmd = &cobra.Command{
	Use:          "lite",
	Short:        "Open the lightweight web interface",
	Long:         `Open the thinkt lite web interface (debugging and development view).`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWebOpen(true)
	},
}

func runWebOpen(isLite bool) error {
	inst := config.FindInstanceByType(config.InstanceServer)
	if inst == nil {
		// Start server in background
		if err := runServerStart(nil, nil); err != nil {
			return err
		}
		// Wait a bit for it to register
		for i := 0; i < 20; i++ {
			time.Sleep(100 * time.Millisecond)
			inst = config.FindInstanceByType(config.InstanceServer)
			if inst != nil {
				break
			}
		}
	}

	if inst == nil {
		return fmt.Errorf("failed to start or find running server")
	}

	targetURL := fmt.Sprintf("http://%s:%d", inst.Host, inst.Port)
	if isLite {
		targetURL += "/lite/"
	}
	if inst.Token != "" {
		targetURL += "?token=" + inst.Token
	}

	fmt.Printf("🌐 Opening %s in browser...\n", targetURL)
	openBrowser(targetURL)
	return nil
}

func runServerStart(cmd *cobra.Command, args []string) error {
	// Check if already running
	if inst := config.FindInstanceByType(config.InstanceServer); inst != nil {
		fmt.Printf("Server is already running (PID: %d, Address: http://%s:%d)\n", inst.PID, inst.Host, inst.Port)
		return nil
	}

	fmt.Println("🚀 Starting server in background...")

	confDir, err := config.Dir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	// Build arguments for thinkt server run
	executable, _ := os.Executable()
	logsDir := filepath.Join(confDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}
	runArgs := []string{"server", "run", "--no-open",
		"--log", filepath.Join(logsDir, "server.log"),
		"--http-log", filepath.Join(logsDir, "server.http.log"),
	}
	if servePort != 0 {
		runArgs = append(runArgs, "--port", fmt.Sprintf("%d", servePort))
	}
	if serveHost != "" {
		runArgs = append(runArgs, "--host", serveHost)
	}
	if serveQuiet {
		runArgs = append(runArgs, "--quiet")
	}
	if serveNoAuth {
		runArgs = append(runArgs, "--no-auth")
	}
	if serveNoIndexer {
		runArgs = append(runArgs, "--no-indexer")
	}

	// Run in background
	c := exec.Command(executable, runArgs...)
	if err := config.StartBackground(c); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	fmt.Printf("✅ Server starting (PID: %d)\n", c.Process.Pid)
	fmt.Printf("   Log:  %s\n", filepath.Join(logsDir, "server.log"))
	fmt.Printf("   HTTP: %s\n", filepath.Join(logsDir, "server.http.log"))
	return nil
}

func runServerStop(cmd *cobra.Command, args []string) error {
	inst := config.FindInstanceByType(config.InstanceServer)
	if inst == nil {
		fmt.Println("Server is not running.")
		return nil
	}

	fmt.Printf("🛑 Stopping server (PID: %d)...\n", inst.PID)
	if err := config.StopInstance(*inst); err != nil {
		return fmt.Errorf("failed to stop server: %w", err)
	}
	fmt.Println("✅ Server stopped.")
	return nil
}

func runServerStatus(cmd *cobra.Command, args []string) error {
	inst := config.FindInstanceByType(config.InstanceServer)

	if outputJSON {
		status := serverStatusJSON{Running: inst != nil}
		if inst != nil {
			status.PID = inst.PID
			status.Host = inst.Host
			status.Port = inst.Port
			status.LogPath = inst.LogPath
			status.HTTPLogPath = inst.HTTPLogPath
			status.StartedAt = &inst.StartedAt
			status.UptimeSeconds = int(time.Since(inst.StartedAt).Seconds())
			status.Address = fmt.Sprintf("http://%s:%d", inst.Host, inst.Port)
		}
		data, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if inst == nil {
		fmt.Println("● thinkt-server.service - Web Interface & API")
		fmt.Println("   Status: Not running")
		return nil
	}

	fmt.Println("● thinkt-server.service - Web Interface & API")
	fmt.Printf("   Status: Running (PID: %d)\n", inst.PID)
	fmt.Printf("   Address: http://%s:%d\n", inst.Host, inst.Port)
	fmt.Printf("   Uptime: %s\n", time.Since(inst.StartedAt).Round(time.Second))
	if inst.Token != "" {
		if len(inst.Token) > 14 {
			fmt.Printf("   Token: %s...\n", inst.Token[:14])
		} else {
			fmt.Printf("   Token: %s\n", inst.Token)
		}
	}
	if inst.LogPath != "" {
		fmt.Printf("   Log: %s\n", inst.LogPath)
	}
	if inst.HTTPLogPath != "" {
		fmt.Printf("   HTTP Log: %s\n", inst.HTTPLogPath)
	}

	return nil
}

var serverMcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server for AI tool integration",
	Long: `Start an MCP (Model Context Protocol) server for AI tool integration.

By default, runs on stdio for use with Claude Desktop and other MCP clients.
Use --port to run over HTTP instead.

Authentication:
  For stdio transport: Set THINKT_MCP_TOKEN environment variable
  For HTTP transport: Use --token flag or THINKT_MCP_TOKEN environment variable
  Clients must pass the token in the Authorization header: "Bearer <token>"
  Generate a secure token with: thinkt server token

Examples:
  thinkt server mcp                          # MCP server on stdio (default)
  thinkt server mcp --stdio                  # Explicitly use stdio transport
  thinkt server mcp --port 8786              # MCP server over HTTP (default port)
  thinkt server mcp --port 8786 --token xyz  # MCP server with authentication`,
	RunE: runServerMCP,
}

var serverTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Generate a secure authentication token",
	Long: `Generate a cryptographically secure random token for API/MCP authentication.

The token can be used with:
  - thinkt server --token <token>      # Secure the REST API
  - thinkt server mcp --token <token>  # Secure the MCP server
  - THINKT_MCP_TOKEN env var           # Same as above

The token format is: thinkt_YYYYMMDD_<random>

Examples:
  thinkt server token                  # Generate and print a token
  thinkt server token | pbcopy         # Copy to clipboard (macOS)
  thinkt server token | xclip -sel c   # Copy to clipboard (Linux)
  thinkt server token | clip           # Copy to clipboard (Windows)
  export THINKT_MCP_TOKEN=$(thinkt server token)
  thinkt server mcp --port 8786        # Uses token from env`,
	RunE: runServerToken,
}

var serverFingerprintCmd = &cobra.Command{
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
assistant sources (Kimi, Claude, Gemini, Copilot, Codex, Qwen) on the same machine.

Examples:
  thinkt server fingerprint            # Display fingerprint
  thinkt server fingerprint --json     # Output as JSON`,
	RunE: runServerFingerprint,
}

func runServerHTTP(cmd *cobra.Command, args []string) error {
	tuilog.Log.Info("Starting HTTP server", "port", servePort, "host", serveHost)

	// Start indexer sidecar if not disabled
	if !serveNoIndexer {
		if sidecar := startIndexerSidecar(logPath); sidecar != nil {
			defer func() {
				tuilog.Log.Info("Stopping indexer sidecar")
				_ = sidecar.Process.Kill()
			}()
		}
	}

	// Create source registry
	registry := CreateSourceRegistry()
	tuilog.Log.Info("Source registry created", "stores", len(registry.All()))

	// Configure authentication
	var authConfig server.AuthConfig
	if serveNoAuth {
		authConfig = server.AuthConfig{Mode: server.AuthModeNone, Realm: "thinkt-api"}
	} else {
		authConfig = server.DefaultAPIAuthConfig()
		if apiToken != "" {
			authConfig = server.AuthConfig{
				Mode:  server.AuthModeToken,
				Token: apiToken,
				Realm: "thinkt-api",
			}
		}
		// Auto-generate token if no auth configured
		if authConfig.Mode == server.AuthModeNone {
			token, err := server.GenerateSecureTokenWithPrefix()
			if err != nil {
				return fmt.Errorf("failed to generate auth token: %w", err)
			}
			authConfig = server.AuthConfig{
				Mode:  server.AuthModeToken,
				Token: token,
				Realm: "thinkt-api",
			}
			fmt.Fprintf(os.Stderr, "Auto-generated auth token: %s\n", token)
		}
	}

	// Resolve the effective token for instance registry
	var resolvedToken string
	switch authConfig.Mode {
	case server.AuthModeToken:
		resolvedToken = authConfig.Token
	case server.AuthModeEnvToken:
		resolvedToken = os.Getenv(authConfig.EnvVar)
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
		fmt.Fprintln(os.Stderr, "\nShutting down...")
		cancel()
	}()

	// Resolve static handler: reverse proxy in dev mode, embedded assets otherwise
	var staticHandler http.Handler
	if serveDev != "" {
		target, err := url.Parse(serveDev)
		if err != nil {
			return fmt.Errorf("invalid --dev URL %q: %w", serveDev, err)
		}
		fmt.Fprintf(os.Stderr, "Dev mode: proxying to %s\n", target)
		staticHandler = httputil.NewSingleHostReverseProxy(target)
	} else {
		staticHandler = server.StaticWebAppHandler()
	}

	// Resolve log paths: use defaults under ~/.thinkt/logs/ when not explicitly set
	confDir, _ := config.Dir()
	httpLogPath := serveHTTPLog
	if httpLogPath == "" && confDir != "" {
		httpLogPath = filepath.Join(confDir, "logs", "server.http.log")
	}
	appLogPath := logPath
	if appLogPath == "" && confDir != "" {
		appLogPath = filepath.Join(confDir, "logs", "server.log")
	}

	// Truncate logs at startup if they've grown too large
	truncateIfLarge(httpLogPath)
	truncateIfLarge(appLogPath)

	// HTTP mode: start HTTP server
	thinktConfig := server.Config{
		Port:          servePort,
		Host:          serveHost,
		Quiet:         serveQuiet,
		HTTPLog:       httpLogPath,
		CORSOrigin:    resolveCORSOrigin(),
		StaticHandler: staticHandler,
		InstanceType:  config.InstanceServer,
		LogPath:       appLogPath,
		HTTPLogPath:   httpLogPath,
		Token:         resolvedToken,
	}
	defer thinktConfig.Close()
	srv := server.NewHTTPServerWithAuth(registry, thinktConfig, authConfig)
	for _, ts := range registry.TeamStores() {
		srv.SetTeamStore(ts)
	}

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

func runServerToken(cmd *cobra.Command, args []string) error {
	// If a server is running, print its token from the instance registry
	if inst := config.FindInstanceByType(config.InstanceServer); inst != nil && inst.Token != "" {
		fmt.Println(inst.Token)
		return nil
	}

	// Otherwise generate a new token
	token, err := server.GenerateSecureTokenWithPrefix()
	if err != nil {
		return fmt.Errorf("failed to generate token: %w", err)
	}
	fmt.Println(token)
	return nil
}

func runServerFingerprint(cmd *cobra.Command, args []string) error {
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

// startIndexerSidecar starts the indexer as a child process if available and not
// already running. Returns the exec.Cmd (caller should defer cmd.Process.Kill())
// or nil if the sidecar was not started.
func startIndexerSidecar(appLogPath string) *exec.Cmd {
	// Skip if an indexer is already registered (e.g. via `thinkt indexer start`)
	if inst := config.FindInstanceByType(config.InstanceIndexerWatch); inst != nil {
		tuilog.Log.Info("Indexer already running, skipping sidecar", "pid", inst.PID)
		return nil
	}

	indexerPath := findIndexerBinary()
	if indexerPath == "" {
		return nil
	}

	tuilog.Log.Info("Starting indexer sidecar", "path", indexerPath)

	indexerArgs := []string{"watch", "--quiet"}
	if appLogPath != "" {
		ext := filepath.Ext(appLogPath)
		base := strings.TrimSuffix(appLogPath, ext)
		indexerLog := base + ".indexer" + ext
		indexerArgs = append(indexerArgs, "--log", indexerLog)
		tuilog.Log.Info("Indexer sidecar logging", "path", indexerLog)
	}

	cmd := exec.Command(indexerPath, indexerArgs...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		tuilog.Log.Error("Failed to start indexer sidecar", "error", err)
		return nil
	}

	return cmd
}

func runServerMCP(cmd *cobra.Command, args []string) error {
	// Start indexer sidecar if not disabled
	if !serveNoIndexer {
		if sidecar := startIndexerSidecar(logPath); sidecar != nil {
			defer func() {
				tuilog.Log.Info("Stopping indexer sidecar")
				_ = sidecar.Process.Kill()
			}()
		}
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
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		tuilog.Log.Info("Received interrupt signal, shutting down")
		fmt.Fprintln(os.Stderr, "\nShutting down...")
		cancel()
	}()

	// Configure authentication
	authConfig := server.DefaultMCPAuthConfig()
	if mcpToken != "" {
		authConfig = server.AuthConfig{
			Mode:  server.AuthModeToken,
			Token: mcpToken,
			Realm: "thinkt-mcp",
		}
	}

	// Configure tool filtering
	allowTools := mcpAllowTools
	if envAllow := os.Getenv("THINKT_MCP_ALLOW_TOOLS"); envAllow != "" && len(allowTools) == 0 {
		allowTools = strings.Split(envAllow, ",")
	}
	denyTools := mcpDenyTools
	if envDeny := os.Getenv("THINKT_MCP_DENY_TOOLS"); envDeny != "" && len(denyTools) == 0 {
		denyTools = strings.Split(envDeny, ",")
	}

	// Create MCP server with authentication and filtering
	tuilog.Log.Info("Creating MCP server")
	mcpServer := server.NewMCPServerWithAuth(registry, authConfig)
	mcpServer.SetToolFilters(allowTools, denyTools)

	if useStdio {
		fmt.Fprintln(os.Stderr, "Starting MCP server on stdio...")
		tuilog.Log.Info("Running MCP server on stdio")
		err := mcpServer.RunStdio(ctx)
		tuilog.Log.Info("MCP server exited", "error", err)
		if err != nil {
			if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "EOF") {
				tuilog.Log.Info("EOF received, treating as normal termination")
				return nil
			}
			return err
		}
		return nil
	}

	tuilog.Log.Info("Running MCP server on HTTP", "host", mcpHost, "port", mcpPort)
	return mcpServer.RunHTTP(ctx, mcpHost, mcpPort)
}

// resolveCORSOrigin returns the CORS origin from the CLI flag or env var, defaulting to "*".
func resolveCORSOrigin() string {
	if serveCORSOrigin != "" {
		return serveCORSOrigin
	}
	if v := os.Getenv("THINKT_CORS_ORIGIN"); v != "" {
		return v
	}
	return "*"
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
	_ = cmd.Start()
}
