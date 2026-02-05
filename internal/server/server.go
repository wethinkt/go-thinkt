// Package server implements the HTTP and MCP server for thinkt serve.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/wethinkt/go-thinkt/internal/server/docs" // swagger docs
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// ErrInvalidConfiguration is returned when server options conflict.
var ErrInvalidConfiguration = errors.New("invalid server configuration")

// Mode specifies how the server should run.
type Mode int

const (
	// ModeHTTPOnly serves only the REST API over HTTP.
	ModeHTTPOnly Mode = iota
	// ModeMCPStdio runs only MCP over stdin/stdout.
	ModeMCPStdio
	// ModeMCPHTTP serves only MCP over HTTP (SSE transport).
	ModeMCPHTTP
	// ModeCombined serves both REST API and MCP over HTTP on the same port.
	ModeCombined
)

// Default port constants for thinkt servers.
const (
	// DefaultPortServe is the default port for 'thinkt serve' (REST API).
	DefaultPortServe = 8784
	// DefaultPortLite is the default port for 'thinkt serve lite'.
	DefaultPortLite = 8785
	// DefaultPortMCP is the default port for 'thinkt serve mcp' over HTTP.
	DefaultPortMCP = 8786
)

// Config holds server configuration.
type Config struct {
	Mode        Mode
	Port        int
	Host        string
	Quiet       bool     // suppress HTTP request logging
	HTTPLog     string   // path to HTTP access log file (empty = stdout, "-" = discard unless Quiet)
	httpLogFile *os.File // internal: opened log file handle
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		Mode: ModeCombined,
		Port: DefaultPortServe,
		Host: "localhost",
	}
}

// logWriter wraps an io.Writer to implement middleware.LoggerInterface.
type logWriter struct {
	w io.Writer
}

func (l *logWriter) Print(v ...interface{}) {
	// Ensure newline - chi's middleware doesn't add them
	fmt.Fprintln(l.w, v...)
}

func (l *logWriter) Println(v ...interface{}) {
	fmt.Fprintln(l.w, v...)
}

func (l *logWriter) Printf(format string, v ...interface{}) {
	fmt.Fprintf(l.w, format, v...)
}

// httpLogWriter returns the writer for HTTP access logs based on config.
// It returns nil if logging should be suppressed.
func (c *Config) httpLogWriter() (*logWriter, io.Closer, error) {
	if c.Quiet {
		return nil, nil, nil
	}
	if c.HTTPLog == "" {
		return &logWriter{w: os.Stdout}, nil, nil
	}
	if c.HTTPLog == "-" {
		return nil, nil, nil
	}
	f, err := os.OpenFile(c.HTTPLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("open http log: %w", err)
	}
	c.httpLogFile = f
	return &logWriter{w: f}, f, nil
}

// Close cleans up any opened resources.
func (c *Config) Close() {
	if c.httpLogFile != nil {
		c.httpLogFile.Close()
		c.httpLogFile = nil
	}
}

// HTTPServer serves the REST API.
type HTTPServer struct {
	registry      *thinkt.StoreRegistry
	router        chi.Router
	config        Config
	pathValidator *PathValidator
	authenticator *APIAuthenticator
}

// NewHTTPServer creates a new HTTP server for the REST API.
func NewHTTPServer(registry *thinkt.StoreRegistry, config Config) *HTTPServer {
	return NewHTTPServerWithAuth(registry, config, DefaultAPIAuthConfig())
}

// NewHTTPServerWithAuth creates a new HTTP server with authentication.
func NewHTTPServerWithAuth(registry *thinkt.StoreRegistry, config Config, authConfig APIAuthConfig) *HTTPServer {
	s := &HTTPServer{
		registry:      registry,
		config:        config,
		pathValidator: NewPathValidator(registry),
		authenticator: NewAPIAuthenticator(authConfig),
	}
	s.router = s.setupRouter()
	return s
}

// setupRouter configures all routes.
func (s *HTTPServer) setupRouter() chi.Router {
	r := chi.NewRouter()

	// HTTP access logging middleware
	logWriter, _, err := s.config.httpLogWriter()
	if err != nil {
		// Log error to stderr but continue (non-fatal)
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
	if logWriter != nil {
		r.Use(middleware.RequestLogger(&middleware.DefaultLogFormatter{
			Logger:  logWriter,
			NoColor: true,
		}))
	}

	// Middleware
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(corsMiddleware)

	// Authentication middleware (if enabled)
	if s.authenticator.IsEnabled() {
		tuilog.Log.Info("API server authentication enabled")
		r.Use(s.authenticator.Middleware)
	} else {
		tuilog.Log.Warn("API server running without authentication - use THINKT_API_TOKEN or --token to secure")
	}

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/sources", s.handleGetSources)
		r.Get("/projects", s.handleGetProjects)
		r.Get("/projects/{projectID}/sessions", s.handleGetProjectSessions)
		r.Get("/sessions/*", s.handleGetSession)

		// Open-in endpoints
		r.Post("/open-in", s.handleOpenIn)
		r.Get("/open-in/apps", s.handleGetAllowedApps)

		// Themes endpoint
		r.Get("/themes", s.handleGetThemes)
	})

	// Swagger documentation
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// Serve embedded webapp for all other routes
	r.Handle("/*", staticHandler())

	return r
}

// Router returns the chi router for combining with other servers.
func (s *HTTPServer) Router() chi.Router {
	return s.router
}

// Addr returns the server address.
func (s *HTTPServer) Addr() string {
	return fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
}

// ListenAndServe starts the HTTP server.
func (s *HTTPServer) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.Addr(),
		Handler: s.router,
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	// Update port if it was auto-assigned
	if s.config.Port == 0 {
		s.config.Port = ln.Addr().(*net.TCPAddr).Port
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("HTTP server running at http://%s\n", s.Addr())
	return srv.Serve(ln)
}

// corsMiddleware adds CORS headers for local development.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// CombinedServer serves both REST API and MCP on the same port.
type CombinedServer struct {
	httpServer *HTTPServer
	mcpServer  *MCPServer
	router     chi.Router
	config     Config
}

// NewCombinedServer creates a server that serves both REST API and MCP over HTTP.
func NewCombinedServer(registry *thinkt.StoreRegistry, config Config) *CombinedServer {
	httpServer := NewHTTPServer(registry, config)
	mcpServer := NewMCPServer(registry)

	// Create combined router
	r := chi.NewRouter()

	// HTTP access logging middleware
	logWriter, _, err := config.httpLogWriter()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
	if logWriter != nil {
		r.Use(middleware.RequestLogger(&middleware.DefaultLogFormatter{
			Logger:  logWriter,
			NoColor: true,
		}))
	}

	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(corsMiddleware)

	// Mount HTTP API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/sources", httpServer.handleGetSources)
		r.Get("/projects", httpServer.handleGetProjects)
		r.Get("/projects/{projectID}/sessions", httpServer.handleGetProjectSessions)
		r.Get("/sessions/*", httpServer.handleGetSession)

		// Open-in endpoints
		r.Post("/open-in", httpServer.handleOpenIn)
		r.Get("/open-in/apps", httpServer.handleGetAllowedApps)

		// Themes endpoint
		r.Get("/themes", httpServer.handleGetThemes)
	})

	// MCP endpoint placeholder (for SSE transport in future)
	// r.Mount("/mcp", mcpHTTPHandler(mcpServer))

	// Root handler
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Thinkt</title></head>
<body>
<h1>Thinkt Server</h1>
<p>REST API: <a href="/api/v1/sources">/api/v1/sources</a></p>
<p>MCP: Available via stdio transport (use --mcp-only)</p>
</body>
</html>`))
	})

	return &CombinedServer{
		httpServer: httpServer,
		mcpServer:  mcpServer,
		router:     r,
		config:     config,
	}
}

// Addr returns the server address.
func (s *CombinedServer) Addr() string {
	return fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
}

// ListenAndServe starts the combined server.
func (s *CombinedServer) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.Addr(),
		Handler: s.router,
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	if s.config.Port == 0 {
		s.config.Port = ln.Addr().(*net.TCPAddr).Port
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("Server running at http://%s\n", s.Addr())
	return srv.Serve(ln)
}

// Server is the legacy interface for backwards compatibility.
// Use NewHTTPServer, NewMCPServer, or NewCombinedServer directly.
type Server struct {
	registry *thinkt.StoreRegistry
	router   chi.Router
	port     int
	host     string
}

// Option configures the server.
type Option func(*Server)

// WithPort sets the server port.
func WithPort(port int) Option {
	return func(s *Server) {
		s.port = port
	}
}

// WithHost sets the server host.
func WithHost(host string) Option {
	return func(s *Server) {
		s.host = host
	}
}

// New creates a new Server (legacy interface - creates CombinedServer behavior).
func New(registry *thinkt.StoreRegistry, opts ...Option) *Server {
	s := &Server{
		registry: registry,
		port:     DefaultPortServe,
		host:     "localhost",
	}

	for _, opt := range opts {
		opt(s)
	}

	// Create HTTP server and use its router
	config := Config{
		Mode: ModeCombined,
		Port: s.port,
		Host: s.host,
	}
	httpServer := NewHTTPServer(registry, config)
	s.router = httpServer.Router()

	return s
}

// Addr returns the server address.
func (s *Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.host, s.port)
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.Addr(),
		Handler: s.router,
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	if s.port == 0 {
		s.port = ln.Addr().(*net.TCPAddr).Port
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("Server running at http://%s\n", s.Addr())
	return srv.Serve(ln)
}
